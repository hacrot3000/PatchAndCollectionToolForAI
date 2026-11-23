package server

import (
	"context"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

// Agent Chat concurrency and backpressure. These are process-local admissions
// intended to prevent runaway provider spend and workspace contention; they
// are NOT a distributed quota. In multi-daemon deployments the durable journal
// plus workspace lease remain the source of truth, and the documented policy
// is one daemon per workspace.
const (
	agentMaxConcurrentPerOwner = agentrun.MaxConcurrentRuns
	agentMaxConcurrentGlobal   = agentrun.MaxConcurrentRuns
	agentMaxQueuedPerOwner     = 8
)

// agentQueueTimeout bounds how long one turn may wait for a slot. It is a
// variable so tests can shorten the wait without leaving goroutines alive.
var agentQueueTimeout = 2 * time.Minute

// agentAdmission tracks active slots and queued waiters. A waiter occupies one
// queue position for the WHOLE wait (not once per retry), so the per-owner cap
// reflects the real number of pending turns. Release broadcasts on a channel
// instead of having callers poll.
type agentAdmission struct {
	mu      sync.Mutex
	active  map[string]int
	total   int
	waiters map[string]int
	notify  chan struct{}
}

var agentAdmissions = agentAdmission{
	active:  make(map[string]int),
	waiters: make(map[string]int),
	notify:  make(chan struct{}),
}

func (a *agentAdmission) broadcastLocked() {
	if a.notify != nil {
		close(a.notify)
	}
	a.notify = make(chan struct{})
}

func (a *agentAdmission) fitsLocked(owner string) bool {
	return a.total < agentMaxConcurrentGlobal && a.active[owner] < agentMaxConcurrentPerOwner
}

func (a *agentAdmission) takeLocked(owner string) {
	a.active[owner]++
	a.total++
}

func (a *agentAdmission) releaseSlot(owner string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			if a.active[owner] > 0 {
				a.active[owner]--
				if a.active[owner] == 0 {
					delete(a.active, owner)
				}
				if a.total > 0 {
					a.total--
				}
			}
			a.broadcastLocked()
			a.mu.Unlock()
		})
	}
}

// acquireAgentSlot admits one model turn with bounded waiting and explicit
// backpressure. The caller's context is honoured, so a cancelled or
// disconnected caller leaves the queue immediately. It returns false when the
// owner/global limit stays saturated for the wait budget or when the
// per-owner waiter cap is reached, so callers answer 429 instead of queueing
// unbounded work.
func acquireAgentSlot(ctx context.Context, owner string) (func(), bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if owner == "" {
		if agentrun.GlobalSlotHeld(ctx) {
			return func() {}, true
		}
		return agentrun.AcquireGlobalSlot(ctx)
	}
	// The wait budget is derived from agentQueueTimeout at call time so tests
	// can shorten it. A shorter caller deadline always wins.
	deadline := time.Now().Add(agentQueueTimeout)
	if existing, ok := ctx.Deadline(); ok && existing.Before(deadline) {
		deadline = existing
	}
	waitCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	agentAdmissions.mu.Lock()
	if agentAdmissions.fitsLocked(owner) {
		agentAdmissions.takeLocked(owner)
		agentAdmissions.mu.Unlock()
		return acquireAgentGlobalWithLocal(waitCtx, agentAdmissions.releaseSlot(owner), agentrun.GlobalSlotHeld(waitCtx))
	}
	if agentAdmissions.waiters[owner] >= agentMaxQueuedPerOwner {
		agentAdmissions.mu.Unlock()
		return nil, false
	}
	agentAdmissions.waiters[owner]++
	agentAdmissions.mu.Unlock()

	// The waiter holds exactly one queue position until it is admitted,
	// refused or cancelled.
	defer func() {
		agentAdmissions.mu.Lock()
		if agentAdmissions.waiters[owner] > 0 {
			agentAdmissions.waiters[owner]--
			if agentAdmissions.waiters[owner] == 0 {
				delete(agentAdmissions.waiters, owner)
			}
		}
		agentAdmissions.mu.Unlock()
	}()

	for {
		agentAdmissions.mu.Lock()
		if agentAdmissions.fitsLocked(owner) {
			agentAdmissions.takeLocked(owner)
			agentAdmissions.mu.Unlock()
			return acquireAgentGlobalWithLocal(waitCtx, agentAdmissions.releaseSlot(owner), agentrun.GlobalSlotHeld(waitCtx))
		}
		wait := agentAdmissions.notify
		agentAdmissions.mu.Unlock()

		select {
		case <-waitCtx.Done():
			return nil, false
		case <-wait:
		}
	}
}

func acquireAgentGlobalWithLocal(ctx context.Context, releaseLocal func(), globalAlreadyHeld bool) (func(), bool) {
	if globalAlreadyHeld {
		return releaseLocal, true
	}
	releaseGlobal, ok := agentrun.AcquireGlobalSlot(ctx)
	if !ok {
		releaseLocal()
		return nil, false
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			releaseGlobal()
			releaseLocal()
		})
	}, true
}

// agentAdmissionState is a read-only inspection helper for tests.
func agentAdmissionState() (active map[string]int, total, waiters int) {
	agentAdmissions.mu.Lock()
	defer agentAdmissions.mu.Unlock()
	active = make(map[string]int, len(agentAdmissions.active))
	for k, v := range agentAdmissions.active {
		active[k] = v
	}
	total = agentAdmissions.total
	for _, v := range agentAdmissions.waiters {
		waiters += v
	}
	return active, total, waiters
}

// resetAgentAdmissionsForTest clears state between tests.
func resetAgentAdmissionsForTest() {
	agentAdmissions.mu.Lock()
	defer agentAdmissions.mu.Unlock()
	agentAdmissions.active = make(map[string]int)
	agentAdmissions.waiters = make(map[string]int)
	agentAdmissions.total = 0
	agentAdmissions.notify = make(chan struct{})
}
