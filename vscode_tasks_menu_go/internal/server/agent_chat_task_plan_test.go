package server

import (
 "context"
 "fmt"
 "net/http"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentTaskPlanAPIRequiresNativeOrExplicitManualEvidence(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 path:="/api/agent/sessions/"+id+"/plan"
 create:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"create","expected_revision":1,
  "goal":"Investigate and fix the application end-to-end",
  "steps":[]map[string]string{
   {"id":"research","title":"Inspect files","acceptance":"Read verified sources"},
   {"id":"repair","title":"Repair code","acceptance":"Confirm native Patch result"},
  },
 })
 if create.Code!=200{t.Fatalf("create plan failed: %d %s",create.Code,create.Body)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan==nil||len(x.TaskPlan.Steps)!=2||x.TaskPlan.AutoContinue{
  t.Fatal("new plan was not durable or enabled automation without consent")
 }
 denied:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_auto_continue","enabled":true,"expected_revision":x.Revision,
 })
 if denied.Code!=http.StatusForbidden{t.Fatalf("auto provider egress enabled without consent: %d",denied.Code)}
 stale:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"pause","expected_revision":1,
 })
 if stale.Code!=409{t.Fatalf("stale plan edit accepted: %d",stale.Code)}
 fake:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_step","step_id":"repair","status":"done","evidence_kind":"patch",
  "evidence_ref":"fabricated-patch-id","expected_revision":x.Revision,
 })
 if fake.Code!=409{t.Fatalf("fake provider Patch receipt accepted: %d",fake.Code)}
 // Persist real, paired read and Native Patch feedback.
 x.History=append(x.History,
  agentchat.HistoryItem{Type:"user",Content:"Original project goal"},
  agentchat.HistoryItem{Type:"function_call",CallID:"collect-proof",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"read","path":"src/main.go"}]}`},
  agentchat.HistoryItem{Type:"function_call_output",CallID:"collect-proof",Content:`{"results":[{"files":[{"path":"src/main.go"}]}]}`},
  agentchat.HistoryItem{Type:"function_call",CallID:"patch-proof",Name:"taskdeck_submit_patch",Arguments:`{"name":"Fix main.go"}`},
  agentchat.HistoryItem{Type:"function_call_output",CallID:"patch-proof",Content:`{"status":"PASS","rc":0}`},
 )
 x.ToolCalls=append(x.ToolCalls,agentchat.ToolCallRecord{
  CallID:"patch-proof",Name:"taskdeck_submit_patch",Status:"done",
  Result:`{"status":"PASS","rc":0}`,FeedbackSent:true,FeedbackStatus:"acknowledged",
 })
 x.Status="completed"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 for _,row:=range []struct{step,kind,ref string}{
  {"research","collection","collect-proof"},{"repair","patch","patch-proof"},
 }{
  res:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
   "action":"set_step","expected_revision":x.Revision,"step_id":row.step,
   "status":"done","evidence_kind":row.kind,"evidence_ref":row.ref,
   "evidence_note":"Verified evidence checked by human",
  })
  if res.Code!=200{t.Fatalf("failed to mark verified step %s: %d %s",row.step,res.Code,res.Body)}
  x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 }
 if !agentchat.TaskPlanAllDone(x.TaskPlan){t.Fatal("verified milestone evidence was lost")}
 closed:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"complete","expected_revision":x.Revision,
 })
 if closed.Code!=200{t.Fatalf("verified plan could not close: %d %s",closed.Code,closed.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan.State!="completed"||x.TaskPlan.AutoContinue{
  t.Fatalf("plan marked incorrectly: %+v",x.TaskPlan)
 }
 summary:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,path,nil)
 if summary.Code!=200||!strings.Contains(summary.Body.String(),`"completed":true`){
  t.Fatalf("owner plan summary incorrect: %d %s",summary.Code,summary.Body)
 }
 handoff,_,err:=store.Handoff(id,"local")
 if err!=nil||!strings.Contains(handoff,"Durable task plan (2/2)")||
  !strings.Contains(handoff,"patch-proof"){t.Fatalf("handoff does not contain milestone proof: %v %s",err,handoff)}
 cannotEdit:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_step","expected_revision":x.Revision,"step_id":"repair","status":"pending",
 })
 if cannotEdit.Code!=409{t.Fatal("completed plan was silently editable")}
 reopened:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"reopen","expected_revision":x.Revision,
 })
 if reopened.Code!=200{t.Fatalf("explicit plan reopen refused: %d %s",reopened.Code,reopened.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan.State!="paused"||x.TaskPlan.AutoContinue{t.Fatal("reopened plan did not become paused")}
}

func TestAgentAutoTaskPlanContinuationStallsWithoutVerifiedWork(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 plan,err:=agentchat.NewTaskPlan("Fix all issues and verify source code",[]agentchat.TaskPlanStep{
  {ID:"finish",Title:"Implement and test",Acceptance:"Verified Collection/Patch or user acceptance"},
 })
 if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.TaskPlan=plan
 x.Status="completed"
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"Original fix-all request"})
 x.TaskPlan.AutoContinue=true // mimics explicit confirmed plan setting
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++
  return agentchat.ResponsesResult{ResponseID:fmt.Sprintf("resp-%d",calls),
   Text:"No verified change yet. I will continue."},nil
 }
 for i:=0;i<agentchat.MaxContinuationNoWorkTurns;i++{
  s.autoContinueTaskPlanOne(context.Background(),store,id)
 }
 got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=agentchat.MaxContinuationNoWorkTurns||got.TaskPlan.State!="paused"||got.TaskPlan.AutoContinue||
  got.TaskPlan.NoProgressTurns!=agentchat.MaxContinuationNoWorkTurns {
  t.Fatalf("no-progress guard failed, calls=%d plan=%+v",calls,got.TaskPlan)
 }
 s.autoContinueTaskPlanOne(context.Background(),store,id)
 if calls!=agentchat.MaxContinuationNoWorkTurns{t.Fatal("paused plan continued without renewed consent")}
 var stalled bool
 for _,ev:=range got.Events{if ev.Kind=="plan_stalled"{stalled=true}}
 if !stalled{t.Fatal("stalled plan did not persist a visible recovery event")}
}
func TestAgentAutoPlanNeverReplaysAmbiguousProviderTurn(t *testing.T){
 plan,err:=agentchat.NewTaskPlan("Goal",[]agentchat.TaskPlanStep{
  {ID:"one",Title:"Inspect",Acceptance:"Check evidence"},
 })
 if err!=nil{t.Fatal(err)}
 plan.AutoContinue=true
 x:=agentchat.Session{Owner:"local",Status:"completed",TaskPlan:plan}
 if !agentchat.TaskPlanAutoContinueCandidate(x){t.Fatal("local opt-in not eligible")}
 x.Status="failed"
 if agentchat.TaskPlanAutoContinueCandidate(x){t.Fatal("failed provider turn was resent")}
 x.Status="completed"
 x.ProviderRun=&agentchat.ProviderRun{Phase:"unknown"}
 if agentchat.TaskPlanAutoContinueCandidate(x){t.Fatal("ambiguous provider turn was resent")}
 x.ProviderRun=nil
 x.ToolCalls=[]agentchat.ToolCallRecord{{Status:"done",FeedbackSent:true,FeedbackStatus:"unknown"}}
 if agentchat.TaskPlanAutoContinueCandidate(x){t.Fatal("unknown Patch feedback was bypassed")}
}

func TestAgentTaskPlanAdmissionBackoffPersistsWithoutProviderEgress(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=agentchat.NewTaskPlan("Continue a large task",[]agentchat.TaskPlanStep{
  {ID:"step1",Title:"Finish research",Acceptance:"Verified Collection evidence"},
 })
 if err!=nil{t.Fatal(err)}
 x.TaskPlan.AutoContinue=true
 x.Status="completed"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 s.deferTaskPlanAdmission(store,id,http.StatusTooManyRequests)
 got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if got.TaskPlan.RetryStreak!=1||got.TaskPlan.NextRetryAt==nil||
  time.Until(*got.TaskPlan.NextRetryAt)<=0||
  agentchat.TaskPlanAutoContinueCandidate(got){
  t.Fatalf("rate admission did not persist a safe wait: %+v",got.TaskPlan)
 }
 // A fresh Store represents daemon reinitialization: the backoff is
 // persisted in the session, not dependent on an in-memory timer.
 reopened,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 afterRestart,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
 if afterRestart.TaskPlan.NextRetryAt==nil||
  agentchat.TaskPlanAutoContinueCandidate(afterRestart){
  t.Fatal("restart forgot retry-after and would hammer provider")
 }
 // A permanently invalid provider request pauses, but never marks
 // the user's unfinished task complete.
 s.deferTaskPlanAdmission(store,id,http.StatusRequestEntityTooLarge)
 blocked,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if blocked.TaskPlan.State!="paused"||blocked.TaskPlan.AutoContinue||
  agentchat.TaskPlanAllDone(blocked.TaskPlan) {
  t.Fatalf("permanent request error silently finished plan: %+v",blocked.TaskPlan)
 }
}
