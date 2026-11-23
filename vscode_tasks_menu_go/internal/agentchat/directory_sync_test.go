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

func TestJournalFsyncBeforeAndAfterManifestRename(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 original,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 x.Status="owner_updated"

 // Failure before journal rename: the old owner revision is still
 // authoritative, including after reopening the Store.
 store.syncDirectoryOverride=func()error{return errors.New("injected precommit directory fsync error")}
 _,err=store.Save(x,x.Revision)
 if err==nil||errors.Is(err,ErrJournalDurabilityUnknown){
  t.Fatalf("precommit error must NOT claim an ambiguous commit: %v",err)
 }
 unchanged,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 if !bytes.Equal(original,unchanged){t.Fatal("precommit fsync error updated canonical journal")}
 reopened,err:=NewStore(store.dir);if err!=nil{t.Fatal(err)}
 prior,err:=reopened.Get(x.ID,"local")
 if err!=nil||prior.Revision!=x.Revision||prior.Title!=""{
  t.Fatalf("precommit fsync error changed owner state: %v",err)
 }

 // Failure after journal rename: the requested revision MAY have committed.
 // A caller must not automatically repeat provider/native side effects.
 calls:=0
 store.syncDirectoryOverride=func()error{
  calls++
  if calls==2{return errors.New("injected postcommit directory fsync error")}
  return syncAgentDirectory(store.dir)
 }
 _,err=store.Save(x,x.Revision)
 if !errors.Is(err,ErrJournalDurabilityUnknown)||calls!=2{
  t.Fatalf("postcommit boundary did not return explicit uncertainty: %v calls=%d",err,calls)
 }
 saved,err:=reopened.Get(x.ID,"local")
 if err!=nil||saved.Revision!=x.Revision+1||saved.Status!="owner_updated"||saved.Title!=""{
  t.Fatalf("journal rename was not observable after postcommit sync error: %v",err)
 }
 store.syncDirectoryOverride=nil
 if err=syncAgentDirectory(store.dir);err!=nil{t.Fatal(err)}
}

func TestSegmentFsyncNeverPublishesMissingPageAndPreservesRecovery(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Directory-ordered proof commitments",planStepFixtures(160))
 if err!=nil{t.Fatal(err)}
 x.TaskPlan.Steps[0].Status="done"
 x.TaskPlan.Steps[0].Evidence=[]TaskPlanEvidence{{
  Kind:"manual",Note:"Prior owner verified evidence",
 }}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 refs:=append([]TaskPlanSegmentRef(nil),x.TaskPlanSegments...)
 journal,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}

 // The immutable new page can exist as an orphan, but the journal MUST
 // NOT point to it until its directory entry has been fsynced.
 store.syncDirectoryOverride=func()error{return errors.New("injected page sync failure")}
 _,err=store.SetSegmentTaskPlanStepStatus(x.ID,"local",x.Revision,
  "step-0135","blocked",128,24)
 if err==nil||errors.Is(err,ErrJournalDurabilityUnknown){
  t.Fatalf("pre-manifest sync failure returned false commit: %v",err)
 }
 now,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 if !bytes.Equal(now,journal){t.Fatal("page fsync failure changed canonical journal")}
 store.syncDirectoryOverride=nil
 if _,err=store.RecoverTaskPlanOrphans();err!=nil{t.Fatal(err)}
 after,err:=store.Get(x.ID,"local")
 if err!=nil||after.Revision!=x.Revision||after.TaskPlan.Steps[135].Status!="pending"{
  t.Fatalf("precommit crash did not retain prior accepted milestone state: %v",err)
 }

 // The canonical journal rename occurs only AFTER a durable new page.
 // If the second directory fsync fails, the new revision may exist and
 // the old page must remain until startup cleanup can safely collect it.
 syncs:=0
 store.syncDirectoryOverride=func()error{
  syncs++
  if syncs==2{return errors.New("injected postmanifest directory fsync error")}
  return syncAgentDirectory(store.dir)
 }
 _,err=store.SetSegmentTaskPlanStepStatus(x.ID,"local",x.Revision,
  "step-0135","blocked",128,24)
 if !errors.Is(err,ErrJournalDurabilityUnknown)||syncs!=2{
  t.Fatalf("unexpected post-manifest fsync failure: %v syncs=%d",err,syncs)
 }
 store.syncDirectoryOverride=nil
 reopened,err:=NewStore(store.dir);if err!=nil{t.Fatal(err)}
 survived,err:=reopened.Get(x.ID,"local")
 if err!=nil||survived.Revision!=x.Revision+1||
  survived.TaskPlan.Steps[135].Status!="blocked"||
  survived.TaskPlan.Steps[0].Evidence[0].Note!="Prior owner verified evidence"{
  t.Fatalf("uncertain journal rename lost milestone evidence: %v",err)
 }
 for i,ref:=range survived.TaskPlanSegments{
  if i==135/taskPlanSegmentSize{
   if ref==refs[i]||ref.Revision!=survived.Revision{
    t.Fatal("changed page not referenced by the new manifest")
   }
  }else if ref!=refs[i]{t.Fatal("unmodified page SHA changed")}
 }
 if _,err=os.Stat(store.taskPlanSegmentPath(x.ID,refs[135/taskPlanSegmentSize]));err!=nil{
  t.Fatalf("old page must survive until safe orphan cleanup: %v",err)
 }
 if _,err=reopened.RecoverTaskPlanOrphans();err!=nil{t.Fatal(err)}
 if _,err=os.Stat(store.taskPlanSegmentPath(x.ID,refs[135/taskPlanSegmentSize]));!os.IsNotExist(err){
  t.Fatal("recovery failed to remove old no-longer-referenced immutable page")
 }
 if err=syncAgentDirectory(store.dir);err!=nil{t.Fatal(err)}
}

func TestPreparedAndProviderAttemptFsyncFailuresRemainSafe(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 payload:=[]byte(`{"model":"m","input":"exact request"}`)
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.ProviderRun=&ProviderRun{
  ID:"r2-e1",Kind:"feedback",Phase:"prepared",
  PayloadSHA256:ProviderPayloadDigest(payload),CreatedAt:now,UpdatedAt:now,
 }
 store.syncDirectoryOverride=func()error{return errors.New("injected staged payload fsync error")}
 if err=store.StageProviderPayload(x,payload);err==nil{
  t.Fatal("prepared request became replayable without payload directory fsync")
 }
 if _,_,err=store.PreparedProviderPayload(x.ID,"local");!errors.Is(err,ErrInvalidSession){
  t.Fatalf("unsynced PREPARED payload was eligible for replay: %v",err)
 }
 store.syncDirectoryOverride=nil
 if err=store.StageProviderPayload(x,payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 x.ProviderRun.Phase="sending"
 x.ProviderRun.UpdatedAt=time.Now().UTC()
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 store.syncDirectoryOverride=func()error{return errors.New("injected attempt directory fsync error")}
 if _,err=store.RecordProviderAttempt(x.ID,"local",x.ProviderRun.ID,1,payload,true);err==nil{
  t.Fatal("attempt was allowed to cross network without support-audit fsync")
 }
 store.syncDirectoryOverride=nil
 audit,err:=store.ProviderAttempts(x.ID,"local")
 if err!=nil||audit==nil||audit.Total!=1||
  !strings.HasPrefix(audit.Attempts[0].ClientRequestID,"td-"){
  t.Fatalf("attempt sidecar rename/metadata unexpected after fsync error: %v",err)
 }
 // On failure, the caller MUST NOT assume either that an attempt was made
 // or that provider accepted it. The original SENDING fence stays active.
 got,err:=store.Get(x.ID,"local")
 if err!=nil||got.ProviderRun.Phase!="sending"{
  t.Fatalf("audit fsync failure bypassed original sending fence: %v",err)
 }
}

func TestDeletingSessionRemovesProviderAttemptAudit(t *testing.T){
 store,x,payload:=sendingAttemptFixture(t)
 if _,err:=store.RecordProviderAttempt(x.ID,"local",x.ProviderRun.ID,1,payload,true);err!=nil{
  t.Fatal(err)
 }
 // Explicit deletion is only allowed after SENDING is resolved; no
 // out-of-band owner request can silently erase an ambiguous provider run.
 x.Status="failed"
 x.ProviderRun.Phase="unknown"
 x,err:=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if err=store.Delete(x.ID,"wrong-owner",x.Revision);!errors.Is(err,ErrNotFound){
  t.Fatalf("owner-scoped deletion was bypassed: %v",err)
 }
 if err=store.Delete(x.ID,"local",x.Revision);err!=nil{t.Fatal(err)}
 if _,err=os.Stat(store.providerAttemptsPath(x.ID));!os.IsNotExist(err){
  t.Fatalf("session deletion left private provider attempt metadata: %v",err)
 }
}
