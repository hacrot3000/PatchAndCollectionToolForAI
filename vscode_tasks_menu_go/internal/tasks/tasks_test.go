package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTasksFile(t *testing.T, workspace, body string) {
	t.Helper()
	dir := filepath.Join(workspace, ".vscode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tasks.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func taskIDByLabel(t *testing.T, items []Task, label string) int {
	t.Helper()
	for _, item := range items {
		if item.Label == label {
			return item.ID
		}
	}
	t.Fatalf("task %q not found", label)
	return 0
}

func TestLoadStableIDAcrossInsertAndReorder(t *testing.T) {
	workspace := t.TempDir()
	writeTasksFile(t, workspace, `{
		"version": "2.0.0",
		"tasks": [
			{"label":"A","type":"shell","command":"echo A"},
			{"label":"B","type":"shell","command":"echo B"}
		]
	}`)
	first, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	idA := taskIDByLabel(t, first, "A")
	idB := taskIDByLabel(t, first, "B")

	writeTasksFile(t, workspace, `{
		"version": "2.0.0",
		"tasks": [
			{"label":"NEW","type":"shell","command":"echo NEW"},
			{"label":"B","type":"shell","command":"echo B"},
			{"label":"A","type":"shell","command":"echo A"}
		]
	}`)
	second, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if got := taskIDByLabel(t, second, "A"); got != idA {
		t.Fatalf("A id changed after reorder/insert: got %d want %d", got, idA)
	}
	if got := taskIDByLabel(t, second, "B"); got != idB {
		t.Fatalf("B id changed after reorder/insert: got %d want %d", got, idB)
	}
}

func TestLoadChangesIDWhenDefinitionChanges(t *testing.T) {
	workspace := t.TempDir()
	writeTasksFile(t, workspace, `{"tasks":[{"label":"A","type":"shell","command":"echo old"}]}`)
	before, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	oldID := taskIDByLabel(t, before, "A")

	writeTasksFile(t, workspace, `{"tasks":[{"label":"A","type":"shell","command":"echo new"}]}`)
	after, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if newID := taskIDByLabel(t, after, "A"); newID == oldID {
		t.Fatalf("task definition changed but id stayed %d", oldID)
	}
}

func TestLoadJSONCCommentsURLsAndTrailingCommas(t *testing.T) {
	workspace := t.TempDir()
	writeTasksFile(t, workspace, `{
		// VS Code JSONC comment
		"version": "2.0.0",
		"tasks": [
			{
				"label": "URL task",
				"type": "shell",
				"command": "printf https://example.test/a//b",
				"args": ["/* literal */",],
			},
		],
	}`)
	items, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d tasks, want 1", len(items))
	}
	if items[0].Command != "printf https://example.test/a//b" {
		t.Fatalf("URL/string content was damaged: %#v", items[0].Command)
	}
}
