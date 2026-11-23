package agentchat

import "time"


// ProviderEgressReview describes the ambiguity of ONE durable egress marker.
// It is a diagnostic, NOT an acknowledgement, a provider-side lookup, an
// authorization to replay the request or an acceptance of native tool output.
type ProviderEgressReview struct {
 RunID string `json:"run_id"`
 Kind string `json:"kind"`
 Phase string `json:"phase"`
 PayloadSHA256 string `json:"payload_sha256"`
 RequiresReconciliation bool `json:"requires_reconciliation"`
 AutomaticReplayAllowed bool `json:"automatic_replay_allowed"`
 Guidance string `json:"guidance"`
}

// ProviderEgressReviewForSession returns a read-only review hint only when
// authoritative provider acknowledgement is absent past the durable sending
// fence. PREPARED is recoverable through strict original-payload checks and
// needs no SENDING/UNKNOWN reconciliation. A heartbeat is not proof of
// acknowledgement, and this helper never makes provider/network requests.
func ProviderEgressReviewForSession(x Session)*ProviderEgressReview{
 if x.ProviderRun==nil{return nil}
 run:=x.ProviderRun
 if run.Phase!="sending"&&run.Phase!="unknown"{return nil}
 review:=&ProviderEgressReview{
  RunID:run.ID,Kind:run.Kind,Phase:run.Phase,
  PayloadSHA256:run.PayloadSHA256,
  RequiresReconciliation:true,AutomaticReplayAllowed:false,
 }
 if run.Phase=="sending" {
  review.Guidance="SENDING was durably recorded before provider network I/O. The request may still be in flight; after restart its delivery and acknowledgement are unknown. Never automatically replay this run. Inspect the provider's authoritative records and the session handoff."
 }else{
  review.Guidance="Provider acknowledgement is UNKNOWN. A timeout or disconnect does not prove that the provider ignored the request. Never automatically replay this run. Inspect the provider's authoritative records and the session handoff."
 }
 return review
}

// ProviderEgressReviewWithLiveness suppresses premature "please reconcile"
// banners during an ordinary in-flight provider turn. Neither an active
// heartbeat nor a fresh SENDING timestamp is an acknowledgement, and this
// function grants NO retry/replay permission. A terminal UNKNOWN must still
// require reconciliation, even when an old heartbeat is present.
//
// Initial SENDING activity may precede the first heartbeat write. Allow a
// short presentation grace interval so normal fast provider calls do not
// flash an alarming banner. After it expires, a missing/stale heartbeat
// exposes the warning instead of hiding an interrupted request indefinitely.
func ProviderEgressReviewWithLiveness(x Session, heartbeat *ProviderHeartbeat, now time.Time) *ProviderEgressReview {
 review:=ProviderEgressReviewForSession(x)
 if review==nil||review.Phase!="sending"||x.Status!="requesting_ai"{return review}
 run:=x.ProviderRun
 if heartbeat!=nil&&heartbeat.RunID==run.ID&&
  !heartbeat.At.After(now.Add(5*time.Second))&&heartbeat.LeaseUntil.After(now){
  return nil
 }
 age:=now.Sub(run.UpdatedAt)
 if age>=0&&age<60*time.Second{return nil}
 return review
}
