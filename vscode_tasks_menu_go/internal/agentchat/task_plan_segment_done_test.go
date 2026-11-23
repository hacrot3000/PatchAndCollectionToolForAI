package agentchat

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"
)

func TestSegmentDoneCASRequiresOwnerEvidenceAndPreservesUntouchedPages(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Owner-controlled 1024-step plan",planStepFixtures(1024))
 if err!=nil{t.Fatal(err)}
 x.TaskPlan.Steps[0].Status="done"
 x.TaskPlan.Steps[0].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Prior owner approval"}}
 acceptedAt:=time.Now().UTC()
 x.TaskPlan.Steps[501].AcceptanceCriteria=[]TaskAcceptanceCriterion{{
  ID:"criterion-review",Description:"Owner verified the implementation",
  ReviewNote:"Owner reviewed this requirement",ReviewedAt:&acceptedAt,
 }}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 oldRefs:=append([]TaskPlanSegmentRef(nil),x.TaskPlanSegments...)
 initial:=x.Revision
 // Invalid notes, forged native receipts and stale/wrong-owner requests
 // must not mint evidence or touch any immutable page.
 for _,tc:=range []struct{kind,ref,note string;want error}{
  {"manual","","  ",ErrTaskPlanCompletionProofRejected},
  {"manual","forged-call","Owner note",ErrTaskPlanCompletionProofRejected},
  {"patch","invented","No native receipt",ErrTaskPlanSegmentWriteUnsupported},
  {"collection","invented","No Collection output",ErrTaskPlanSegmentWriteUnsupported},
 }{
  _,e:=store.SetSegmentTaskPlanStepDone(x.ID,"local",initial,
   "step-0501",tc.kind,tc.ref,tc.note,480,24)
  if !errors.Is(e,tc.want){t.Fatalf("%+v returned %v, want %v",tc,e,tc.want)}
 }
 got,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if got.Revision!=initial||len(got.TaskPlan.Steps[501].Evidence)!=0{
  t.Fatal("refused evidence updated the canonical journal")
 }
 if _,err=store.SetSegmentTaskPlanStepDone(x.ID,"other",initial,
  "step-0501","manual","","Owner note",480,24);!errors.Is(err,ErrNotFound){
  t.Fatalf("wrong owner admitted: %v",err)
 }
 page,err:=store.SetSegmentTaskPlanStepDone(x.ID,"local",initial,
  "step-0501","manual","","Owner attests all acceptance criteria reviewed",480,24)
 if err!=nil{t.Fatal(err)}
 if page.Revision!=initial+1||page.Done!=2||page.Total!=1024||
  page.AllDone||page.Completed||page.Plan==nil||
  page.Plan.State!="active"||len(page.Plan.Steps)!=24||
  page.Plan.Steps[21].ID!="step-0501"||
  page.Plan.Steps[21].Status!="done"||
  len(page.Plan.Steps[21].Evidence)!=1||
  page.Plan.Steps[21].Evidence[0].Kind!="manual"||
  page.Plan.Steps[21].AcceptanceCriteria[0].ReviewNote!="Owner reviewed this requirement"{
  t.Fatalf("bad projected done page: %+v",page)
 }
 current,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if current.TaskPlan.Steps[0].Evidence[0].Note!="Prior owner approval"||
  current.TaskPlan.Steps[501].Evidence[0].Note!="Owner attests all acceptance criteria reviewed"{
  t.Fatal("owner evidence was lost across segmented edit")
 }
 changed:=501/taskPlanSegmentSize
 for i,ref:=range current.TaskPlanSegments{
  if i==changed {
   if ref==oldRefs[i]||ref.Revision!=page.Revision{
    t.Fatal("single modified segment did not receive new revision")
   }
  }else if ref!=oldRefs[i]{
   t.Fatalf("untouched page %d unexpectedly changed",i)
  }
 }
 if _,err=os.Stat(store.taskPlanSegmentPath(x.ID,oldRefs[changed]));!os.IsNotExist(err){
  t.Fatalf("obsolete immutable segment still exists: %v",err)
 }
 if _,err=store.SetSegmentTaskPlanStepDone(x.ID,"local",initial,
  "step-0501","manual","","stale",480,24);!errors.Is(err,ErrStaleRevision){
  t.Fatalf("stale manual completion admitted: %v",err)
 }
 handoff,rev,err:=store.Handoff(x.ID,"local")
 if err!=nil||rev!=page.Revision||!strings.Contains(handoff,"2/1024"){
  t.Fatalf("handoff not derived from verified manual acceptance: %v",err)
 }
}

func TestSegmentDoneEnforcesGoSuiteAndReviewedCriteria(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Native suite cannot be bypassed",planStepFixtures(160))
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 x.TaskPlan.Steps[135].AcceptanceCriteria=[]TaskAcceptanceCriterion{{
  ID:"accept",Description:"Owner must explicitly review coverage",
 }}
 x.TaskPlan.Steps[140].TaskCommandGuard=&TaskCommandGuard{
  TaskID:1,CommandSHA256:strings.Repeat("f",64),BoundAt:now,
 }
 x.TaskPlan.Steps[140].GoTestRequirements=&TaskGoTestRequirements{
  Package:"example/pkg",Tests:[]string{"TestExact"},BoundAt:now,
 }
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 for _,tc:=range []struct{step string;want error}{
  {"step-0135",ErrTaskPlanCompletionProofRejected},
  {"step-0140",ErrTaskPlanSegmentWriteUnsupported},
 }{
  _,e:=store.SetSegmentTaskPlanStepDone(x.ID,"local",x.Revision,
   tc.step,"manual","","Owner says done",128,24)
  if !errors.Is(e,tc.want){t.Fatalf("%s: got %v want %v",tc.step,e,tc.want)}
 }
 stored,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if stored.Revision!=x.Revision||stored.TaskPlan.Steps[135].Status!="pending"||
  stored.TaskPlan.Steps[140].Status!="pending"{
  t.Fatal("unreviewed criteria or native suite bypassed")
 }
}

func TestSegmentDoneCASRejectsCorruptOffscreenPageAndConcurrentWriters(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Reject damage and stale acceptance",planStepFixtures(192))
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 offscreen:=store.taskPlanSegmentPath(x.ID,x.TaskPlanSegments[0])
 original,err:=os.ReadFile(offscreen);if err!=nil{t.Fatal(err)}
 if err=os.WriteFile(offscreen,[]byte("[{}]"),0600);err!=nil{t.Fatal(err)}
 if _,err=store.SetSegmentTaskPlanStepDone(x.ID,"local",x.Revision,
  "step-0150","manual","","Approved",144,24);err==nil{
  t.Fatal("corrupt offscreen page allowed a completion write")
 }
 if err=os.WriteFile(offscreen,original,0600);err!=nil{t.Fatal(err)}
 const writers=7
 var wg sync.WaitGroup
 results:=make(chan error,writers)
 for i:=0;i<writers;i++{
  wg.Add(1)
  go func(i int){
   defer wg.Done()
   _,e:=store.SetSegmentTaskPlanStepDone(x.ID,"local",x.Revision,
    fmt.Sprintf("step-%04d",150+i),"manual","","Independent owner-attested test completion",144,24)
   results<-e
  }(i)
 }
 wg.Wait()
 close(results)
 accepted,stale:=0,0
 for e:=range results{
  if e==nil{accepted++}else if errors.Is(e,ErrStaleRevision){stale++}else{
   t.Fatalf("unexpected result: %v",e)
  }
 }
 if accepted!=1||stale!=writers-1{t.Fatalf("writers admitted=%d stale=%d",accepted,stale)}
 final,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if final.Revision!=x.Revision+1{
  t.Fatal("multiple accepted proof writes raced past revision CAS")
 }
 done,total:=TaskPlanProgress(final.TaskPlan)
 if done!=1||total!=192{t.Fatalf("wrong progress %d/%d",done,total)}
}
