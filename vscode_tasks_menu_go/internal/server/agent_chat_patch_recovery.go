package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
	"bletonfc/vscode_tasks_menu/internal/session"
)

// Patch recovery is deliberately NOT part of provider retry. "send_existing"
// never requeues/executes anything; "rerun_patch" requires a conclusively
// failed earlier attempt and leaves a fresh, manual approval pending.
func (s *Server) agentChatPatchRecoveryAPI(w http.ResponseWriter, r *http.Request, sessionID, callID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		Action           string `json:"action"`
		ExpectedRevision uint64 `json:"expected_revision"`
		ConfirmExternal  bool   `json:"confirm_external"`
		ConfirmExecution bool   `json:"confirm_execution"`
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil {
		http.Error(w, "invalid Patch recovery request", 400)
		return
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one request object required", 400)
		return
	}
	if req.Action != "send_existing" && req.Action != "rerun_patch" && req.Action != "rerun_and_send" {
		http.Error(w, "invalid Patch recovery action", 400)
		return
	}
	if (req.Action == "send_existing" || req.Action == "rerun_and_send") && !req.ConfirmExternal {
		http.Error(w, "consent required to send verified native Patch result to AI", 403)
		return
	}
	if req.Action == "rerun_patch" || req.Action == "rerun_and_send" {
		if !req.ConfirmExecution {
			http.Error(w, "explicit approval to prepare another Patch run required", 403)
			return
		}
		if !s.requireSharedActionPermission(w, r, identity.PermissionPatchRun, "agent.retry_patch", sessionID) {
			return
		}
	}
	if req.Action == "rerun_and_send" && s.Config.SharedServerEnabled {
		http.Error(w, "one-click native Patch reapply is only available in local Agent sessions", 409)
		return
	}
	owner := aiPlanOwner(r)
	release, ok := lockAgentTurn(s.agentOwnerWorkspace(), owner, sessionID)
	if !ok {
		http.Error(w, "Agent conversation is busy", 409)
		return
	}
	defer release()
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "session journal unavailable", 500)
		return
	}
	x, err := store.Get(sessionID, owner)
	if err != nil {
		http.Error(w, "Agent session not found", 404)
		return
	}
	if x.Workspace != s.agentOwnerWorkspace() || x.Revision != req.ExpectedRevision {
		http.Error(w, "stale session or workspace; refresh before recovery", 409)
		return
	}
	identityOfWorkspace, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil || identityOfWorkspace.ID != x.ProjectID {
		http.Error(w, "project identity mismatch", 409)
		return
	}
	idx := -1
	for i, call := range x.ToolCalls {
		if call.CallID == callID {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.Error(w, "Patch call not found", 404)
		return
	}
	call := x.ToolCalls[idx]
	if call.Name != "taskdeck_submit_patch" {
		http.Error(w, "cannot recover a non-Patch tool", 409)
		return
	}
	if call.Status == "running" || call.Status == "dispatching" || call.Status == "unknown" {
		view := agentToolReconcileView{Record: call}
		s.agentReconcilePatchRecord(&view)
		// Legacy native terminals may have multiple failed History records for
		// the same immutable package. Only a user-initiated recovery may choose
		// the latest matching timestamped record; background sync stays strict.
		if view.Record.Status != "done" && view.Record.Status != "error" && !s.agentNativePatchActive(call) {
			st, res, run, ok := s.agentLatestResultFromHistory(call)
			if ok {
				view.Record.Status = st
				view.Record.Result = res
				view.Record.RunID = run
				view.Changed = true
			}
		}
		if view.Changed {
			x.ToolCalls[idx] = view.Record
			call = view.Record
			if call.Status == "done" || call.Status == "error" {
				x.Status = "waiting_tool"
			}
			x.AppendEvent("tool_recovered", call.Status, callID, "verified_native_evidence")
			x, err = store.Save(x, x.Revision)
			if err != nil {
				http.Error(w, "cannot persist recovered native status", 409)
				return
			}
		}
	}
	if req.Action == "send_existing" {
		if call.Status != "done" && call.Status != "error" && call.Status != "rejected" {
			http.Error(w, "No verified old Patch result. Check the linked native run or Patch History. Nothing was resent or re-executed.", 409)
			return
		}
		if call.FeedbackSent {
			http.Error(w, "Old result delivery was already attempted; provider acknowledgement may be ambiguous. Do not duplicate tool output.", 409)
			return
		}
		if x.Status != "waiting_tool" && x.Status != "tool_rejected" {
			http.Error(w, "conversation cannot deliver this result in its current state", 409)
			return
		}
		result, err := s.agentDeliverToolResult(s.agentServerContext(), store, x, idx, true)
		if err != nil {
			writeAgentContinueError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{
			"session": result.Session, "answer": result.Result.Text, "recovery": "old_verified_result_sent",
			"old_run_id": call.RunID, "patch_reexecuted": false,
		})
		return
	}
	// A native FAIL/PREFLIGHT_FAIL/INCOMPLETE is an observable completed
	// attempt. Re-execution after an ambiguous RUNNING/UNKNOWN or a PASS must be
	// refused: the workspace may have changed without a trustworthy receipt.
	if call.Status != "error" || call.FeedbackSent || call.RunID == "" ||
		len(call.PackageSHA256) != 64 || len(call.Attempts) >= 8 {
		http.Error(w, "Rerun requires a verified failed native Patch with an undelivered result; reconcile old run first", 409)
		return
	}
	var evidence struct {
		Status string `json:"status"`
	}
	if json.Unmarshal([]byte(call.Result), &evidence) != nil ||
		(evidence.Status != "FAIL" && evidence.Status != "PREFLIGHT_FAIL" && evidence.Status != "INCOMPLETE") {
		http.Error(w, "existing Patch failure lacks a verifiable final result", 409)
		return
	}
	if err := s.agentVerifyFinalPackage(call); err != nil {
		http.Error(w, "original native package/receipt cannot be verified; manual inspection required", 409)
		return
	}
	source, found := findAgentToolCall(x.History, callID)
	if !found {
		http.Error(w, "original Patch proposal unavailable", 409)
		return
	}
	args, err := parseAgentPatchArguments(source.Arguments)
	if err != nil {
		http.Error(w, "invalid original Patch proposal", 409)
		return
	}
	if _, err = pastedPythonPackage(args.PatchPython, identityOfWorkspace, patchPasteSlug(args.Name)); err != nil {
		http.Error(w, "original proposal no longer meets Patch contract", 409)
		return
	}
	previous := agentchat.ToolAttemptRecord{
		Status: call.Status, Result: call.Result, QueuePath: call.QueuePath,
		PackageSHA256: call.PackageSHA256, PackageSize: call.PackageSize,
		PatchSessionID: call.PatchSessionID, NativeStartedAt: call.NativeStartedAt, PromptID: call.PromptID,
		SelectedIndex: call.SelectedIndex, RunID: call.RunID,
	}
	call.Attempts = append(call.Attempts, previous)
	call.Status = "pending"
	call.Result = ""
	call.QueuePath = ""
	call.PackageSHA256 = ""
	call.PackageSize = 0
	call.PatchSessionID = ""
	call.NativeStartedAt = nil
	call.AutoRunOnce = req.Action == "rerun_and_send"
	call.SendAfterRetry = req.Action == "rerun_and_send"
	call.PromptID = ""
	call.SelectedIndex = 0
	call.RunID = ""
	call.ExpectedHead = ""
	call.ExpectedBranch = ""
	call.WorkspaceStatusSHA256 = ""
	call.GitIsRepo = false
	call.ApprovalExpiresAt = nil
	call.ManualRetryOnly = req.Action != "rerun_and_send"
	x.ToolCalls[idx] = call
	x.Status = "waiting_tool"
	if req.Action == "rerun_and_send" {
		x.AppendEvent("patch_retry_ready", "waiting_tool", callID, "single_explicit_reapply_and_send")
	} else {
		x.AppendEvent("patch_retry_ready", "waiting_tool", callID, "manual_approval_required")
	}
	x, err = store.Save(x, x.Revision)
	if err != nil {
		http.Error(w, "cannot persist fresh manual Patch approval", 409)
		return
	}
	s.recordAgentDebugEvent(owner, sessionID, callID, "tool", "patch.retry_ready", "", "Old failed run archived; manual review and approval required before any new native execution")
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"session": x, "recovery": map[bool]string{true: "reapply_and_send_accepted", false: "fresh_review_required"}[req.Action == "rerun_and_send"], "old_run_id": previous.RunID,
		"patch_reexecuted": false, "note": map[bool]string{true: "Re-apply approved once. Native Patch will run and its final result will be sent to AI. Old failure preserved.", false: "Review and approve the pending Patch; its old failed run is preserved."}[req.Action == "rerun_and_send"],
		"expires_at": time.Now().UTC().Add(agentApprovalTTL),
	})
}

// agentNativePatchActive is a read-only guard: an old History result must
// never replace an active native run when its final outcome is still unknown.
func (s *Server) agentNativePatchActive(call agentchat.ToolCallRecord) bool {
	if s.Sessions == nil || call.PatchSessionID == "" {
		return false
	}
	if _, exists := s.Sessions.Metadata(call.PatchSessionID); !exists {
		return false
	}
	provider, ok := s.Sessions.(session.ProtocolStateProvider)
	if !ok {
		return true
	}
	state, err := provider.ProtocolState(call.PatchSessionID)
	if err != nil {
		return true
	}
	if strings.Contains(string(state.LastEvent), `"run_finished"`) {
		return false
	}
	for _, item := range state.Items {
		if item.Name != filepath.Base(call.QueuePath) || !strings.EqualFold(item.Kind, "PATCH") {
			continue
		}
		switch strings.ToUpper(item.Status) {
		case "RUNNING", "QUEUED", "PENDING":
			return true
		case "PASS", "FAIL", "PREFLIGHT_FAIL", "INCOMPLETE":
			return false
		}
	}
	return state.CommandsEnabled
}
