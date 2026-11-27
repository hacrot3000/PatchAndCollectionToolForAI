package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

// Live owner opt-out and pending legacy work always win over a pinned
// automation grant. No provider or workspace is selected here.
func runAutoContinueMode(x agentchat.Session, policy agentrun.AutomationSnapshot) string {
	if x.Owner != "local" || x.Archived || agentchat.HasUnresolvedWork(x) { return "" }
	if x.ProviderRun != nil {
		switch x.ProviderRun.Phase {
		case "prepared", "sending", "unknown":
			return ""
		}
	}
	for _, call := range x.ToolCalls {
		if (call.Status == "done" || call.Status == "error" || call.Status == "rejected") &&
			(!call.FeedbackSent || call.FeedbackStatus != "acknowledged") { return "" }
	}
	if x.TaskPlan != nil {
		if policy.AutoContinuePlan && x.TaskPlan.AutoContinue && agentchat.TaskPlanNext(x.TaskPlan) != nil {
			return "plan"
		}
		return ""
	}
	if policy.AutoContinueText && x.AutoContinueText { return "conversation" }
	return ""
}

// Only the successful current provider attempt contributes a turn advisory.
// A transport retry invalidates every partial delta from its prior request.
func finalRunTurnText(run agentrun.Record) string {
	var b strings.Builder
	for _, event := range run.Events {
		if event.AttemptID != run.Spec.AttemptID { continue }
		switch event.Type {
		case agentchat.RunEventTextDelta:
			if b.Len()+len(event.Payload) > 512<<10 { return "" }
			b.Write(event.Payload)
		case agentchat.RunEventRetry:
			var retry struct { Reset bool `json:"reset_partial_output"` }
			if json.Unmarshal(event.Payload, &retry) == nil && retry.Reset { b.Reset() }
		}
	}
	return b.String()
}

// Native read-only receipts and owner-accepted milestones are progress signals,
// not semantic acceptance. Repeated identical reads do not reset the brake.
// Owner-pinned exact Collection proofs are adopted before this signature is used.
func runVerifiedProgressSignature(run agentrun.Record, session agentchat.Session) string {
	calls := map[string]string{}
	verified := map[string]bool{}
	for _, item := range run.History {
		if item.OriginRunID != run.Spec.RunID { continue }
		if item.Type == "function_call" && item.Name == "taskdeck_collect_workspace" {
			var canonical any
			if json.Unmarshal([]byte(item.Arguments), &canonical) == nil {
				// Provider request IDs/titles are not actual work.
				if object, ok := canonical.(map[string]any); ok {
					delete(object, "id")
					delete(object, "title")
					delete(object, "ai_context")
				}
				body, _ := json.Marshal(canonical)
				calls[item.CallID] = string(body)
			}
		}
		if item.Type != "function_call_output" || calls[item.CallID] == "" { continue }
		var receipt struct {
			Source string `json:"source"`
			Status string `json:"status"`
			NativeStatus string `json:"native_status"`
			NativeSessionID string `json:"native_session_id"`
			RunID string `json:"run_id"`
			Verified bool `json:"evidence_verified"`
			RC *int `json:"rc"`
		}
		if json.Unmarshal([]byte(item.Content), &receipt) == nil && receipt.Source == "native_patch_tool_collect" &&
			receipt.Status == "PASS" && receipt.NativeStatus == "PASS" && receipt.Verified &&
			receipt.RC != nil && *receipt.RC == 0 && receipt.NativeSessionID != "" && receipt.RunID != "" {
			verified[calls[item.CallID]] = true
		}
	}
	keys := make([]string, 0, len(verified))
	for key := range verified { keys = append(keys, key) }
	sort.Strings(keys)
	done, total := agentchat.TaskPlanProgress(session.TaskPlan)
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d/%d\n%s", done, total, strings.Join(keys, "\n"))))
	return hex.EncodeToString(digest[:])
}

func (e ownerBoundAgentRunExecutor) ResolveCompletedTurn(ctx context.Context, run agentrun.Record) (agentrun.TurnDecision, error) {
	text := finalRunTurnText(run)
	// Even an empty successful provider turn can acknowledge an exact native receipt.
	progress := agentchat.AnalyzeTurnText(text)
	decision := agentrun.TurnDecision{Progress:progress}
	s := e.server
	if s == nil || s.Config.SharedServerEnabled || e.owner != "local" || ctx.Err() != nil {
		return decision, nil
	}
	inputs, err := s.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(ctx, e.owner), run.Spec)
	if err != nil { progress.Reason = "run_binding_unavailable"; return decision, nil }
	if inputs.Snapshot.Policy == nil { return decision, nil }
	release, ok := lockAgentTurn(inputs.OwnerWorkspacePath, e.owner, run.Spec.ConversationID)
	if !ok { progress.Reason = "conversation_busy_review_required"; return decision, nil }
	defer release()
	store, err := s.agentChatStore()
	if err != nil { return decision, err }
	x, err := store.Get(run.Spec.ConversationID, e.owner)
	if err != nil || x.ProjectID != inputs.ProjectID { return decision, errorsRunContinuationScope() }
	// Adopt only explicitly owner-pinned, run-qualified mechanical proofs
	// before choosing the next milestone. A failed CAS must not send the
	// next prompt using an obsolete plan.
	if changed, proofErr := applyRunCollectionAutoChecks(ctx, &x, run, inputs.Snapshot); proofErr != nil {
		return decision, proofErr
	} else if changed {
		if ctx.Err()!=nil{return decision,ctx.Err()}
		x, err = store.Save(x, x.Revision)
		if err != nil { return decision, err }
	}
	if strings.TrimSpace(text)=="" {return agentrun.TurnDecision{},nil}
	mode := runAutoContinueMode(x, inputs.Snapshot.Policy.Automation)
	signature := runVerifiedProgressSignature(run,x)
	// The inner reducer has exact receipt proof and scope. Do not let the
	// outer scheduler treat request-shape changes or duplicate receipts
	// as independently verified work, nor miss a changed source revision.
	if g,ok:=readRunCollectionProgress(e.owner,run.Spec,inputs);ok{
		done,total:=agentchat.TaskPlanProgress(x.TaskPlan)
		digest:=sha256.Sum256([]byte(fmt.Sprintf("clp-v1:%s:%d:%d:%d:%d/%d",g.Counters.Objective.Key,g.Counters.NewEvidence,g.Counters.VerifiedProgressEvents,g.Counters.AttestedProgressEvents,done,total)))
		signature=hex.EncodeToString(digest[:])
	}else if len(inputs.Snapshot.Policy.CollectionProgress)!=0{
		done,total:=agentchat.TaskPlanProgress(x.TaskPlan)
		digest:=sha256.Sum256([]byte(fmt.Sprintf("clp-unknown:%d/%d",done,total)))
		signature=hex.EncodeToString(digest[:])
	}
	progress.LastWorkSignature = signature
	if old := run.Continuation; old != nil {
		progress.NoProgressTurns = old.NoProgressTurns
		if old.LastWorkSignature == signature { progress.NoProgressTurns++ } else { progress.NoProgressTurns = 0 }
		if old.ProgressSignature == progress.ProgressSignature {
			progress.RepeatedTurns = old.RepeatedTurns+1
			if progress.RepeatedTurns>100 { progress.RepeatedTurns=100 }
		}
	}
	if progress.NoProgressTurns >= agentchat.MaxContinuationNoWorkTurns {
		progress.NoProgressTurns = agentchat.MaxContinuationNoWorkTurns
		progress.Reason = "stalled_without_verified_work"
		return decision,nil
	}
	if mode == "" {
		progress.Reason = "automatic_run_continuation_not_authorized"
		return decision,nil
	}
	if progress.Disposition == agentchat.TurnBlocked || progress.Disposition == agentchat.TurnWaitingTool {
		return decision,nil
	}
	message := ""
	if mode == "plan" {
		step := agentchat.TaskPlanNext(x.TaskPlan)
		if step == nil { return decision,nil }
		message = fmt.Sprintf("TaskDeck owner-authorized continuation of the ORIGINAL task plan. Next unmet milestone %q: %s. Acceptance: %s. Only real native receipts or explicit owner acceptance count. Model claims never complete the plan.",step.ID,step.Title,step.Acceptance)
	} else {
		if progress.Disposition != agentchat.TurnInProgress && progress.Disposition != agentchat.TurnNeedsContinuation { return decision,nil }
		message = "TaskDeck owner-authorized continuation of the ORIGINAL unfinished conversation, not a new task. Verify the next feasible action and do not claim success without independent evidence."
	}
	if progress.NextAction != "" {
		message += fmt.Sprintf(" Model-proposed next action (UNVERIFIED): %q. Follow it only when consistent with the original task.",progress.NextAction)
	}
	if progress.NoProgressTurns >= 2 {
		message += " No verified new work in the last two automatic turns. Prioritize a narrow native read-only Collection or state the precise blocker."
	}
	message += " Never bypass native Patch approval or repeat an ambiguous request."
	assistant := agentchat.HistoryItem{Type:"assistant",Content:text,OriginRunID:run.Spec.RunID,ProviderID:run.Spec.ProviderID,ModelID:run.Spec.ModelID,ParentTurnID:run.Spec.ParentTurnID}
	next := agentchat.HistoryItem{Type:"user",Content:message,OriginRunID:run.Spec.RunID,ProviderID:run.Spec.ProviderID,ModelID:run.Spec.ModelID,ParentTurnID:run.Spec.ParentTurnID}
	history := append(append([]agentchat.HistoryItem(nil),run.History...),assistant,next)
	policy := *inputs.Snapshot.Policy
	policy.Automation.ContinuationMode = mode
	snapshots,err := s.agentContextSnapshotStore(e.owner)
	if err != nil { return decision,err }
	snapshot,err := snapshots.CreateWithPolicy(run.Spec.OwnerWorkspaceID,run.Spec.ConversationID,inputs.ProjectID,history,inputs.Snapshot.SourceRefs,&policy)
	if err != nil {
		progress.Reason = "automatic_continuation_snapshot_unavailable"
		return decision,nil
	}
	decision.Next = []agentchat.HistoryItem{assistant,next}
	decision.SnapshotID = snapshot.ID
	progress.Reason = "run_continuation_prepared"
	return decision,nil
}

func errorsRunContinuationScope() error {
	return fmt.Errorf("run continuation conversation scope changed")
}
