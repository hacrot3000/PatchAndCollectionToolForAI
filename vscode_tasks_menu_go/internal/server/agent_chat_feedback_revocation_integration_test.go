package server

import (
	"context"
	"errors"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

// A durable FeedbackAutoResume run must re-check live owner consent at the
// SENDING CAS. Stop/cancel before SENDING wins; no transport egress occurs and
// the canonical PATCH receipt stays byte-exact with UNKNOWN acknowledgement.
func TestFeedbackResumeStopBeforeSendingFencesEgress(t *testing.T) {
	for _, tc := range []struct{ name, option string }{
		{"feedback_revoked", "feedback"},
		{"continue_revoked", "continue"},
		{"persistent_feedback_revoked", "feedback_legacy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, id := newAgentConversationFixture(t, "stateless")
			store, err := s.agentChatStore()
			if err != nil { t.Fatal(err) }
			x, err := store.Get(id, "local")
			if err != nil { t.Fatal(err) }
			x.AutoContinueText = true
			x.AutoFeedback = true
			x.AutoFeedbackPersistent = true
			x.RetryMode = agentchat.RetryUnlimited
			x.Status = "requesting_ai"
			const receipt = `{"status":"PASS","rc":0,"name":"approved.zip"}`
			x.History = append(x.History,
				agentchat.HistoryItem{Type: "user", Content: "Original task"},
				agentchat.HistoryItem{Type: "function_call", Name: "taskdeck_submit_patch", CallID: "patch", Arguments: `{}`},
				agentchat.HistoryItem{Type: "function_call_output", CallID: "patch", Content: receipt})
			p := agentProviderFixture()
			payload := []byte(`{"model":"gpt-4.1-mini","input":"feedback resume"}`)
			if err = prepareAgentProviderRun(store, &x, "message", "", p, p.Models[0], payload); err != nil { t.Fatal(err) }
			x.ProviderRun.FeedbackAutoResume = true
			x, err = store.Save(x, x.Revision)
			if err != nil { t.Fatal(err) }
			stale := x

			// All revocations commit at the SAME turn revision via UpdateAutomation.
			// A real Stop writes a cancel event/status and advances the turn revision;
			// that path is fenced by ErrStaleRevision before egress and is covered by
			// TestAgentStopCancelsDurablePreparedRunBeforeEgress.
			if tc.option == "feedback_legacy" {
				if _, e := store.UpdateAutomation(id, "local", x.Workspace, x.Revision, "feedback_legacy", false, agentchat.RetryOff); e != nil { t.Fatal(e) }
			} else if _, e := store.UpdateAutomation(id, "local", x.Workspace, x.Revision, tc.option, false, ""); e != nil { t.Fatal(e) }
			}
			if err = markAgentProviderSending(store, &stale); !errors.Is(err, agentchat.ErrCollectionReplanRevoked) {
				t.Fatalf("revocation did not fence SENDING: %v", err)
			}
			if stale.ProviderRun.Phase != "blocked" { t.Fatal("caller retained stale sending state") }
			got, e := store.Get(id, "local")
			if e != nil { t.Fatal(e) }
			if got.ProviderRun.Phase != "blocked" { t.Fatalf("egress crossed: phase=%s", got.ProviderRun.Phase) }
			for _, ev := range got.Events { if ev.Kind == "provider_egress" { t.Fatal("revoked resume crossed SENDING") } }
			if len(got.History) < 3 || got.History[len(got.History)-1].Content != receipt {
				t.Fatal("canonical receipt changed")
			}
			if _, e = store.PreparedProviderPayload(id, "local"); !errors.Is(e, agentchat.ErrInvalidSession) {
				t.Fatal("blocked request remained replayable")
			}
		})
	}
}

// Restart with a revoked automation grant: the durable marker survives but the
// live consent predicate refuses automatic resume. The previous ACK remains
// UNKNOWN and no second native PATCH intent is created.
func TestFeedbackResumeRestartRevokedConsentRefused(t *testing.T) {
	base := agentchat.Session{Owner: "local", Status: "failed", AutoContinueText: true, AutoFeedback: true,
		AutoFeedbackPersistent: true, RetryMode: agentchat.RetryUnlimited,
		ProviderRun: &agentchat.ProviderRun{ID: "r2-e2", Kind: "feedback", Phase: "unknown", CallID: "patch",
			PayloadSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ProviderFingerprint: "fp",
			FeedbackAutoResume: true}}
	if !agentchat.FeedbackAutoResumeAllowed(base) { t.Fatal("durable consent should be eligible before revocation") }
	for _, mutate := range []func(*agentchat.Session){
		func(y *agentchat.Session) { y.AutoFeedback = false },
		func(y *agentchat.Session) { y.AutoFeedbackPersistent = false },
		func(y *agentchat.Session) { y.AutoContinueText = false },
		func(y *agentchat.Session) { y.Status = "cancelled" },
		func(y *agentchat.Session) { y.Status = "failed"; y.TaskPlan = &agentchat.TaskPlan{} },
		func(y *agentchat.Session) { y.Archived = true },
	} {
		y := base
		run := *base.ProviderRun
		y.ProviderRun = &run
		mutate(&y)
		if agentchat.FeedbackAutoResumeAllowed(y) { t.Fatal("revoked/stopped consent admitted automatic resume") }
	}
}

// An explicit fresh owner request is not automatic: it remains allowed after
// automation is disabled, while the old ambiguous request is never replayed.
func TestFeedbackResumeExplicitOwnerRequestAfterAutomationDisabled(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	store, err := s.agentChatStore()
	if err != nil { t.Fatal(err) }
	x, err := store.Get(id, "local")
	if err != nil { t.Fatal(err) }
	x.AutoContinueText = false
	x.AutoFeedback = false
	x.AutoFeedbackPersistent = false
	x.RetryMode = agentchat.RetryOff
	x.Status = "requesting_ai"
	p := agentProviderFixture()
	payload := []byte(`{"model":"gpt-4.1-mini","input":"explicit owner request"}`)
	if err = prepareAgentProviderRun(store, &x, "message", "", p, p.Models[0], payload); err != nil { t.Fatal(err) }
	// An ordinary explicit turn has no FeedbackAutoResume marker and must not be
	// blocked by the automatic-resume predicate.
	if x.ProviderRun.FeedbackAutoResume { t.Fatal("explicit request marked as automatic") }
	if err = markAgentProviderSending(store, &x); err != nil { t.Fatalf("explicit owner egress blocked: %v", err) }
	got, err := store.Get(id, "local")
	if err != nil { t.Fatal(err) }
	if got.ProviderRun.Phase != "sending" { t.Fatal("explicit authorized request was not admitted") }
	_ = context.Background()
}
