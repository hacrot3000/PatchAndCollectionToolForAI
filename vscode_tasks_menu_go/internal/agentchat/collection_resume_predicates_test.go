package agentchat

import (
	"strings"
	"testing"
	"time"
)

// resumeFenceSession builds a canonical terminal UNKNOWN collection delivery:
// an acknowledged read-only native observation whose call/output pair is
// complete in the canonical journal. Every test below mutates one invariant.
func resumeFenceSession(t *testing.T) Session {
	t.Helper()
	payload := `{"status":"INCOMPLETE","run_id":"native-run"}`
	now := time.Now().UTC()
	run := &ProviderRun{
		ID:                "run-old",
		Kind:              "collection",
		Phase:             "unknown",
		CallID:            "read-1",
		PayloadSHA256:     strings.Repeat("a", 64),
		ProviderFingerprint: "fp-1",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	return Session{
		Owner:       "local",
		Status:      "failed",
		ProviderRun: run,
		History: []HistoryItem{
			{Type: "user", Content: "Audit the source"},
			{Type: "function_call", Name: "taskdeck_collect_workspace", CallID: "read-1", Arguments: `{}`},
			{Type: "function_call_output", CallID: "read-1", Content: payload},
		},
	}
}

func TestUnknownCollectionDeliveryMarkerIsTerminalAndNarrow(t *testing.T) {
	base := resumeFenceSession(t)
	if !HasUnknownCollectionDelivery(base) {
		t.Fatal("canonical UNKNOWN collection delivery not recognized")
	}
	cancelled := base
	cancelled.Status = "cancelled"
	if !HasUnknownCollectionDelivery(cancelled) {
		t.Fatal("cancelled terminal UNKNOWN delivery not recognized")
	}
	for _, mutate := range []func(*Session){
		func(x *Session) { x.Status = "completed" },
		func(x *Session) { x.ProviderRun.Phase = "sending" },
		func(x *Session) { x.ProviderRun.Kind = "feedback" },
		func(x *Session) { x.ProviderRun = nil },
	} {
		x := resumeFenceSession(t)
		mutate(&x)
		if HasUnknownCollectionDelivery(x) {
			t.Fatal("non-terminal or non-collection run admitted as UNKNOWN delivery")
		}
	}
}

func TestCollectionResumeReceiptRequiresExactCanonicalPair(t *testing.T) {
	base := resumeFenceSession(t)
	want := ProviderPayloadDigest([]byte(base.History[2].Content))
	digest, ok := CollectionResumeReceipt(base)
	if !ok || digest != want {
		t.Fatalf("canonical exact receipt not recovered: %s %v", digest, ok)
	}

	for name, mutate := range map[string]func(*Session){
		"foreign owner":     func(x *Session) { x.Owner = "remote" },
		"archived":          func(x *Session) { x.Archived = true },
		"auto collection off": func(x *Session) { off := false; x.AutoCollection = &off },
		"no run":            func(x *Session) { x.ProviderRun = nil },
		"wrong call id":     func(x *Session) { x.History[2].CallID = "other" },
		"wrong call name":   func(x *Session) { x.History[1].Name = "taskdeck_submit_patch" },
		"empty observation": func(x *Session) { x.History[2].Content = "  " },
		"pending pair": func(x *Session) {
			x.History = append(x.History, HistoryItem{Type: "function_call", Name: "taskdeck_collect_workspace", CallID: "unanswered"})
		},
		"unacknowledged tool": func(x *Session) {
			x.ToolCalls = []ToolCallRecord{{CallID: "read-1", Status: "error", FeedbackSent: true, FeedbackStatus: "unknown"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			x := resumeFenceSession(t)
			mutate(&x)
			if _, ok := CollectionResumeReceipt(x); ok {
				t.Fatal("unsafe or non-exact state admitted for collection resume")
			}
		})
	}
}

func TestCanSendAfterUnknownCollectionRequiresTerminalToolCalls(t *testing.T) {
	base := resumeFenceSession(t)
	if !CanSendAfterUnknownCollection(base) {
		t.Fatal("canonical UNKNOWN collection with no tool calls must allow a new user message")
	}
	for name, calls := range map[string][]ToolCallRecord{
		"prepared feedback": {{CallID: "read-1", Status: "done", FeedbackSent: true, FeedbackStatus: "prepared"}},
		"sending feedback":  {{CallID: "read-1", Status: "done", FeedbackSent: true, FeedbackStatus: "sending"}},
		"in flight":         {{CallID: "read-1", Status: "running"}},
	} {
		t.Run(name, func(t *testing.T) {
			x := resumeFenceSession(t)
			x.ToolCalls = calls
			if CanSendAfterUnknownCollection(x) {
				t.Fatal("in-flight or unacknowledged tool call bypassed the egress fence")
			}
		})
	}
	archived := resumeFenceSession(t)
	archived.Archived = true
	if CanSendAfterUnknownCollection(archived) {
		t.Fatal("archived session admitted after UNKNOWN collection")
	}
}

func TestCollectionAutoResumeAllowedNeedsLiveOwnerConsent(t *testing.T) {
	base := resumeFenceSession(t)
	base.AutoContinueText = true
	if !CollectionAutoResumeAllowed(base) {
		t.Fatal("explicit live owner opt-in was refused")
	}
	optOut := resumeFenceSession(t)
	if CollectionAutoResumeAllowed(optOut) {
		t.Fatal("auto resume bypassed owner opt-out")
	}
	exhausted := resumeFenceSession(t)
	exhausted.AutoContinueText = true
	exhausted.Continuation = &TurnContinuation{NoProgressTurns: MaxContinuationNoWorkTurns}
	if !CollectionAutoResumeAllowed(exhausted) {
		t.Fatal("exact eligible receipt was blocked by diagnostic no-progress count")
	}
	stalledPlan := resumeFenceSession(t)
	stalledPlan.TaskPlan = &TaskPlan{State: "blocked"}
	stalledPlan.AutoContinueText = true
	if CollectionAutoResumeAllowed(stalledPlan) {
		t.Fatal("non-active TaskPlan admitted auto resume")
	}
}

func TestCollectionResumeHistoryRejectsReplayAndConflictingProof(t *testing.T) {
	if !validCollectionResumeHistory(nil) {
		t.Fatal("empty history must be valid")
	}
	base := resumeFenceSession(t)
	valid := CollectionResumeCheckpoint{Previous: *base.ProviderRun, NewRunID: "run-new", ReceiptUnavailable: true}
	if !validCollectionResumeHistory([]CollectionResumeCheckpoint{valid}) {
		t.Fatal("bounded UNKNOWN marker rejected")
	}
	sameID := valid
	sameID.NewRunID = base.ProviderRun.ID
	if validCollectionResumeHistory([]CollectionResumeCheckpoint{sameID}) {
		t.Fatal("checkpoint reused the previous run ID")
	}
	conflicting := valid
	// ReceiptUnavailable=true must not coexist with a fabricated digest.
	conflicting.ReceiptSHA256 = strings.Repeat("f", 64)
	if validCollectionResumeHistory([]CollectionResumeCheckpoint{conflicting}) {
		t.Fatal("receipt conflict accepted")
	}
	duplicate := valid
	if validCollectionResumeHistory([]CollectionResumeCheckpoint{valid, duplicate}) {
		t.Fatal("duplicate logical observation admitted")
	}
	tooMany := make([]CollectionResumeCheckpoint, maxCollectionResumeCheckpoints+1)
	for i := range tooMany {
		tooMany[i] = valid
	}
	tooMany[len(tooMany)-1].Previous.ID = "other-run"
	if validCollectionResumeHistory(tooMany) {
		t.Fatal("unbounded resume history admitted")
	}
}
