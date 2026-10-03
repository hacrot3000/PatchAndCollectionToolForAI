package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupMergeConflictRepo(t *testing.T) (string, *Server, string, string) {
	t.Helper()
	workspace, s, mainBranch := setupGitQuickRepo(t)
	gitQuickRun(t, workspace, "switch", "-c", "feature/conflict")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("feature side\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "tracked.txt")
	gitQuickRun(t, workspace, "commit", "-m", "feature change")
	gitQuickRun(t, workspace, "switch", mainBranch)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("main side\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "tracked.txt")
	gitQuickRun(t, workspace, "commit", "-m", "main change")
	return workspace, s, mainBranch, "feature/conflict"
}

func startMergeConflict(t *testing.T, workspace, branch string) {
	t.Helper()
	cmd := exec.Command("git", "merge", "--no-edit", branch)
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected merge conflict, got success: %s", out)
	}
	if !strings.Contains(string(out), "CONFLICT") {
		t.Fatalf("merge did not report conflict: %s", out)
	}
}

func TestGitConflictStateReportsStagesAndProjectPath(t *testing.T) {
	workspace, s, _, feature := setupMergeConflictRepo(t)
	startMergeConflict(t, workspace, feature)

	state, err := s.gitConflictState(context.Background())
	if err != nil { t.Fatal(err) }
	if state.Operation != "merge" || len(state.Files) != 1 {
		t.Fatalf("state=%+v", state)
	}
	file := state.Files[0]
	if file.Path != "tracked.txt" || file.ProjectPath != "tracked.txt" || file.Status != "UU" {
		t.Fatalf("file=%+v", file)
	}
	if !file.HasBase || !file.HasCurrent || !file.HasIncoming {
		t.Fatalf("expected base/current/incoming stages: %+v", file)
	}
}

func TestGitConflictResolveCurrentAndIncoming(t *testing.T) {
	for _, tc := range []struct{
		side string
		want string
	}{
		{"current", "main side\n"},
		{"incoming", "feature side\n"},
	} {
		t.Run(tc.side, func(t *testing.T) {
			workspace, s, _, feature := setupMergeConflictRepo(t)
			startMergeConflict(t, workspace, feature)
			out, _, err := s.gitResolveConflictSide(context.Background(), "tracked.txt", tc.side)
			if err != nil { t.Fatalf("resolve: %v\n%s", err, out) }
			data, err := os.ReadFile(filepath.Join(workspace, "tracked.txt"))
			if err != nil { t.Fatal(err) }
			if string(data) != tc.want { t.Fatalf("content=%q want=%q", data, tc.want) }
			state, err := s.gitConflictState(context.Background())
			if err != nil { t.Fatal(err) }
			if state.Operation != "merge" || len(state.Files) != 0 {
				t.Fatalf("state after resolution=%+v", state)
			}
		})
	}
}

func TestGitConflictMarkEditedFileResolved(t *testing.T) {
	workspace, s, _, feature := setupMergeConflictRepo(t)
	startMergeConflict(t, workspace, feature)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("manual merged result\n"), 0o644); err != nil { t.Fatal(err) }
	out, _, err := s.gitMarkConflictResolved(context.Background(), "tracked.txt")
	if err != nil { t.Fatalf("mark resolved: %v\n%s", err, out) }
	state, err := s.gitConflictState(context.Background())
	if err != nil { t.Fatal(err) }
	if len(state.Files) != 0 { t.Fatalf("state=%+v", state) }
	if got := gitQuickRun(t, workspace, "show", ":tracked.txt"); got != "manual merged result" {
		t.Fatalf("staged content=%q", got)
	}
}

func TestGitMergeActionReturnsConflictDetails(t *testing.T) {
	_, s, mainBranch, feature := setupMergeConflictRepo(t)
	body := `{"action":"merge","branch":"feature/conflict","source":"local","expected_current":"`+mainBranch+`"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
	var response struct {
		OK bool `json:"ok"`
		FailureCode string `json:"failure_code"`
		ConflictState gitConflictState `json:"conflict_state"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil { t.Fatal(err) }
	if response.OK || response.FailureCode != "conflicts" {
		t.Fatalf("response=%+v body=%s", response, rr.Body.String())
	}
	if response.ConflictState.Operation != "merge" || len(response.ConflictState.Files) != 1 || response.ConflictState.Files[0].Path != "tracked.txt" {
		t.Fatalf("conflict_state=%+v", response.ConflictState)
	}
}

func TestGitConflictRepairResponseRefreshesState(t *testing.T) {
	workspace, s, _, feature := setupMergeConflictRepo(t)
	startMergeConflict(t, workspace, feature)
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"conflict_take_side","path":"tracked.txt","conflict_side":"current"}`)
	if rr.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
	var response struct {
		OK bool `json:"ok"`
		ConflictState gitConflictState `json:"conflict_state"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil { t.Fatal(err) }
	if !response.OK || response.ConflictState.Operation != "merge" || len(response.ConflictState.Files) != 0 {
		t.Fatalf("response=%+v body=%s", response, rr.Body.String())
	}
}

func TestGitResolveAllConflictsRequiresConfirmationThroughRepair(t *testing.T) {
	workspace, s, _, feature := setupMergeConflictRepo(t)
	startMergeConflict(t, workspace, feature)

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"conflict_resolve_all","conflict_side":"current"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "explicit confirmation") {
		t.Fatalf("unconfirmed status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"conflict_resolve_all","conflict_side":"current","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("confirmed status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestClassifyMergeToConflictSeparately(t *testing.T) {
	text := "CONFLICT (content): Merge conflict in tracked.txt\nAutomatic merge failed; fix conflicts and then commit the result."
	if got := classifyGitFailure("merge_to", text, ""); got != "merge_to_conflicts" {
		t.Fatalf("merge_to classify=%q", got)
	}
	if got := classifyGitFailure("merge", text, ""); got != "conflicts" {
		t.Fatalf("merge classify=%q", got)
	}
}

func TestMergeToPrepareResolutionCreatesConflictBranch(t *testing.T) {
	workspace, s, mainBranch, feature := setupMergeConflictRepo(t)
	source := gitQuickRun(t, workspace, "rev-parse", feature)
	target := gitQuickRun(t, workspace, "rev-parse", mainBranch)
	req := gitActionRequest{
		NewBranch: "taskdeck/resolve-test",
		ExpectedSourceSHA: source,
		ExpectedTargetSHA: target,
		Confirmed: true,
	}
	output, _, err := s.gitMergeToPrepareResolution(context.Background(), req)
	if err == nil {
		t.Fatalf("expected conflict, output=%s", output)
	}
	if got := gitQuickRun(t, workspace, "branch", "--show-current"); got != "taskdeck/resolve-test" {
		t.Fatalf("branch=%q", got)
	}
	state, stateErr := s.gitConflictState(context.Background())
	if stateErr != nil { t.Fatal(stateErr) }
	if state.Operation != "merge" || len(state.Files) != 1 {
		t.Fatalf("state=%+v output=%s err=%v", state, output, err)
	}
	if !strings.Contains(output, "Created local Merge To resolution branch") {
		t.Fatalf("output=%q", output)
	}
}

func TestNestedRepositoryConflictProjectPath(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "projects", "client")
	if err := os.MkdirAll(nested, 0o755); err != nil { t.Fatal(err) }
	gitQuickRun(t, nested, "init")
	gitQuickRun(t, nested, "config", "user.name", "TaskDeck Test")
	gitQuickRun(t, nested, "config", "user.email", "taskdeck@example.invalid")
	if err := os.WriteFile(filepath.Join(nested, "a.txt"), []byte("base\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, nested, "add", "a.txt")
	gitQuickRun(t, nested, "commit", "-m", "base")
	mainBranch := gitQuickRun(t, nested, "branch", "--show-current")
	gitQuickRun(t, nested, "switch", "-c", "other")
	if err := os.WriteFile(filepath.Join(nested, "a.txt"), []byte("other\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, nested, "commit", "-am", "other")
	gitQuickRun(t, nested, "switch", mainBranch)
	if err := os.WriteFile(filepath.Join(nested, "a.txt"), []byte("main\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, nested, "commit", "-am", "main")
	startMergeConflict(t, nested, "other")

	ctx := withGitRepository(context.Background(), "projects/client")
	state, err := (&Server{Workspace: root}).gitConflictState(ctx)
	if err != nil { t.Fatal(err) }
	if len(state.Files) != 1 || state.Files[0].ProjectPath != "projects/client/a.txt" {
		t.Fatalf("state=%+v", state)
	}
}
