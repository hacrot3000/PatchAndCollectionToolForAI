package server

import (
 "context"
 "fmt"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

const auditProgress="Đã tiếp tục thu thập bằng chứng cho M03–M06 và lưu checkpoint bền vững. "+
 "Audit chưa hoàn tất: chưa phân loại đủ 37 trường hợp. "+
 "Bước tiếp theo: kiểm tra save/worker/runtime của M03–M06, sau đó M07–M10."

func newStandaloneAutoContinueFixture(t *testing.T)(*Server,*agentchat.Store,string){
 t.Helper()
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.Status="completed"
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"Audit 37 cases"})
 agentchat.RecordTurnDisposition(&x,auditProgress)
 _,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 return s,store,id
}
func TestAgentStandaloneContinuationRequiresUserConsent(t *testing.T){
 s,store,id:=newStandaloneAutoContinueFixture(t)
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++;return agentchat.ResponsesResult{ResponseID:"not-authorized",Text:auditProgress},nil
 }
 s.autoContinueConversationOne(context.Background(),store,id)
 if calls!=0{t.Fatal("sent provider call without owner consent")}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/automation"
 unauthorized:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"continue","enabled":true,"expected_revision":x.Revision,
 })
 if unauthorized.Code!=403{t.Fatalf("consent gate refused wrong status: %d",unauthorized.Code)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 enabled:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"continue","enabled":true,"expected_revision":x.Revision,"confirm_external":true,
 })
 if enabled.Code!=200{t.Fatalf("opt-in refused: %d %s",enabled.Code,enabled.Body)}
 s.autoContinueConversationOne(context.Background(),store,id)
 if calls!=1{t.Fatalf("no automatic continuation after opt-in: %d",calls)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Continuation==nil||x.Continuation.NoProgressTurns!=1{
  t.Fatalf("auto turn disposition was not recorded: %+v",x.Continuation)
 }
}
func TestAgentStandaloneUnfinishedAuditAutomaticallyContinuesThenStalls(t *testing.T){
 s,store,id:=newStandaloneAutoContinueFixture(t)
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.AutoContinueText=true // simulates explicit owner opt-in after separate consent test
 _,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls==1&&(!strings.Contains(string(payload),"M07")||!strings.Contains(string(payload),"Collection")){
   t.Fatal("continuation failed to preserve concrete audit action and Collection instruction")
  }
  return agentchat.ResponsesResult{ResponseID:fmt.Sprintf("follow-up-%d",calls),Text:auditProgress},nil
 }
 for i:=0;i<agentchat.MaxContinuationNoWorkTurns;i++{
  s.autoContinueConversationOne(context.Background(),store,id)
 }
 got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=agentchat.MaxContinuationNoWorkTurns||got.AutoContinueText||
  got.Continuation==nil||got.Continuation.NoProgressTurns!=agentchat.MaxContinuationNoWorkTurns{
  t.Fatalf("text loop not stopped safely, calls=%d continuation=%+v",calls,got.Continuation)
 }
 var stalled bool
 for _,event:=range got.Events{if event.Kind=="continuation_stalled"{stalled=true}}
 if !stalled{t.Fatal("no durable stall event")}
 s.autoContinueConversationOne(context.Background(),store,id)
 if calls!=agentchat.MaxContinuationNoWorkTurns{t.Fatal("stalled conversation requested provider again")}
}
func TestAgentStandaloneContinuationRespectsNativeAndProviderFences(t *testing.T){
 s,store,id:=newStandaloneAutoContinueFixture(t)
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.AutoContinueText=true
 x.Status="waiting_tool"
 _,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++;return agentchat.ResponsesResult{ResponseID:"never",Text:auditProgress},nil
 }
 s.autoContinueConversationOne(context.Background(),store,id)
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.Status="completed"
 x.ToolCalls=[]agentchat.ToolCallRecord{{CallID:"pending-result",Name:"taskdeck_submit_patch",
  Status:"done",FeedbackStatus:"unknown"}}
 // A manufactured final receipt is not a valid canonical Store record;
 // check the scheduler's pure safety predicate, not Store.Save.
 if agentchat.StandaloneAutoContinueCandidate(x){t.Fatal("unresolved tool feedback was replayed")}
 x.ToolCalls=nil
 if calls!=0{t.Fatal("unresolved tool unexpectedly sent provider data")}
 x.ProviderRun=&agentchat.ProviderRun{Phase:"unknown"}
 if agentchat.StandaloneAutoContinueCandidate(x){t.Fatal("ambiguous provider outcome bypassed")}
}


// Reproduce the incident: existing 20 admissions from the former quota
// must not block a consented, otherwise eligible continuation.
func TestAgentStandaloneContinuationAfterLegacyHourlyQuota(t *testing.T) {
 s,store,id:=newStandaloneAutoContinueFixture(t)
 exhaustLegacyAgentQuota(t,s)
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.AutoContinueText=true
 if _,err=store.Save(x,x.Revision);err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++
  return agentchat.ResponsesResult{ResponseID:"quota-free-continuation",Text:auditProgress},nil
 }
 s.autoContinueConversationOne(context.Background(),store,id)
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=1||x.Status!="completed"||x.Continuation==nil||x.Continuation.NextRetryAt!=nil{
  t.Fatalf("continuation blocked by obsolete quota: calls=%d state=%s continuation=%+v",calls,x.Status,x.Continuation)
 }
}
