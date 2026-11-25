package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
	"unicode/utf8"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
	"bletonfc/vscode_tasks_menu/internal/session"
)

// agentApprovalTTL bounds how long a one-time human approval may be consumed.
// A stale approval must be re-reviewed, never applied implicitly later.
const agentApprovalTTL = 10 * time.Minute

type agentPatchArguments struct {
	Name        string `json:"name"`
	PatchPython string `json:"patch_python"`
	PatchManifest string `json:"patch_manifest,omitempty"`
}

func parseAgentPatchArguments(raw string) (agentPatchArguments, error) {
	if len(raw) == 0 || len(raw) > 512<<10 || !utf8.ValidString(raw) {
		return agentPatchArguments{}, errors.New("invalid or oversized patch tool arguments")
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var args agentPatchArguments
	if err := dec.Decode(&args); err != nil {
		return agentPatchArguments{}, errors.New("invalid patch function arguments")
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return agentPatchArguments{}, errors.New("patch function must contain exactly one JSON object")
	}
	args.Name = strings.TrimSpace(args.Name)
	if args.Name == "" || len(args.Name) > 80 {
		return agentPatchArguments{}, errors.New("invalid patch name")
	}
	if (strings.TrimSpace(args.PatchPython) == "") == (strings.TrimSpace(args.PatchManifest) == "") {
		return agentPatchArguments{}, errors.New("provide exactly one nonempty patch_python or patch_manifest")
	}
	if len(args.PatchPython) > 480<<10 || strings.ContainsRune(args.PatchPython, 0) || len(args.PatchManifest)>64<<10 || strings.ContainsRune(args.PatchManifest,0) {
		return agentPatchArguments{}, errors.New("invalid or oversized patch body")
	}
	if strings.TrimSpace(args.PatchManifest)!="" {
		if _,err:=parseCommandOnlyManifest(args.PatchManifest);err!=nil{return agentPatchArguments{},err}
	}
	return args, nil
}

func findAgentToolCall(history []agentchat.HistoryItem, callID string) (agentchat.HistoryItem, bool) {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Type == "function_call" && history[i].CallID == callID && history[i].Name == "taskdeck_submit_patch" {
			return history[i], true
		}
	}
	return agentchat.HistoryItem{}, false
}

// agentQueueViaNativePaste calls the pre-existing validated Patch Paste queue
// entrypoint in-process. It DOES NOT execute Python or bypass native Patch
// schema / project-key checks. The caller must separately authorize patch.run.
func (s *Server) agentQueueViaNativePaste(r *http.Request, args agentPatchArguments) (patchUploadResult, error) {
	kind,body:=args.pastePayload()
	input, err := json.Marshal(patchPasteRequest{Kind: kind, Name: args.Name, Text: body})
	if err != nil {
		return patchUploadResult{}, err
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/api/patch/paste", bytes.NewReader(input))
	if err != nil {
		return patchUploadResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	// Preserve verified auth/owner and browser origin context for native lease
	// and mutation audit. The handler is reused, never reimplemented.
	req = req.WithContext(r.Context())
	rec := httptest.NewRecorder()
	s.patchPaste(rec, req)
	if rec.Code != http.StatusCreated {
		return patchUploadResult{}, fmt.Errorf("native Patch queue refused package: HTTP %d (%s)", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	var upload patchUploadResult
	if err := json.Unmarshal(rec.Body.Bytes(), &upload); err != nil {
		return patchUploadResult{}, err
	}
	if upload.Name == "" || upload.QueuePath == "" || len(upload.SHA256) != 64 {
		return patchUploadResult{}, errors.New("native Patch queue returned incomplete receipt")
	}
	return upload, nil
}

func (s *Server) agentChatToolDecisionAPI(w http.ResponseWriter, r *http.Request, sessionID, callID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if len(callID) == 0 || len(callID) > 256 || strings.Contains(callID, "/") {
		http.Error(w, "invalid tool call id", 400)
		return
	}
	if !isInternalAgentRunRequest(r) {
		owner := aiPlanOwner(r)
		run, found, err := s.agentRunForToolCall(owner, sessionID, callID)
		if err != nil {
			http.Error(w, "Agent Run tool binding is ambiguous", 409)
			return
		}
		if found {
			s.agentRunToolDecisionAPI(w, r, owner, run.Spec.RunID, callID)
			return
		}
	}
	if !s.requireSharedActionPermission(w, r, identity.PermissionPatchRun, "agent.patch_decision", sessionID) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var request struct {
		Action           string `json:"action"`
		ExpectedRevision uint64 `json:"expected_revision"`
		ConfirmExecution bool   `json:"confirm_execution"`
		Automatic        bool   `json:"automatic,omitempty"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&request) != nil {
		http.Error(w, "invalid agent tool decision", 400)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one tool decision required", 400)
		return
	}
	if request.Action != "approve" && request.Action != "reject" {
		http.Error(w, "action must be approve or reject", 400)
		return
	}
	if request.Action == "approve" && !request.ConfirmExecution {
		http.Error(w, "explicit patch execution approval is required", 403)
		return
	}
	owner := aiPlanOwner(r)
	release, ok := lockAgentTurn(s.agentOwnerWorkspace(), owner, sessionID)
	if !ok {
		http.Error(w, "Agent session is busy", 409)
		return
	}
	defer release()
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	x, err := store.Get(sessionID, owner)
	if errors.Is(err, agentchat.ErrNotFound) {
		http.Error(w, "Agent session not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	// A background auto-approval is authorized only by an owner-controlled
	// session flag and only for a local (non-shared) workspace. A model cannot
	// set this flag by proposing a Patch.
	if request.Automatic && (x.Owner != "local" || s.Config.SharedServerEnabled || !agentAutoApprovalAuthorized(x, callID)) {
		http.Error(w, "automatic Patch approval is not authorized for this session", 403)
		return
	}
	if x.Workspace != s.agentOwnerWorkspace() || x.Status != "waiting_tool" {
		http.Error(w, "Agent session no longer awaits Patch approval; refresh the conversation to view the native status", 409)
		return
	}
	// Do not reject an explicit decision solely because background status
	// polling or an Automation toggle bumped the journal revision. The exact
	// call ID, immutable argument hash, project identity, original workspace
	// snapshot and pending state are verified below under this turn lock.
	// Automatic approvals remain strict so the scheduler cannot act on stale
	// preferences or queue a second package.
	if request.Automatic && x.Revision != request.ExpectedRevision {
		http.Error(w, "automatic approval revision changed; retry on the next scheduler pass", 409)
		return
	}
	if request.Automatic && !isInternalAgentRunRequest(r) {
		currentID, eligible := agentAutoPatchCandidate(x)
		if !eligible || currentID != callID || !s.agentLegacyPatchSchedulerAllowed(x, callID) {
			http.Error(w, "automatic Patch ownership or eligibility changed", 409)
			return
		}
	}
	identity, err := patchtool.EnsureProjectIdentity(s.Workspace)
	if err != nil || identity.ID != x.ProjectID {
		http.Error(w, "project identity changed", 409)
		return
	}
	idx := -1
	for i, item := range x.ToolCalls {
		if item.CallID == callID {
			idx = i
			break
		}
	}
	if idx < 0 || x.ToolCalls[idx].Status != "pending" {
		http.Error(w, "pending approved tool call not found", 409)
		return
	}
	call, found := findAgentToolCall(x.History, callID)
	if !found {
		http.Error(w, "original model call missing", 409)
		return
	}
	// The exact pending, never-dispatched legacy proposal can be refused
	// durably before native side effects. The existing feedback scheduler
	// sends this structured denial under the user's existing consent.
	denyInvalidAutomatic := func() bool {
		if !request.Automatic || x.ToolCalls[idx].AgentRunID != "" ||
			!agentPatchHasNoNativeAttempt(x.ToolCalls[idx]) {
			return false
		}
		x.ToolCalls[idx].Status = "rejected"
		x.ToolCalls[idx].Result = agentRejectedPatchFeedback("INVALID_PATCH_ARGUMENTS",
			"Native Patch contract validation refused the proposal before queueing. No Patch was queued or executed. Correct the project-bound arguments and produce a NEW proposal with a new call ID.", true)
		x.ToolCalls[idx].AutoRunOnce = false
		x.ToolCalls[idx].SendAfterRetry = false
		x.Status = "tool_rejected"
		x.AppendEvent("tool_rejected", x.Status, callID, "pre_queue_contract_invalid")
		if _, saveErr := store.Save(x, x.Revision); saveErr != nil {
			http.Error(w, "cannot persist invalid Patch rejection", 409)
			return true
		}
		writeJSON(w, 200, map[string]any{"session_id":sessionID,"call_id":callID,"status":"rejected","execution_enabled":false})
		return true
	}
	arguments, err := parseAgentPatchArguments(call.Arguments)
	if err != nil {
		if denyInvalidAutomatic() { return }
		http.Error(w, "invalid original tool arguments", 400)
		return
	}
	// The exact model arguments are bound at proposal time. An approval must
	// never apply to a different payload (e.g. a caller edited the transcript).
	if digest := agentPatchPayloadSHA256(call.Arguments); x.ToolCalls[idx].ArgumentsSHA256 != "" && x.ToolCalls[idx].ArgumentsSHA256 != digest {
		http.Error(w, "tool arguments changed since the model proposed the patch", 409)
		return
	}
	if request.Action == "reject" {
		x.ToolCalls[idx].Status = "rejected"
		x.ToolCalls[idx].Result = agentRejectedPatchFeedback(
            "USER_REJECTED",
            "The user rejected this Patch. No Patch was queued or executed. Do not resubmit the rejected Patch without a new user instruction.",
            false)
		x.AppendEvent("tool_rejected", "tool_rejected", callID, "")
		x.Status = "tool_rejected"
		x, err = store.Save(x, x.Revision)
		if err != nil {
			http.Error(w, "cannot persist tool rejection", 409)
			return
		}
		s.recordAgentDebugEvent(owner, sessionID, callID, "tool", "tool.rejected", "", "User rejected the proposed patch; nothing was queued")
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"session_id": sessionID, "status": "rejected", "revision": x.Revision, "execution_enabled": false})
		return
	}
	// Native validation checks BOTH identity headers against this workspace.
	if _, err = agentPatchPackage(arguments, identity); err != nil {
		if denyInvalidAutomatic() { return }
		http.Error(w, "Patch Tool contract validation failed: "+err.Error(), 400)
		return
	}
	// Reserve the canonical workspace for the whole queue/launch/select sequence
	// so two Agent approvals cannot mutate the same worktree concurrently.
	lease, err := agentReserveWorkspaceExecution(r.Context(), s.Workspace)
	if err != nil {
		http.Error(w, "another Agent execution currently owns this workspace", 409)
		return
	}
	defer lease()
	// The proposal normally captured Git evidence when the model returned the
	// tool call. Legacy/pending records without a snapshot are captured here.
	// An already expired approval is refused; a fresh one gets a bounded TTL.
	if x.ToolCalls[idx].ApprovalExpiresAt != nil && time.Now().UTC().After(*x.ToolCalls[idx].ApprovalExpiresAt) {
		// A definite pre-queue refusal must not leave Auto Patch in pending
		// forever. Preserve the expired approval and return a real denial.
		if request.Automatic && x.ToolCalls[idx].AgentRunID == "" && agentPatchHasNoNativeAttempt(x.ToolCalls[idx]) {
			agentRecordStalePatchDenial(&x, idx)
			if _, err = store.Save(x, x.Revision); err != nil {
				http.Error(w, "cannot persist expired Patch rejection", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"session_id":sessionID,"call_id":callID,"status":"rejected","execution_enabled":false,"note":"Approval expired before queueing; request a new Patch proposal."})
			return
		}
		http.Error(w, "approval has expired; request a new proposal", 409)
		return
	}
	if x.ToolCalls[idx].ExpectedHead == "" && !x.ToolCalls[idx].GitIsRepo {
		snapshot, err := agentWorkspaceSnapshotOf(r.Context(), s.Workspace)
		if err != nil {
			http.Error(w, "workspace drift guard unavailable: "+err.Error(), 409)
			return
		}
		x.ToolCalls[idx].ExpectedHead = snapshot.Head
		x.ToolCalls[idx].ExpectedBranch = snapshot.Branch
		x.ToolCalls[idx].WorkspaceStatusSHA256 = snapshot.StatusSHA256
		x.ToolCalls[idx].GitIsRepo = snapshot.IsRepo
	}
	if x.ToolCalls[idx].ApprovalExpiresAt == nil {
		expires := time.Now().UTC().Add(agentApprovalTTL)
		x.ToolCalls[idx].ApprovalExpiresAt = &expires
	}
	// A stale approval before the durable QUEUING marker is a definite
	// refusal, not an ambiguous execution. Never reuse the old approval.
	if err := s.agentVerifyApprovalStillValid(r.Context(), &x.ToolCalls[idx]); err != nil {
		if agentPatchHasNoNativeAttempt(x.ToolCalls[idx]) {
			agentRecordStalePatchDenial(&x, idx)
			if _, err = store.Save(x, x.Revision); err != nil {
				http.Error(w, "cannot persist stale Patch rejection", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"session_id": sessionID, "call_id": callID, "status": "rejected", "execution_enabled": false, "note": "Workspace changed before queueing; request a new Patch proposal."})
			return
		}
		x.ToolCalls[idx].Status = "unknown"
		x.ToolCalls[idx].Result = "Workspace changed since review; no patch was queued. Request a new proposal."
		x.Status = "tool_unknown"
		_, _ = store.Save(x, x.Revision)
		http.Error(w, "workspace changed since review; approval is stale", 409)
		return
	}
	// Commit a non-retryable QUEUING marker BEFORE the file-side effect.
	// A crash after this point is explicitly UNKNOWN; no duplicate package or
	// execution is permitted through the same call_id.
	x.ToolCalls[idx].Status = "queuing"
	x.Status = "tool_queuing"
	x.AppendEvent("tool_queuing", "tool_queuing", callID, "")
	x, err = store.Save(x, x.Revision)
	if err != nil {
		http.Error(w, "cannot record tool idempotency marker", 409)
		return
	}
	if request.Automatic {
		x.AppendEvent("tool_auto_approved", "tool_queuing", callID, "session_setting")
	}
	s.recordAgentDebugEvent(owner, sessionID, callID, "tool", "tool.queuing", "", "Approval recorded before native queue handoff")
	// Re-check immediately before creating a queue package. If the worktree
	// moved between the approval request and this write, stop and require a
	// fresh review instead of applying the patch to an unreviewed state.
	if err := s.agentVerifyApprovalStillValid(r.Context(), &x.ToolCalls[idx]); err != nil {
		// We still own the turn lease and have not called native paste.
		// The QUEUING marker alone does not make this live refusal ambiguous.
		if agentPatchHasNoNativeAttempt(x.ToolCalls[idx]) && x.ToolCalls[idx].AgentRunID == "" {
			agentRecordStalePatchDenial(&x, idx)
			if _, err = store.Save(x, x.Revision); err != nil {
				http.Error(w, "cannot persist pre-handoff Patch rejection", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"session_id":sessionID,"call_id":callID,"status":"rejected","execution_enabled":false,"note":"Workspace changed before native paste; request a new proposal."})
			return
		}
		x.ToolCalls[idx].Status = "unknown"
		x.ToolCalls[idx].Result = "Workspace changed before queue handoff; no patch was queued. Request a new proposal."
		x.Status = "tool_unknown"
		_, _ = store.Save(x, x.Revision)
		http.Error(w, "workspace changed before queue handoff; approval is stale", 409)
		return
	}
	receipt, err := s.agentQueueViaNativePaste(r, arguments)
	if err != nil {
		x.ToolCalls[idx].Status = "unknown"
		x.ToolCalls[idx].Result = "Patch queue handoff failed/ambiguous; inspect Patch Tool queue before retry."
		x.Status = "tool_unknown"
		_, _ = store.Save(x, x.Revision)
		http.Error(w, "native Patch queue handoff failed; call is marked UNKNOWN and will not be retried automatically", 502)
		return
	}
	// The hash/filename receipt is verified against the native queue result.
	if len(receipt.SHA256) != 64 {
		x.ToolCalls[idx].Status = "unknown"
		x.ToolCalls[idx].Result = "Invalid native Patch queue receipt; queue outcome unconfirmed. Inspect Patch Tool before recovery."
		x.Status = "tool_unknown"
		x.AppendEvent("tool_queue_interrupted", x.Status, callID, "invalid_queue_receipt")
		if _, err = store.Save(x, x.Revision); err != nil {
			http.Error(w, "invalid native Patch receipt and journal update failed; inspect queue, do not retry", 409)
			return
		}
		http.Error(w, "invalid native Patch queue receipt; marked UNKNOWN, do not retry automatically", 502)
		return
	}
	x.ToolCalls[idx].Status = "queued"
	x.AppendEvent("tool_queued", "waiting_tool", callID, "")
	x.ToolCalls[idx].QueuePath = receipt.QueuePath
	x.ToolCalls[idx].PackageSHA256 = receipt.SHA256
	x.ToolCalls[idx].PackageSize = receipt.Size
	x.Status = "waiting_tool"
	x, err = store.Save(x, x.Revision)
	if err != nil {
		http.Error(w, "Patch was queued but journal update failed; inspect queue, do not retry", 409)
		return
	}
	s.recordAgentDebugEvent(owner, sessionID, callID, "tool", "tool.queued", "", "Native Patch queue accepted the approved package")
	// A live daemon launches the EXISTING native Patch Tool session and
	// selects ONLY this approved package. A non-running test fixture can still
	// verify the queue stage independently without a session service.
	persistStatus := func() bool {
		saved, saveErr := store.Save(x, x.Revision)
		if saveErr != nil {
			http.Error(w, "Native Patch state became uncertain; inspect the existing Patch Tool session, do not resubmit", 409)
			return false
		}
		x = saved
		return true
	}
	status := "queued"
	note := "Native Patch Tool queue received the package; execution has not started."
	if s.Sessions != nil {
		if err := s.agentVerifyQueuedPackage(receipt); err != nil {
			x.ToolCalls[idx].Status = "unknown"
			x.ToolCalls[idx].Result = "Queued package changed before native dispatch. No execution started."
			x.Status = "tool_unknown"
			if !persistStatus() {
				return
			}
			status = "unknown"
			note = "Queued package failed digest verification; inspect queue before retry."
		} else {
			patchID, startErr := s.agentStartNativePatch(r)
			if startErr != nil {
				note = "Native Patch session could not start; package remains in queue for manual review."
			} else if err := s.agentVerifyApprovalAfterOwnQueue(r.Context(), &x.ToolCalls[idx], receipt.QueuePath); err != nil {
				x.ToolCalls[idx].Status = "unknown"
				x.ToolCalls[idx].Result = "Workspace changed after the native session started; selection refused. Inspect the existing Patch session."
				x.ToolCalls[idx].PatchSessionID = patchID
				x.Status = "tool_unknown"
				if !persistStatus() {
					return
				}
				status = "unknown"
				note = "Workspace drift detected before native selection; no patch was executed."
			} else {
				x.ToolCalls[idx].Status = "dispatching"
				x.ToolCalls[idx].PatchSessionID = patchID
				started := time.Now().UTC()
				x.ToolCalls[idx].NativeStartedAt = &started
				if !persistStatus() {
					return
				}
				if err := s.agentVerifyQueuedPackage(receipt); err != nil {
					x.ToolCalls[idx].Status = "unknown"
					x.ToolCalls[idx].Result = "Approved package changed after native session started; selection refused."
					x.Status = "tool_unknown"
					if !persistStatus() {
						return
					}
					status = "unknown"
					note = "Approved patch package changed before selection; inspect existing native session."
				} else if promptID, index, err := s.agentSelectNativePatch(r.Context(), r, patchID, receipt.Name); err != nil {
					// A batch resource preflight can fail before the native
					// queue selection prompt. Recover an exact, verified
					// PREFLIGHT_FAIL from this session without converting it
					// into an ambiguous execution. Feed the failure to AI.
					proven := false
					if provider, ok := s.Sessions.(session.ProtocolStateProvider); ok {
						if native, e := provider.ProtocolState(patchID); e == nil {
							if finalStatus, evidence, runID, valid := s.agentVerifiedPreflight(x.ToolCalls[idx], native); valid {
								x.ToolCalls[idx].Status = finalStatus
								x.ToolCalls[idx].Result = evidence
								x.ToolCalls[idx].RunID = runID
								x.Status = "waiting_tool"
								x.AppendEvent("tool_final", "waiting_tool", callID, "preflight_failed_before_selection")
								status = finalStatus
								note = "Native Patch preflight failed before source changes; verified failure is ready for AI feedback."
								proven = true
							}
						}
					}
					if !proven {
						x.ToolCalls[idx].Status = "unknown"
						x.ToolCalls[idx].Result = "Native Patch selection is ambiguous; inspect linked Patch session; no automatic retry."
						x.Status = "tool_unknown"
						status = "unknown"
						note = "Native Patch selection was not confirmed; inspect existing session, do not requeue automatically."
					}
					if !persistStatus() {
						return
					}
				} else {
					x.ToolCalls[idx].Status = "running"
					x.AppendEvent("tool_running", "waiting_tool", callID, "")
					x.ToolCalls[idx].PromptID = promptID
					x.ToolCalls[idx].SelectedIndex = index
					x.Status = "waiting_tool"
					if !persistStatus() {
						return
					}
					status = "running"
					note = "Native Patch Tool accepted an exact-name selection; await validated per-item result."
				}
			}
		}
	}
	s.recordAgentDebugEvent(owner, sessionID, callID, "tool", "tool."+status, "", "Native Patch lifecycle is "+status)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"session_id": sessionID, "call_id": callID, "status": status,
		"queue_path": receipt.QueuePath, "sha256": receipt.SHA256,
		"patch_session_id": x.ToolCalls[idx].PatchSessionID,
		"revision":         x.Revision, "note": note,
		"executed": false, "execution_enabled": status == "running",
	})
}

// Stable digest used by tests and future ledger reconciliation; never expose
// the raw patch body in a notification or log.
func agentPatchPayloadSHA256(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}
