package agentchat

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func sendingAttemptFixture(t *testing.T)(*Store,Session,[]byte){
 t.Helper()
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project-id","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 payload:=[]byte(`{"model":"m","input":"original immutable prompt"}`)
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.ProviderRun=&ProviderRun{
  ID:"r2-e1",Kind:"message",Phase:"sending",PayloadSHA256:ProviderPayloadDigest(payload),
  ProviderFingerprint:"provider-fingerprint",CreatedAt:now,UpdatedAt:now,
 }
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 return store,x,payload
}

func TestProviderAttemptIDsPersistPerRetryAndNeverAuthorizeReplay(t *testing.T){
 store,x,payload:=sendingAttemptFixture(t)
 ids:=map[string]bool{}
 for i:=uint64(1);i<=3;i++{
  id,err:=store.RecordProviderAttempt(x.ID,"local",x.ProviderRun.ID,i,payload,true)
  if err!=nil{t.Fatal(err)}
  if !strings.HasPrefix(id,"td-")||len(id)!=67||ids[id]{
   t.Fatalf("nonunique provider request correlation: %q",id)
  }
  ids[id]=true
 }
 reopened,err:=NewStore(store.dir);if err!=nil{t.Fatal(err)}
 audit,err:=reopened.ProviderAttempts(x.ID,"local")
 if err!=nil{t.Fatal(err)}
 if audit==nil||audit.RunID!=x.ProviderRun.ID||audit.Total!=3||
  audit.Truncated||len(audit.Attempts)!=3{
  t.Fatalf("audit did not survive restart: %+v",audit)
 }
 for i,item:=range audit.Attempts{
  if item.Attempt!=uint64(i+1)||!ids[item.ClientRequestID]||
   !item.HeaderConfigured||item.PayloadSHA256!=ProviderPayloadDigest(payload)||
   item.RecordedAt.IsZero(){
   t.Fatal("attempt audit lost exact pre-send correlation record")
  }
 }
 if _,err=reopened.ProviderAttempts(x.ID,"other");!errors.Is(err,ErrNotFound){
  t.Fatalf("cross-owner provider audit was exposed: %v",err)
 }
 if _,err=store.RecordProviderAttempt(x.ID,"other",x.ProviderRun.ID,4,payload,true);
  !errors.Is(err,ErrNotFound){t.Fatal("cross-owner audit write admitted")}
 if _,err=store.RecordProviderAttempt(x.ID,"local",x.ProviderRun.ID,3,payload,true);
  !errors.Is(err,ErrStaleRevision){t.Fatal("duplicate attempt number was reused")}
 if _,err=store.RecordProviderAttempt(x.ID,"local",x.ProviderRun.ID,5,payload,true);
  !errors.Is(err,ErrStaleRevision){t.Fatal("attempt index gap was admitted")}
 if _,err=store.RecordProviderAttempt(x.ID,"local","another-run",4,payload,true);
  !errors.Is(err,ErrInvalidSession){t.Fatal("foreign run ID was admitted")}
 x.ProviderRun.Phase="unknown"
 x.Status="failed"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if _,err=store.RecordProviderAttempt(x.ID,"local",x.ProviderRun.ID,4,payload,true);
  !errors.Is(err,ErrInvalidSession){
  t.Fatal("UNKNOWN provider egress created a replayable attempt")
 }
 review:=ProviderEgressReviewForSession(x)
 if review==nil||review.AutomaticReplayAllowed||!review.RequiresReconciliation{
  t.Fatal("provider correlation incorrectly authorized replay")
 }
 retained,err:=reopened.ProviderAttempts(x.ID,"local")
 if err!=nil||retained.Total!=3{
  t.Fatalf("unknown delivery destroyed support audit: %v %+v",err,retained)
 }
}

func TestProviderAttemptAuditRejectsCorruptionAndHasBoundedHistory(t *testing.T){
 store,x,payload:=sendingAttemptFixture(t)
 for i:=uint64(1);i<=132;i++{
  if _,err:=store.RecordProviderAttempt(x.ID,"local",x.ProviderRun.ID,i,payload,i%2==0);err!=nil{
   t.Fatalf("attempt %d: %v",i,err)
  }
 }
 audit,err:=store.ProviderAttempts(x.ID,"local");if err!=nil{t.Fatal(err)}
 if audit.Total!=132||!audit.Truncated||len(audit.Attempts)!=maxProviderAttemptRecords||
  audit.Attempts[0].Attempt!=5||audit.Attempts[len(audit.Attempts)-1].Attempt!=132{
  t.Fatalf("unbounded or incorrectly ordered provider attempt audit: %+v",audit)
 }
 fi,err:=os.Stat(store.providerAttemptsPath(x.ID));if err!=nil{t.Fatal(err)}
 if fi.Mode().Perm()&0077!=0{t.Fatal("provider attempt audit exposed to another user")}
 if err=os.WriteFile(store.providerAttemptsPath(x.ID),[]byte(`{"version":1,"run_id":"r2-e1","total":132,"sha256":"fake","attempts":[]}`),0600);
  err!=nil{t.Fatal(err)}
 if _,err=store.ProviderAttempts(x.ID,"local");!errors.Is(err,ErrInvalidSession){
  t.Fatalf("tampered audit was trusted: %v",err)
 }
 if _,err=store.RecordProviderAttempt(x.ID,"local",x.ProviderRun.ID,133,payload,true);
  !errors.Is(err,ErrInvalidSession){
  t.Fatalf("corrupt audit overwritten before new provider send: %v",err)
 }
}

func TestProviderClientRequestIDHeaderOnlyForOfficialOpenAIHosts(t *testing.T){
 id:="td-"+strings.Repeat("a",64)
 for _,tc:=range []struct{endpoint string;want bool}{
  {"https://api.openai.com/v1/responses",true},
  {"https://us.api.openai.com/v1/chat/completions",true},
  {"https://eu.api.openai.com/v1/responses",true},
  {"https://api.openai.com.evil.invalid/v1/responses",false},
  {"https://api.example.com/v1/responses",false},
  {"https://openrouter.ai/api/v1/responses",false},
 }{
  ctx:=WithProviderClientRequestID(context.Background(),id)
  req,err:=newProviderRequest(ctx,tc.endpoint,"synthetic-key",[]byte(`{"model":"m"}`))
  if err!=nil{t.Fatal(err)}
  if (req.Header.Get("X-Client-Request-Id")==id)!=tc.want{
   t.Fatalf("%s unexpected header %q",tc.endpoint,req.Header.Get("X-Client-Request-Id"))
  }
  if req.Header.Get("Authorization")!="Bearer synthetic-key"{
   t.Fatal("provider correlation unexpectedly affected authentication")
  }
 }
 invalid:=WithProviderClientRequestID(context.Background(),"td-"+strings.Repeat("é",64))
 req,err:=newProviderRequest(invalid,"https://api.openai.com/v1/responses","key",[]byte("{}"))
 if err!=nil||req.Header.Get("X-Client-Request-Id")!=""{
  t.Fatal("malformed non-ASCII request ID was emitted")
 }
}
