package agentrun

import (
	"context"
	"testing"
)

func TestGlobalSlotContextRequiresLiveAcquiredLease(t *testing.T) {
	if GlobalSlotHeld(context.Background()) {
		t.Fatal("plain context bypassed admission")
	}
	ctx, release, ok := AcquireGlobalSlotContext(context.Background())
	if !ok || !GlobalSlotHeld(ctx) {
		t.Fatal("acquired slot context did not carry its live lease")
	}
	release()
	if GlobalSlotHeld(ctx) {
		t.Fatal("released slot lease still bypasses admission")
	}
}
