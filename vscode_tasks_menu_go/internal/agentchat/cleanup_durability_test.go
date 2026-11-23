package agentchat

import (
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

// A cleanup unlink may already be visible even when its enclosing
// directory fsync fails. The next startup scan must be allowed to retry,
// but the canonical journal and owner evidence remain authoritative.
func TestOrphanUnlinkFsyncFailureIsExplicitAndRestartRecoveryRetries(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,refs:=testRecoveryPlan(t,store,"owner-A","/workspace-A","project-A")
 other,otherRefs:=testRecoveryPlan(t,store,"owner-B","/workspace-B","project-B")
 orphan:=store.taskPlanSegmentPath(x.ID,TaskPlanSegmentRef{Revision:x.Revision+1,Index:1})
 if err=os.WriteFile(orphan,[]byte("uncommitted page"),0600);err!=nil{t.Fatal(err)}
 rawBefore,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 syncs:=0
 store.syncDirectoryOverride=func()error{
  syncs++
  return errors.New("injected orphan unlink fsync failure")
 }
 err=store.EnsureTaskPlanOrphanRecovery()
 if !errors.Is(err,ErrCleanupDurabilityUnknown)||syncs!=1{
  t.Fatalf("orphan unlink failure not classified as uncertain: err=%v syncs=%d",err,syncs)
 }
 if _,err=os.Stat(orphan);!os.IsNotExist(err){
  t.Fatalf("unlink before failed fsync did not occur: %v",err)
 }
 current,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 if string(rawBefore)!=string(current){
  t.Fatal("orphan recovery must never change canonical owner journal")
 }
 for _,ref:=range append(refs,otherRefs...){
  id:=x.ID
  if ref==otherRefs[0]{
   // Reference lists may be equal across sessions; use separate loops below.
   break
  }
  if _,err=os.Stat(store.taskPlanSegmentPath(id,ref));err!=nil{t.Fatalf("referenced page lost: %v",err)}
 }
 // A simulated power interruption can resurrect a previously un-fsynced
 // directory entry. The incomplete startup scan must NOT be cached.
 if err=os.WriteFile(orphan,[]byte("resurrected stale page"),0600);err!=nil{t.Fatal(err)}
 store.syncDirectoryOverride=func()error{
  syncs++
  return syncAgentDirectory(store.dir)
 }
 if err=store.EnsureTaskPlanOrphanRecovery();err!=nil{t.Fatal(err)}
 if syncs!=2{t.Fatalf("startup cleanup failure incorrectly cached: syncs=%d",syncs)}
 if _,err=os.Stat(orphan);!os.IsNotExist(err){t.Fatalf("retry did not remove stale page: %v",err)}
 // Successful once-per-startup scan is now cached; explicit scan remains
 // available if another process dies before daemon shutdown.
 if err=store.EnsureTaskPlanOrphanRecovery();err!=nil||syncs!=2{
  t.Fatalf("successful scan not cached: err=%v syncs=%d",err,syncs)
 }
 for _,tc:=range []struct{id string;refs []TaskPlanSegmentRef;owner string}{
  {x.ID,refs,"owner-A"},{other.ID,otherRefs,"owner-B"},
 }{
  for _,ref:=range tc.refs{
   if _,err=os.Stat(store.taskPlanSegmentPath(tc.id,ref));err!=nil{
    t.Fatalf("another owner's referenced page deleted: %v",err)
   }
  }
  loaded,e:=store.Get(tc.id,tc.owner)
  if e!=nil||loaded.TaskPlan.Steps[0].Evidence[0].Note!="Keep this owner-approved evidence"{
   t.Fatalf("orphan cleanup changed verified evidence: %v",e)
  }
 }
}

func TestOrphanCleanupWithoutChangesDoesNotRequireDirectorySync(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"));if err!=nil{t.Fatal(err)}
 x,_:=testRecoveryPlan(t,store,"local","/workspace","project")
 store.syncDirectoryOverride=func()error{
  t.Fatal("no unlinks: no need to fsync directory")
  return errors.New("unreachable")
 }
 removed,err:=store.RecoverTaskPlanOrphans()
 if err!=nil||removed!=0{t.Fatalf("clean scan failed: removed=%d err=%v",removed,err)}
 if _,err=store.Get(x.ID,"local");err!=nil{t.Fatal(err)}
}

func TestCommittedSegmentStatusSurvivesBestEffortCleanupSyncFailure(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"));if err!=nil{t.Fatal(err)}
 x,refs:=testRecoveryPlan(t,store,"local","/workspace","project")
 oldPage:=store.taskPlanSegmentPath(x.ID,refs[0])
 syncs:=0
 store.syncDirectoryOverride=func()error{
  syncs++
  if syncs==3{return errors.New("injected postcommit obsolete page unlink fsync failure")}
  return syncAgentDirectory(store.dir)
 }
 page,err:=store.SetSegmentTaskPlanStepStatus(x.ID,"local",x.Revision,
  "step-0000","blocked",0,24)
 if err!=nil{
  t.Fatalf("postcommit cleanup fsync must not make a committed status retryable: %v",err)
 }
 if syncs!=3{t.Fatalf("expected pre-page, journal, post-unlink syncs: got %d",syncs)}
 if page.Revision!=x.Revision+1{t.Fatalf("status revision was not committed: %d",page.Revision)}
 if _,err=os.Stat(oldPage);!os.IsNotExist(err){
  t.Fatalf("old page not unlinked: %v",err)
 }
 store.syncDirectoryOverride=nil
 reopened,err:=NewStore(store.dir);if err!=nil{t.Fatal(err)}
 actual,err:=reopened.Get(x.ID,"local")
 if err!=nil||actual.Revision!=x.Revision+1||
  actual.TaskPlan.Steps[0].Status!="blocked"||
  actual.TaskPlan.Steps[0].Evidence[0].Note!="Keep this owner-approved evidence"{
  t.Fatalf("committed Task Plan/proof lost due to garbage-collection error: %v",err)
 }
 if _,err=reopened.RecoverTaskPlanOrphans();err!=nil{t.Fatal(err)}
}

func TestObsoletePreparedPayloadUnlinkFsyncIsIndependentFromProviderACK(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 payload:=[]byte(`{"model":"m","input":"private original prompt"}`)
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.RetryMode=RetryUnlimited
 x.ProviderRun=&ProviderRun{
  ID:"r2-e1",Kind:"message",Phase:"prepared",
  PayloadSHA256:ProviderPayloadDigest(payload),
  CreatedAt:now,UpdatedAt:now,
 }
 if err=store.StageProviderPayload(x,payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 x.ProviderRun.Phase="sending"
 x.ProviderRun.UpdatedAt=time.Now().UTC()
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if _,_,err=store.PreparedProviderPayload(x.ID,"local");!errors.Is(err,ErrInvalidSession){
  t.Fatalf("stale PREPARED file made SENDING replayable: %v",err)
 }
 syncs:=0
 store.syncDirectoryOverride=func()error{
  syncs++
  return errors.New("injected private sidecar unlink directory fsync failure")
 }
 err=store.DeleteProviderPayload(x.ID)
 if !errors.Is(err,ErrCleanupDurabilityUnknown)||syncs!=1{
  t.Fatalf("sensitive payload unlink failure not classified correctly: err=%v syncs=%d",err,syncs)
 }
 if _,err=os.Stat(store.providerRunFile(x.ID));!os.IsNotExist(err){
  t.Fatalf("obsolete payload still visible after unlink: %v",err)
 }
 // The cleanup error must not become a provider ACK or re-arm SENDING.
 current,err:=store.Get(x.ID,"local")
 if err!=nil||current.ProviderRun.Phase!="sending"||
  current.RetryMode!=RetryUnlimited{
  t.Fatalf("cleanup modified canonical SENDING journal: %v",err)
 }
 review:=ProviderEgressReviewForSession(current)
 if review==nil||review.AutomaticReplayAllowed||!review.RequiresReconciliation{
  t.Fatal("sidecar cleanup failure incorrectly authorized provider replay")
 }
 // A second, already-absent deletion is idempotent, not a repeated sync.
 if err=store.DeleteProviderPayload(x.ID);err!=nil||syncs!=1{
  t.Fatalf("idempotent sidecar deletion unexpectedly failed: err=%v syncs=%d",err,syncs)
 }
 store.syncDirectoryOverride=nil
 reopened,err:=NewStore(store.dir);if err!=nil{t.Fatal(err)}
 after,err:=reopened.Get(x.ID,"local")
 if err!=nil||after.ProviderRun.Phase!="sending"{
  t.Fatalf("canonical SENDING phase lost: %v",err)
 }
 if _,_,err=reopened.PreparedProviderPayload(x.ID,"local");!errors.Is(err,ErrInvalidSession){
  t.Fatalf("deleted sidecar affected provider replay fence: %v",err)
 }
}

func TestCleanupUnknownErrorContainsNoProviderPromptBytes(t *testing.T){
 if strings.Contains(ErrCleanupDurabilityUnknown.Error(),"prompt"){
  t.Fatal("cleanup error text must not include any provider prompt")
 }
}
