package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Opt-in automatic continuation. Consent is recorded per session (not implied
// by a provider key) and is consumed exactly once per completed tool call.
// The scheduler never queues, starts, selects or retries a patch; it only
// delivers a verified native result as function_call_output with the original
// call_id, and only while its bounded grant is valid.
const (
	agentAutoFeedbackMaxAge = 30 * time.Minute
	agentAutoFeedbackPoll   = 3 * time.Second
)

// agentAutoFeedbackCandidate reports whether a session currently qualifies.
// It is a pure read used by both the scheduler and tests.
func agentAutoFeedbackCandidate(x agentchat.Session) (int, bool) {
	if x.Archived {
		return 0, false
	}
	granted:=x.AutoFeedback && (x.AutoFeedbackPersistent ||
  (x.AutoFeedbackUntil!=nil && !time.Now().UTC().After(*x.AutoFeedbackUntil)))
 if !granted {
  for _,call:=range x.ToolCalls {
   if call.SendAfterRetry && !call.FeedbackSent {granted=true;break}
  }
 }
 if !granted {return 0,false}
	if x.Status != "waiting_tool" {
		return 0, false
	}
	for _, call := range x.ToolCalls {
		switch call.Status {
		case "running", "dispatching", "queued":
			return 0, false // wait until the native item is final
		}
	}
	for i, call := range x.ToolCalls {
		if call.FeedbackSent {
			continue
		}
		switch call.Status {
		case "done", "error", "rejected":
            if x.AutoFeedback || call.SendAfterRetry {return i,true}
            return 0,false
		}
		return 0, false
	}
	return 0, false
}

// runAgentAutoFeedback performs one bounded pass. Each session is processed
// under its turn lock with a fresh owner/project/provider authorization.
func (s *Server) runAgentAutoFeedback(ctx context.Context) {
	if s == nil || s.Workspace == "" {
		return
	}
	store, err := s.agentChatStore()
	if err != nil {
		return
	}
	// The scheduler only runs for the single-user local owner. Shared-server
	// owners must opt in and continue explicitly; an implicit background pass
	// must never egress on behalf of a principal it cannot re-authenticate.
	sessions, err := store.List("local")
	if err != nil {
		return
	}
	for i, sess := range sessions {
		if i >= 32 || ctx.Err() != nil {
			return
		}
		if sess.Owner != "local" {
			continue
		}
		// An explicit local per-conversation Auto Patch grant is sufficient to
		// invoke the SAME native approval gateway; there is no second executor.
		s.autoPatchOne(ctx, store, sess.ID)
		// Reconcile running native jobs in the background. GUI polling is not
		// required for final-state adoption or automatic feedback.
		s.reconcileAgentSession(ctx, store, sess.ID)
		fresh, err := store.Get(sess.ID, "local")
		if err != nil {
			continue
		}
		if _, ok := agentAutoFeedbackCandidate(fresh); !ok {
			continue
		}
		s.autoFeedbackOne(ctx, store, sess.ID)
	}
}

// Auto Patch performs a single exact-call approval under the existing native
// workspace lease, identity validation, revision fence and idempotency marker.
// No automatic retry is attempted after an ambiguous queue/dispatch result.
// A one-shot user-approved reapply is independent from persistent Auto Patch.
func agentAutoApprovalAuthorized(x agentchat.Session,callID string) bool {
 if x.AutoPatch {return true}
 for _,call:=range x.ToolCalls {
  if call.CallID==callID {return call.Status=="pending" && call.AutoRunOnce && !call.ManualRetryOnly}
 }
 return false
}

func agentAutoPatchCandidate(x agentchat.Session) (string, bool) {
	if x.Owner != "local" || x.Archived || x.Status != "waiting_tool" {
		return "",false
	}
	for _,call:=range x.ToolCalls {
		if call.Status == "pending" && !call.FeedbackSent && !call.ManualRetryOnly && (x.AutoPatch || call.AutoRunOnce) {
			return call.CallID,true
		}
		switch call.Status {
		case "queuing","queued","dispatching","running","unknown":
			return "",false
		}
	}
	return "",false
}
func (s *Server) autoPatchOne(ctx context.Context, store *agentchat.Store, id string) {
	if s.Config.SharedServerEnabled || ctx.Err()!=nil {return}
	// Do not hold the turn lock: the existing approval handler acquires it,
	// then records the QUEUING marker before the native file-side effect.
	x,err:=store.Get(id,"local")
	if err!=nil || x.Workspace!=s.Workspace {return}
	callID,ok:=agentAutoPatchCandidate(x)
	if !ok {return}
	body,err:=json.Marshal(map[string]any{
		"action":"approve","automatic":true,
		"confirm_execution":true,"expected_revision":x.Revision,
	})
	if err!=nil {return}
	req:=httptest.NewRequest(http.MethodPost,
		"/api/agent/sessions/"+id+"/tools/"+callID+"/decision",
		bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type","application/json")
	w:=httptest.NewRecorder()
	s.agentChatToolDecisionAPI(w,req,id,callID)
	if w.Code!=http.StatusOK {
        // Do not repeat a refused one-shot approval every scheduler tick.
        release,locked:=lockAgentTurn(s.Workspace,"local",id)
        if locked {
          current,e:=store.Get(id,"local")
          if e==nil {
           for i:=range current.ToolCalls {
            if current.ToolCalls[i].CallID==callID && current.ToolCalls[i].Status=="pending" && current.ToolCalls[i].AutoRunOnce {
             current.ToolCalls[i].AutoRunOnce=false
             current.ToolCalls[i].SendAfterRetry=false
             current.ToolCalls[i].ManualRetryOnly=true
             _,_=store.Save(current,current.Revision)
            }
           }
          }
          release()
        }
		s.recordAgentDebugEvent("local",id,callID,"tool","tool.auto_patch_blocked",
			"AUTO_PATCH_FAILED","Auto Patch was refused or became stale; no automatic retry")
	}
}

// reconcileAgentSession adopts final evidence directly from the native
// Session Service while holding the same per-session lock used by approval,
// sync and feedback. It never starts, selects, cancels or repeats a patch.
func (s *Server) reconcileAgentSession(ctx context.Context, store *agentchat.Store, id string) {
	release, ok := lockAgentTurn(s.Workspace, "local", id)
	if !ok {
		return
	}
	defer release()
	x, err := store.Get(id, "local")
	if err != nil || x.Owner != "local" || x.Workspace != s.Workspace {
		return
	}
	changed := false
	for i, record := range x.ToolCalls {
		if ctx.Err() != nil {
			return
		}
		if record.Status != "running" && record.Status != "unknown" && record.Status != "dispatching" {
			continue
		}
		view := agentToolReconcileView{Record: record}
		s.agentReconcilePatchRecord(&view)
		if view.Changed {
			x.ToolCalls[i] = view.Record
			if view.Record.Status=="done" || view.Record.Status=="error" {x.Status="waiting_tool"}
			x.AppendEvent("tool_final", view.Record.Status, record.CallID, "background_reconcile")
			changed = true
		}
	}
	if changed {
		_, _ = store.Save(x, x.Revision)
	}
}

func (s *Server) autoFeedbackOne(ctx context.Context, store *agentchat.Store, id string) {
	release, ok := lockAgentTurn(s.Workspace, "local", id)
	if !ok {
		return
	}
	defer release()
	x, err := store.Get(id, "local")
	if err != nil || x.Archived || x.Owner != "local" {
		return
	}
	// Re-validate the candidate under the lock: another turn may have run.
	idx, still := agentAutoFeedbackCandidate(x)
	if !still {
		return
	}
	// Consent must still be valid at the moment of egress.
	if !x.ToolCalls[idx].SendAfterRetry && (!x.AutoFeedback || (!x.AutoFeedbackPersistent &&
        (x.AutoFeedbackUntil == nil || time.Now().UTC().After(*x.AutoFeedbackUntil)))) {
		return
	}
	if _, err = s.agentDeliverToolResult(ctx, store, x, idx, true); err != nil {
		var sc agentContinueError
		if errors.As(err, &sc) && sc.status == 429 {
			return // transient admission pressure; a later pass retries safely
		}
		// failed/unknown is already durable; never retry automatically.
	}
}

// startAgentAutoFeedback runs the bounded scheduler until the context ends.
func (s *Server) startAgentAutoFeedback(ctx context.Context) {
	ticker := time.NewTicker(agentAutoFeedbackPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runAgentAutoFeedback(ctx)
		}
	}
}
