package agentchat

import (
 "path/filepath"
 "strings"
 "fmt"
 "testing"
)

func TestDurableTaskPlanRequiresEvidenceAndPreservesOriginalGoal(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/test/project","project-id","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Implement and verify the requested application",
  []TaskPlanStep{{ID:"research",Title:"Inspect sources",Acceptance:"Verified Collection evidence"},
   {ID:"fix",Title:"Apply patch",Acceptance:"Acknowledged native Patch receipt"}})
 if err!=nil{t.Fatal(err)}
 x.History=append(x.History,HistoryItem{Type:"user",Content:"Original user request"})
 x,err=store.Save(x,0)
 if err!=nil{t.Fatal(err)}
 x.History=append(x.History,
  HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"collect-1",Arguments:`{"actions":[{"type":"read","path":"main.go"}]}`},
  HistoryItem{Type:"function_call_output",CallID:"collect-1",Content:`{"results":[{"path":"main.go","content":"verified source"}]}`},
  HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"patch-1",Arguments:`{"name":"Fix one bug"}`},
  HistoryItem{Type:"function_call_output",CallID:"patch-1",Content:`{"status":"PASS","rc":0}`},
 )
 x.ToolCalls=append(x.ToolCalls,ToolCallRecord{
  CallID:"patch-1",Name:"taskdeck_submit_patch",Status:"done",Result:`{"status":"PASS","rc":0}`,
  FeedbackSent:true,FeedbackStatus:"acknowledged",
 })
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 if _,err=VerifyTaskPlanEvidence(x,"patch","not-real","");err==nil{
  t.Fatal("model claim was accepted without native receipt")
 }
 read,e:=VerifyTaskPlanEvidence(x,"collection","collect-1","read completed")
 if e!=nil{t.Fatal(e)}
 if err=UpdateTaskPlanStep(&x,"research","done",&read);err!=nil{t.Fatal(err)}
 if TaskPlanAllDone(x.TaskPlan){t.Fatal("plan finished after only first task")}
 patch,e:=VerifyTaskPlanEvidence(x,"patch","patch-1","verified native result")
 if e!=nil{t.Fatal(e)}
 if err=UpdateTaskPlanStep(&x,"fix","done",&patch);err!=nil{t.Fatal(err)}
 if !TaskPlanAllDone(x.TaskPlan){t.Fatal("both evidenced tasks should be eligible for owner close")}
 x.TaskPlan.State="completed"
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 reopened,err:=NewStore(store.dir);if err!=nil{t.Fatal(err)}
 got,err:=reopened.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if got.TaskPlan==nil||got.TaskPlan.State!="completed"||
  len(got.TaskPlan.Steps[1].Evidence)!=1||
  got.History[0].Content!="Original user request"{t.Fatal("plan, evidence or original goal lost after restart")}
 handoff,_,err:=reopened.Handoff(x.ID,"local")
 if err!=nil||!strings.Contains(handoff,"Durable task plan (2/2)")||
  !strings.Contains(handoff,"Acknowledged native Patch receipt"){t.Fatalf("handoff lost acceptance proof: %v\n%s",err,handoff)}
}
func TestTaskPlanInputIntegrityAndUnverifiedWorkDoesNotAdvance(t *testing.T){
 _,err:=NewTaskPlan("goal",[]TaskPlanStep{
  {ID:"duplicate",Title:"First",Acceptance:"test"},
  {ID:"duplicate",Title:"Second",Acceptance:"test"},
 })
 if err==nil{t.Fatal("duplicate milestone accepted")}
 p,err:=NewTaskPlan("plan",[]TaskPlanStep{{ID:"a",Title:"A",Acceptance:"Acceptance gate"}})
 if err!=nil{t.Fatal(err)}
 x:=Session{TaskPlan:p}
 if err=UpdateTaskPlanStep(&x,"a","done",nil);err==nil{
  t.Fatal("AI model completion without evidence was accepted")
 }
 if x.TaskPlan.Steps[0].Status=="done"{t.Fatal("step mutated after failed validation")}
 x.Owner="local";x.Status="completed"
 p.AutoContinue=true
 if !TaskPlanAutoContinueCandidate(x){t.Fatal("valid local plan did not qualify for autonomous continuation")}
 x.ProviderRun=&ProviderRun{Phase:"unknown"}
 if TaskPlanAutoContinueCandidate(x){t.Fatal("ambiguous provider egress allowed automatic new request")}
}

func TestTaskPlanEvidenceLooksUpOldArchivedCollectionsAndPatches(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/project","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Finish a multi-day project",[]TaskPlanStep{
  {ID:"study",Title:"Research old file",Acceptance:"Read evidence"},
  {ID:"implement",Title:"Implement old patch",Acceptance:"Native Patch verified"},
 })
 if err!=nil{t.Fatal(err)}
 x.History=append(x.History,HistoryItem{Type:"user",Content:"Original durable task"})
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 for i:=0;i<30;i++{
  id:=fmt.Sprintf("old-patch-%d",i)
  x.History=append(x.History,
   HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_submit_patch",Arguments:`{"name":"Patch older code"}`},
   HistoryItem{Type:"function_call_output",CallID:id,Content:`{"status":"PASS","rc":0}`})
  x.ToolCalls=append(x.ToolCalls,ToolCallRecord{
   CallID:id,Name:"taskdeck_submit_patch",Status:"done",
   Result:`{"status":"PASS","rc":0}`,
   FeedbackSent:true,FeedbackStatus:"acknowledged",
  })
 }
 for i:=0;i<160;i++{
  id:=fmt.Sprintf("old-collect-%d",i)
  x.History=append(x.History,
   HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"read","path":"src/file.go"}]}`},
   HistoryItem{Type:"function_call_output",CallID:id,Content:`{"results":[{"files":[{"path":"src/file.go"}]}]}`})
 }
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatalf("bulk task evidence save: %v",err)}
 if x.PatchArchiveParts==0||x.HistoryArchiveParts==0{
  t.Fatalf("test did not archive real evidence: patch=%d read=%d",x.PatchArchiveParts,x.HistoryArchiveParts)
 }
 oldPatch,err:=VerifyArchivedTaskPlanEvidence(store,x,"patch","old-patch-0","old receipt")
 if err!=nil{t.Fatal(err)}
 oldRead,err:=VerifyArchivedTaskPlanEvidence(store,x,"collection","old-collect-0","old files")
 if err!=nil{t.Fatal(err)}
 if oldRead.Digest==""||oldPatch.Digest==""{t.Fatal("archived evidence proof has no digest")}
 if err=UpdateTaskPlanStep(&x,"study","done",&oldRead);err!=nil{t.Fatal(err)}
 if err=UpdateTaskPlanStep(&x,"implement","done",&oldPatch);err!=nil{t.Fatal(err)}
 x.TaskPlan.State="completed"
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatalf("archived evidence did not survive another session revision: %v",err)}
 if _,err=VerifyArchivedTaskPlanEvidence(store,x,"patch","invented","");err==nil{
  t.Fatal("fabricated old Patch passed archived proof lookup")
 }
}
