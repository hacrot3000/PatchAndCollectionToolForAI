package agentchat

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestTaskPlanPageStillVerifiesOffscreenSegmentsAndLegacyInline(t *testing.T){
 for _,size:=range []int{128,320}{
  t.Run(fmt.Sprintf("steps-%d",size),func(t *testing.T){
   store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
   if err!=nil{t.Fatal(err)}
   x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
   if err!=nil{t.Fatal(err)}
   steps:=planStepFixtures(size)
   x.TaskPlan,err=NewTaskPlan("Exact owner-bound milestone plan",steps)
   if err!=nil{t.Fatal(err)}
   x.TaskPlan.Steps[0].Status="done"
   x.TaskPlan.Steps[0].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Owner-attested acceptance"}}
   x,err=store.Save(x,0)
   if err!=nil{t.Fatal(err)}
   paged,err:=store.TaskPlanPage(x.ID,"local",0,24)
   if err!=nil{t.Fatal(err)}
   if paged.Done!=1||paged.Total!=size||paged.AllDone||
    paged.Revision!=x.Revision||paged.Plan==nil||
    len(paged.Plan.Steps)!=24||!paged.Page.HasNext{
    t.Fatalf("bad verified partial plan response: %+v",paged)
   }
   if paged.Plan.Steps[0].Evidence[0].Note!="Owner-attested acceptance"{
    t.Fatal("selected page omitted accepted proof")
   }
   if _,err=store.TaskPlanPage(x.ID,"other-owner",0,24);!errors.Is(err,ErrNotFound){
    t.Fatalf("cross-owner paged GET leaked data: %v",err)
   }
   if _,err=store.TaskPlanPage(x.ID,"local",size+1,24);!errors.Is(err,ErrTaskPlanPageOutOfRange){
    t.Fatalf("out-of-range page not rejected: %v",err)
   }
   if size<=MaxInlineTaskPlanSteps{return}
   // Deliberately tamper an OFFSCREEN page. The bounded reader checks all
   // immutable segment hashes before it reports any whole-plan progress.
   last:=x.TaskPlanSegments[len(x.TaskPlanSegments)-1]
   path:=store.taskPlanSegmentPath(x.ID,last)
   original,err:=os.ReadFile(path)
   if err!=nil{t.Fatal(err)}
   if err=os.WriteFile(path,[]byte(`[{"id":"forged"}]`),0600);err!=nil{t.Fatal(err)}
   if _,err=store.TaskPlanPage(x.ID,"local",0,24);err==nil{
    t.Fatal("offscreen corrupted segment was ignored by paginated reader")
   }
   if err=os.WriteFile(path,original,0600);err!=nil{t.Fatal(err)}
   after,err:=store.TaskPlanPage(x.ID,"local",0,24)
   if err!=nil||after.Done!=1{
    t.Fatalf("repaired immutable page was rejected: %v",err)
   }
   // Even a complete first page is NOT authoritative for all 320 steps.
   if after.AllDone||after.Total!=320{t.Fatal("page-only acceptance leaked into global all_done")}
  })
 }
}

func TestTaskPlanPageMissingOffscreenPartIsFailClosed(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("All pages must be present",planStepFixtures(130))
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0)
 if err!=nil{t.Fatal(err)}
 ref:=x.TaskPlanSegments[len(x.TaskPlanSegments)-1]
 if err=os.Remove(store.taskPlanSegmentPath(x.ID,ref));err!=nil{t.Fatal(err)}
 if _,err=store.TaskPlanPage(x.ID,"local",0,24);err==nil||
  !strings.Contains(err.Error(),"missing Task plan segment"){
  t.Fatalf("offscreen missing segment was trusted: %v",err)
 }
}

func TestTaskPlanPageNoPlanAndInvalidLimit(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 p,err:=store.TaskPlanPage(x.ID,"local",0,24)
 if err!=nil||p.Plan!=nil||p.Total!=0||p.AllDone||p.Page.TotalSteps!=0{
  t.Fatalf("empty plan result is not compatible: %+v %v",p,err)
 }
 for _,query:=range []struct{offset,limit int}{
  {-1,24},{0,0},{0,33},{1,24},
 }{
  if _,err=store.TaskPlanPage(x.ID,"local",query.offset,query.limit);
   !errors.Is(err,ErrTaskPlanPageOutOfRange){
   t.Fatalf("invalid pagination accepted: %+v %v",query,err)
  }
 }
}
