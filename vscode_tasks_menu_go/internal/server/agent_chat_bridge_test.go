package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

type fakeAgentNativeBridge struct {
	session.Service
	workspace       string
	meta            session.Metadata
	state           session.ProtocolState
	started         int
	selectedIndexes []int
}

func (b *fakeAgentNativeBridge) List() []session.Metadata {
	if b.started > 0 {
		return []session.Metadata{b.meta}
	}
	return nil
}
func (b *fakeAgentNativeBridge) Start(spec tasks.Execution) (session.Metadata, error) {
	b.started++
	if b.started > 1 {
		return session.Metadata{}, fmt.Errorf("native Patch started twice")
	}
	if !spec.ProtocolCommands || !spec.ProtocolEvents || spec.TaskID != -1 {
		return session.Metadata{}, fmt.Errorf("non-native Patch execution spec")
	}
	entries, err := os.ReadDir(filepath.Join(b.workspace, "patchs"))
	if err != nil {
		return session.Metadata{}, err
	}
	if len(entries) != 1 {
		return session.Metadata{}, fmt.Errorf("expected one queued agent package, got %d", len(entries))
	}
	approved := entries[0].Name()
	b.state = session.ProtocolState{
		Available: true, Enabled: true, CommandsEnabled: true,
	}
	prompt := map[string]any{
		"protocol": "taskdeck.patch", "version": 1, "type": "prompt", "prompt_id": "agent-prompt-1", "prompt_kind": "queue_selection",
		"actions": []string{"select", "cancel"},
		"items": []any{
			map[string]any{"index": 1, "name": "unrelated.zip", "kind": "PATCH", "selectable": true},
			map[string]any{"index": 2, "name": approved, "kind": "PATCH", "selectable": true},
		},
	}
	b.state.Prompt, _ = json.Marshal(prompt)
	b.meta = session.Metadata{ID: "native-agent-1", TaskID: -1, Kind: tasks.SessionKindPatch, Cwd: b.workspace, Label: "Patch Tool · Queue", Status: "running"}
	return b.meta, nil
}
func (b *fakeAgentNativeBridge) Metadata(id string) (session.Metadata, bool) {
	return b.meta, id == b.meta.ID
}
func (b *fakeAgentNativeBridge) ProtocolState(id string) (session.ProtocolState, error) {
	if id != b.meta.ID {
		return session.ProtocolState{}, fmt.Errorf("invalid session ID")
	}
	return b.state, nil
}
func (b *fakeAgentNativeBridge) ProtocolCommand(id string, data []byte) error {
	if id != b.meta.ID {
		return fmt.Errorf("unexpected session")
	}
	var cmd struct {
		Command string `json:"command"`
		Payload struct {
			PromptID string `json:"prompt_id"`
			Action   string `json:"action"`
			Indexes  []int  `json:"indexes"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &cmd); err != nil {
		return err
	}
	if cmd.Command != "prompt_response" || cmd.Payload.PromptID != "agent-prompt-1" || cmd.Payload.Action != "select" {
		return fmt.Errorf("native prompt command mismatch")
	}
	b.selectedIndexes = cmd.Payload.Indexes
	return nil
}
func TestAgentApprovedPatchRunsExactlyOneNativeQueueEntry(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	runtimeDir := t.TempDir()
	entry := filepath.Join(runtimeDir, "python_patch_entry.py")
	python := filepath.Join(runtimeDir, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)
	fake := &fakeAgentNativeBridge{workspace: s.Workspace}
	s.Sessions = fake
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	response := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
		map[string]any{"action": "approve", "confirm_execution": true, "expected_revision": pending.Revision})
	if response.Code != 200 {
		t.Fatalf("native dispatch failed: %d %s", response.Code, response.Body)
	}
	current, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if fake.started != 1 || len(fake.selectedIndexes) != 1 || fake.selectedIndexes[0] != 2 {
		t.Fatalf("wrong native patch launched: start=%d selection=%v", fake.started, fake.selectedIndexes)
	}
	if current.ToolCalls[0].Status != "running" || current.ToolCalls[0].PatchSessionID != fake.meta.ID {
		t.Fatalf("approved native run not linked to chat: %+v", current.ToolCalls[0])
	}
	retry := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
		map[string]any{"action": "approve", "confirm_execution": true, "expected_revision": current.Revision})
	if retry.Code != 409 || fake.started != 1 {
		t.Fatalf("replayed native execution: code=%d start=%d", retry.Code, fake.started)
	}
}

func TestAgentNativePatchCancelAndReconcileKeepSingleExecution(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	runtimeDir := t.TempDir()
	entry := filepath.Join(runtimeDir, "python_patch_entry.py")
	python := filepath.Join(runtimeDir, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)
	fake := &fakeAgentNativeBridge{workspace: s.Workspace}
	s.Sessions = fake
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
		map[string]any{"action": "approve", "confirm_execution": true, "expected_revision": pending.Revision}); w.Code != 200 {
		t.Fatalf("dispatch failed: %d %s", w.Code, w.Body)
	}
	current, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	// Cancelling the chat does NOT claim the native Patch execution stopped.
	cancel := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/cancel", map[string]any{})
	if cancel.Code != 200 || !strings.Contains(cancel.Body.String(), `"patch_execution_untouched":true`) {
		t.Fatalf("cancel misreported native execution state: %d %s", cancel.Code, cancel.Body)
	}
	if fake.started != 1 {
		t.Fatalf("cancel started another native run: %d", fake.started)
	}
	// While the native item has no final status, reconciliation must not invent one.
	rc := 0
	fake.state.Items = []session.ProtocolItemState{{Index: 2, Name: filepath.Base(current.ToolCalls[0].QueuePath), Kind: "PATCH", Status: "RUNNING"}}
	sync1 := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/sync", map[string]any{})
	if sync1.Code != 200 {
		t.Fatalf("sync failed: %d %s", sync1.Code, sync1.Body)
	}
	still, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if still.ToolCalls[0].Status != "running" {
		t.Fatalf("non-final item adopted: %s", still.ToolCalls[0].Status)
	}
	// Final evidence resolves it exactly once.
	fake.state.Items = []session.ProtocolItemState{{Index: 2, Name: filepath.Base(current.ToolCalls[0].QueuePath), Kind: "PATCH", Status: "FAIL", RC: &rc, RunID: "run-cancel-1"}}
	sync2 := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/sync", map[string]any{})
	if sync2.Code != 200 {
		t.Fatalf("final sync failed: %d %s", sync2.Code, sync2.Body)
	}
	done, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if done.ToolCalls[0].Status != "error" || done.ToolCalls[0].RunID != "run-cancel-1" {
		t.Fatalf("final evidence not adopted: %+v", done.ToolCalls[0])
	}
	// A second sync is a no-op, not a new run or a new ledger entry.
	before := done.Revision
	again := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/sync", map[string]any{})
	if again.Code != 200 {
		t.Fatalf("no-op sync failed: %d", again.Code)
	}
	after, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before || len(after.ToolCalls) != 1 || fake.started != 1 {
		t.Fatalf("reconciliation duplicated work: revision %d→%d calls=%d started=%d", before, after.Revision, len(after.ToolCalls), fake.started)
	}
}

func TestAgentConcurrentApprovalsStartOneNativeExecution(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	runtimeDir := t.TempDir()
	entry := filepath.Join(runtimeDir, "python_patch_entry.py")
	python := filepath.Join(runtimeDir, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)
	fake := &fakeAgentNativeBridge{workspace: s.Workspace}
	s.Sessions = fake
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	const attempts = 6
	var wg sync.WaitGroup
	codes := make(chan int, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
				map[string]any{"action": "approve", "confirm_execution": true, "expected_revision": pending.Revision})
			codes <- w.Code
		}()
	}
	wg.Wait()
	close(codes)
	ok := 0
	for code := range codes {
		if code == 200 {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("expected exactly one winning approval, got %d", ok)
	}
	if fake.started != 1 {
		t.Fatalf("native execution started %d times", fake.started)
	}
}
