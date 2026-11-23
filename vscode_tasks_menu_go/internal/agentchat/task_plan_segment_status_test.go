package agentchat

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
)

func TestSegmentStatusCASWritesOnlyOnePartAndPreservesProofs(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Long owner-controlled plan",planStepFixtures(1024))
 if err!=nil{t.Fatal(err)}
 x.TaskPlan.Steps[0].Status="done"
 x.TaskPlan.Steps[0].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Owner accepted first milestone"}}
 x.TaskPlan.Steps[501].Status="done"
 x.TaskPlan.Steps[501].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Keep accepted proof after reopening"}}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 before:=append([]TaskPlanSegmentRef(nil),x.TaskPlanSegments...)
 oldRevision:=x.Revision
 page,err:=store.SetSegmentTaskPlanStepStatus(x.ID,"local",oldRevision,
  "step-0501","blocked",480,24)
 if err!=nil{t.Fatal(err)}
 if page.Revision!=oldRevision+1||page.Done!=1||page.Total!=1024||
  page.AllDone||page.Completed||page.Plan==nil||
  page.Plan.State!="active"||page.Page.Offset!=480||
  page.Page.Limit!=24||!page.Page.HasNext||len(page.Plan.Steps)!=24||
  page.Plan.Steps[21].ID!="step-0501"||
  page.Plan.Steps[21].Status!="blocked"||
  page.Plan.Steps[21].Evidence[0].Note!="Keep accepted proof after reopening"{
  t.Fatalf("wrong projected response after one-page edit: %+v",page)
 }
 next,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if next.Revision!=page.Revision||next.TaskPlan.Steps[501].Status!="blocked"||
  next.TaskPlan.Steps[501].Evidence[0].Note!="Keep accepted proof after reopening"||
  next.TaskPlan.Steps[0].Evidence[0].Note!="Owner accepted first milestone"{
  t.Fatal("journal rehydration lost the changed status or native evidence")
 }
 if len(next.Events)==0||next.Events[len(next.Events)-1].Detail!="set_step"{
  t.Fatal("direct path missed the durable plan_updated event")
 }
 changedIndex:=501/taskPlanSegmentSize
 for i,ref:=range next.TaskPlanSegments{
  if i==changedIndex{
   if ref.Revision!=page.Revision||ref==before[i]{
    t.Fatal("target segment was not copied to the new revision")
   }
  }else if ref!=before[i]{
   t.Fatalf("unchanged segment %d unexpectedly rewritten",i)
  }
 }
 if _,err=os.Stat(store.taskPlanSegmentPath(x.ID,before[changedIndex]));!os.IsNotExist(err){
  t.Fatalf("obsolete segment was not pruned after commit: %v",err)
 }
 if _,err=store.SetSegmentTaskPlanStepStatus(x.ID,"local",oldRevision,
  "step-0501","pending",480,24);!errors.Is(err,ErrStaleRevision){
  t.Fatalf("stale update not rejected: %v",err)
 }
 if _,err=store.SetSegmentTaskPlanStepStatus(x.ID,"other-owner",page.Revision,
  "step-0501","pending",480,24);!errors.Is(err,ErrNotFound){
  t.Fatalf("owner isolation failed: %v",err)
 }
 // The handoff is regenerated from the new authoritative journal, never
 // read from an old materialized derivative.
 handoff,rev,err:=store.Handoff(x.ID,"local")
 if err!=nil||rev!=page.Revision||!strings.Contains(handoff,"1/1024"){
  t.Fatalf("handoff did not reflect direct single-page status change: %v",err)
 }
}

func TestSegmentStatusRejectsCorruptionAndInvalidRequestsBeforeCommit(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("An owner-managed plan",planStepFixtures(140))
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 initial:=x.Revision
 for _,caseData:=range []struct{step,status string;offset,limit int;want error}{
  {"step-0100","done",96,24,ErrTaskPlanSegmentWriteUnsupported},
  {"step-0100","",96,24,ErrTaskPlanSegmentWriteUnsupported},
  {"step-0100","pending",-1,24,ErrTaskPlanPageOutOfRange},
  {"step-0100","pending",141,24,ErrTaskPlanPageOutOfRange},
  {"step-9999","blocked",96,24,ErrTaskPlanStepNotFound},
 }{
  _,err:=store.SetSegmentTaskPlanStepStatus(x.ID,"local",initial,
   caseData.step,caseData.status,caseData.offset,caseData.limit)
  if !errors.Is(err,caseData.want){
   t.Fatalf("invalid write %+v returned %v",caseData,err)
  }
  got,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
  if got.Revision!=initial||got.TaskPlan.Steps[100].Status!="pending"{
   t.Fatal("invalid status edit modified the canonical plan")
  }
 }
 // Corrupt an OFFSCREEN page (far from the target). The update must not
 // alter any manifest or page until every referenced page verifies.
 ref:=x.TaskPlanSegments[0]
 file:=store.taskPlanSegmentPath(x.ID,ref)
 original,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
 if err=os.WriteFile(file,[]byte("[{}]"),0600);err!=nil{t.Fatal(err)}
 if _,err=store.SetSegmentTaskPlanStepStatus(x.ID,"local",initial,
  "step-0100","blocked",96,24);err==nil{
  t.Fatal("corrupted unrelated page allowed a committed edit")
 }
 if err=os.WriteFile(file,original,0600);err!=nil{t.Fatal(err)}
 got,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if got.Revision!=initial||got.TaskPlan.Steps[100].Status!="pending"{
  t.Fatal("failed write mutated the journal")
 }
}

func TestSegmentStatusCompletedAndLegacyPlansKeepOldPath(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Legacy small task plan",planStepFixtures(128))
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 if _,err=store.SetSegmentTaskPlanStepStatus(x.ID,"local",x.Revision,
  "step-0001","active",0,24);!errors.Is(err,ErrTaskPlanSegmentWriteUnsupported){
  t.Fatalf("inline plan must stay on old Save path: %v",err)
 }
 // Full plan completed after all steps receive owner-attested evidence;
 // the direct status path must not silently reopen it.
 for i:=range x.TaskPlan.Steps{
  x.TaskPlan.Steps[i].Status="done"
  x.TaskPlan.Steps[i].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Owner acceptance for milestone"}}
 }
 for i:=128;i<140;i++{
  x.TaskPlan.Steps=append(x.TaskPlan.Steps,TaskPlanStep{
   ID:fmt.Sprintf("step-%04d",i),Title:"Later milestone",
   Acceptance:"Owner verified",
   Status:"done",Evidence:[]TaskPlanEvidence{{Kind:"manual",Note:"Owner accepted later step"}},
  })
 }
 x.TaskPlan.State="completed"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if _,err=store.SetSegmentTaskPlanStepStatus(x.ID,"local",x.Revision,
  "step-0001","pending",0,24);!errors.Is(err,ErrTaskPlanNotEditable){
  t.Fatalf("direct status path bypassed completed-plan reopen gate: %v",err)
 }
}

func TestConcurrentSegmentStatusCASAdmitsOnlyOneWriter(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Serialized CAS plan",planStepFixtures(160))
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 const writers=8
 var wg sync.WaitGroup
 outcomes:=make(chan error,writers)
 for i:=0;i<writers;i++{
  wg.Add(1)
  go func(i int){
   defer wg.Done()
   _,err:=store.SetSegmentTaskPlanStepStatus(x.ID,"local",x.Revision,
    fmt.Sprintf("step-%04d",130+i),"active",128,24)
   outcomes<-err
  }(i)
 }
 wg.Wait()
 close(outcomes)
 ok,stale:=0,0
 for err:=range outcomes{
  if err==nil{ok++}else if errors.Is(err,ErrStaleRevision){stale++}else{
   t.Fatalf("unexpected concurrent CAS outcome: %v",err)
  }
 }
 if ok!=1||stale!=writers-1{
  t.Fatalf("CAS admitted %d writes, %d stale; want 1 and %d",ok,stale,writers-1)
 }
 final,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if final.Revision!=x.Revision+1{t.Fatal("concurrent writes advanced session revision incorrectly")}
}
