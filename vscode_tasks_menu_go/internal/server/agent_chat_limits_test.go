package server

import (
	"testing"
	"time"
)

func TestAgentAdmissionEnforcesGlobalLimitAndReleases(t *testing.T) {
	agentAdmissions.mu.Lock()
	agentAdmissions.active = make(map[string]int)
	agentAdmissions.waiters = make(map[string]int)
	agentAdmissions.total = 0
	agentAdmissions.mu.Unlock()

	previousTimeout := agentQueueTimeout
	agentQueueTimeout = 200 * time.Millisecond
	t.Cleanup(func() { agentQueueTimeout = previousTimeout })
	releases := make([]func(), 0, agentMaxConcurrentGlobal)
	// Distinct owners fill the global cap.
	for i := 0; i < agentMaxConcurrentGlobal; i++ {
		release, ok := acquireAgentSlot("owner-" + string(rune('a'+i)))
		if !ok {
			t.Fatalf("slot %d refused below global cap", i)
		}
		releases = append(releases, release)
	}
	blocked := make(chan bool, 1)
	go func() {
		// A queued waiter must be refused once the bound is reached, not hang.
		_, ok := acquireAgentSlot("owner-overflow")
		blocked <- ok
	}()
	select {
	case ok := <-blocked:
		if ok {
			t.Fatal("admitted past the global cap")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queue admission did not report backpressure")
	}
	for _, release := range releases {
		release()
	}
	agentAdmissions.mu.Lock()
	total := agentAdmissions.total
	agentAdmissions.mu.Unlock()
	if total != 0 {
		t.Fatalf("releases leaked %d slots", total)
	}
}

func TestAgentAdmissionPerOwnerLimit(t *testing.T) {
	agentAdmissions.mu.Lock()
	agentAdmissions.active = make(map[string]int)
	agentAdmissions.waiters = make(map[string]int)
	agentAdmissions.total = 0
	agentAdmissions.mu.Unlock()
	releases := make([]func(), 0, agentMaxConcurrentPerOwner)
	for i := 0; i < agentMaxConcurrentPerOwner; i++ {
		release, ok := acquireAgentSlot("same-owner")
		if !ok {
			t.Fatalf("per-owner slot %d refused", i)
		}
		releases = append(releases, release)
	}
	agentAdmissions.mu.Lock()
	active := agentAdmissions.active["same-owner"]
	agentAdmissions.mu.Unlock()
	if active != agentMaxConcurrentPerOwner {
		t.Fatalf("per-owner accounting wrong: %d", active)
	}
	for _, release := range releases {
		release()
	}
}
