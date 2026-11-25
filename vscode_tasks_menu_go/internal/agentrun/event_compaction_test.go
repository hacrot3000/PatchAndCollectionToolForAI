package agentrun

import (
	"reflect"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func m179Events(n int) []Event {
	out := make([]Event, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Event{Sequence: uint64(i + 1), Type: agentchat.RunEventTextDelta, Payload: []byte("x")})
	}
	return out
}

func TestPlanEventCompactionBoundedHotSet(t *testing.T) {
	events := m179Events(10)
	plan := PlanEventCompaction(events, 0, 5, time.Time{})
	if plan.Reason != "bounded_hot_projection" || plan.ArchiveFrom != 0 ||
		plan.ArchiveCount != 5 || plan.KeepCount != 5 || plan.NextCursor != 6 {
		t.Fatalf("unexpected compaction plan: %+v", plan)
	}
	kept := ApplyEventCompaction(events, plan)
	if len(kept) != 5 || kept[0].Sequence != 6 || kept[len(kept)-1].Sequence != 10 {
		t.Fatalf("hot set is not the newest bounded suffix: %+v", kept)
	}
	if len(events) != 10 {
		t.Fatal("planner/apply mutated the input event slice")
	}
}

func TestPlanEventCompactionNoopWithinLimit(t *testing.T) {
	events := m179Events(3)
	plan := PlanEventCompaction(events, 0, 5, time.Time{})
	if plan.Reason != "no_compaction" || plan.ArchiveCount != 0 || plan.ArchiveFrom != 3 || plan.NextCursor != 0 {
		t.Fatalf("expected no-op plan: %+v", plan)
	}
	if !reflect.DeepEqual(events, ApplyEventCompaction(events, plan)) {
		t.Fatal("no-op plan changed the event set")
	}
}

func TestPlanEventCompactionPreservesProtectedEvents(t *testing.T) {
	events := m179Events(6)
	events[2].Type = agentchat.RunEventToolCall
	plan := PlanEventCompaction(events, 0, 3, time.Time{})
	// hotStart=3; indices 0,1 are elidable, index 2 is protected.
	if plan.ArchiveFrom != 0 || plan.ArchiveCount != 2 {
		t.Fatalf("protected event did not bound the archive run: %+v", plan)
	}
	kept := ApplyEventCompaction(events, plan)
	found := false
	for _, e := range kept {
		if e.Sequence == 3 && e.Type == agentchat.RunEventToolCall {
			found = true
		}
	}
	if !found {
		t.Fatal("a protected tool-call event was archived")
	}
}

func TestPlanEventCompactionNoElidablePrefix(t *testing.T) {
	events := m179Events(6)
	events[0].Type = agentchat.RunEventToolCall
	events[1].Type = agentchat.RunEventToolResult
	plan := PlanEventCompaction(events, 0, 3, time.Time{})
	// The first contiguous elidable run starts at index 2.
	if plan.ArchiveFrom != 2 || plan.ArchiveCount != 1 {
		t.Fatalf("unexpected archive boundary: %+v", plan)
	}
}

func TestPlanEventCompactionCursorMonotonicAndClamped(t *testing.T) {
	events := m179Events(10)
	plan := PlanEventCompaction(events, 100, 5, time.Time{})
	if plan.NextCursor != 100 {
		t.Fatalf("cursor decreased below its input: %+v", plan)
	}
	stamp := time.Now().UTC()
	clamped := PlanEventCompaction(events, 0, 0, stamp)
	if clamped.Reason != "bounded_hot_projection" || clamped.ArchivedAt.IsZero() || clamped.ArchiveCount != 9 {
		t.Fatalf("maxHotEvents<1 must clamp and stamp: %+v", clamped)
	}
}

func TestElidableEventTypes(t *testing.T) {
	for _, typ := range []agentchat.RunEventType{agentchat.RunEventTextDelta, agentchat.RunEventUsage, agentchat.RunEventRetry} {
		if !elidableEvent(typ) {
			t.Fatalf("expected elidable event type %q", typ)
		}
	}
	for _, typ := range []agentchat.RunEventType{agentchat.RunEventToolCall, agentchat.RunEventToolResult,
		agentchat.RunEventToolCallDelta, agentchat.RunEventTerminal, agentchat.RunEventError} {
		if elidableEvent(typ) {
			t.Fatalf("protected event type was marked elidable: %q", typ)
		}
	}
}
