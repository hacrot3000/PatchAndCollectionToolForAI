package server

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAgentAdmissionEnforcesGlobalLimitAndReleases(t *testing.T) {
	resetAgentAdmissionsForTest()
	previousTimeout := agentQueueTimeout
	agentQueueTimeout = 150 * time.Millisecond
	t.Cleanup(func() { agentQueueTimeout = previousTimeout })

	releases := make([]func(), 0, agentMaxConcurrentGlobal)
	for i := 0; i < agentMaxConcurrentGlobal; i++ {
		release, ok := acquireAgentSlot(context.Background(), "owner-"+string(rune('a'+i)))
		if !ok {
			t.Fatalf("slot %d refused below global cap", i)
		}
		releases = append(releases, release)
	}
	started := time.Now()
	if _, ok := acquireAgentSlot(context.Background(), "owner-overflow"); ok {
		t.Fatal("admitted past the global cap")
	}
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond {
		t.Fatalf("waiter gave up too early (%v), suggests busy polling instead of bounded wait", elapsed)
	}
	if _, total, waiters := agentAdmissionState(); total != agentMaxConcurrentGlobal || waiters != 0 {
		t.Fatalf("state after refusal: total=%d waiters=%d", total, waiters)
	}
	for _, release := range releases {
		release()
	}
	release := releases[0]
	release() // double release must not underflow
	if _, total, _ := agentAdmissionState(); total != 0 {
		t.Fatalf("releases leaked %d slots", total)
	}
}

func TestAgentAdmissionWaiterCountsOnceWhileWaiting(t *testing.T) {
	resetAgentAdmissionsForTest()
	previousTimeout := agentQueueTimeout
	agentQueueTimeout = 5 * time.Second
	t.Cleanup(func() { agentQueueTimeout = previousTimeout })

	// Saturate one owner.
	var held []func()
	for i := 0; i < agentMaxConcurrentPerOwner; i++ {
		rel, ok := acquireAgentSlot(context.Background(), "same")
		if !ok {
			t.Fatalf("prefill %d refused", i)
		}
		held = append(held, rel)
	}
	var wg sync.WaitGroup
	for i := 0; i < agentMaxQueuedPerOwner; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, ok := acquireAgentSlot(context.Background(), "same")
			if ok {
				rel()
			}
		}()
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, _, waiters := agentAdmissionState(); waiters == agentMaxQueuedPerOwner {
			break
		}
		if time.Now().After(deadline) {
			_, _, w := agentAdmissionState()
			t.Fatalf("waiter count drifted: got %d want %d", w, agentMaxQueuedPerOwner)
		}
		time.Sleep(5 * time.Millisecond)
	}
	// The 9th waiter must be refused immediately, not queue.
	if _, ok := acquireAgentSlot(context.Background(), "same"); ok {
		t.Fatal("9th waiter admitted past queue bound")
	}
	for _, rel := range held {
		rel()
	}
	wg.Wait()
	if _, total, waiters := agentAdmissionState(); total != 0 || waiters != 0 {
		t.Fatalf("leaked after drain: total=%d waiters=%d", total, waiters)
	}
}

func TestAgentAdmissionCancelReleasesQueuePosition(t *testing.T) {
	resetAgentAdmissionsForTest()
	previousTimeout := agentQueueTimeout
	agentQueueTimeout = 5 * time.Second
	t.Cleanup(func() { agentQueueTimeout = previousTimeout })

	held := make([]func(), 0, agentMaxConcurrentPerOwner)
	for i := 0; i < agentMaxConcurrentPerOwner; i++ {
		rel, ok := acquireAgentSlot(context.Background(), "cancel-owner")
		if !ok {
			t.Fatal("prefill refused")
		}
		held = append(held, rel)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() {
		_, ok := acquireAgentSlot(ctx, "cancel-owner")
		done <- ok
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, _, waiters := agentAdmissionState(); waiters == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancel waiter never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if ok := <-done; ok {
		t.Fatal("cancelled waiter was admitted")
	}
	if _, _, waiters := agentAdmissionState(); waiters != 0 {
		t.Fatalf("cancelled waiter leaked a queue position: %d", waiters)
	}
	for _, rel := range held {
		rel()
	}
}

func TestAgentAdmissionAdmitsWaiterAfterRelease(t *testing.T) {
	resetAgentAdmissionsForTest()
	previousTimeout := agentQueueTimeout
	agentQueueTimeout = 5 * time.Second
	t.Cleanup(func() { agentQueueTimeout = previousTimeout })

	held := make([]func(), 0, agentMaxConcurrentPerOwner)
	for i := 0; i < agentMaxConcurrentPerOwner; i++ {
		rel, ok := acquireAgentSlot(context.Background(), "handoff")
		if !ok {
			t.Fatal("prefill refused")
		}
		held = append(held, rel)
	}
	got := make(chan bool, 1)
	go func() {
		rel, ok := acquireAgentSlot(context.Background(), "handoff")
		if ok {
			rel()
		}
		got <- ok
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, _, waiters := agentAdmissionState(); waiters == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("waiter never queued")
		}
		time.Sleep(5 * time.Millisecond)
	}
	held[0]()
	select {
	case ok := <-got:
		if !ok {
			t.Fatal("waiter was not admitted after a slot was released")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiter was never notified of the freed slot")
	}
}
