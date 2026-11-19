package server

import (
	"context"
	"errors"
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
	if !x.AutoFeedback || x.Archived {
		return 0, false
	}
	if x.AutoFeedbackUntil == nil || time.Now().UTC().After(*x.AutoFeedbackUntil) {
		return 0, false
	}
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
			return i, true
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
		if _, ok := agentAutoFeedbackCandidate(sess); !ok {
			continue
		}
		s.autoFeedbackOne(ctx, store, sess.ID)
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
	if x.AutoFeedbackUntil == nil || time.Now().UTC().After(*x.AutoFeedbackUntil) {
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
