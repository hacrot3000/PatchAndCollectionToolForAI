package agentchat

import (
 "strings"
 "testing"
 "time"
)

func TestProviderReviewWithLivenessDistinguishesActiveSendingAndRealAmbiguity(t *testing.T){
 now:=time.Now().UTC()
 x:=Session{
  Status:"requesting_ai",
  ProviderRun:&ProviderRun{
   ID:"r-one",Kind:"collection",Phase:"sending",
   PayloadSHA256:strings.Repeat("f",64),
   UpdatedAt:now,CreatedAt:now,
  },
 }
 if review:=ProviderEgressReviewWithLiveness(x,nil,now);review!=nil{
  t.Fatal("fresh normal in-flight request displayed an alarming warning")
 }
 x.ProviderRun.UpdatedAt=now.Add(-2*time.Minute)
 if review:=ProviderEgressReviewWithLiveness(x,nil,now);
  review==nil||review.AutomaticReplayAllowed||review.Phase!="sending"{
  t.Fatalf("stale SENDING without worker heartbeat was hidden: %+v",review)
 }
 good:=&ProviderHeartbeat{RunID:"r-one",At:now.Add(-15*time.Second),LeaseUntil:now.Add(time.Minute)}
 if review:=ProviderEgressReviewWithLiveness(x,good,now);review!=nil{
  t.Fatal("live matching heartbeat should suppress only the visual review banner")
 }
 stale:=*good;stale.LeaseUntil=now.Add(-time.Second)
 if review:=ProviderEgressReviewWithLiveness(x,&stale,now);review==nil{
  t.Fatal("stale heartbeat hid the ambiguous provider request")
 }
 mismatched:=*good;mismatched.RunID="r-other"
 if review:=ProviderEgressReviewWithLiveness(x,&mismatched,now);review==nil{
  t.Fatal("unrelated worker heartbeat hid ambiguous request")
 }
 x.Status="failed";x.ProviderRun.Phase="unknown"
 if review:=ProviderEgressReviewWithLiveness(x,good,now);
  review==nil||review.Phase!="unknown"||review.AutomaticReplayAllowed{
  t.Fatal("UNKNOWN must require reconciliation even with a fresh old heartbeat")
 }
 x.Status="tool_rejected";x.ProviderRun.Phase="completed"
 if review:=ProviderEgressReviewWithLiveness(x,nil,now);review!=nil{
  t.Fatal("completed provider response is not a SENDING ambiguity")
 }
}
