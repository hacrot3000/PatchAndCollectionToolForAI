package remoterelay

import (
	"testing"
	"time"
)

func TestDeviceFrameBudgetRejectsSingleSocketFlood(t *testing.T) {
	start := time.Unix(1700000000, 0)
	budget := newDeviceFrameBudget(start)
	for i := 0; i < maxDeviceFrameBurst; i++ {
		if !budget.allow(start) {
			t.Fatalf("legitimate initial Agent frame %d rejected", i)
		}
	}
	if budget.allow(start) || budget.allow(start.Add(deviceFrameRefillEvery-time.Nanosecond)) {
		t.Fatal("rapid frames bypassed Agent WebSocket budget")
	}
	if budget.allow(start.Add(-time.Second)) {
		t.Fatal("clock rollback restored Agent message capacity")
	}
	if !budget.allow(start.Add(deviceFrameRefillEvery)) ||
		budget.allow(start.Add(deviceFrameRefillEvery)) {
		t.Fatal("one refill interval must admit exactly one new frame")
	}
	if !budget.allow(start.Add(time.Hour)) {
		t.Fatal("idle Agent connection did not regain capacity")
	}
	for i := 1; i < maxDeviceFrameBurst; i++ {
		if !budget.allow(start.Add(time.Hour)) {
			t.Fatalf("idle budget failed to refill burst slot %d", i)
		}
	}
	if budget.allow(start.Add(time.Hour)) {
		t.Fatal("Agent message bucket exceeded maximum burst capacity")
	}
}

func TestDeviceFrameBudgetDoesNotShareStateAcrossAgents(t *testing.T) {
	now := time.Unix(1700000000, 0)
	first := newDeviceFrameBudget(now)
	second := newDeviceFrameBudget(now)
	for i := 0; i < maxDeviceFrameBurst; i++ {
		if !first.allow(now) {
			t.Fatal("unexpected depletion before burst was consumed")
		}
	}
	if first.allow(now) {
		t.Fatal("first Agent socket not throttled")
	}
	if !second.allow(now) {
		t.Fatal("one Agent socket's traffic throttled another independent Agent")
	}
}
