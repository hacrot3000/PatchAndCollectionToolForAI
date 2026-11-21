package agentchat

import (
 "errors"
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
 if _,_,err=store.PreparedProviderPayload(x.ID,"local");err==nil{
  t.Fatal("corrupted prepared provider request was replayable")
 }
}
