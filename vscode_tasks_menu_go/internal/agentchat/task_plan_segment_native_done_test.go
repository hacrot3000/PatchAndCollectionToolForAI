package agentchat

import (
 "bytes"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

// The injected function models the trusted server verifier boundary; the
// HTTP integration test exercises ACTUAL native SessionService output and
// must prove real receipts, rather than accepting a mock as native authority.
func TestSegmentNativeDoneCASUsesLockedExactMilestoneSnapshot(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("1024-step owner-verified native test plan",planStepFixtures(1024))
 if err!=nil{t.Fatal(err)}
 x.TaskPlan.Steps[0].Status="done"
 x.TaskPlan.Steps[0].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Keep earlier accepted evidence"}}
 at:=time.Now().UTC()
 target:=&x.TaskPlan.Steps[501]
 target.TaskCommandGuard=&TaskCommandGuard{
  TaskID:22,CommandSHA256:strings.Repeat("a",64),BoundAt:at.Add(-2*time.Minute),
 }
 target.GoTestRequirements=&TaskGoTestRequirements{
  Package:"example.com/verified",Tests:[]string{"TestOne","TestTwo"},
  BoundAt:at.Add(-time.Minute),
 }
 target.AcceptanceCriteria=[]TaskAcceptanceCriterion{{
  ID:"owner_coverage",Description:"Owner verified all required workflows",
  ReviewNote:"Reviewed package behavior and regression cases",ReviewedAt:&at,
 }}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 beforeRefs:=append([]TaskPlanSegmentRef(nil),x.TaskPlanSegments...)
 const ref="real-native-run"
 legitimate:=TaskPlanEvidence{
  Kind:"go_test_suite",Ref:ref,Digest:strings.Repeat("c",64),
  Note:"Host-verified complete native Go test suite",
 }
 checked:=0
 verifier:=func(snapshot Session)(TaskPlanEvidence,error){
  checked++
  if snapshot.Owner!="local"||snapshot.ProjectID!="project"||
   snapshot.Revision!=x.Revision||snapshot.TaskPlan==nil||
   !snapshot.TaskPlan.CreatedAt.Equal(x.TaskPlan.CreatedAt)||
   len(snapshot.TaskPlan.Steps)!=1||snapshot.TaskPlan.Steps[0].ID!="step-0501"||
   snapshot.TaskPlan.Steps[0].GoTestRequirements==nil||
   snapshot.TaskPlan.Steps[0].GoTestRequirements.Tests[1]!="TestTwo"||
   snapshot.TaskPlan.Steps[0].TaskCommandGuard.CommandSHA256!=strings.Repeat("a",64)||
   !TaskPlanStepCriteriaReviewed(snapshot.TaskPlan.Steps[0]){
   t.Fatal("native verifier did not receive authoritative owner-bound single-step snapshot")
  }
  return legitimate,nil
 }
 page,err:=store.SetSegmentTaskPlanStepDoneNative(
  x.ID,"local",x.Revision,"step-0501","go_test_suite",ref,480,24,verifier)
 if err!=nil{t.Fatal(err)}
 if checked!=1||page.Revision!=x.Revision+1||page.Done!=2||
  page.Total!=1024||page.AllDone||page.Completed||page.Plan==nil||
  len(page.Plan.Steps)!=24||page.Plan.Steps[21].Status!="done"||
  page.Plan.Steps[21].Evidence[0].Digest!=legitimate.Digest{
  t.Fatalf("wrong verified native single-page completion: %+v",page)
 }
 saved,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if saved.TaskPlan.Steps[0].Evidence[0].Note!="Keep earlier accepted evidence"||
  saved.TaskPlan.Steps[501].Evidence[0].Kind!="go_test_suite"{
  t.Fatal("prior or newly verified acceptance evidence was lost")
 }
 for i,part:=range saved.TaskPlanSegments{
  if i==501/taskPlanSegmentSize {
   if part==beforeRefs[i]||part.Revision!=page.Revision{
    t.Fatal("target part not updated with new journal revision")
   }
  }else if part!=beforeRefs[i]{
   t.Fatalf("unrelated page %d rewritten by native milestone proof",i)
  }
 }
 if _,err=store.SetSegmentTaskPlanStepDoneNative(x.ID,"local",x.Revision,
  "step-0501","go_test_suite",ref,480,24,verifier);!errors.Is(err,ErrStaleRevision){
  t.Fatalf("stale native proof replay accepted: %v",err)
 }
 if _,err=store.SetSegmentTaskPlanStepDoneNative(x.ID,"another",page.Revision,
  "step-0501","go_test_suite",ref,480,24,verifier);!errors.Is(err,ErrNotFound){
  t.Fatalf("native proof crossed owner boundary: %v",err)
 }
 if checked!=1{t.Fatal("failed CAS request invoked expensive native verification")}
}

func TestSegmentNativeDoneCannotAcceptWrongKindsOrCorruptJournal(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Native proof must validate",planStepFixtures(160))
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 original,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 fail:=func(kind,ref string,proof TaskPlanEvidence,want error) {
  t.Helper()
  _,e:=store.SetSegmentTaskPlanStepDoneNative(x.ID,"local",x.Revision,
   "step-0135",kind,ref,128,24,func(Session)(TaskPlanEvidence,error){return proof,nil})
  if !errors.Is(e,want){t.Fatalf("kind=%s got %v want %v",kind,e,want)}
  after,e:=os.ReadFile(store.file(x.ID));if e!=nil{t.Fatal(e)}
  if !bytes.Equal(original,after){t.Fatal("rejected native receipt changed canonical journal")}
 }
 fail("manual","native",TaskPlanEvidence{Kind:"manual",Note:"Bypass"},ErrTaskPlanSegmentWriteUnsupported)
 fail("task_run","actual",TaskPlanEvidence{
  Kind:"go_test_case",Ref:"actual",Digest:strings.Repeat("a",64),
 },ErrTaskPlanCompletionProofRejected)
 fail("task_run","actual",TaskPlanEvidence{
  Kind:"task_run",Ref:"another",Digest:strings.Repeat("a",64),
 },ErrTaskPlanCompletionProofRejected)
 fail("task_run","actual",TaskPlanEvidence{
  Kind:"task_run",Ref:"actual",Digest:strings.Repeat("z",64),
 },ErrTaskPlanCompletionProofRejected)
 fail("task_run","actual",TaskPlanEvidence{
  Kind:"task_run",Ref:"actual",Digest:strings.Repeat("a",64),
  Note:strings.Repeat("x",513),
 },ErrTaskPlanCompletionProofRejected)
 // Predeclared named tests cannot be downgraded to an ordinary successful
 // Task, even when the host verifier says this Task had exit code zero.
 target:=&x.TaskPlan.Steps[135]
 now:=time.Now().UTC()
 target.TaskCommandGuard=&TaskCommandGuard{TaskID:11,
  CommandSHA256:strings.Repeat("a",64),BoundAt:now.Add(-time.Minute)}
 target.GoTestRequirements=&TaskGoTestRequirements{
  Package:"example/pkg",Tests:[]string{"TestExact"},BoundAt:now}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 invoked:=false
 _,err=store.SetSegmentTaskPlanStepDoneNative(x.ID,"local",x.Revision,
  "step-0135","task_run","actual",128,24,func(Session)(TaskPlanEvidence,error){
   invoked=true
   return TaskPlanEvidence{Kind:"task_run",Ref:"actual",Digest:strings.Repeat("a",64)},nil
  })
 if !errors.Is(err,ErrTaskPlanSegmentWriteUnsupported)||invoked{
  t.Fatalf("native exit-zero bypassed pinned Go suite: %v invoked=%v",err,invoked)
 }
}
