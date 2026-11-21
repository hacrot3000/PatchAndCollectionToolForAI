package agentchat

import (
 "errors"
 "path/filepath"
 "testing"
 "time"
)

func TestProviderHeartbeatPersistsWithoutChangingSessionRevision(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.ProviderRun=&ProviderRun{ID:"run-17",Kind:"collection",Phase:"sending",
  PayloadSHA256:ProviderPayloadDigest([]byte("req")),CreatedAt:now,UpdatedAt:now}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 revision:=x.Revision
 if err=store.TouchActiveProviderHeartbeat(x.ID,"local");err!=nil{t.Fatal(err)}
 record,err:=store.LatestProviderHeartbeat(x.ID,"local")
 if err!=nil||record.RunID!="run-17"||record.At.IsZero(){t.Fatalf("heartbeat did not persist: %v %+v",err,record)}
 latest,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if latest.Revision!=revision{t.Fatal("heartbeat changed canonical revision and would cause stale provider result")}
 if err=store.TouchActiveProviderHeartbeat(x.ID,"other-owner");!errors.Is(err,ErrNotFound){
  t.Fatal("cross-owner heartbeat mutation")
 }
 if _,err=store.LatestProviderHeartbeat(x.ID,"other-owner");!errors.Is(err,ErrNotFound){
  t.Fatal("cross-owner heartbeat access")
 }
 x.ProviderRun.Phase="completed";x.Status="completed"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if err=store.TouchActiveProviderHeartbeat(x.ID,"local");!errors.Is(err,ErrInvalidSession){
  t.Fatal("inactive provider heartbeat updated after completion")
 }
 if err=store.Delete(x.ID,"local",x.Revision);err!=nil{t.Fatal(err)}
 if _,err=store.LatestProviderHeartbeat(x.ID,"local");!errors.Is(err,ErrNotFound){
  t.Fatal("deleted session heartbeat was still accessible")
 }
}
