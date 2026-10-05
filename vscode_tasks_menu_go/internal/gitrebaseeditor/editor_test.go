package gitrebaseeditor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSequenceEditorCopiesPreparedTodo(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "prepared.todo")
	target := filepath.Join(dir, "git-rebase-todo")
	want := "pick " + strings.Repeat("a", 40) + " first\nreword " + strings.Repeat("b", 40) + " second\n"
	if err := os.WriteFile(source, []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunSequenceEditor(target, source); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("todo=%q want=%q", data, want)
	}
}

func TestRunMessageEditorAppliesRewordByOriginalSHA(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	rebaseDir := filepath.Join(gitDir, "rebase-merge")
	if err := os.MkdirAll(rebaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	full := strings.Repeat("c", 40)
	if err := os.WriteFile(filepath.Join(rebaseDir, "done"), []byte("reword "+full+" subject\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(gitDir, "taskdeck-rebase-editor-state.json")
	if err := WriteState(statePath, State{GitDir: gitDir, Reword: map[string]string{full: "rewritten subject"}}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "COMMIT_EDITMSG")
	if err := os.WriteFile(target, []byte("original message\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunMessageEditor(target, statePath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "rewritten subject\n" {
		t.Fatalf("message=%q", data)
	}
}

func TestRunMessageEditorMatchesAbbreviatedDoneSHA(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	rebaseDir := filepath.Join(gitDir, "rebase-merge")
	if err := os.MkdirAll(rebaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	full := "1234567890abcdef1234567890abcdef12345678"
	if err := os.WriteFile(filepath.Join(rebaseDir, "done"), []byte("reword "+full[:12]+" subject\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(gitDir, "state.json")
	if err := WriteState(statePath, State{GitDir: gitDir, Reword: map[string]string{full: "short-sha rewrite"}}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "message")
	if err := os.WriteFile(target, []byte("prepared\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunMessageEditor(target, statePath); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "short-sha rewrite\n" {
		t.Fatalf("message=%q", data)
	}
}

func TestRunMessageEditorPreservesPreparedMessageWithoutOverride(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	rebaseDir := filepath.Join(gitDir, "rebase-merge")
	if err := os.MkdirAll(rebaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("d", 40)
	if err := os.WriteFile(filepath.Join(rebaseDir, "done"), []byte("squash "+sha+" subject\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(gitDir, "state.json")
	if err := WriteState(statePath, State{GitDir: gitDir, Reword: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "message")
	if err := os.WriteFile(target, []byte("Git prepared squash message\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunMessageEditor(target, statePath); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "Git prepared squash message\n" {
		t.Fatalf("message unexpectedly changed: %q", data)
	}
}

func TestRunAutoModeRoutesTodoByFilename(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "prepared.todo")
	target := filepath.Join(dir, "git-rebase-todo")
	if err := os.WriteFile(source, []byte("pick abc subject\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(TodoPathEnv, source)
	if err := Run("auto", []string{target}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "pick abc subject\n" {
		t.Fatalf("todo=%q", data)
	}
}
