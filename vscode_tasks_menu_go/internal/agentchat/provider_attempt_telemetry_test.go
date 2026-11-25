package agentchat

import (
 "context"
 "errors"
 "os"
 "strings"
 "testing"
)

func TestProviderAttemptTelemetryPreservesLegacyAuditAndNoAck(t *testing.T){
 s,x,payload:=sendingAttemptFixture(t)
 if _,err:=s.RecordProviderAttempt(x.ID,x.Owner,x.ProviderRun.ID,1,payload,true);err!=nil{t.Fatal(err)}
 // Existing v1 records without optional fields must still validate.
 for i:=uint64(2);i<=4;i++{
  if _,err:=s.RecordProviderAttempt(x.ID,x.Owner,x.ProviderRun.ID,i,payload,false,ProviderAttemptPolicy{Mode:RetryUnlimited});err!=nil{t.Fatal(err)}
  if err:=s.RecordProviderAttemptOutcome(x.ID,x.Owner,x.ProviderRun.ID,i,ProviderPayloadDigest(payload),AttemptConnectTimeout);err!=nil{t.Fatal(err)}
 }
 opened,err:=NewStore(s.dir);if err!=nil{t.Fatal(err)}
 audit,err:=opened.ProviderAttempts(x.ID,x.Owner);if err!=nil{t.Fatal(err)}
 if audit.Total!=4||audit.Attempts[0].Outcome!=""||audit.Attempts[3].Outcome!=AttemptConnectTimeout||audit.Attempts[3].PayloadBytes!=len(payload)||audit.Attempts[3].RetryMode!=RetryUnlimited{t.Fatalf("bad telemetry %+v",audit)}
 if err=opened.RecordProviderAttemptOutcome(x.ID,x.Owner,x.ProviderRun.ID,3,ProviderPayloadDigest(payload),AttemptResponse);!errors.Is(err,ErrStaleRevision){t.Fatal("old attempt outcome overwritten")}
 if err=opened.RecordProviderAttemptOutcome(x.ID,x.Owner,x.ProviderRun.ID,4,ProviderPayloadDigest(payload),AttemptResponse);!errors.Is(err,ErrStaleRevision){t.Fatal("conflicting outcome accepted")}
 raw,err:=os.ReadFile(s.providerAttemptsPath(x.ID));if err!=nil{t.Fatal(err)}
 if strings.Contains(string(raw),"original immutable prompt"){t.Fatal("prompt leaked into diagnostics")}
 got,err:=opened.Get(x.ID,x.Owner);if err!=nil||got.ProviderRun.Phase!="sending"||got.Revision!=x.Revision{t.Fatal("telemetry changed canonical journal")}
}

func TestProviderAttemptOutcomeClassification(t *testing.T){
 for _,tc:=range []struct{err error;want ProviderAttemptOutcome}{
  {nil,AttemptResponse},{context.Canceled,AttemptCancelled},{context.DeadlineExceeded,AttemptRequestTimeout},
  {&ProviderError{Code:"DIAL_TIMEOUT"},AttemptConnectTimeout},{&ProviderError{Code:"RESPONSE_HEADER_TIMEOUT"},AttemptHeaderTimeout},
  {&ProviderError{HTTPStatus:499},AttemptHTTP499},{&ProviderError{HTTPStatus:400,ProviderErrorCode:"context_length_exceeded"},AttemptContextRejected},
  {&ProviderError{HTTPStatus:400,ProviderErrorCode:"context_length_exceeded",Ambiguous:true},AttemptHTTPRejected},
  {&ProviderError{Code:"RESPONSE_INCOMPLETE"},AttemptIncomplete},{errors.New("synthetic secret"),AttemptUnknown},
 }{if got:=ClassifyProviderAttemptOutcome(nil,tc.err);got!=tc.want{t.Fatalf("got %q want %q",got,tc.want)}}
 if ClassifyProviderAttemptOutcome(context.Canceled,nil)!=AttemptCancelled{t.Fatal("Stop did not win")}
}
