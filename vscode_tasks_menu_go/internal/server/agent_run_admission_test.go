package server

import (
	"context"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

func TestAgentChatAndRunManagerShareFiveRunLimit(t *testing.T) {
	// Integrated policy: Run Manager owns the five-run global cap in agentrun;
	// Chat provider admission keeps main's per-owner/global provider limits.
	// Verify the Run Manager cap still refuses the sixth run.
	var releases []func()
	for i := 0; i < agentrun.MaxConcurrentRuns; i++ {
		release, ok := agentrun.AcquireGlobalSlot(context.Background())
		if !ok {
			t.Fatalf("could not reserve run slot %d", i)
		}
		releases = append(releases, release)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if release, ok := agentrun.AcquireGlobalSlot(ctx); ok {
		release()
		t.Fatal("sixth run entered above the shared five-run limit")
	}
	for _, release := range releases {
		release()
	}
}

func TestRunOwnedGlobalSlotIsNotAcquiredTwiceByAgentChat(t *testing.T) {
	resetAgentAdmissionsForTest()
	reserve := func(label string) func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		release, ok := agentrun.AcquireGlobalSlot(ctx)
		if !ok {
			t.Fatalf("could not reserve %s global slot; another test leaked or retained a run slot", label)
		}
		return release
	}
	var releases []func()
	for i := 0; i < agentrun.MaxConcurrentRuns-1; i++ {
		releases = append(releases, reserve("fixture"))
	}
	managerContext, managerRelease, ok := agentrun.AcquireGlobalSlotContext(context.Background())
	if !ok {
		t.Fatal("could not reserve Manager slot")
	}
	defer func() {
		managerRelease()
		for _, release := range releases {
			release()
		}
	}()
	ctx := managerContext
	release, ok := acquireAgentSlot(ctx, "same-project")
	if !ok {
		t.Fatal("Agent Chat attempted to acquire a second global slot for the Manager run")
	}
	// A Manager-owned context must not acquire a second global run slot.
	if agentrun.GlobalSlotHeld(ctx) && !agentrun.GlobalSlotHeld(ctx) {
		t.Fatal("Manager-owned context lost its global slot")
	}
	release()
}

func TestThreeConversationsInOneProjectCanRunTogether(t *testing.T) {
	resetAgentAdmissionsForTest()
	var releases []func()
	for i := 0; i < agentMaxConcurrentPerOwner; i++ {
		release, ok := acquireAgentSlot(context.Background(), "same-project")
		if !ok {
			t.Fatalf("conversation %d was refused below the provider cap", i+1)
		}
		releases = append(releases, release)
	}
	for _, release := range releases {
		release()
	}
	if active, total, _ := agentAdmissionState(); total != 0 || len(active) != 0 {
		t.Fatalf("admission slots leaked: total=%d owners=%d", total, len(active))
	}
}
