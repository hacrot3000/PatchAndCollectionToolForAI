package agentchat

import (
 "bytes"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"
)

func newSegmentAcceptanceFixture(t *testing.T)(*Store,Session){
 t.Helper()
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Owner-reviewed milestone coverage in 1024-step plan",planStepFixtures(1024))
 if err!=nil{t.Fatal(err)}
 x.TaskPlan.Steps[0].Status="done"
 x.TaskPlan.Steps[0].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Keep earlier accepted milestone"}}
 at:=time.Now().UTC()
 x.TaskPlan.Steps[501].TaskCommandGuard=&TaskCommandGuard{
  TaskID:7,CommandSHA256:strings.Repeat("a",64),BoundAt:at.Add(-time.Minute),
 }
 x.TaskPlan.Steps[501].GoTestRequirements=&TaskGoTestRequirements{
  Package:"example.com/task",Tests:[]string{"TestOne","TestTwo"},BoundAt:at,
 }
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 if len(x.TaskPlanSegments)!=32{t.Fatal("fixture not segmented into 32 pages")}
 return store,x
}

func TestSegmentAcceptanceCriteriaAndReviewsOnlyRewriteTargetPage(t *testing.T){
 store,x:=newSegmentAcceptanceFixture(t)
 oldRefs:=append([]TaskPlanSegmentRef(nil),x.TaskPlanSegments...)
 criteria:=[]TaskAcceptanceCriterion{
  {ID:"scenario",Description:"Expected input behavior",RequiredTests:[]string{"TestOne"},
   ReviewNote:"caller-supplied review MUST be erased"},
  {ID:"compat",Description:"Legacy scenario works",RequiredTests:[]string{"TestTwo"}},
 }
 p,err:=store.SetSegmentTaskPlanAcceptanceCriteria(x.ID,"local",x.Revision,
  "step-0501",criteria,480,24)
 if err!=nil{t.Fatal(err)}
 if p.Revision!=x.Revision+1||p.Done!=1||p.Total!=1024||p.AllDone||p.Completed||
  p.Plan==nil||len(p.Plan.Steps)!=24||
  p.Plan.Steps[21].ID!="step-0501"||
  len(p.Plan.Steps[21].AcceptanceCriteria)!=2||
  p.Plan.Steps[21].AcceptanceCriteria[0].ReviewNote!=""||
  p.Plan.Steps[21].AcceptanceCriteria[0].ReviewedAt!=nil{
  t.Fatalf("matrix replace did not reset reviews or project correct page: %+v",p)
 }
 after,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 for i,ref:=range after.TaskPlanSegments{
  if i==501/taskPlanSegmentSize{
   if ref==oldRefs[i]||ref.Revision!=p.Revision{
    t.Fatal("only changed segment must have new journal revision")
   }
  }else if ref!=oldRefs[i]{
   t.Fatalf("unrelated immutable segment %d rewritten",i)
  }
 }
 if after.TaskPlan.Steps[0].Evidence[0].Note!="Keep earlier accepted milestone"{
  t.Fatal("previously accepted milestone evidence lost")
 }
 if len(after.Events)==0||after.Events[len(after.Events)-1].Detail!="set_acceptance_criteria"{
  t.Fatal("canonical journal did not record owner matrix replacement")
 }
 for _,tc:=range []struct{id,note string}{
  {"scenario","I inspected the exact input behavior"},
  {"compat","I checked compatibility with existing callers"},
 }{
  p,err=store.ReviewSegmentTaskPlanAcceptanceCriterion(
   x.ID,"local",after.Revision,"step-0501",tc.id,tc.note,480,24)
  if err!=nil{t.Fatal(err)}
  if p.Done!=1||p.Plan.Steps[21].Status!="pending"||
   len(p.Plan.Steps[21].Evidence)!=0{
   t.Fatal("human criterion review silently granted milestone completion")
  }
  after,err=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 }
 if !TaskPlanStepCriteriaReviewed(after.TaskPlan.Steps[501])||
  after.TaskPlan.Steps[501].AcceptanceCriteria[0].ReviewNote!="I inspected the exact input behavior"||
  after.TaskPlan.Steps[501].AcceptanceCriteria[1].ReviewNote!="I checked compatibility with existing callers"{
  t.Fatal("canonical reviewed matrix lost required attestations")
 }
 if len(after.Events)==0||after.Events[len(after.Events)-1].Detail!="review_acceptance_criterion"{
  t.Fatal("review action missing from durable journal")
 }
 // A change in the owner matrix must invalidate ALL prior reviews even
 // when the caller preserves criterion IDs and required Go test mappings.
 replacement:=[]TaskAcceptanceCriterion{
  {ID:"scenario",Description:"Modified expected behavior",RequiredTests:[]string{"TestOne"}},
  {ID:"compat",Description:"Legacy scenario works",RequiredTests:[]string{"TestTwo"}},
 }
 p,err=store.SetSegmentTaskPlanAcceptanceCriteria(
  x.ID,"local",after.Revision,"step-0501",replacement,480,24)
 if err!=nil{t.Fatal(err)}
 if TaskPlanStepCriteriaReviewed(p.Plan.Steps[21])||
  p.Plan.Steps[21].AcceptanceCriteria[0].ReviewedAt!=nil||
  p.Plan.Steps[21].AcceptanceCriteria[1].ReviewNote!=""{
  t.Fatal("changed owner acceptance text inherited stale reviews")
 }
 handoff,rev,err:=store.Handoff(x.ID,"local")
 if err!=nil||rev!=p.Revision||!strings.Contains(handoff,"1/1024"){
  t.Fatalf("paged owner matrix mutation did not update canonical handoff: %v",err)
 }
}

func TestSegmentAcceptanceRefusesCorruptionForgedMappingsAndStaleOwner(t *testing.T){
 store,x:=newSegmentAcceptanceFixture(t)
 original,err:=os.ReadFile(store.file(x.ID))
 if err!=nil{t.Fatal(err)}
 reject:=func(want error,fn func()error){
  t.Helper()
  e:=fn()
  if want==nil{
   if e==nil{t.Fatal("corrupted offscreen page admitted")}
  }else if !errors.Is(e,want){t.Fatalf("expected %v; got %v",want,e)}
  now,e:=os.ReadFile(store.file(x.ID))
  if e!=nil{t.Fatal(e)}
  if !bytes.Equal(original,now){t.Fatal("rejected acceptance edit modified canonical journal")}
 }
 reject(ErrNotFound,func()error{
  _,e:=store.SetSegmentTaskPlanAcceptanceCriteria(x.ID,"wrong-owner",x.Revision,
   "step-0501",nil,480,24);return e
 })
 reject(ErrStaleRevision,func()error{
  _,e:=store.SetSegmentTaskPlanAcceptanceCriteria(x.ID,"local",x.Revision+1,
   "step-0501",nil,480,24);return e
 })
 reject(ErrTaskPlanAcceptanceInvalid,func()error{
  _,e:=store.SetSegmentTaskPlanAcceptanceCriteria(x.ID,"local",x.Revision,
   "step-0501",[]TaskAcceptanceCriterion{{
    ID:"unmapped",Description:"No such test was bound",RequiredTests:[]string{"TestInvented"},
   }},480,24);return e
 })
 reject(ErrTaskPlanAcceptanceInvalid,func()error{
  _,e:=store.SetSegmentTaskPlanAcceptanceCriteria(x.ID,"local",x.Revision,
   "step-0501",[]TaskAcceptanceCriterion{
    {ID:"duplicate",Description:"One"},
    {ID:"duplicate",Description:"Two"},
   },480,24);return e
 })
 reject(ErrTaskPlanCriterionNotFound,func()error{
  _,e:=store.ReviewSegmentTaskPlanAcceptanceCriterion(x.ID,"local",x.Revision,
   "step-0501","missing","Human note",480,24);return e
 })
 reject(ErrTaskPlanAcceptanceInvalid,func()error{
  _,e:=store.ReviewSegmentTaskPlanAcceptanceCriterion(x.ID,"local",x.Revision,
   "step-0501","existing","  ",480,24);return e
 })
 reject(ErrTaskPlanAcceptanceLocked,func()error{
  _,e:=store.SetSegmentTaskPlanAcceptanceCriteria(x.ID,"local",x.Revision,
   "step-0000",[]TaskAcceptanceCriterion{{ID:"a",Description:"Cannot modify done step"}},0,24);return e
 })
 reject(ErrTaskPlanPageOutOfRange,func()error{
  _,e:=store.SetSegmentTaskPlanAcceptanceCriteria(x.ID,"local",x.Revision,
   "step-0501",nil,1025,24);return e
 })
 // A damaged OFFSCREEN page must block mutation before any new immutable
 // segment or revised manifest is created.
 file:=store.taskPlanSegmentPath(x.ID,x.TaskPlanSegments[0])
 raw,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
 if err=os.WriteFile(file,[]byte("[{}]"),0600);err!=nil{t.Fatal(err)}
 reject(nil,func()error{
  _,e:=store.SetSegmentTaskPlanAcceptanceCriteria(x.ID,"local",x.Revision,
   "step-0501",[]TaskAcceptanceCriterion{{ID:"a",Description:"A valid criterion"}},480,24)
  return e
 })
 if err=os.WriteFile(file,raw,0600);err!=nil{t.Fatal(err)}
}

func TestConcurrentSegmentAcceptanceCASAdmitsOnlyOneOwnerReview(t *testing.T){
 store,x:=newSegmentAcceptanceFixture(t)
 p,err:=store.SetSegmentTaskPlanAcceptanceCriteria(x.ID,"local",x.Revision,
  "step-0501",[]TaskAcceptanceCriterion{{ID:"workflow",Description:"Review native behavior"}},480,24)
 if err!=nil{t.Fatal(err)}
 const workers=8
 outcomes:=make(chan error,workers)
 var wg sync.WaitGroup
 for i:=0;i<workers;i++{
  wg.Add(1)
  go func(i int){
   defer wg.Done()
   _,e:=store.ReviewSegmentTaskPlanAcceptanceCriterion(x.ID,"local",p.Revision,
    "step-0501","workflow",fmt.Sprintf("User review %d",i),480,24)
   outcomes<-e
  }(i)
 }
 wg.Wait();close(outcomes)
 ok,stale:=0,0
 for e:=range outcomes{
  switch{
  case e==nil:ok++
  case errors.Is(e,ErrStaleRevision):stale++
  default:t.Fatalf("unexpected concurrent outcome: %v",e)
  }
 }
 if ok!=1||stale!=workers-1{
  t.Fatalf("multiple concurrent owner review writes accepted: ok=%d stale=%d",ok,stale)
 }
 current,err:=store.Get(x.ID,"local")
 if err!=nil{t.Fatal(err)}
 if current.Revision!=p.Revision+1||
  len(current.TaskPlan.Steps[501].AcceptanceCriteria)!=1||
  current.TaskPlan.Steps[501].AcceptanceCriteria[0].ReviewedAt==nil{
  t.Fatal("accepted owner review did not survive serialized CAS")
 }
}
