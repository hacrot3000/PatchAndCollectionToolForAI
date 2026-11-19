package server

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func newAgentConversationFixture(t *testing.T,mode string)(*Server,string){
 t.Helper()
 s:=newAIServerForTest(t)
 if err:=os.MkdirAll(s.Workspace,0700);err!=nil{t.Fatal(err)}
 p:=agentProviderFixture()
 providers,err:=s.agentChatProviders();if err!=nil{t.Fatal(err)}
 if err:=providers.Put("local",p);err!=nil{t.Fatal(err)}
 if err:=s.ConnectionSecrets.Put(agentProviderSecretID(s.Workspace,"local",p.ID),[]byte("TEST-KEY"));err!=nil{t.Fatal(err)}
 // Tool execution is gated on probe evidence, so fixtures that exercise the
 // native Patch path must record a verified tool-calling capability for this
 // exact endpoint/model fingerprint. Tests that want the unverified path use
 // their own fixture without this record.
 caps,err:=s.agentChatCapabilities();if err!=nil{t.Fatal(err)}
 if err:=caps.Put("local",agentchat.CapabilityRecord{
  ProviderID:p.ID,ModelID:p.Models[0].ID,
  Fingerprint:agentchat.CapabilityFingerprint(p,p.Models[0]),
  Version:agentchat.CapabilityProbeVersion,
  Capabilities:map[string]string{agentchat.ProbeToolCalling:agentchat.CapabilityVerified},
  ProbedAt:time.Now().UTC(),
 });err!=nil{t.Fatal(err)}
 created:=invokeAI(t,s.agentChatSessionsAPI,"POST","/api/agent/sessions",map[string]any{
  "mode":mode,"provider_id":p.ID,"model_id":p.Models[0].ID,
 })
 if created.Code!=201{t.Fatalf("session create: %d %s",created.Code,created.Body)}
 var view struct {Session agentchat.Session `json:"session"`}
 if err:=json.Unmarshal(created.Body.Bytes(),&view);err!=nil{t.Fatal(err)}
 s.AgentChatPack=func(workspace,executable string)(patchtool.AIPackResult,error){
  return patchtool.AIPackResult{
   TextContent:"TEST VERIFIED AI PACK CONTRACT\nIdentity: "+view.Session.ProjectID,
   Fingerprint:"fingerprint-001",
   ProjectIdentity:patchtool.ProjectIdentity{ID:view.Session.ProjectID,Key:"project"},
  },nil
 }
 return s,view.Session.ID
}
func TestAgentChatMessagesPreviousResponseContextAndPersistence(t *testing.T){
 s,id:=newAgentConversationFixture(t,"previous_response")
 count:=0
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,body []byte)(agentchat.ResponsesResult,error){
  count++
  if key!="TEST-KEY"||p.ID!="custom-r2d"{t.Fatal("wrong provider key/config")}
  var payload map[string]any
  if err:=json.Unmarshal(body,&payload);err!=nil{t.Fatal(err)}
  if !strings.Contains(payload["instructions"].(string),"Verified project ID"){t.Fatal("missing required instructions")}
  items:=payload["input"].([]any)
  if len(items)!=1{t.Fatalf("expected one delta per turn, got %d",len(items))}
  content:=items[0].(map[string]any)["content"].(string)
  if count==1 {
   if !strings.Contains(content,"TEST VERIFIED AI PACK CONTRACT")||!strings.Contains(content,"First prompt"){
    t.Fatal("first turn did not contain AI Pack and user request")
   }
   if _,ok:=payload["previous_response_id"];ok{t.Fatal("first turn unexpectedly chained")}
  }else if count==2 {
   if payload["previous_response_id"]!="resp_1"||strings.Contains(content,"AI PACK")||content!="Second prompt"{
    t.Fatalf("second turn did not use response chain: %#v",payload)
   }
  }else{t.Fatal("unexpected extra provider call")}
  return agentchat.ResponsesResult{ResponseID:map[int]string{1:"resp_1",2:"resp_2"}[count],Text:"Answer",InputTokens:5,OutputTokens:2},nil
 }
 first:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"First prompt","confirm_external":true,"expected_revision":1})
 if first.Code!=200{t.Fatalf("first turn %d %s",first.Code,first.Body)}
 second:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"Second prompt","confirm_external":true,"expected_revision":3})
 if second.Code!=200{t.Fatalf("second turn %d %s",second.Code,second.Body)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 saved,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if saved.Revision!=5||saved.PreviousResponseID!="resp_2"||saved.AIPackFingerprint!="fingerprint-001"||len(saved.History)!=4{
  t.Fatalf("context not correctly persisted: %+v",saved)
 }
 if count!=2{t.Fatalf("expected two provider calls, got %d",count)}
}
func TestAgentChatMessagesRequireConsentAndRevision(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 count:=0
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,body []byte)(agentchat.ResponsesResult,error){count++;return agentchat.ResponsesResult{ResponseID:"resp",Text:"ok"},nil}
 noConsent:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"test","expected_revision":1})
 if noConsent.Code!=403{t.Fatalf("no consent accepted: %d",noConsent.Code)}
 stale:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"test","confirm_external":true,"expected_revision":999})
 if stale.Code!=409||count!=0{t.Fatalf("stale revision called provider: %d / %d",stale.Code,count)}
}
func TestAgentChatToolCallsRemainNonExecuting(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,body []byte)(agentchat.ResponsesResult,error){
  return agentchat.ResponsesResult{ResponseID:"resp",Calls:[]agentchat.FunctionCall{{CallID:"call_1",Name:"taskdeck_submit_patch",Arguments:`{"patch_text":"dangerous operation"}`}}},nil
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"test","confirm_external":true,"expected_revision":1})
 if w.Code!=200||!strings.Contains(w.Body.String(),"\"waiting_tool\"")||!strings.Contains(w.Body.String(),"\"execution_enabled\":false"){
  t.Fatalf("tool call should await explicit gateway: %d %s",w.Code,w.Body)
 }
 repeat:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"continue","confirm_external":true,"expected_revision":3})
 if repeat.Code!=409{t.Fatalf("waiting tool session incorrectly advanced: %d",repeat.Code)}
}
func TestAgentChatMessageSharedPermission(t *testing.T){
 req:=httptest.NewRequest(http.MethodPost,"/api/agent/sessions/deadbeef/messages",nil)
 perms:=sharedRoutePermissions(req)
 if len(perms)!=2||perms[0]!=identity.PermissionSettingsRead||perms[1]!=identity.PermissionFilesRead{
  t.Fatalf("missing required read permissions: %+v",perms)
 }
}

func TestAgentChatCompactionPreservesToolPairAndRefusesUnsafe(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 // Fill with completed plain-text turns, then a completed tool exchange and
 // a newer user turn.
 for i:=0;i<220;i++{
  role:="user";if i%2==1{role="assistant"}
  x.History=append(x.History,agentchat.HistoryItem{Type:role,Content:"filler"})
 }
 x.History=append(x.History,
  agentchat.HistoryItem{Type:"assistant",Content:"proposal"},
  agentchat.HistoryItem{Type:"function_call",CallID:"call_old",Name:"taskdeck_submit_patch",Arguments:`{"name":"old","patch_python":"y"}`},
  agentchat.HistoryItem{Type:"function_call_output",CallID:"call_old",Content:`{"status":"PASS"}`},
 )
 x.ToolCalls=append(x.ToolCalls,agentchat.ToolCallRecord{CallID:"call_old",Name:"taskdeck_submit_patch",Status:"done",Result:`{"status":"PASS"}`,FeedbackSent:true,FeedbackStatus:"acknowledged"})
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error){
  return agentchat.ResponsesResult{ResponseID:"resp_compact",Text:"ok"},nil
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"next question","confirm_external":true,"expected_revision":x.Revision})
 if w.Code!=200{t.Fatalf("compacted turn failed: %d %s",w.Code,w.Body)}
 y,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 seenCall,seenOutput:=false,false
 for _,item:=range y.History{
  if item.Type=="function_call"&&item.CallID=="call_old"{seenCall=true}
  if item.Type=="function_call_output"&&item.CallID=="call_old"{seenOutput=true}
 }
 if !seenCall||!seenOutput{t.Fatalf("compaction split or dropped the tool pair: call=%v output=%v len=%d",seenCall,seenOutput,len(y.History))}
 if len(y.History)>=222{t.Fatalf("history did not shrink: %d",len(y.History))}
 // A history consisting only of a tool exchange/tail cannot be compacted.
 z,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 z.History=[]agentchat.HistoryItem{
  {Type:"assistant",Content:"p"},
  {Type:"function_call",CallID:"c",Name:"taskdeck_submit_patch",Arguments:`{"name":"a","patch_python":"b"}`},
  {Type:"function_call_output",CallID:"c",Content:"{}"},
 }
 z.ToolCalls=[]agentchat.ToolCallRecord{{CallID:"c",ModelTurnID:"r",Name:"taskdeck_submit_patch",Status:"done",Result:"{}",FeedbackSent:true,FeedbackStatus:"acknowledged"}}
 z.Status="completed"
 // Bypass the >200 trigger by duplicating safe text up to the threshold is
 // unnecessary: CompactHistory itself is asserted to refuse here.
 if res:=agentchat.CompactHistory(z.History,agentchat.CompactionConfig{KeepTail:1,MaxDropped:10});res.Dropped!=0{
  t.Fatalf("unsafe compaction would drop tool commitments: %+v",res.Dropped)
 }
}

func TestAgentChatTimeoutRetainsTurnAndIsNotRetried(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 calls:=0
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  return agentchat.ResponsesResult{},context.DeadlineExceeded
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"slow","confirm_external":true,"expected_revision":1})
 if w.Code!=504{t.Fatalf("timeout not distinguished: %d %s",w.Code,w.Body)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Status!="failed"{t.Fatalf("timeout status %q",x.Status)}
 if len(x.History)!=1||x.History[0].Type!="user"{t.Fatalf("user turn not retained once: %+v",x.History)}
 // A second manual send is allowed (new revision) but still performs exactly
 // one provider call per attempt; nothing is auto-retried.
 next:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"again","confirm_external":true,"expected_revision":x.Revision})
 if next.Code!=504||calls!=2{t.Fatalf("unexpected retry semantics: %d calls=%d",next.Code,calls)}
 y,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if len(y.History)!=2{t.Fatalf("duplicate or lost turns: %+v",y.History)}
}

func TestAgentChatConversationModeFallsBackUntilVerified(t *testing.T){
 s,id:=newAgentConversationFixture(t,"conversation")
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error){
  var req map[string]any
  if err:=json.Unmarshal(payload,&req);err!=nil{t.Fatal(err)}
  if _,ok:=req["conversation"];ok{t.Fatal("unverified Conversations ID was sent")}
  inp,ok:=req["input"].([]any)
  if !ok||len(inp)==0{t.Fatal("stateless fallback did not replay history")}
  return agentchat.ResponsesResult{ResponseID:"resp_conv_fallback",Text:"ok",ConversationID:"conv_x"},nil
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"hello","confirm_external":true,"expected_revision":1})
 if w.Code!=200{t.Fatalf("fallback turn failed: %d %s",w.Code,w.Body)}
 // Once verified, the same stored mode may use the conversation ID.
 p:=agentProviderFixture()
 caps,err:=s.agentChatCapabilities();if err!=nil{t.Fatal(err)}
 if err:=caps.Put("local",agentchat.CapabilityRecord{
  ProviderID:p.ID,ModelID:p.Models[0].ID,
  Fingerprint:agentchat.CapabilityFingerprint(p,p.Models[0]),
  Version:agentchat.CapabilityProbeVersion,
  Capabilities:map[string]string{
   agentchat.ProbeConversation:agentchat.CapabilityVerified,
   agentchat.ProbeToolCalling:agentchat.CapabilityVerified,
  },
  ProbedAt:time.Now().UTC(),
 });err!=nil{t.Fatal(err)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 seenConversation:=false
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error){
  var req map[string]any
  if err:=json.Unmarshal(payload,&req);err!=nil{t.Fatal(err)}
  if req["conversation"]=="conv_x"{seenConversation=true}
  return agentchat.ResponsesResult{ResponseID:"resp_conv_verified",Text:"ok",ConversationID:"conv_x"},nil
 }
 w=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"again","confirm_external":true,"expected_revision":x.Revision})
 if w.Code!=200{t.Fatalf("verified conversation turn failed: %d %s",w.Code,w.Body)}
 if !seenConversation{t.Fatal("verified conversation ID was not reused")}
}

func TestAgentChatIdempotentRequestReplaysWithoutSecondProviderCall(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 calls:=0
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  return agentchat.ResponsesResult{ResponseID:"resp_idem",Text:"answer"},nil
 }
 body:=map[string]any{"message":"one question","confirm_external":true,"expected_revision":1,"request_id":"req-abc"}
 first:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",body)
 if first.Code!=200{t.Fatalf("first send failed: %d %s",first.Code,first.Body)}
 // Same key, now-stale revision: must replay, not re-call the provider.
 second:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",body)
 if second.Code!=200||!strings.Contains(second.Body.String(),`"replay":true`){
  t.Fatalf("idempotent retry not replayed: %d %s",second.Code,second.Body)
 }
 if calls!=1{t.Fatalf("provider called %d times for one logical turn",calls)}
 // Reusing the key with different content is a conflict, not a replay.
 body["message"]="different"
 conflict:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",body)
 if conflict.Code!=409{t.Fatalf("key reuse with different message accepted: %d",conflict.Code)}
 if calls!=1{t.Fatal("conflict still called the provider")}
}
