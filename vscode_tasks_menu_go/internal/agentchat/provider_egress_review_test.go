package agentchat

import (
 "strings"
 "testing"
 "time"
)

func TestProviderEgressReviewOnlyAmbiguousPhasesAreDisplayed(t *testing.T){
 for _,phase:=range []string{"prepared","sending","acknowledged","unknown","completed","blocked"}{
  t.Run(phase,func(t *testing.T){
   run:=&ProviderRun{
    ID:"r2-e7",Kind:"feedback",Phase:phase,
    PayloadSHA256:strings.Repeat("c",64),ProviderFingerprint:"fingerprint",
    CreatedAt:time.Now().UTC(),UpdatedAt:time.Now().UTC(),
   }
   x:=Session{ProviderRun:run}
   before:=*run
   review:=ProviderEgressReviewForSession(x)
   ambiguous:=phase=="sending"||phase=="unknown"
   if ambiguous{
    if review==nil||!review.RequiresReconciliation||
     review.AutomaticReplayAllowed||review.RunID!=run.ID||
     review.PayloadSHA256!=run.PayloadSHA256||
     review.Kind!=run.Kind||review.Phase!=phase||
     !strings.Contains(review.Guidance,"Never automatically replay"){
     t.Fatalf("unsafe or missing read-only reconciliation warning: %+v",review)
    }
   }else if review!=nil{
    t.Fatalf("non-ambiguous %s run incorrectly requested reconciliation: %+v",phase,review)
   }
   if *run!=before{t.Fatal("diagnostic mutated provider egress journal")}
  })
 }
 if ProviderEgressReviewForSession(Session{})!=nil{
  t.Fatal("absent provider run should have no review warning")
 }
}
