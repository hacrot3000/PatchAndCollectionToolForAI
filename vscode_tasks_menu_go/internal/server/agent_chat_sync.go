package server

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/session"
)

// Explicit POST reconciliation is intentional. It reads authoritative native
// protocol evidence and atomically updates the local durable tool ledger.
// GET session remains side-effect-free.
func (s *Server) agentChatSyncAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	owner := aiPlanOwner(r)
	release, ok := lockAgentTurn(s.Workspace, owner, id)
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
	x, err := store.Get(id, owner)
	if errors.Is(err, agentchat.ErrNotFound) {
		http.Error(w, "Agent session not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	changed := false
	for i, record := range x.ToolCalls {
		if (record.Status != "running" && record.Status != "unknown" && record.Status != "dispatching") || record.PatchSessionID == "" {
			continue
		}
		view := agentToolReconcileView{Record: record}
		s.agentReconcilePatchRecord(&view)
		if view.Changed {
			x.ToolCalls[i] = view.Record
			if view.Record.Status=="done" || view.Record.Status=="error" {x.Status="waiting_tool"}
			x.AppendEvent("tool_final", view.Record.Status, record.CallID, "")
			changed = true
		}
	}
	if changed {
		x, err = store.Save(x, x.Revision)
		if err != nil {
			http.Error(w, "agent journal revision changed during sync", 409)
			return
		}
		for _, record := range x.ToolCalls {
			if record.Status == "done" || record.Status == "error" || record.Status == "unknown" {
				s.recordAgentDebugEvent(owner, id, record.CallID, "tool", "tool.final", record.Status, "Native Patch final evidence reconciled: "+record.Status)
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"session_id": id, "status": x.Status, "revision": x.Revision,
		"tool_calls": x.ToolCalls, "execution_enabled": false,
	})
}

// agentReconcilePendingOnStartup is a best-effort, read-only reconciliation
// pass. It never queues, starts or selects a patch. It only converts a
// running record with unambiguous final native evidence to done/error; every
// unknown or ambiguous case is left untouched for explicit manual recovery.
// Runs are bounded so a large journal cannot stall daemon start.
func (s *Server) agentReconcilePendingOnStartup(ctx context.Context) {
	if s == nil || s.Sessions == nil || s.Workspace == "" {
		return
	}
	store, err := s.agentChatStore()
	if err != nil {
		return
	}
	// Agent sessions are owner-scoped; the startup pass covers the single-user
	// local owner only. Shared-server owners reconcile on demand via /sync.
	sessions, err := store.List("local")
	if err != nil {
		return
	}
	// A project with long-running work may have many more than 64 sessions.
	// Startup reconciliation is read-only native evidence inspection: do not
	// silently skip older sessions merely because they are beyond a fixed index.
	for _, sess := range sessions {
		if ctx.Err() != nil { return }
		changed := false
		// Provider turns are process-owned, so a requesting_ai journal loaded
		// at daemon startup has no live provider worker. Do not leave it stuck
		// forever and do not blindly replay an ambiguous provider request.
		if sess.Status=="requesting_ai" {
			sess.Status="failed"
			sess.AppendEvent("turn_interrupted","failed","","provider_egress_after_restart_unknown")
			for j:=range sess.ToolCalls{
				if sess.ToolCalls[j].FeedbackStatus=="sending"{
					sess.ToolCalls[j].FeedbackStatus="unknown"
				}
			}
			changed=true
		}
		for j, record := range sess.ToolCalls {
			if record.Status != "running" || record.PatchSessionID == "" {
				continue
			}
			// Persisting after disconnect must not extend an expired approval
			// or bypass owner/project checks; reconciliation only reads state.
			view := agentToolReconcileView{Record: record}
			s.agentReconcilePatchRecord(&view)
			if view.Changed {
				sess.ToolCalls[j] = view.Record
				if view.Record.Status=="done" || view.Record.Status=="error" {sess.Status="waiting_tool"}
				changed = true
			}
		}
		if !changed {
			continue
		}
		if _, err := store.Save(sess, sess.Revision); err != nil {
			// A concurrent writer owns the newer revision; leave it to the
			// authoritative /sync path rather than overwriting.
			continue
		}
	}
}

// agentChatExecutionAPI exposes read-only, owner-scoped execution evidence for
// a tool call: native per-item status, bounded command summary and opaque
// artifact references. It never returns raw log paths, argv or file contents,
// and never executes or mutates anything.
func (s *Server) agentChatExecutionAPI(w http.ResponseWriter, r *http.Request, id, callID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	owner := aiPlanOwner(r)
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	x, err := store.Get(id, owner)
	if errors.Is(err, agentchat.ErrNotFound) {
		http.Error(w, "Agent session not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	if x.Workspace != s.Workspace {
		http.Error(w, "workspace mismatch", 403)
		return
	}
	var record *agentchat.ToolCallRecord
	for i := range x.ToolCalls {
		if x.ToolCalls[i].CallID == callID {
			record = &x.ToolCalls[i]
			break
		}
	}
	if record == nil {
		http.Error(w, "tool call not found", 404)
		return
	}
	view := map[string]any{
		"session_id": id, "call_id": record.CallID, "name": record.Name,
		"status": record.Status, "feedback_status": record.FeedbackStatus,
		"patch_session_id": record.PatchSessionID, "run_id": record.RunID,
		"queue_path": record.QueuePath, "execution_started": false,
	}
	if (record.Status == "done" || record.Status == "error" || record.Status == "unknown") && record.Result != "" {
		// Durable, already-bounded and sanitized result. This stays readable
		// even after the native Patch PTY has exited or been removed.
		view["verified_result"] = scrubAIContext(record.Result, 4000)
	}
	// Reuse the authoritative reconciliation read to describe item evidence.
	if s.Sessions != nil && record.PatchSessionID != "" && record.Status != "unknown" {
		if provider, ok := s.Sessions.(session.ProtocolStateProvider); ok {
			if state, stErr := provider.ProtocolState(record.PatchSessionID); stErr == nil {
				for _, item := range state.Items {
					if item.Name != filepath.Base(record.QueuePath) || strings.ToUpper(item.Kind) != "PATCH" {
						continue
					}
					if record.SelectedIndex > 0 && item.Index != record.SelectedIndex {
						continue
					}
					view["native_status"] = strings.ToUpper(item.Status)
					view["native_run_id"] = item.RunID
					view["commands"] = agentCommandSummary(item.Commands)
					view["stdout_tail"] = scrubAIContext(item.OutputTail, 2000)
					break
				}
				view["artifacts"] = agentArtifactRefs(state.Artifacts, record)
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"execution": view, "execution_enabled": false})
}

func agentCommandSummary(commands []session.ProtocolCommandEvidenceState) []map[string]any {
	out := make([]map[string]any, 0, len(commands))
	for _, c := range commands {
		row := map[string]any{"index": c.Index, "stage": c.Stage, "status": c.Status, "timed_out": c.TimedOut}
		if c.RC != nil {
			row["rc"] = *c.RC
		}
		out = append(out, row)
		if len(out) >= 24 {
			break
		}
	}
	return out
}

// agentArtifactRefs exposes only identity/timing metadata, not local paths.
func agentArtifactRefs(artifacts []session.ProtocolArtifactState, record *agentchat.ToolCallRecord) []map[string]any {
	out := make([]map[string]any, 0, 4)
	for _, a := range artifacts {
		if record.RunID != "" && a.RunID != "" && a.RunID != record.RunID {
			continue
		}
		if a.ItemName != "" && a.ItemName != filepath.Base(record.QueuePath) {
			continue
		}
		out = append(out, map[string]any{
			"kind": a.ArtifactKind, "run_id": a.RunID, "item_name": a.ItemName,
			"primary": a.Primary,
		})
		if len(out) >= 8 {
			break
		}
	}
	return out
}

// agentChatEventsAPI returns the durable transition journal after a client
// cursor so a reconnected browser can resync without replaying whole
// transcripts. It is read-only and contains no prompt/result content.
func (s *Server) agentChatEventsAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	owner := aiPlanOwner(r)
	since := uint64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			http.Error(w, "invalid since cursor", 400)
			return
		}
		since = parsed
	}
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	x, err := store.Get(id, owner)
	if errors.Is(err, agentchat.ErrNotFound) {
		http.Error(w, "Agent session not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	if x.Workspace != s.Workspace {
		http.Error(w, "workspace mismatch", 403)
		return
	}
	events := make([]agentchat.SessionEvent, 0, len(x.Events))
	for _, ev := range x.Events {
		if ev.Seq > since {
			events = append(events, ev)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"session_id": id, "status": x.Status, "revision": x.Revision,
		"event_seq": x.EventSeq, "events": events, "cursor": x.EventSeq,
		"execution_enabled": false,
	})
}
