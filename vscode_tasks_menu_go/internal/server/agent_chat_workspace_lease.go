package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// agentWorkspaceLeases serializes Agent Patch side effects per canonical
// workspace. The in-process map rejects concurrent goroutines; the OS
// directory lock (where implemented) also excludes a second TaskDeck daemon.
//
// Scope limitation (deliberate, see docs/taskdeck_ai_agent/06-parallel-remote.md):
// the native Patch Panel does not take this lock, so this is NOT by itself a
// guarantee against a concurrent manual patch. HEAD/branch/worktree digests
// recorded at proposal and approval time remain the additional guard, and an
// unverifiable state is parked as UNKNOWN instead of being re-executed.
var agentWorkspaceLeases = struct {
	sync.Mutex
	active map[string]bool
}{active: make(map[string]bool)}

var errAgentWorkspaceBusy = errors.New("another Agent execution owns this workspace")

func agentReserveWorkspaceExecution(ctx context.Context, workspace string) (func(), error) {
	if ctx == nil || ctx.Err() != nil || workspace == "" {
		return nil, errAgentWorkspaceBusy
	}
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return nil, errAgentWorkspaceBusy
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, errAgentWorkspaceBusy
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, errAgentWorkspaceBusy
	}
	agentWorkspaceLeases.Lock()
	if agentWorkspaceLeases.active[root] {
		agentWorkspaceLeases.Unlock()
		return nil, errAgentWorkspaceBusy
	}
	agentWorkspaceLeases.active[root] = true
	agentWorkspaceLeases.Unlock()
	releaseOS, err := acquireRemoteWorkspaceOSLock(root, info)
	if err != nil {
		agentWorkspaceLeases.Lock()
		delete(agentWorkspaceLeases.active, root)
		agentWorkspaceLeases.Unlock()
		return nil, errAgentWorkspaceBusy
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			releaseOS()
			agentWorkspaceLeases.Lock()
			delete(agentWorkspaceLeases.active, root)
			agentWorkspaceLeases.Unlock()
		})
	}, nil
}
