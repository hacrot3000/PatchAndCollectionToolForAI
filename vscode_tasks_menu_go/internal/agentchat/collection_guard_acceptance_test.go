package agentchat

import(
 "path/filepath"
 "strings"
 "testing"
 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
)

func TestCollectionGuardAcceptedStepProgressIsScopedAndIdempotent(t *testing.T){
 for _,manual:=range []bool{false,true}{
  t.Run(map[bool]string{false:"verified",true:"attested"}[manual],func(t *testing.T){
   s,err:=NewStore(filepath.Join(t.TempDir(),"private"));if err!=nil{t.Fatal(err)}
   x,err:=NewSession("local","/workspace","project","provider","model",Stateless);if err!=nil{t.Fatal(err)}
   x.TaskPlan,err=NewTaskPlan("Fix bug",[]TaskPlanStep{{ID:"a",Title:"Fix source",Acceptance:"Check evidence"},{ID:"b",Title:"Review other",Acceptance:"Review"}});if err!=nil{t.Fatal(err)}
   step:=x.TaskPlan.Steps[0]
   if !x.BindCollectionGuard(x.TaskPlan.Goal+"\n"+step.Title+"\n"+step.Acceptance,step.ID,true){t.Fatal("bind")}
   g,_:=x.ReadCollectionGuard()
   x.AppendEvent("clp_collection_observed",x.Status,"read","")
   g.Counters,_,_,err=cp.ApplyCounterEvent(g.Policy,g.Counters,cp.CounterEvent{Sequence:x.EventSeq,Identity:guardDigest("read"),Kind:cp.CollectionObserved,Novelty:cp.VerifiedNew,NativeRead:true},true)
   if err!=nil||!x.SetCollectionGuard(g){t.Fatal("seed")}
   x,err=s.Save(x,0);if err!=nil{t.Fatal(err)}
   evidence:=TaskPlanEvidence{Kind:"collection",Ref:"verified-read",Digest:strings.Repeat("a",64)}
   if manual{evidence=TaskPlanEvidence{Kind:"manual",Note:"Owner checked milestone"}}
   if err=UpdateTaskPlanStep(&x,"b","done",&evidence);err!=nil{t.Fatal(err)}
   other,_:=x.ReadCollectionGuard();if other.Counters.Progress.ReadsSinceProgress!=1{t.Fatal("unrelated step reset progress")}
   if err=UpdateTaskPlanStep(&x,"a","done",&evidence);err!=nil{t.Fatal(err)}
   x,err=s.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
   after,ok:=x.ReadCollectionGuard()
   if !ok||!after.AcceptedStep||after.Counters.Progress.ReadsSinceProgress!=0||after.Counters.VerifiedProgressEvents+after.Counters.AttestedProgressEvents!=1||
    (manual&&after.Counters.AttestedProgressEvents!=1)||(!manual&&after.Counters.VerifiedProgressEvents!=1){t.Fatalf("invalid acceptance projection: %+v",after)}
   if err=UpdateTaskPlanStep(&x,"a","pending",nil);err!=nil{t.Fatal(err)}
   if err=UpdateTaskPlanStep(&x,"a","done",&evidence);err!=nil{t.Fatal(err)}
   again,_:=x.ReadCollectionGuard();if again.Counters.VerifiedProgressEvents+again.Counters.AttestedProgressEvents!=1{t.Fatal("same generation acceptance double counted")}
  })
 }
}
