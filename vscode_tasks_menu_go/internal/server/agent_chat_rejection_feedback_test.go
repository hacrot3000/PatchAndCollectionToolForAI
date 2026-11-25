package server

import (
 "context"
 "encoding/json"
 "os"
 "path/filepath"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentInvalidPatchAutomaticallyReportsRejectionAndAcceptsFixedProposal(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 expiry:=time.Now().UTC().Add(10*time.Minute)
 x.AutoFeedback=true
 x.AutoFeedbackUntil=&expiry
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}

 requests:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  requests++
  if requests==1 {
   return agentchat.ResponsesResult{ResponseID:"response-bad",
    Calls:[]agentchat.FunctionCall{{CallID:"call_bad",Name:"taskdeck_submit_patch",
     Arguments:`{"patch_script":"print('not executed')"}`}}},nil
  }
  if requests!=2{t.Fatalf("rejection was sent repeatedly: %d",requests)}
  text:=string(payload)
  if !strings.Contains(text,`"call_id":"call_bad"`)||
    !strings.Contains(text,"INVALID_PATCH_ARGUMENTS")||
    !strings.Contains(text,`"type":"function_call_output"`){
   t.Fatalf("rejection missing from provider continuation: %s",text)
  }
  return agentchat.ResponsesResult{ResponseID:"response-fixed",
   Calls:[]agentchat.FunctionCall{{CallID:"call_fixed",Name:"taskdeck_submit_patch",
    Arguments:`{"name":"fixed-patch","patch_python":"print('fixed')"}`}}},nil
 }
 reply:=invokeAI(t,s.agentChatSessionItemAPI,"POST",
  "/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"Submit patch","confirm_external":true,"expected_revision":x.Revision})
 if reply.Code!=200{t.Fatalf("initial provider call: %d %s",reply.Code,reply.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Status!="tool_rejected"||len(x.ToolCalls)!=1||
   x.ToolCalls[0].Status!="rejected"||x.ToolCalls[0].FeedbackSent{
  t.Fatalf("invalid tool was not recorded as undelivered rejection: %+v",x)
 }
 if idx,ok:=agentAutoFeedbackCandidate(x);!ok||idx!=0{
  t.Fatalf("rejection did not qualify for consented feedback: %v %d",ok,idx)
 }
 s.runAgentAutoFeedback(context.Background())
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if requests!=2||len(x.ToolCalls)!=2||x.Status!="waiting_tool"||
    x.ToolCalls[0].FeedbackStatus!="acknowledged"||!x.ToolCalls[0].FeedbackSent||
    x.ToolCalls[1].CallID!="call_fixed"||x.ToolCalls[1].Status!="pending"{
  t.Fatalf("AI did not continue to corrected proposal: calls=%d state=%+v",requests,x)
 }
 if _,err=os.Stat(filepath.Join(s.Workspace,"patchs"));!os.IsNotExist(err){
  t.Fatalf("rejected or pending Patch was executed/queued: %v",err)
 }
 s.runAgentAutoFeedback(context.Background())
 if requests!=2{t.Fatalf("acknowledged rejection replayed: %d",requests)}
}

func TestAgentManualRejectAutoFeedbackReportsUserRejectionOnce(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 expiry:=time.Now().UTC().Add(10*time.Minute)
 x.AutoFeedback=true
 x.AutoFeedbackUntil=&expiry
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 decision:=invokeAI(t,s.agentChatSessionItemAPI,"POST",
   "/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
   map[string]any{"action":"reject","expected_revision":x.Revision})
 if decision.Code!=200{t.Fatalf("reject: %d %s",decision.Code,decision.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Status!="tool_rejected"||len(x.ToolCalls)!=1||
    x.ToolCalls[0].Status!="rejected"{t.Fatalf("rejection not saved: %+v",x)}
 var feedback struct {
  Status string `json:"status"`
  ReasonCode string `json:"reason_code"`
  Retryable bool `json:"retryable_by_ai"`
  PatchQueued bool `json:"patch_queued"`
 }
 if err=json.Unmarshal([]byte(x.ToolCalls[0].Result),&feedback);err!=nil{
  t.Fatal(err)
 }
 if feedback.Status!="REJECTED"||feedback.ReasonCode!="USER_REJECTED"||
    feedback.Retryable||feedback.PatchQueued{
  t.Fatalf("bad human rejection semantics: %+v",feedback)
 }
 requests:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  requests++
  if !strings.Contains(string(payload),`"call_id":"call_agent_001"`)||
     !strings.Contains(string(payload),"USER_REJECTED"){
   t.Fatalf("wrong tool result sent: %s",payload)
  }
  return agentchat.ResponsesResult{ResponseID:"reject-ack",Text:"I will not apply that Patch."},nil
 }
 s.runAgentAutoFeedback(context.Background())
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if requests!=1||x.ToolCalls[0].FeedbackStatus!="acknowledged"||!x.ToolCalls[0].FeedbackSent{
  t.Fatalf("human rejection not acknowledged: %d %+v",requests,x.ToolCalls)
 }
 s.runAgentAutoFeedback(context.Background())
 if requests!=1{t.Fatalf("human rejection sent twice: %d",requests)}
 if _,err=os.Stat(filepath.Join(s.Workspace,"patchs"));!os.IsNotExist(err){
  t.Fatalf("reject unexpectedly queued native patch: %v",err)
 }
}

func TestAgentInvalidPatchRequiresConsentForManualForwarding(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 requests:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  requests++
  if requests==1{return agentchat.ResponsesResult{
    ResponseID:"bad-no-grant",Calls:[]agentchat.FunctionCall{{
      CallID:"call_bad",Name:"taskdeck_submit_patch",Arguments:`{"patch_script":"x"}`,
    }}},nil}
  if !strings.Contains(string(payload),"INVALID_PATCH_ARGUMENTS"){t.Fatal("missing rejection feedback")}
  return agentchat.ResponsesResult{ResponseID:"manual-ack",Text:"Correcting the format."},nil
 }
 first:=invokeAI(t,s.agentChatSessionItemAPI,"POST",
  "/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"Submit patch","confirm_external":true,"expected_revision":1})
 if first.Code!=200{t.Fatalf("invalid patch: %d %s",first.Code,first.Body)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 s.runAgentAutoFeedback(context.Background())
 if requests!=1{t.Fatal("sent rejection without auto-feedback consent")}
 path:="/api/agent/sessions/"+id+"/tools/call_bad/continue"
 denied:=invokeAI(t,s.agentChatSessionItemAPI,"POST",path,
  map[string]any{"expected_revision":x.Revision})
 if denied.Code!=403{t.Fatalf("missing explicit consent accepted: %d",denied.Code)}
 sent:=invokeAI(t,s.agentChatSessionItemAPI,"POST",path,
  map[string]any{"expected_revision":x.Revision,"confirm_external":true})
 if sent.Code!=200{t.Fatalf("manual rejection feedback: %d %s",sent.Code,sent.Body)}
 if requests!=2{t.Fatalf("manual forward not sent once: %d",requests)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.ToolCalls[0].FeedbackStatus!="acknowledged"{t.Fatalf("manual feedback state: %+v",x.ToolCalls[0])}
}

func TestAgentDeniedShellSudoIsReportedToAIAndModelCanContinue(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.AutoFeedback=true;x.AutoFeedbackPersistent=true
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  switch calls{
  case 1:
    return agentchat.ResponsesResult{ResponseID:"blocked-shell-response",
      Calls:[]agentchat.FunctionCall{{CallID:"call_shell_sudo",Name:"shell",
      Arguments:`{"command":"sudo /usr/bin/true"}`}}},nil
  case 2:
    var body struct{Input []struct{
      Type string `json:"type"`
      CallID string `json:"call_id"`
      Output string `json:"output"`
    } `json:"input"`}
    if err:=json.Unmarshal(payload,&body);err!=nil{t.Fatal(err)}
    found:=false
    for _,item:=range body.Input{
      if item.Type!="function_call_output"||item.CallID!="call_shell_sudo"{continue}
      var denial struct{
       Status string `json:"status"`
       Code string `json:"reason_code"`
       Queued bool `json:"patch_queued"`
       Executed bool `json:"patch_executed"`
      }
      if err:=json.Unmarshal([]byte(item.Output),&denial);err!=nil{t.Fatal(err)}
      if denial.Status!="REJECTED"||denial.Code!="UNSUPPORTED_TOOL"||denial.Queued||denial.Executed{
       t.Fatalf("unsafe or missing denial feedback: %+v",denial)
      }
      found=true
    }
    if !found{t.Fatalf("shell call_id missing from continuation: %s",payload)}
    return agentchat.ResponsesResult{ResponseID:"fixed-after-denial",
      Calls:[]agentchat.FunctionCall{{CallID:"call_safe_patch",
        Name:"taskdeck_submit_patch",
        Arguments:`{"name":"safe proposal","patch_python":"print('proposed only')"}`}}},nil
  default:t.Fatalf("unexpected provider call %d",calls);return agentchat.ResponsesResult{},nil
  }
 }
 first:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
   map[string]any{"message":"inspect then update","confirm_external":true,"expected_revision":x.Revision})
 if first.Code!=200{t.Fatalf("initial denied turn: %d %s",first.Code,first.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Status!="tool_rejected"||len(x.ToolCalls)!=1||x.ToolCalls[0].Name!="shell"||
    x.ToolCalls[0].Status!="rejected"||x.ToolCalls[0].QueuePath!=""{
  t.Fatalf("shell was not quarantined as rejected-only: %+v",x)
 }
 if _,ok:=agentAutoFeedbackCandidate(x);!ok{t.Fatal("denied shell not scheduled for notification")}
 if _,err:=os.Stat(filepath.Join(s.Workspace,"patchs"));!os.IsNotExist(err){t.Fatalf("shell created Patch queue: %v",err)}
 s.runAgentAutoFeedback(context.Background())
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=2||x.Status!="waiting_tool"||len(x.ToolCalls)!=2||
    x.ToolCalls[0].FeedbackStatus!="acknowledged"||
    x.ToolCalls[1].Name!="taskdeck_submit_patch"||
    x.ToolCalls[1].Status!="pending"{
  t.Fatalf("AI did not recover into safe proposal: calls=%d state=%+v",calls,x)
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"patchs"));!os.IsNotExist(err){t.Fatalf("denied shell or unapproved Patch executed: %v",err)}
 s.runAgentAutoFeedback(context.Background())
 if calls!=2{t.Fatalf("denial feedback sent twice: %d",calls)}
}

func TestAgentEveryNonWhitelistedFunctionInBatchGetsSingleSafeFeedbackTurn(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.AutoFeedback=true;x.AutoFeedbackPersistent=true
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls==1{
   return agentchat.ResponsesResult{ResponseID:"rejected-batch",Calls:[]agentchat.FunctionCall{
    {CallID:"call_unsafe_shell",Name:"shell",Arguments:`{"command":"sudo /usr/bin/true"}`},
    {CallID:"call_unsafe_exec",Name:"exec",Arguments:`{"command":"whoami"}`},
   }},nil
  }
  if calls!=2{t.Fatalf("rejected tools triggered %d provider calls",calls)}
  var body struct{Input []struct{
   Type string `json:"type"`
   CallID string `json:"call_id"`
   Output string `json:"output"`
  } `json:"input"`}
  if err:=json.Unmarshal(payload,&body);err!=nil{t.Fatal(err)}
  found:=map[string]bool{}
  for _,item:=range body.Input{
   if item.Type!="function_call_output"{continue}
   if item.CallID!="call_unsafe_shell"&&item.CallID!="call_unsafe_exec"{continue}
   if found[item.CallID]{t.Fatalf("duplicate rejection for %s",item.CallID)}
   if !strings.Contains(item.Output,`"reason_code":"MULTIPLE_TOOL_CALLS"`)||
      !strings.Contains(item.Output,`"patch_executed":false`){
    t.Fatalf("incorrect rejection for %s: %s",item.CallID,item.Output)
   }
   found[item.CallID]=true
  }
  if len(found)!=2{t.Fatalf("missing denied call IDs: %+v",found)}
  return agentchat.ResponsesResult{ResponseID:"ack-batch",Text:"Continuing without prohibited tools"},nil
 }
 response:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"work in allowed workspace only","confirm_external":true,"expected_revision":x.Revision})
 if response.Code!=200{t.Fatalf("initial turn: %d %s",response.Code,response.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Status!="tool_rejected"||len(x.ToolCalls)!=2{t.Fatalf("batch not denied: %+v",x)}
 s.runAgentAutoFeedback(context.Background())
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=2||x.Status!="completed"{t.Fatalf("batch did not recover: calls=%d status=%s",calls,x.Status)}
 for _,call:=range x.ToolCalls{
  if call.Status!="rejected"||!call.FeedbackSent||call.FeedbackStatus!="acknowledged"||
     call.QueuePath!=""||call.PatchSessionID!=""{
    t.Fatalf("unsafe multi-call handling: %+v",call)
  }
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"patchs"));!os.IsNotExist(err){t.Fatalf("denied batch created native queue: %v",err)}
 s.runAgentAutoFeedback(context.Background())
 if calls!=2{t.Fatalf("batch replayed after acknowledgement: %d",calls)}
}

func TestAgentAutoContinueRecoversDeniedShellAfterTextCollection(t *testing.T) {
 s,id,identity:=agentTextOnlyFixture(t)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.AutoContinueText=true
 x.AutoFeedback=false
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 collection,err:=json.Marshal(agentCollectionArguments{
  Project:identity,Actions:[]agentCollectionAction{{Type:"ls",Path:".",MaxEntries:100}},
 });if err!=nil{t.Fatal(err)}
 reads,calls:=0,0
 s.AgentCollectionCall=func(_ string,_ patchtool.ProjectIdentity,_ []byte)string{
  reads++;return `{"results":[{"type":"ls","entries":[]}]}`
 }
 proposal:=agentTextOnlyHelloProposal(t,identity)
 s.AgentChatCall=func(_ context.Context,p agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if p.ID!=x.ProviderID{t.Fatal("provider binding drift")}
  var req map[string]any
  if err:=json.Unmarshal(payload,&req);err!=nil{t.Fatal(err)}
  if _,ok:=req["tools"];ok{t.Fatal("unverified tool schemas advertised")}
  switch calls{
  case 1:return agentchat.ResponsesResult{ResponseID:"collection",Text:string(collection)},nil
  case 2:return agentchat.ResponsesResult{ResponseID:"denied",Calls:[]agentchat.FunctionCall{{CallID:"shell-after-collect",Name:"shell",Arguments:`{"command":"whoami"}`}}},nil
  case 3:
   found:=false
   for _,raw:=range req["input"].([]any){
    item:=raw.(map[string]any)
    if item["type"]=="function_call_output"&&item["call_id"]=="shell-after-collect"{
     output,_:=item["output"].(string)
     found=agentRepairableDenialOutput(output)&&strings.Contains(output,"text JSON fallback")
    }
   }
   if !found{t.Fatal("denial not sent on the original call ID with fallback guidance")}
   return agentchat.ResponsesResult{ResponseID:"corrected",Text:proposal},nil
  default:t.Fatalf("unexpected provider request %d",calls);return agentchat.ResponsesResult{},nil
  }
 }
 reply:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",map[string]any{
  "message":"inspect then patch","confirm_external":true,"expected_revision":x.Revision,
 })
 if reply.Code!=200{t.Fatalf("initial request: %d %s",reply.Code,reply.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Status!="tool_rejected"||reads!=1||calls!=2{t.Fatalf("unexpected initial state: %s reads=%d calls=%d",x.Status,reads,calls)}
 s.runAgentAutoFeedback(context.Background())
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=3||x.Status!="waiting_tool"||len(x.ToolCalls)!=2||x.ToolCalls[0].FeedbackStatus!="acknowledged"||
  x.ToolCalls[0].QueuePath!=""||x.ToolCalls[1].Status!="pending"||!x.ToolCalls[1].TextFallback{
  t.Fatalf("denial recovery failed: calls=%d state=%+v",calls,x)
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"helloworld.php"));!os.IsNotExist(err){t.Fatal("unapproved patch changed source")}
 s.runAgentAutoFeedback(context.Background())
 if calls!=3{t.Fatal("denial replayed")}
}

func TestAgentDenialAutoContinueRespectsConsentAndLoopBrake(t *testing.T){
 denial:=agentchat.ToolCallRecord{CallID:"denied",Name:"shell",Status:"rejected",Result:agentRejectedUnsupportedToolFeedback("shell","UNSUPPORTED_TOOL")}
 x:=agentchat.Session{Owner:"local",Status:"tool_rejected",ToolCalls:[]agentchat.ToolCallRecord{denial}}
 if _,ok:=agentAutoFeedbackCandidate(x);ok{t.Fatal("no owner opt-in")}
 x.AutoContinueText=true
 if _,ok:=agentAutoFeedbackCandidate(x);!ok{t.Fatal("auto continue cannot recover machine denial")}
 x.ToolCalls[0].Result=agentRejectedPatchFeedback("USER_REJECTED","owner rejected",false)
 if _,ok:=agentAutoFeedbackCandidate(x);ok{t.Fatal("owner rejection bypassed")}
 x.ToolCalls[0]=denial
 x.ProviderRun=&agentchat.ProviderRun{Phase:"unknown"}
 if _,ok:=agentAutoFeedbackCandidate(x);ok{t.Fatal("unknown provider egress replayed")}
 x.ProviderRun=nil
 for i:=0;i<agentMaxConsecutiveDenials;i++{
  x.History=append(x.History,agentchat.HistoryItem{Type:"function_call_output",Content:denial.Result})
 }
 x.AutoFeedback=true;x.AutoFeedbackPersistent=true
 if _,ok:=agentAutoFeedbackCandidate(x);ok{t.Fatal("rejection loop exceeded bounded recovery")}
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"New task"})
 if _,ok:=agentAutoFeedbackCandidate(x);!ok{t.Fatal("new user turn did not reset brake")}
}
