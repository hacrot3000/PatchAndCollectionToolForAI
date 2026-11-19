package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// agentRequestDigest binds an idempotency key to the exact user message so a
// reused key with different content is rejected instead of replaying.
func agentRequestDigest(message string) string {
	sum := sha256.Sum256([]byte(message))
	return hex.EncodeToString(sum[:])
}

// agentRequestRegistry tracks the ONE in-flight provider turn per session so
// cancellation can act on server state rather than a browser abort. It is
// process-local by design: after a restart no request can still be in flight,
// so a missing entry means "nothing to cancel here", not "cancelled".
type agentRequestRegistry struct {
	mu      sync.Mutex
	entries map[string]*agentRequestEntry
}

type agentRequestEntry struct {
	cancel context.CancelFunc
}

var agentRequests = agentRequestRegistry{entries: make(map[string]*agentRequestEntry)}

func agentRequestKey(workspace, owner, id string) string {
	return workspace + "\x00" + owner + "\x00" + id
}

// registerAgentRequest records the cancellation handle for an active turn.
// It returns a release function that must be called when the turn ends.
func registerAgentRequest(key string, cancel context.CancelFunc) func() {
	agentRequests.mu.Lock()
	agentRequests.entries[key] = &agentRequestEntry{cancel: cancel}
	agentRequests.mu.Unlock()
	return func() {
		agentRequests.mu.Lock()
		delete(agentRequests.entries, key)
		agentRequests.mu.Unlock()
	}
}

// cancelAgentRequest cancels only the request registered for this exact
// session identity. It reports whether an active request existed.
func cancelAgentRequest(workspace, owner, id string) bool {
	agentRequests.mu.Lock()
	entry, ok := agentRequests.entries[agentRequestKey(workspace, owner, id)]
	agentRequests.mu.Unlock()
	if !ok || entry == nil {
		return false
	}
	entry.cancel()
	return true
}

// agentRequestActive reports whether this session currently has a provider
// turn in flight in this process.
func agentRequestActive(workspace, owner, id string) bool {
	agentRequests.mu.Lock()
	defer agentRequests.mu.Unlock()
	_, ok := agentRequests.entries[agentRequestKey(workspace, owner, id)]
	return ok
}
