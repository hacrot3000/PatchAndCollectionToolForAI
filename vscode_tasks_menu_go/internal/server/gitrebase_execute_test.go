package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/gitrebaseeditor"
)

func TestGitRebaseEditorHelperProcess(t *testing.T) {
	if os.Getenv(gitrebaseeditor.EditorModeEnv) != "1" {
		return
	}
	index := -1
	for i, arg := range os.Args {
		if arg == "--" {
			index = i
			break
		}
	}
	if index < 0 {
		os.Stderr.WriteString("missing helper separator")
		os.Exit(2)
	}
	if err := gitrebaseeditor.Run("auto", os.Args[index+1:]); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Exit(2)
	}
	os.Exit(0)
}

func configureGitRebaseTestEditor(s *Server) {
	s.GitRebaseEditorCommand = gitEditorCommand(os.Args[0]) + " -test.run=^TestGitRebaseEditorHelperProcess$ --"
}

func commitFileForRebase(t *testing.T, workspace, name, content, message string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "--", name)
	gitQuickRun(t, workspace, "commit", "-m", message)
	return gitQuickRun(t, workspace, "rev-parse", "HEAD")
}

func interactiveRebaseBody(t *testing.T, branch, base, head string, plan []gitRebaseActionItem) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"action":           "interactive_rebase",
		"ref":              base,
		"expected_sha":     head,
		"expected_current": branch,
		"rebase_plan":      plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func TestInteractiveRebaseExecutesReorderRewordSquashFixupAndDrop(t *testing.T) {
	workspace, s, branch := setupGitQuickRepo(t)
	configureGitRebaseTestEditor(s)
	base := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	a := commitFileForRebase(t, workspace, "a.txt", "A\n", "A original")
	b := commitFileForRebase(t, workspace, "b.txt", "B\n", "B original")
	c := commitFileForRebase(t, workspace, "c.txt", "C\n", "C original")
	d := commitFileForRebase(t, workspace, "d.txt", "D\n", "D original")
	e := commitFileForRebase(t, workspace, "e.txt", "E\n", "E original")
	head := e

	plan := []gitRebaseActionItem{
		{SHA: b, Action: "pick"},
		{SHA: a, Action: "reword", Message: "A rewritten"},
		{SHA: c, Action: "squash"},
		{SHA: d, Action: "fixup"},
		{SHA: e, Action: "drop"},
	}
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", interactiveRebaseBody(t, branch, base, head, plan))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("interactive rebase status=%d body=%s", rr.Code, rr.Body.String())
	}
	if state, err := s.gitOperationState(withGitRepository(t.Context(), gitRepository{ID: ".", Root: workspace})); err != nil || state != "" {
		t.Fatalf("rebase state=%q err=%v", state, err)
	}
	subjects := gitQuickRun(t, workspace, "log", "--reverse", "--format=%s", base+"..HEAD")
	lines := strings.Split(subjects, "\n")
	if len(lines) != 2 || lines[0] != "B original" || lines[1] != "A rewritten" {
		t.Fatalf("subjects=%q", subjects)
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
		if _, err := os.Stat(filepath.Join(workspace, name)); err != nil {
			t.Fatalf("%s missing after rebase: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "e.txt")); !os.IsNotExist(err) {
		t.Fatalf("dropped file still exists err=%v", err)
	}
	gitDir := gitQuickRun(t, workspace, "rev-parse", "--absolute-git-dir")
	for _, name := range []string{"taskdeck-rebase-todo", "taskdeck-rebase-editor-state.json"} {
		if _, err := os.Stat(filepath.Join(gitDir, name)); !os.IsNotExist(err) {
			t.Fatalf("temporary rebase state %s remains err=%v", name, err)
		}
	}
}

func TestInteractiveRebaseEditPausesThenRecoveryContinueCompletes(t *testing.T) {
	workspace, s, branch := setupGitQuickRepo(t)
	configureGitRebaseTestEditor(s)
	base := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	a := commitFileForRebase(t, workspace, "a.txt", "A\n", "A")
	b := commitFileForRebase(t, workspace, "b.txt", "B\n", "B")
	head := b

	plan := []gitRebaseActionItem{{SHA: a, Action: "edit"}, {SHA: b, Action: "pick"}}
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", interactiveRebaseBody(t, branch, base, head, plan))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("edit rebase status=%d body=%s", rr.Code, rr.Body.String())
	}
	ctx := withGitRepository(t.Context(), gitRepository{ID: ".", Root: workspace})
	if state, err := s.gitOperationState(ctx); err != nil || state != "rebase" {
		t.Fatalf("expected paused rebase state=%q err=%v body=%s", state, err, rr.Body.String())
	}

	cont := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"continue_in_progress"}`)
	if cont.Code != http.StatusOK || !strings.Contains(cont.Body.String(), `"ok":true`) {
		t.Fatalf("continue status=%d body=%s", cont.Code, cont.Body.String())
	}
	if state, err := s.gitOperationState(ctx); err != nil || state != "" {
		t.Fatalf("rebase still active state=%q err=%v", state, err)
	}
	if got := gitQuickRun(t, workspace, "log", "--reverse", "--format=%s", base+"..HEAD"); got != "A\nB" {
		t.Fatalf("history=%q", got)
	}
}

func TestInteractiveRebaseConflictCanAbortAndRestoresOriginalHEAD(t *testing.T) {
	workspace, s, branch := setupGitQuickRepo(t)
	configureGitRebaseTestEditor(s)
	base := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "commit", "-am", "A modifies tracked")
	a := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "commit", "-am", "B modifies tracked")
	b := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	originalHead := b

	plan := []gitRebaseActionItem{{SHA: b, Action: "pick"}, {SHA: a, Action: "pick"}}
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", interactiveRebaseBody(t, branch, base, originalHead, plan))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":false`) {
		t.Fatalf("conflicting rebase status=%d body=%s", rr.Code, rr.Body.String())
	}
	ctx := withGitRepository(t.Context(), gitRepository{ID: ".", Root: workspace})
	if state, err := s.gitOperationState(ctx); err != nil || state != "rebase" {
		t.Fatalf("expected conflict rebase state=%q err=%v body=%s", state, err, rr.Body.String())
	}

	abort := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"abort_in_progress"}`)
	if abort.Code != http.StatusOK || !strings.Contains(abort.Body.String(), `"ok":true`) {
		t.Fatalf("abort status=%d body=%s", abort.Code, abort.Body.String())
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != originalHead {
		t.Fatalf("HEAD=%q want original %q", got, originalHead)
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "tracked.txt")); err != nil || string(data) != "B\n" {
		t.Fatalf("working file=%q err=%v", data, err)
	}
	if state, err := s.gitOperationState(ctx); err != nil || state != "" {
		t.Fatalf("rebase state after abort=%q err=%v", state, err)
	}
}

func TestInteractiveRebaseRejectsDirtyAndStalePlanWithoutMutation(t *testing.T) {
	workspace, s, branch := setupGitQuickRepo(t)
	configureGitRebaseTestEditor(s)
	base := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	a := commitFileForRebase(t, workspace, "a.txt", "A\n", "A")
	head := a
	plan := []gitRebaseActionItem{{SHA: a, Action: "pick"}}

	stale := strings.Repeat("f", len(head))
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", interactiveRebaseBody(t, branch, base, stale, plan))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "HEAD changed after planning") {
		t.Fatalf("stale status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != head {
		t.Fatalf("stale plan moved HEAD=%q", got)
	}

	if err := os.WriteFile(filepath.Join(workspace, "a.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", interactiveRebaseBody(t, branch, base, head, plan))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "working tree and index must be clean") {
		t.Fatalf("dirty status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != head {
		t.Fatalf("dirty plan moved HEAD=%q", got)
	}
}
