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

func TestAgentFilesystemAdmissionBlocksAllStoreMutationWithoutChangingJournal(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 original,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 lockFile:=filepath.Join(store.dir,".agent-store.lock")
 if _,err=os.Stat(lockFile);err!=nil{
  t.Fatalf("fixture Store did not use a real journal lock: %v",err)
 }
 // Fault injection is private to this Store object. In production the
 // kernel's statfs classification is used before acquiring the lock.
 checks:=0
 store.filesystemCheckOverride=func(path string)error{
  checks++
  if path!=dir{t.Fatal("unexpected mount path inspected")}
  return unsupportedAgentStateFilesystem("simulated NFS")
 }
 if _,err=store.Get(x.ID,"local");!errors.Is(err,ErrUnsupportedAgentStateFilesystem){
  t.Fatalf("read was admitted from unsafe shared mount: %v",err)
 }
 x.Title="Must not be saved"
 if _,err=store.Save(x,x.Revision);!errors.Is(err,ErrUnsupportedAgentStateFilesystem){
  t.Fatalf("journal CAS admitted after filesystem refusal: %v",err)
 }
 if _,err=store.Get(x.ID,"different-owner");!errors.Is(err,ErrUnsupportedAgentStateFilesystem){
  t.Fatalf("unsupported mount bypassed on wrong-owner access: %v",err)
 }
 if _,err=store.RecordProviderAttempt(
  x.ID,"local","r-test",1,[]byte(`{"model":"m"}`),true,
 );!errors.Is(err,ErrUnsupportedAgentStateFilesystem){
  t.Fatalf("provider attempt crossed unsupported mount: %v",err)
 }
 if _,err=store.RecoverTaskPlanOrphans();!errors.Is(err,ErrUnsupportedAgentStateFilesystem){
  t.Fatalf("startup orphan scanner ignored shared mount admission: %v",err)
 }
 if err=store.Delete(x.ID,"local",x.Revision);!errors.Is(err,ErrUnsupportedAgentStateFilesystem){
  t.Fatalf("owner deletion ignored unsupported mount: %v",err)
 }
 if checks!=6{t.Fatalf("not all filesystem admissions were checked: %d",checks)}
 after,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 if !bytes.Equal(original,after){t.Fatal("rejected mount mutated canonical owner journal")}
 if _,err=os.Stat(store.providerAttemptsPath(x.ID));!os.IsNotExist(err){
  t.Fatalf("rejected egress created provider support sidecar: %v",err)
 }
 // A separately opened Store on a REAL local directory continues to
 // support ordinary owner-scoped CAS after a simulated shared mount error.
 reopened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 latest,err:=reopened.Get(x.ID,"local")
 if err!=nil||latest.Revision!=x.Revision||latest.Title!=""{
  t.Fatalf("local store unexpectedly corrupted or unavailable: %v",err)
 }
 latest.Title="Only local journal writes are permitted"
 updated,err:=reopened.Save(latest,latest.Revision)
 if err!=nil||updated.Revision!=x.Revision+1{
  t.Fatalf("local CAS did not survive refusal: %v",err)
 }
}

func TestAgentStateFilesystemAdmissionDoesNotInvalidatePreparedAndUnknownSafety(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 payload:=[]byte(`{"model":"m","input":"safe PREPARED attempt"}`)
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.ProviderRun=&ProviderRun{
  ID:"r2-e5",Kind:"message",Phase:"prepared",
  PayloadSHA256:ProviderPayloadDigest(payload),
  CreatedAt:now,UpdatedAt:now,
 }
 if err=store.StageProviderPayload(x,payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 store.filesystemCheckOverride=func(string)error{
  return unsupportedAgentStateFilesystem("simulated CIFS")
 }
 if _,_,err=store.PreparedProviderPayload(x.ID,"local");!errors.Is(err,ErrUnsupportedAgentStateFilesystem){
  t.Fatalf("unsafe mount made PREPARED eligible for automatic replay: %v",err)
 }
 store.filesystemCheckOverride=nil
 _,restored,err:=store.PreparedProviderPayload(x.ID,"local")
 if err!=nil||!bytes.Equal(restored,payload){t.Fatalf("local PREPARED evidence damaged: %v",err)}
 x.ProviderRun.Phase="sending"
 x.ProviderRun.UpdatedAt=time.Now().UTC()
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if _,_,err=store.PreparedProviderPayload(x.ID,"local");!errors.Is(err,ErrInvalidSession){
  t.Fatalf("SENDING became replayable after local admission: %v",err)
 }
 audit,err:=store.ProviderAttempts(x.ID,"local")
 if err!=nil||audit!=nil{t.Fatalf("fabricated provider delivery on admission: %v",err)}
 if !strings.Contains(ProviderEgressReviewForSession(x).Reason,"") {
  t.Fatal("unreachable")
 }
}
