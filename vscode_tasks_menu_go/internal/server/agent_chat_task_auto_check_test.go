package server

import (
 "context"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAutoCheckReconcilesVerifiedPatchReceiptExactlyOnce(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=agentchat.NewTaskPlan("Verify an implemented file",[]agentchat.TaskPlanStep{
  {ID:"fix",Title:"Apply approved Patch",Acceptance:"Verified PASS native Patch wrapper"},
 })
 if err!=nil{t.Fatal(err)}
 x.TaskPlan.Steps[0].AutoCheck=&agentchat.TaskAutoCheck{Kind:"patch_pass",CallID:"proof-native"}
 x.Status="completed"
 x.History=append(x.History,
  agentchat.HistoryItem{Type:"user",Content:"Complete the fix"},
  agentchat.HistoryItem{Type:"function_call",CallID:"proof-native",Name:"taskdeck_submit_patch",Arguments:`{"name":"Approved Patch"}`},
  agentchat.HistoryItem{Type:"function_call_output",CallID:"proof-native",Content:`{"status":"PASS","run_id":"run-native-1","rc":0}`},
 )
 x.ToolCalls=append(x.ToolCalls,agentchat.ToolCallRecord{
  CallID:"proof-native",Name:"taskdeck_submit_patch",Status:"done",
  FeedbackSent:true,FeedbackStatus:"acknowledged",RunID:"run-native-1",
  PackageSHA256:strings.Repeat("a",64),PackageSize:100,PatchSessionID:"native-session-1",
  Result:`{"status":"PASS","run_id":"run-native-1","rc":0}`,
 })
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 s.autoCheckTaskPlanOne(context.Background(),store,id)
 checked,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if checked.TaskPlan.Steps[0].Status!="done"||len(checked.TaskPlan.Steps[0].Evidence)!=1{
  t.Fatalf("verified native proof was not adopted: %+v",checked.TaskPlan)
 }
 if checked.TaskPlan.State=="completed"{t.Fatal("Patch wrapper PASS was treated as completed whole plan")}
 if checked.TaskPlan.Steps[0].Evidence[0].Kind!="patch"{t.Fatal("wrong tool evidence provenance")}
 revision:=checked.Revision
 s.autoCheckTaskPlanOne(context.Background(),store,id)
 again,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if again.Revision!=revision||len(again.TaskPlan.Steps[0].Evidence)!=1{
  t.Fatal("same verified receipt was recorded twice")
 }
}
func TestAutoCheckAPIRequiresExplicitOwnerConsentAndExactProof(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 path:="/api/agent/sessions/"+id+"/plan"
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 created:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"create","expected_revision":x.Revision,"goal":"Read one source file",
  "steps":[]map[string]string{{"id":"read","title":"Inspect source","acceptance":"Full read of src/main.go"}},
 })
 if created.Code!=200{t.Fatalf("plan creation: %d %s",created.Code,created.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 unapproved:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_auto_check","expected_revision":x.Revision,"step_id":"read","enabled":true,
  "auto_kind":"full_file_read","auto_call_id":"read-exact","auto_path":"src/main.go",
 })
 if unapproved.Code!=403{t.Fatalf("owner consent not enforced: %d %s",unapproved.Code,unapproved.Body)}
 approved:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_auto_check","expected_revision":x.Revision,"step_id":"read","enabled":true,
  "confirm_auto_accept":true,
  "auto_kind":"full_file_read","auto_call_id":"read-exact","auto_path":"src/main.go",
 })
 if approved.Code!=200{t.Fatalf("explicit safe auto-check refused: %d %s",approved.Code,approved.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan.Steps[0].Status!="pending"||x.TaskPlan.Steps[0].AutoCheck==nil{
  t.Fatal("unobserved Collection was prematurely accepted")
 }
 x.History=append(x.History,
  agentchat.HistoryItem{Type:"user",Content:"Inspect the source file"},
  agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"read-exact",
   Arguments:`{"actions":[{"type":"read","path":"src/main.go"}]}`},
  agentchat.HistoryItem{Type:"function_call_output",CallID:"read-exact",
   Content:`{"results":[{"type":"read","files":[{"path":"src/main.go","content":"package main
"}]}]}`},
 )
 x.Status="completed"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if !agentchat.TaskAutoCheckReady(x){t.Fatal("completed exact read did not activate scheduler")}
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 checked,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if checked.TaskPlan.Steps[0].Status!="done"||len(checked.TaskPlan.Steps[0].Evidence)!=1{
  t.Fatalf("future exact Collection success not automatically accepted: %+v",checked.TaskPlan)
 }
 if checked.TaskPlan.State=="completed"{t.Fatal("machine-read proof silently completed whole plan")}
}
