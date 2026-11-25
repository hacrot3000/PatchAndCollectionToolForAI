package agentrun

import (
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

// EventCompactionPlan is a pure, content-free decision about which bounded hot
// events may be archived without losing canonical evidence. It never deletes
// anything itself: wiring the plan to a durable append-only archive is a
// separate, reviewed change. No provider text, tool payload or file content is
// carried in the plan, and no replay permission is granted.
type EventCompactionPlan struct {
	NextCursor   uint64    `json:"next_cursor"`
	ArchiveFrom  int       `json:"archive_from"`
	ArchiveCount int       `json:"archive_count"`
	KeepCount    int       `json:"keep_count"`
	Reason       string    `json:"reason"`
	ArchivedAt   time.Time `json:"archived_at,omitempty"`
}

// elidableEvent reports whether an event type is a safe compaction candidate.
// Tool-call/result pairs, terminal and error events are NEVER elidable: an
// unmatched call/output pair or a final result must survive compaction.
func elidableEvent(t agentchat.RunEventType) bool {
	switch t {
	case agentchat.RunEventTextDelta, agentchat.RunEventUsage, agentchat.RunEventRetry:
		return true
	default:
		return false
	}
}

// PlanEventCompaction computes a bounded hot-set compaction decision. It is
// deterministic and pure: no files, no persistence, no provider and no mutation
// of the input slice. Invariants:
//   - the newest maxHotEvents events are always kept;
//   - protected types (tool calls/results, terminal, error) are always kept;
//   - the cursor never decreases below its input value;
//   - when nothing can be elided the plan is a no-op (ArchiveFrom == len(events)).
//
// This is the policy half of M17-09. Atomic checkpoint publication, an
// append-only checksummed archive segment, snapshot GC and disk backpressure
// remain separate, reviewed wiring changes.
func PlanEventCompaction(events []Event, cursor uint64, maxHotEvents int, now time.Time) EventCompactionPlan {
	if maxHotEvents < 1 {
		maxHotEvents = 1
	}
	plan := EventCompactionPlan{
		NextCursor: cursor,
		ArchiveFrom: len(events),
		KeepCount: len(events),
		Reason: "no_compaction",
	}
	if len(events) <= maxHotEvents {
		return plan
	}
	hotStart := len(events) - maxHotEvents
	// Archive only the contiguous prefix of elidable events strictly older
	// than the hot suffix. A protected event stops the boundary, so an
	// unmatched tool call/output pair can never be split across the archive.
	from, end := -1, -1
	for i := 0; i < hotStart; i++ {
		if !elidableEvent(events[i].Type) {
			continue
		}
		if from < 0 {
			from = i
		}
		end = i
		if i+1 < hotStart && !elidableEvent(events[i+1].Type) {
			break
		}
	}
	if from < 0 || end < from {
		return plan
	}
	count := end - from + 1
	next := cursor
	if boundary := events[end].Sequence + 1; boundary > next {
		next = boundary
	}
	plan.NextCursor = next
	plan.ArchiveFrom = from
	plan.ArchiveCount = count
	plan.KeepCount = len(events) - count
	plan.Reason = "bounded_hot_projection"
	if !now.IsZero() {
		plan.ArchivedAt = now.UTC()
	}
	return plan
}

// ApplyEventCompaction returns the retained hot events for a plan. It is pure
// and never mutates the input slice; the caller persists the archive row
// separately under a reviewed durable contract.
func ApplyEventCompaction(events []Event, plan EventCompactionPlan) []Event {
	if plan.ArchiveCount <= 0 || plan.ArchiveFrom < 0 || plan.ArchiveFrom >= len(events) {
		return append([]Event(nil), events...)
	}
	keep := make([]Event, 0, len(events)-plan.ArchiveCount)
	keep = append(keep, events[:plan.ArchiveFrom]...)
	keep = append(keep, events[plan.ArchiveFrom+plan.ArchiveCount:]...)
	return keep
}
