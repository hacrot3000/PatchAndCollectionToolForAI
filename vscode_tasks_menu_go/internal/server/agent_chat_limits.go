package server

import (
	"sync"
	"time"
)

// Agent Chat concurrency and backpressure. These are process-local admissions
// intended to prevent runaway provider spend and workspace contention; they
// are NOT a distributed quota. In multi-daemon deployments the durable journal
// plus workspace lease remain the source of truth, and the documented policy
// is one daemon per workspace.
const (
	agentMaxConcurrentPerOwner = 2
	agentMaxConcurrentGlobal   = 4
	agentMaxQueuedPerOwner     = 8
)

// agentQueueTimeout is a variable so tests can shorten the bounded wait
// without leaving goroutines alive for the production timeout.
var agentQueueTimeout = 2 * time.Minute

type agentAdmission struct {
	mu      sync.Mutex
	active  map[string]int
	total   int
	waiters map[string]int
}

var agentAdmissions = agentAdmission{active: make(map[string]int), waiters: make(map[string]int)}

// acquireAgentSlot admits one model turn with bounded waiting and explicit
// backpressure. It returns false when the owner/global limit stays saturated
// for agentQueueTimeout or when the queued-waiter cap is exceeded, so callers
// can answer 429 rather than queue unbounded work.
func acquireAgentSlot(owner string) (func(), bool) {
	if owner == "" {
		return func() {}, true
	}
	deadline := time.Now().Add(agentQueueTimeout)
	for {
		agentAdmissions.mu.Lock()
		if agentAdmissions.total < agentMaxConcurrentGlobal && agentAdmissions.active[owner] < agentMaxConcurrentPerOwner {
			agentAdmissions.active[owner]++
			agentAdmissions.total++
			agentAdmissions.mu.Unlock()
			return func() {
				agentAdmissions.mu.Lock()
				if agentAdmissions.active[owner] > 0 {
					agentAdmissions.active[owner]--
					if agentAdmissions.active[owner] == 0 {
						delete(agentAdmissions.active, owner)
					}
					agentAdmissions.total--
				}
				agentAdmissions.mu.Unlock()
			}, true
		}
		if agentAdmissions.waiters[owner] >= agentMaxQueuedPerOwner || time.Now().After(deadline) {
			agentAdmissions.mu.Unlock()
			return nil, false
		}
		agentAdmissions.waiters[owner]++
		agentAdmissions.mu.Unlock()
		time.Sleep(25 * time.Millisecond)
		agentAdmissions.mu.Lock()
		agentAdmissions.waiters[owner]--
		if agentAdmissions.waiters[owner] <= 0 {
			delete(agentAdmissions.waiters, owner)
		}
		agentAdmissions.mu.Unlock()
	}
}
