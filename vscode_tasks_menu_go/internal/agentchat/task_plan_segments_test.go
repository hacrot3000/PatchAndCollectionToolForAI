package agentchat

import (
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func planStepFixtures(count int)[]TaskPlanStep{
 steps:=make([]TaskPlanStep,count)
 for i:=range steps{
  steps[i]=TaskPlanStep{ID:fmt.Sprintf("step-%04d",i),
   Title:fmt.Sprintf("Task milestone %d",i),
   Acceptance:fmt.Sprintf("Evidence verified for milestone %d",i)}
 }
 return steps
}

func TestTaskPlanSegmentsMigrateInline129AndPreserveEvidence(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project-id","p","m",Stateless)
 if err!=nil{t.Fatal(err)}
 plan,err:=NewTaskPlan("Owner-authored plan",planStepFixtures(MaxInlineTaskPlanSteps))
 if err!=nil{t.Fatal(err)}
 plan.Steps[0].Status="done"
 plan.Steps[0].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Owner accepted a real result"}}
 x.TaskPlan=plan
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 original,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 if strings.Contains(string(original),"task_plan_segments")||!strings.Contains(string(original),"step-0127"){
  t.Fatal("legacy <=128-step session no longer persisted inline")
 }
 initialRevision:=x.Revision
 x.TaskPlan.Steps=append(x.TaskPlan.Steps,TaskPlanStep{
  ID:"step-0128",Title:"Milestone 129",Acceptance:"Retain all old proof",
  Status:"pending",
 })
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatalf("migrate inline to segments: %v",err)}
 if len(x.TaskPlanSegments)!=5||x.TaskPlanSegments[4].Count!=1{
  t.Fatalf("incorrect page metadata after migration: %+v",x.TaskPlanSegments)
 }
 raw,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(raw),"\"task_plan_segments\"")||
  strings.Contains(string(raw),"\"step-0128\"")||
  strings.Contains(string(raw),"\"Owner accepted a real result\""){
  t.Fatal("journal did not commit a pointer-only segment-backed Task plan")
 }
 newStore,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 got,err:=newStore.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if got.Revision!=x.Revision||len(got.TaskPlan.Steps)!=129||
  got.TaskPlan.Steps[0].Evidence[0].Note!="Owner accepted a real result"||
  got.TaskPlan.Steps[128].ID!="step-0128"{
  t.Fatal("new reader did not hydrate full legacy and migrated steps")
 }
 if _,err=newStore.Get(x.ID,"other-owner");!errors.Is(err,ErrNotFound){
  t.Fatal("segment-backed plan exposed across owners")
 }
 if _,err=newStore.Save(got,initialRevision);!errors.Is(err,ErrStaleRevision){
  t.Fatalf("stale CAS updated a segmented plan: %v",err)
 }
 oldRefs:=append([]TaskPlanSegmentRef(nil),got.TaskPlanSegments...)
 got.TaskPlan.Steps[128].Status="done"
 got.TaskPlan.Steps[128].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Accepted migrated milestone"}}
 got,err=newStore.Save(got,got.Revision);if err!=nil{t.Fatal(err)}
 for i:=0;i<4;i++{
  if got.TaskPlanSegments[i]!=oldRefs[i]{
   t.Fatalf("unchanged segment %d was needlessly rewritten",i)
  }
 }
 if got.TaskPlanSegments[4].Revision==oldRefs[4].Revision{
  t.Fatal("changed segment did not receive a fresh immutable revision")
 }
 if _,err=os.Stat(store.taskPlanSegmentPath(x.ID,oldRefs[4]));!os.IsNotExist(err){
  t.Fatalf("superseded segment was not cleaned after commit: %v",err)
 }
 restored,err:=newStore.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if len(restored.TaskPlan.Steps)!=129||TaskPlanAllDone(restored.TaskPlan){
  t.Fatal("reopened plan lost milestones or became spuriously completed")
 }
 if restored.TaskPlan.Steps[128].Evidence[0].Note!="Accepted migrated milestone"{
  t.Fatal("reopened evidence differs from committed segment")
 }
 refs:=append([]TaskPlanSegmentRef(nil),restored.TaskPlanSegments...)
 if err=newStore.Delete(restored.ID,"local",restored.Revision);err!=nil{t.Fatal(err)}
 for _,ref:=range refs{
  if _,err:=os.Stat(newStore.taskPlanSegmentPath(x.ID,ref));!os.IsNotExist(err){
   t.Fatalf("deleted journal leaked referenced plan page: %v",err)
  }
 }
}

func TestLargeTaskPlan1024StepsCanExceedOldJournalSizeSafely(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project-id","p","m",Stateless)
 if err!=nil{t.Fatal(err)}
 plan,err:=NewTaskPlan("Large bounded plan",planStepFixtures(MaxTaskPlanSteps))
 if err!=nil{t.Fatal(err)}
 // Hundreds of accepted receipts should fit in private segments but not
 // in a single 4-MiB session JSON.
 for i:=0;i<800;i++{
  plan.Steps[i].Status="done"
  for j:=0;j<MaxTaskPlanEvidence;j++{
   plan.Steps[i].Evidence=append(plan.Steps[i].Evidence,TaskPlanEvidence{
    Kind:"manual",Note:fmt.Sprintf("Owner verified %d/%d: %s",i,j,strings.Repeat("A",480)),
   })
  }
 }
 x.TaskPlan=plan
 full,err:=json.Marshal(x);if err!=nil{t.Fatal(err)}
 if len(full)<=maxSessionBytes{
  t.Fatalf("fixture should exceed old monolithic limit but uses %d bytes",len(full))
 }
 x,err=store.Save(x,0);if err!=nil{t.Fatalf("1024-step save failed: %v",err)}
 disk,err:=os.Stat(store.file(x.ID));if err!=nil{t.Fatal(err)}
 if disk.Size()>maxSessionBytes||len(x.TaskPlanSegments)!=32{
  t.Fatalf("oversized journal or wrong segment count: %d; %d",disk.Size(),len(x.TaskPlanSegments))
 }
 reopened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 got,err:=reopened.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if len(got.TaskPlan.Steps)!=MaxTaskPlanSteps||
  len(got.TaskPlan.Steps[799].Evidence)!=MaxTaskPlanEvidence{
  t.Fatal("segment hydration discarded accepted plan evidence")
 }
 done,total:=TaskPlanProgress(got.TaskPlan)
 if done!=800||total!=1024||TaskPlanAllDone(got.TaskPlan){
  t.Fatalf("incorrect full-plan progress after segment reads: %d/%d",done,total)
 }
 mark,rev,err:=reopened.Handoff(x.ID,"local")
 if err!=nil||rev!=got.Revision||!strings.Contains(mark,"800/1024"){
  t.Fatalf("bounded handoff lost canonical progress: %v",err)
 }
}

func TestSegmentedPlanFailsClosedOnMissingOrTamperedPart(t *testing.T){
 for _,mode:=range []string{"missing","tampered","permissions","link"}{
  t.Run(mode,func(t *testing.T){
   dir:=filepath.Join(t.TempDir(),"private","agent")
   store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
   x,err:=NewSession("local","/workspace","project-id","p","m",Stateless)
   if err!=nil{t.Fatal(err)}
   x.TaskPlan,err=NewTaskPlan("Verify content hash",planStepFixtures(140))
   if err!=nil{t.Fatal(err)}
   x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
   file:=store.taskPlanSegmentPath(x.ID,x.TaskPlanSegments[1])
   switch mode{
   case "missing":
    if err=os.Remove(file);err!=nil{t.Fatal(err)}
   case "tampered":
    if err=os.WriteFile(file,[]byte("[{\"id\":\"forged\"}]"),0600);err!=nil{t.Fatal(err)}
   case "permissions":
    if err=os.Chmod(file,0644);err!=nil{t.Fatal(err)}
   case "link":
    if err=os.Remove(file);err!=nil{t.Fatal(err)}
    if err=os.Symlink(store.file(x.ID),file);err!=nil{t.Fatal(err)}
   }
   if _,err=store.Get(x.ID,"local");err==nil{
    t.Fatalf("plan %s was trusted despite bad native page",mode)
   }
  })
 }
}
