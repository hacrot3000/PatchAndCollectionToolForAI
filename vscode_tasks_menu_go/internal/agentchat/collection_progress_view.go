package agentchat

// CollectionProgressView is a bounded read-only projection of committed
// objective metadata. It does not expose source paths, proof/goal hashes,
// prompts, credentials, or any permission to retry or execute tools.
type CollectionProgressView struct {
 State string `json:"state"`
 Reason string `json:"reason,omitempty"`
 Generation uint64 `json:"generation,omitempty"`
 Intent string `json:"intent,omitempty"`
 Requested uint64 `json:"requested"`
 Observed uint64 `json:"observed"`
 NativeReads uint64 `json:"native_reads"`
 NewEvidence uint64 `json:"verified_new"`
 DuplicateEvidence uint64 `json:"verified_duplicate"`
 UnknownEvidence uint64 `json:"unknown"`
 Errors uint64 `json:"errors"`
 HTTPAttempts uint64 `json:"recorded_http_attempts"`
 PatchProposals uint64 `json:"patch_proposals"`
 PatchExecutions uint64 `json:"patch_executions"`
 VerifiedProgress uint64 `json:"verified_progress"`
 Replans uint64 `json:"replans"`
 LegacyIncomplete bool `json:"legacy_incomplete,omitempty"`
 AttemptAuditIncomplete bool `json:"attempt_audit_incomplete,omitempty"`
}
func CollectionProgressViewForSession(x Session)*CollectionProgressView{
 if len(x.CollectionGuard)==0{return nil}
 g,ok:=x.ReadCollectionGuard()
 if !ok{return &CollectionProgressView{State:"metadata_unavailable"}}
 c:=g.Counters
 v:=&CollectionProgressView{State:"idle",Generation:g.Generation,Intent:string(c.Objective.Intent),
  Requested:c.Requested,Observed:c.Observed,NativeReads:c.NativeReads,NewEvidence:c.NewEvidence,
  DuplicateEvidence:c.DuplicateEvidence,UnknownEvidence:c.UnknownEvidence,Errors:c.Errors,
  HTTPAttempts:c.HTTPAttempts,PatchProposals:c.PatchProposals,PatchExecutions:c.PatchExecutions,
  VerifiedProgress:c.VerifiedProgressEvents,Replans:g.Recovery.Attempts,LegacyIncomplete:g.Legacy.Incomplete,
  AttemptAuditIncomplete:g.AttemptAuditIncomplete}
 if x.Status=="cancelled"{v.State="stopped";return v}
 if x.ProviderRun!=nil&&x.ProviderRun.Phase=="unknown"{v.State="recovery_required";return v}
 if x.Status=="waiting_tool"||x.Status=="tool_queuing"{v.State="waiting_tool";return v}
 // Read the most recent relevant event in the current turn only. Stale
 // checkpoints must not mark a later successful/user-started turn stalled.
 for i:=len(x.Events)-1;i>=0;i--{
  e:=x.Events[i]
  switch e.Kind{
  case "turn_started","turn_completed":return v
  case "clp_checkpoint":
   if x.Status=="failed"{
    v.State="stalled"
    switch e.Detail{
    case "recovery_budget_exhausted","recovery_not_authorized","recovery_already_recorded","user_stopped","provider_egress_unresolved","canonical_call_output_pending","invalid_recovery_state":v.Reason=e.Detail
    }
    return v
   }
  case "clp_replan":
   if x.Status=="requesting_ai"{v.State="replanning";return v}
  case "collect_started":
   if x.Status=="requesting_ai"{v.State="reading";return v}
  case "provider_egress","collect_observed":
   if x.Status=="requesting_ai"{v.State="waiting_provider";return v}
  }
 }
 if x.Status=="requesting_ai"{v.State="waiting_provider"}
 return v
}
