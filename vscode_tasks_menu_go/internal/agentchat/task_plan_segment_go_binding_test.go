package agentchat

import (
 "bytes"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"
 "fmt"
)

func newSegmentGoBindingFixture(t *testing.T)(*Store,Session){
 t.Helper()
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Owner selects native Go tests before accepting work",planStepFixtures(1024))
 if err!=nil{t.Fatal(err)}
 x.TaskPlan.Steps[0].Status="done"
 x.TaskPlan.Steps[0].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Earlier owner-approved milestone"}}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 return store,x
}

func TestSegmentGoBindingCASPreservesPriorProofsAndSinglePage(t *testing.T){
 store,x:=newSegmentGoBindingFixture(t)
 var all=append([]TaskPlanSegmentRef(nil),x.TaskPlanSegments...)
 taskID:=42
 sha:=strings.Repeat("a",64)
 lastNow:=time.Time{}
 verify:=func(page TaskPlanPageResult){
  t.Helper()
  if page.Revision!=x.Revision+1||page.Done!=1||page.Total!=1024||
   page.Completed||page.AllDone||page.Plan==nil||
   len(page.Plan.Steps)!=24||page.Plan.Steps[21].ID!="step-0501"{
   t.Fatalf("incorrect paged CAS projection: %+v",page)
  }
  y,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
  if y.Revision!=page.Revision||
   y.TaskPlan.Steps[0].Evidence[0].Note!="Earlier owner-approved milestone"||
   y.TaskPlan.Steps[501].Status!="pending"||
   len(y.TaskPlan.Steps[501].Evidence)!=0{
   t.Fatal("binding write lost existing owner proof or marked target as done")
  }
  for i,ref:=range y.TaskPlanSegments{
   if i==501/taskPlanSegmentSize{
    if ref==all[i]||ref.Revision!=page.Revision{
     t.Fatal("target immutable page was not rewritten")
    }
   }else if ref!=all[i]{
    t.Fatalf("unrelated segment %d changed while editing Go bindings",i)
   }
  }
  if !lastNow.IsZero()&&!y.TaskPlan.Steps[501].UpdatedAt.After(lastNow){
   // UpdatedAt must never move backward; equality can occur on very fast
   // machines only at clock resolution. Require >=, not strictly >.
   if y.TaskPlan.Steps[501].UpdatedAt.Before(lastNow){
    t.Fatal("milestone UpdatedAt moved backward")
   }
  }
  lastNow=y.TaskPlan.Steps[501].UpdatedAt
  all=append([]TaskPlanSegmentRef(nil),y.TaskPlanSegments...)
  x=y
 }

 page,err:=store.SetSegmentTaskPlanCommandGuard(x.ID,"local",x.Revision,
  "step-0501",true,taskID,sha,480,24)
 if err!=nil{t.Fatal(err)}
 verify(page)
 guard:=x.TaskPlan.Steps[501].TaskCommandGuard
 if guard==nil||guard.TaskID!=taskID||guard.CommandSHA256!=sha||guard.BoundAt.IsZero(){
  t.Fatal("owner command fingerprint was not persisted")
 }

 page,err=store.SetSegmentTaskPlanGoTestRequirements(x.ID,"local",x.Revision,
  "step-0501",true,"example.com/verified",[]string{"TestAlpha","TestBeta/sub"},480,24)
 if err!=nil{t.Fatal(err)}
 verify(page)
 req:=x.TaskPlan.Steps[501].GoTestRequirements
 if req==nil||req.Package!="example.com/verified"||len(req.Tests)!=2||
  req.BoundAt.Before(guard.BoundAt){
  t.Fatalf("owner-bound required tests lost command lock: %+v",req)
 }

 // Go-suite binding is a hard gate: an explicit user attestation or an
 // archived Patch cannot silently bypass the expected exact named suite.
 if _,err=store.SetSegmentTaskPlanStepDone(x.ID,"local",x.Revision,
  "step-0501","manual","","I reviewed work",480,24);!errors.Is(err,ErrTaskPlanSegmentWriteUnsupported){
  t.Fatalf("manual completion bypassed Go checklist: %v",err)
 }
 if _,err=store.SetSegmentTaskPlanCommandGuard(x.ID,"local",x.Revision,
  "step-0501",false,0,"",480,24);!errors.Is(err,ErrTaskPlanGoBindingConflict){
  t.Fatalf("command lock changed with required tests still bound: %v",err)
 }
 page,err=store.SetSegmentTaskPlanAcceptanceCriteria(x.ID,"local",x.Revision,
  "step-0501",[]TaskAcceptanceCriterion{{
   ID:"behavior",Description:"Tests cover owner's expected behavior",RequiredTests:[]string{"TestAlpha"},
  }},480,24)
 if err!=nil{t.Fatal(err)}
 verify(page)
 if _,err=store.SetSegmentTaskPlanGoTestRequirements(x.ID,"local",x.Revision,
  "step-0501",false,"",nil,480,24);!errors.Is(err,ErrTaskPlanGoBindingConflict){
  t.Fatalf("removed Go requirements while owner matrix still depended on it: %v",err)
 }

 // Explicit removal order is matrix -> Go tests -> guard. Future owner
 // changes must never silently retain a reviewed matrix for different tests.
 page,err=store.SetSegmentTaskPlanAcceptanceCriteria(x.ID,"local",x.Revision,
  "step-0501",nil,480,24)
 if err!=nil{t.Fatal(err)};verify(page)
 page,err=store.SetSegmentTaskPlanGoTestRequirements(x.ID,"local",x.Revision,
  "step-0501",false,"",nil,480,24)
 if err!=nil{t.Fatal(err)};verify(page)
 if x.TaskPlan.Steps[501].GoTestRequirements!=nil{t.Fatal("Go tests not explicitly cleared")}
 page,err=store.SetSegmentTaskPlanCommandGuard(x.ID,"local",x.Revision,
  "step-0501",false,0,"",480,24)
 if err!=nil{t.Fatal(err)};verify(page)
 if x.TaskPlan.Steps[501].TaskCommandGuard!=nil{t.Fatal("native command lock not explicitly cleared")}
 handoff,rev,err:=store.Handoff(x.ID,"local")
 if err!=nil||rev!=x.Revision||!strings.Contains(handoff,"1/1024"){
  t.Fatalf("handoff did not reflect revised canonical bindings: %v",err)
 }
}

func TestSegmentGoBindingFailClosedBeforeJournalMutation(t *testing.T){
 store,x:=newSegmentGoBindingFixture(t)
 original,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 reject:=func(want error,edit func()error){
  t.Helper()
  e:=edit()
  if want==nil{if e==nil{t.Fatal("damaged page was admitted")}}else if !errors.Is(e,want){
   t.Fatalf("want %v, got %v",want,e)
  }
  after,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
  if !bytes.Equal(after,original){t.Fatal("rejected owner edit changed canonical journal")}
 }
 reject(ErrTaskPlanGoBindingInvalid,func()error{
  _,e:=store.SetSegmentTaskPlanCommandGuard(x.ID,"local",x.Revision,
   "step-0501",true,1,strings.Repeat("z",64),480,24);return e
 })
 reject(ErrTaskPlanGoBindingInvalid,func()error{
  _,e:=store.SetSegmentTaskPlanCommandGuard(x.ID,"local",x.Revision,
   "step-0501",true,0,strings.Repeat("a",64),480,24);return e
 })
 reject(ErrTaskPlanGoBindingMissing,func()error{
  _,e:=store.SetSegmentTaskPlanGoTestRequirements(x.ID,"local",x.Revision,
   "step-0501",true,"example/pkg",[]string{"TestOne"},480,24);return e
 })
 reject(ErrNotFound,func()error{
  _,e:=store.SetSegmentTaskPlanCommandGuard(x.ID,"different-owner",x.Revision,
   "step-0501",true,1,strings.Repeat("a",64),480,24);return e
 })
 reject(ErrStaleRevision,func()error{
  _,e:=store.SetSegmentTaskPlanCommandGuard(x.ID,"local",x.Revision+1,
   "step-0501",true,1,strings.Repeat("a",64),480,24);return e
 })
 reject(ErrTaskPlanAcceptanceLocked,func()error{
  _,e:=store.SetSegmentTaskPlanCommandGuard(x.ID,"local",x.Revision,
   "step-0000",true,1,strings.Repeat("a",64),0,24);return e
 })
 reject(ErrTaskPlanPageOutOfRange,func()error{
  _,e:=store.SetSegmentTaskPlanCommandGuard(x.ID,"local",x.Revision,
   "step-0501",false,0,"",1025,24);return e
 })
 // Full scan verifies integrity of an unrelated page before ANY write.
 file:=store.taskPlanSegmentPath(x.ID,x.TaskPlanSegments[0])
 raw,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
 if err=os.WriteFile(file,[]byte("[{}]"),0600);err!=nil{t.Fatal(err)}
 reject(nil,func()error{
  _,e:=store.SetSegmentTaskPlanCommandGuard(x.ID,"local",x.Revision,
   "step-0501",true,1,strings.Repeat("a",64),480,24);return e
 })
 if err=os.WriteFile(file,raw,0600);err!=nil{t.Fatal(err)}

 page,err:=store.SetSegmentTaskPlanCommandGuard(x.ID,"local",x.Revision,
  "step-0501",true,1,strings.Repeat("a",64),480,24)
 if err!=nil{t.Fatal(err)}
 x,err=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if page.Revision!=x.Revision{t.Fatal("unexpected revision after valid guard write")}
 original,err=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 reject(ErrTaskPlanGoBindingInvalid,func()error{
  _,e:=store.SetSegmentTaskPlanGoTestRequirements(x.ID,"local",x.Revision,
   "step-0501",true,"example/pkg",[]string{"TestOne","TestOne"},480,24);return e
 })
 reject(ErrTaskPlanGoBindingInvalid,func()error{
  _,e:=store.SetSegmentTaskPlanGoTestRequirements(x.ID,"local",x.Revision,
   "step-0501",true,"../badpkg",[]string{"TestOne"},480,24);return e
 })
}

func TestConcurrentSegmentGoBindingCASAllowsExactlyOneWriter(t *testing.T){
 store,x:=newSegmentGoBindingFixture(t)
 var wg sync.WaitGroup
 const count=8
 results:=make(chan error,count)
 for i:=0;i<count;i++{
  wg.Add(1)
  go func(i int){
   defer wg.Done()
   _,e:=store.SetSegmentTaskPlanCommandGuard(x.ID,"local",x.Revision,
    "step-0501",true,i+1,strings.Repeat("a",64),480,24)
   results<-e
  }(i)
 }
 wg.Wait();close(results)
 ok,stale:=0,0
 for e:=range results {
  if e==nil{ok++}else if errors.Is(e,ErrStaleRevision){stale++}else{
   t.Fatalf("unexpected concurrent result: %v",e)
  }
 }
 if ok!=1||stale!=count-1{t.Fatalf("concurrent writers ok=%d stale=%d",ok,stale)}
 final,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if final.Revision!=x.Revision+1||final.TaskPlan.Steps[501].TaskCommandGuard==nil{
  t.Fatal("concurrent native test command binding was not serialized")
 }
 _=fmt.Sprintf("%s",final.TaskPlan.Steps[501].ID)
}
