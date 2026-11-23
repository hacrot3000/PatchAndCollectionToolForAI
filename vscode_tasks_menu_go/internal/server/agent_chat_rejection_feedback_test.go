package server

import (
 "context"
 "encoding/json"
 "os"
 "path/filepath"
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
