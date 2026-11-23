package agentchat

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func testRecoveryPlan(t *testing.T,store *Store,owner,workspace,project string)(Session,[]TaskPlanSegmentRef){
 t.Helper()
 x,err:=NewSession(owner,workspace,project,"provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Preserve evidence while recovering orphans",planStepFixtures(130))
 if err!=nil{t.Fatal(err)}
 x.TaskPlan.Steps[0].Status="done"
 x.TaskPlan.Steps[0].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Keep this owner-approved evidence"}}
 x,err=store.Save(x,0)
 if err!=nil{t.Fatal(err)}
 return x,append([]TaskPlanSegmentRef(nil),x.TaskPlanSegments...)
}

func TestCrashOrphanRecoveryRemovesOnlyUnreferencedPagesAcrossProjects(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 a,oldA:=testRecoveryPlan(t,store,"owner-A","/workspace-A","project-A")
 b,oldB:=testRecoveryPlan(t,store,"owner-B","/workspace-B","project-B")
 // Simulate a crash after staging a new immutable segment but before the
 // canonical journal rename: only a changed page gets revision+1.
 abandoned:=a
 abandoned.TaskPlan.Steps[129].Status="done"
 abandoned.TaskPlan.Steps[129].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"Uncommitted update"}}
 err=store.withLock(func()error{
  return store.stageTaskPlanSegments(&abandoned,&a,a.Revision+1)
 })
 if err!=nil{t.Fatal(err)}
 orphan:=store.taskPlanSegmentPath(a.ID,abandoned.TaskPlanSegments[4])
 if orphan==store.taskPlanSegmentPath(a.ID,oldA[4]){t.Fatal("staged page reused old bytes unexpectedly")}
 if _,err=os.Stat(orphan);err!=nil{t.Fatal(err)}
 // Simulate a different session that was deleted after a crash left only
 // its page. Never confuse it with another owner's active journal.
 missingID:=strings.Repeat("c",32)
 deletedOrphan:=store.taskPlanSegmentPath(missingID,TaskPlanSegmentRef{Revision:1,Index:1})
 if err=os.WriteFile(deletedOrphan,[]byte("[]"),0600);err!=nil{t.Fatal(err)}
 // Unknown/suspicious names must be left alone.
 strange:=filepath.Join(dir,missingID+".plan.01.1.json")
 if err=os.WriteFile(strange,[]byte("unknown future artifact"),0600);err!=nil{t.Fatal(err)}
 // A scoped store cannot use its project filter to delete owner-B pages.
 scoped,err:=NewProjectStore(dir,a.Workspace,a.ProjectID)
 if err!=nil{t.Fatal(err)}
 removed,err:=scoped.RecoverTaskPlanOrphans()
 if err!=nil{t.Fatal(err)}
 if removed!=2{t.Fatalf("recovery removed %d pages; want exactly 2 crash orphans",removed)}
 for _,p:=range []string{orphan,deletedOrphan}{
  if _,err=os.Stat(p);!os.IsNotExist(err){t.Fatalf("crash orphan survived cleanup: %s, %v",p,err)}
 }
 if _,err=os.Stat(strange);err!=nil{t.Fatalf("unknown artifact was deleted: %v",err)}
 for _,item:=range []struct{x Session;refs []TaskPlanSegmentRef}{{a,oldA},{b,oldB}}{
  for _,ref:=range item.refs{
   if _,err=os.Stat(store.taskPlanSegmentPath(item.x.ID,ref));err!=nil{
    t.Fatalf("live page of %s removed: %v",item.x.Owner,err)
   }
  }
  actual,err:=store.Get(item.x.ID,item.x.Owner)
  if err!=nil||actual.TaskPlan.Steps[0].Evidence[0].Note!="Keep this owner-approved evidence"{
   t.Fatalf("canonical session changed by recovery: %v",err)
  }
 }
 again,err:=scoped.RecoverTaskPlanOrphans()
 if err!=nil||again!=0{t.Fatalf("recovery not idempotent: count=%d err=%v",again,err)}
}

func TestCrashOrphanRecoveryFailsClosedBeforeRemovingAnyFile(t *testing.T){
 for _,kind:=range []string{"corrupt-journal","missing-live-part","tampered-live-part","unsafe-journal"}{
  t.Run(kind,func(t *testing.T){
   dir:=filepath.Join(t.TempDir(),"private","agent")
   store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
   x,refs:=testRecoveryPlan(t,store,"local","/workspace","project")
   orphan:=store.taskPlanSegmentPath(x.ID,TaskPlanSegmentRef{Revision:x.Revision+1,Index:5})
   if err=os.WriteFile(orphan,[]byte("orphan"),0600);err!=nil{t.Fatal(err)}
   switch kind{
   case "corrupt-journal":
    invalidID:=strings.Repeat("d",32)
    if err=os.WriteFile(store.file(invalidID),[]byte("{invalid"),0600);err!=nil{t.Fatal(err)}
   case "missing-live-part":
    if err=os.Remove(store.taskPlanSegmentPath(x.ID,refs[0]));err!=nil{t.Fatal(err)}
   case "tampered-live-part":
    if err=os.WriteFile(store.taskPlanSegmentPath(x.ID,refs[0]),[]byte("[]"),0600);err!=nil{t.Fatal(err)}
   case "unsafe-journal":
    if err=os.Chmod(store.file(x.ID),0644);err!=nil{t.Fatal(err)}
   }
   count,err:=store.RecoverTaskPlanOrphans()
   if err==nil||count!=0{t.Fatalf("%s recovery ignored damaged authoritative journal: count=%d error=%v",kind,count,err)}
   if _,err=os.Stat(orphan);err!=nil{t.Fatal("fail-closed scan deleted unrelated orphan before verifying all journals")}
  })
 }
}

func TestCrashOrphanRecoverySkipsLinksAndNeverDeletesNamedFutureArtifacts(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 if err=store.ensureDir();err!=nil{t.Fatal(err)}
 id:=strings.Repeat("e",32)
 link:=store.taskPlanSegmentPath(id,TaskPlanSegmentRef{Revision:1,Index:1})
 if err=os.Symlink(filepath.Join(dir,"does-not-exist"),link);err!=nil{t.Skipf("symlinks unsupported: %v",err)}
 public:=store.taskPlanSegmentPath(id,TaskPlanSegmentRef{Revision:1,Index:2})
 if err=os.WriteFile(public,[]byte("{}"),0644);err!=nil{t.Fatal(err)}
 if err=os.Chmod(public,0644);err!=nil{t.Fatal(err)}
 unknown:=filepath.Join(dir,fmt.Sprintf("%s.plan.%d.%d.json",id,0,3))
 if err=os.WriteFile(unknown,[]byte("future format"),0600);err!=nil{t.Fatal(err)}
 count,err:=store.RecoverTaskPlanOrphans()
 if err!=nil||count!=0{t.Fatalf("unsafe/unrecognized page removed: count=%d err=%v",count,err)}
 for _,file:=range []string{link,public,unknown}{
  if _,err=os.Lstat(file);err!=nil{t.Fatalf("non-regular/unknown page should be quarantined, not removed: %s %v",file,err)}
 }
}

func TestRecoveryAfterRestartDoesNotCacheAnEarlierFailedAttempt(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","agent")
 old,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,_:=testRecoveryPlan(t,old,"local","/workspace","project")
 orphan:=old.taskPlanSegmentPath(x.ID,TaskPlanSegmentRef{Revision:x.Revision+1,Index:5})
 if err=os.WriteFile(orphan,[]byte("crash orphan"),0600);err!=nil{t.Fatal(err)}
 broken:=old.taskPlanSegmentPath(x.ID,x.TaskPlanSegments[0])
 contents,err:=os.ReadFile(broken);if err!=nil{t.Fatal(err)}
 if err=os.WriteFile(broken,[]byte("corrupted"),0600);err!=nil{t.Fatal(err)}
 restarted,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 if err=restarted.EnsureTaskPlanOrphanRecovery();err==nil{t.Fatal("damaged page silently allowed startup recovery")}
 if _,err=os.Stat(orphan);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(broken,contents,0600);err!=nil{t.Fatal(err)}
 if err=restarted.EnsureTaskPlanOrphanRecovery();err!=nil{t.Fatal(err)}
 if _,err=os.Stat(orphan);!os.IsNotExist(err){t.Fatal("recovery did not retry after repairing the journal")}
 if _,err=restarted.Get(x.ID,"local");err!=nil{t.Fatal(err)}
 if err=restarted.EnsureTaskPlanOrphanRecovery();err!=nil{t.Fatal(err)}
 // Another explicit scan remains available when an external process dies
 // later during the current daemon lifetime.
 next:=restarted.taskPlanSegmentPath(x.ID,TaskPlanSegmentRef{Revision:x.Revision+2,Index:5})
 if err=os.WriteFile(next,[]byte("another interrupted writer"),0600);err!=nil{t.Fatal(err)}
 if _,err=restarted.RecoverTaskPlanOrphans();err!=nil{t.Fatal(err)}
 if _,err=os.Stat(next);!errors.Is(err,os.ErrNotExist){t.Fatal("manual audit could not recover a later orphan")}
}
