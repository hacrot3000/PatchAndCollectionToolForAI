package agentchat

import (
 "errors"
 "bytes"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func TestProviderRunPreparedPayloadSurvivesRestartAndIsNotReplayableAfterSending(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project-id","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 payload:=[]byte(`{"model":"model","input":[{"role":"user","content":"Collect evidence"}]}`)
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.ProviderRun=&ProviderRun{ID:"r2-e1",Kind:"message",Phase:"prepared",
  PayloadSHA256:ProviderPayloadDigest(payload),ProviderFingerprint:"stable-model-config",
  CreatedAt:now,UpdatedAt:now}
 if err=store.StageProviderPayload(x,payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 reopened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 got,data,err:=reopened.PreparedProviderPayload(x.ID,"local")
 if err!=nil||string(data)!=string(payload)||got.ProviderRun.ID!="r2-e1" {
  t.Fatalf("valid pre-egress intent did not survive restart: %v",err)
 }
 if _,_,err=reopened.PreparedProviderPayload(x.ID,"other-owner");!errors.Is(err,ErrNotFound){
  t.Fatal("cross-owner egress payload was exposed")
 }
 if err=store.DeleteProviderPayload(x.ID);!errors.Is(err,ErrInvalidSession){t.Fatalf("prepared cleanup accepted: %v",err)}
 reopened,err=NewStore(dir);if err!=nil{t.Fatal(err)}
 got,data,err=reopened.PreparedProviderPayload(x.ID,"local")
 if err!=nil||got.ProviderRun==nil||got.ProviderRun.ID!="r2-e1"||string(data)!=string(payload){t.Fatalf("prepared payload lost after cleanup: %v",err)}
 x.ProviderRun.Phase="sending"
 x.ProviderRun.UpdatedAt=time.Now().UTC()
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 if _,_,err=reopened.PreparedProviderPayload(x.ID,"local");!errors.Is(err,ErrInvalidSession){
  t.Fatal("sending provider bytes were incorrectly eligible for auto replay")
 }
 if err=store.DeleteProviderPayload(x.ID);err!=nil{t.Fatal(err)}
 if _,err=os.Stat(store.providerRunFile(x.ID));!os.IsNotExist(err){t.Fatal("prepared private source persisted after deletion")}
}
func TestProviderRunStageDoesNotOverwritePreparedPayload(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project-id","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 first:=[]byte(`{"model":"model","input":"first"}`)
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.ProviderRun=&ProviderRun{ID:"first",Kind:"message",Phase:"prepared",PayloadSHA256:ProviderPayloadDigest(first),CreatedAt:now,UpdatedAt:now}
 if err=store.StageProviderPayload(x,first);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 second:=[]byte(`{"model":"model","input":"second"}`)
 replacement:=x
 replacement.ProviderRun=&ProviderRun{ID:"second",Kind:"message",Phase:"prepared",PayloadSHA256:ProviderPayloadDigest(second),CreatedAt:now,UpdatedAt:now}
 if err=store.StageProviderPayload(replacement,second);!errors.Is(err,ErrInvalidSession){t.Fatalf("replacement staging accepted: %v",err)}
 reopened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 recovered,data,err:=reopened.PreparedProviderPayload(x.ID,"local")
 if err!=nil||recovered.ProviderRun.ID!="first"||string(data)!=string(first){t.Fatalf("original prepared request lost after reopen: %v",err)}
}

func TestProviderRunConcurrentStagingPreservesCommittedPrepared(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project-id","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 first:=[]byte(`{"model":"model","input":"first"}`)
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.ProviderRun=&ProviderRun{ID:"first",Kind:"message",Phase:"prepared",PayloadSHA256:ProviderPayloadDigest(first),CreatedAt:now,UpdatedAt:now}
 if err=store.StageProviderPayload(x,first);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 second:=[]byte(`{"model":"model","input":"second"}`)
 replacement:=x
 replacement.ProviderRun=&ProviderRun{ID:"second",Kind:"message",Phase:"prepared",PayloadSHA256:ProviderPayloadDigest(second),CreatedAt:now,UpdatedAt:now}
 results:=make(chan error,8)
 for i:=0;i<cap(results);i++ { go func(){results<-store.StageProviderPayload(replacement,second)}() }
 for i:=0;i<cap(results);i++ { if err:=<-results;!errors.Is(err,ErrInvalidSession){t.Fatalf("concurrent staging accepted: %v",err)} }
 reopened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 recovered,data,err:=reopened.PreparedProviderPayload(x.ID,"local")
 if err!=nil{t.Fatal(err)}
 if recovered.ProviderRun==nil || recovered.ProviderRun.ID!="first" || string(data)!=string(first){t.Fatal("concurrent staging damaged committed payload")}
}

func TestProviderRunCompetingStagedPayloadCAS(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 base,err:=NewSession("local","/workspace","project-id","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 base,err=store.Save(base,0);if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 first:=base;first.Status="requesting_ai"
 a:=[]byte(`{"input":"first"}`)
 first.ProviderRun=&ProviderRun{ID:"first",Kind:"message",Phase:"prepared",PayloadSHA256:ProviderPayloadDigest(a),CreatedAt:now,UpdatedAt:now}
 second:=base;second.Status="requesting_ai"
 b:=[]byte(`{"input":"second"}`)
 second.ProviderRun=&ProviderRun{ID:"second",Kind:"message",Phase:"prepared",PayloadSHA256:ProviderPayloadDigest(b),CreatedAt:now,UpdatedAt:now}
 if err=store.StageProviderPayload(first,a);err!=nil{t.Fatal(err)}
 if err=store.StageProviderPayload(second,b);err!=nil{t.Fatal(err)}
 if _,err=store.Save(first,base.Revision);!errors.Is(err,ErrInvalidSession){t.Fatalf("mismatched sidecar committed: %v",err)}
 current,err:=store.Get(base.ID,"local");if err!=nil{t.Fatal(err)}
 if current.Revision!=base.Revision||current.ProviderRun!=nil{t.Fatal("failed CAS changed journal")}
 saved,err:=store.Save(second,base.Revision);if err!=nil{t.Fatal(err)}
 if _,err=store.Save(first,base.Revision);!errors.Is(err,ErrStaleRevision){t.Fatalf("stale CAS accepted: %v",err)}
 reopened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 got,data,err:=reopened.PreparedProviderPayload(saved.ID,"local")
 if err!=nil||got.ProviderRun==nil||got.ProviderRun.ID!="second"||string(data)!=string(b){t.Fatalf("winning payload lost: %v",err)}
}

func TestStageProviderPayloadLockedRejectsInvalidInput(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "private", "agent")
 store, err := NewStore(dir); if err != nil { t.Fatal(err) }
 x, err := NewSession("local", "/workspace", "project-id", "provider", "model", Stateless); if err != nil { t.Fatal(err) }
 payload := []byte("valid payload")
 now := time.Now().UTC()
 x.ProviderRun = &ProviderRun{ID:"run", Kind:"message", Phase:"prepared", PayloadSHA256:ProviderPayloadDigest(payload), CreatedAt:now, UpdatedAt:now}
 err = store.withLock(func() error { return store.stageProviderPayloadLocked(x, []byte("different")) })
 if !errors.Is(err, ErrInvalidSession) { t.Fatalf("mismatched payload accepted: %v", err) }
 if _, err = os.Lstat(store.providerRunFile(x.ID)); !os.IsNotExist(err) { t.Fatalf("rejected input created sidecar: %v", err) }
}

func TestProviderRunPayloadTamperingFailsClosed(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project-id","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 payload:=[]byte(`{"model":"model","input":"original"}`)
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.ProviderRun=&ProviderRun{ID:"r2",Kind:"collection",Phase:"prepared",
  PayloadSHA256:ProviderPayloadDigest(payload),CreatedAt:now,UpdatedAt:now}
 if err=store.StageProviderPayload(x,payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if err=os.WriteFile(store.providerRunFile(x.ID),[]byte(`{"version":1,"run_id":"r2","sha256":"bad","payload":"YXJiaXRyYXJ5"}`),0600);err!=nil{t.Fatal(err)}
 if _,err=store.Save(x,x.Revision);!errors.Is(err,ErrInvalidSession){
  t.Fatalf("same-run PREPARED save accepted corrupted sidecar: %v",err)
 }
 reopened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 persisted,err:=reopened.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if persisted.Revision!=x.Revision||persisted.ProviderRun==nil||persisted.ProviderRun.ID!=x.ProviderRun.ID{t.Fatal("rejected save changed durable journal")}
 if _,_,err=store.PreparedProviderPayload(x.ID,"local");err==nil{
  t.Fatal("corrupted prepared provider request was replayable")
 }
}


func TestProviderRunPayloadMatchesSerializerLimit(t *testing.T){
 for _,size:=range []int{(2<<20)+1,maxRequestBytes,maxRequestBytes+1}{
  t.Run(fmt.Sprint(size),func(t *testing.T){
   store,err:=NewStore(filepath.Join(t.TempDir(),"private"));if err!=nil{t.Fatal(err)}
   x,err:=NewSession("local","/workspace","project-id","provider","model",Stateless);if err!=nil{t.Fatal(err)}
   x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
   // Valid JSON with exact byte length, no session history duplication.
   prefix,suffix:=[]byte(`{"model":"model","input":"`),[]byte(`"}`)
   payload:=append(append(append([]byte(nil),prefix...),bytes.Repeat([]byte("x"),size-len(prefix)-len(suffix))...),suffix...)
   now:=time.Now().UTC()
   x.Status="requesting_ai"
   x.ProviderRun=&ProviderRun{ID:"r2-e1",Kind:"message",Phase:"prepared",PayloadSHA256:ProviderPayloadDigest(payload),CreatedAt:now,UpdatedAt:now}
   err=store.StageProviderPayload(x,payload)
   if size>maxRequestBytes{
    if !errors.Is(err,ErrInvalidSession){t.Fatalf("over-limit staging: %v",err)}
    if _,e:=os.Stat(store.providerRunFile(x.ID));!os.IsNotExist(e){t.Fatal("oversize request staged")}
    return
   }
   if err!=nil{t.Fatal(err)}
   x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
   reopened,err:=NewStore(store.dir);if err!=nil{t.Fatal(err)}
   _,got,err:=reopened.PreparedProviderPayload(x.ID,x.Owner);if err!=nil||!bytes.Equal(got,payload){t.Fatalf("large PREPARED restart: %v",err)}
   x.ProviderRun.Phase="sending"
   x,err=reopened.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
   if _,err=reopened.RecordProviderAttempt(x.ID,x.Owner,x.ProviderRun.ID,1,payload,false,ProviderAttemptPolicy{Mode:RetryUnlimited});err!=nil{t.Fatal(err)}
   audit,err:=reopened.ProviderAttempts(x.ID,x.Owner);if err!=nil||audit==nil||audit.Attempts[0].PayloadBytes!=size{t.Fatalf("attempt ledger mismatch: %v",err)}
   if _,_,err=reopened.PreparedProviderPayload(x.ID,x.Owner);err==nil{t.Fatal("SENDING became replayable")}
  })
 }
}
