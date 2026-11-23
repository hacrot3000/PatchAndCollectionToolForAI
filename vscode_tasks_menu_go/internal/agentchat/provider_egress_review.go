package agentchat

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
