package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProtocolCommandJournalRestoresLiveQueuedRunningAndFinishedRows(t *testing.T) {
	root := t.TempDir()
	runID := "1795061800896644_898883"
	folder := filepath.Join(root, "artifacts", "patch_tool", "runs", runID,
		"command_items", "001_example", "command_logs")
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	state := ProtocolState{Items: []ProtocolItemState{{
		Index: 1, Total: 1, RunID: runID, Name: "example.zip",
		Kind: "PATCH", Status: "RUNNING",
	}}}
	writeJournal := func(status string, elapsed float64) {
		t.Helper()
		journal := map[string]interface{}{
			"format": "taskdeck.patch.command-states.v1",
			"commands": []map[string]interface{}{{
				"index": 1, "name": "Compile", "stage": "POST PATCH",
				"argv": []string{"sleep", "5"}, "status": status,
				"started_at": "2026-11-19T10:00:00Z",
				"elapsed_seconds": elapsed,
				"log_path": "artifacts/patch_tool/runs/" + runID + "/command_items/001_example/command_logs/001_post_patch.log",
			}},
		}
		raw, err := json.Marshal(journal)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "command_states.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeJournal("QUEUE", 0)
	applyCommandJournalToState(root, &state)
	if len(state.Items[0].Commands) != 1 || state.Items[0].Commands[0].Status != "QUEUE" {
		t.Fatalf("queued row missing from current run: %#v", state.Items)
	}
	writeJournal("RUNNING", 1.5)
	// Read through the same Manager.ProtocolState method used by the HTTP
	// endpoint, not only the isolated journal helper.
	manager := NewManager(0)
	manager.sessions["active"] = &managedSession{
		meta: Metadata{Cwd: root},
		protocol: ProtocolState{Items: append([]ProtocolItemState(nil), state.Items...)},
	}
	snapshot, err := manager.ProtocolState("active")
	if err != nil || len(snapshot.Items) != 1 || len(snapshot.Items[0].Commands) != 1 ||
		snapshot.Items[0].Commands[0].Status != "RUNNING" {
		t.Fatalf("live protocol API snapshot omitted RUNNING journal: %#v, %v", snapshot.Items, err)
	}
	applyCommandJournalToState(root, &state)
	if state.Items[0].Commands[0].Status != "RUNNING" {
		t.Fatalf("running row was not refreshed: %#v", state.Items)
	}
	writeJournal("PASS", 5.1)
	applyCommandJournalToState(root, &state)
	if state.Items[0].Commands[0].Status != "PASS" ||
		state.Items[0].Commands[0].ElapsedSeconds == nil ||
		*state.Items[0].Commands[0].ElapsedSeconds != 5.1 {
		t.Fatalf("completed command and duration missing: %#v", state.Items)
	}
	// A new/late terminal snapshot may carry no command array; the journal
	// must still restore completed rows to the Current run panel.
	state.Items[0].Status = "PASS"
	state.Items[0].Commands = nil
	applyCommandJournalToState(root, &state)
	if len(state.Items[0].Commands) != 1 || state.Items[0].Commands[0].Status != "PASS" {
		t.Fatalf("finished current run lost command rows: %#v", state.Items)
	}
	// Abrupt exit may leave the journal's most recent state as RUNNING; it
	// must never look running after the parent item has completed.
	writeJournal("RUNNING", 2)
	state.Items[0].Commands = nil
	applyCommandJournalToState(root, &state)
	if len(state.Items[0].Commands) != 1 || state.Items[0].Commands[0].Status != "SKIPPED" {
		t.Fatalf("stale running state revived under successful parent: %#v", state.Items)
	}
	state.Items[0].Status = "FAIL"
	state.Items[0].Commands = nil
	applyCommandJournalToState(root, &state)
	if len(state.Items[0].Commands) != 1 || state.Items[0].Commands[0].Status != "INTERRUPTED" {
		t.Fatalf("stale running state revived under failed parent: %#v", state.Items)
	}
}

func TestProtocolCommandJournalRejectsInvalidPathsAndOversizedData(t *testing.T) {
	root := t.TempDir()
	state := ProtocolState{Items: []ProtocolItemState{{
		Index: 1, RunID: "safe_run", Name: "safe.zip", Kind: "PATCH", Status: "RUNNING",
	}}}
	folder := filepath.Join(root, "artifacts", "patch_tool", "runs", "safe_run", "command_items", "001_safe", "command_logs")
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(folder, "command_states.json")
	if err := os.WriteFile(file, make([]byte, 65537), 0600); err != nil {
		t.Fatal(err)
	}
	applyCommandJournalToState(root, &state)
	if len(state.Items[0].Commands) != 0 {
		t.Fatal("oversized journal accepted")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "other_file.json"), file); err == nil {
		applyCommandJournalToState(root, &state)
		if len(state.Items[0].Commands) != 0 {
			t.Fatal("symlinked command journal accepted")
		}
	}
}
