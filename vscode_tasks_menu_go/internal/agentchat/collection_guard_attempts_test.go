package agentchat

import (
 "os"
 "testing"
 "time"
)

func TestCollectionGuardReconcilesAttemptLedgerOnceAcrossRestart(t *testing.T){
 s,x,payload:=sendingAttemptFixture(t)
 if !x.BindCollectionGuard("Fix the issue","",false){t.Fatal("bind")}
 var err error
 x,err=s.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 for i:=uint64(1);i<=132;i++{
  if _,err=s.RecordProviderAttempt(x.ID,x.Owner,x.ProviderRun.ID,i,payload,true);err!=nil{t.Fatal(err)}
 }
 reopened,err:=NewStore(s.dir);if err!=nil{t.Fatal(err)}
 x,err=reopened.Get(x.ID,x.Owner);if err!=nil{t.Fatal(err)}
 x.ProviderRun.Phase="unknown";x.Status="failed"
 x,err=reopened.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 g,ok:=x.ReadCollectionGuard()
 if !ok||g.Counters.HTTPAttempts!=132||g.AttemptCursor!=132||g.Counters.Observed!=0||g.Counters.NativeReads!=0||x.ProviderRun.Phase!="unknown"{t.Fatalf("incorrect attempt projection: %+v",g)}
 x,err=reopened.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 g,_=x.ReadCollectionGuard();if g.Counters.HTTPAttempts!=132{t.Fatal("attempt counted twice")}
 // A new explicit run preserves previous attempt count and never reuses old
 // run identity. Test the CAS that moves from unknown to a new sending run.
 now:=time.Now().UTC()
 x.ProviderRun=&ProviderRun{ID:"next-run",Kind:"message",Phase:"sending",PayloadSHA256:ProviderPayloadDigest(payload),ProviderFingerprint:"provider-fingerprint",CreatedAt:now,UpdatedAt:now}
 x.Status="requesting_ai"
 x,err=reopened.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if _,err=reopened.RecordProviderAttempt(x.ID,x.Owner,x.ProviderRun.ID,1,payload,true);err!=nil{t.Fatal(err)}
 x.ProviderRun.Phase="acknowledged"
 x,err=reopened.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 g,_=x.ReadCollectionGuard()
 if g.Counters.HTTPAttempts!=133||g.AttemptRunID!="next-run"||g.AttemptCursor!=1||g.Counters.Observed!=0{t.Fatalf("wrong new run projection: %+v",g)}
}

func TestCollectionGuardCorruptAttemptLedgerDoesNotInventCountOrACK(t *testing.T){
 s,x,payload:=sendingAttemptFixture(t)
 if !x.BindCollectionGuard("Fix the issue","",false){t.Fatal("bind")}
 var err error
 x,err=s.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if _,err=s.RecordProviderAttempt(x.ID,x.Owner,x.ProviderRun.ID,1,payload,true);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(s.providerAttemptsPath(x.ID),[]byte("{}"),0600);err!=nil{t.Fatal(err)}
 x.ProviderRun.Phase="unknown";x.Status="failed"
 x,err=s.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 g,ok:=x.ReadCollectionGuard()
 if !ok||!g.AttemptAuditIncomplete||g.Counters.HTTPAttempts!=0||x.ProviderRun.Phase!="unknown"{t.Fatalf("corrupt audit trusted: %+v",g)}
}
