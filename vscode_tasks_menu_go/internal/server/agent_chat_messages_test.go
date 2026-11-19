package server

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"

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
