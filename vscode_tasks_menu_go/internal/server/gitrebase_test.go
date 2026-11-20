package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func setupLinearRebaseRepo(t *testing.T) (string, *Server, string, string, string) {
	t.Helper()
	workspace, s, _ := setupGitQuickRepo(t)
	base := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "commit", "-am", "second commit")
	second := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("three\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "commit", "-am", "third commit")
	head := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	return workspace, s, base, second, head
}

func TestGitRebasePlanReturnsOldestToNewestWithSHAContract(t *testing.T) {
	_, s, base, second, head := setupLinearRebaseRepo(t)
	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=rebase-plan&base="+base, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("rebase plan status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Branch          string                `json:"branch"`
		BaseRef         string                `json:"base_ref"`
		BaseSHA         string                `json:"base_sha"`
		HeadSHA         string                `json:"head_sha"`
		Clean           bool                  `json:"clean"`
		HasMergeCommits bool                  `json:"has_merge_commits"`
		Commits         []gitRebasePlanCommit `json:"commits"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Branch == "" || got.BaseRef != base || got.BaseSHA != base || got.HeadSHA != head || !got.Clean || got.HasMergeCommits {
		t.Fatalf("plan=%+v", got)
	}
	if len(got.Commits) != 2 || got.Commits[0].SHA != second || got.Commits[1].SHA != head {
		t.Fatalf("commit order=%+v", got.Commits)
	}
	if got.Commits[0].Subject != "second commit" || got.Commits[1].Subject != "third commit" {
		t.Fatalf("subjects=%+v", got.Commits)
	}
}

func TestGitRebasePlanReportsDirtyWithoutMutating(t *testing.T) {
	workspace, s, base, _, head := setupLinearRebaseRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=rebase-plan&base="+base, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("dirty plan status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Clean   bool   `json:"clean"`
		HeadSHA string `json:"head_sha"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Clean || got.HeadSHA != head {
		t.Fatalf("dirty plan=%+v", got)
	}
	if current := gitQuickRun(t, workspace, "rev-parse", "HEAD"); current != head {
		t.Fatalf("planner mutated HEAD=%q want=%q", current, head)
	}
}

func TestGitRebasePlanRejectsDetachedEqualAndNonAncestorBase(t *testing.T) {
	workspace, s, base, _, head := setupLinearRebaseRepo(t)

	equal := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=rebase-plan&base="+head, "")
	if equal.Code != http.StatusBadRequest {
		t.Fatalf("equal base status=%d body=%s", equal.Code, equal.Body.String())
	}

	gitQuickRun(t, workspace, "switch", "-c", "other", base)
	if err := os.WriteFile(filepath.Join(workspace, "other.txt"), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "other.txt")
	gitQuickRun(t, workspace, "commit", "-m", "other commit")
	other := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	gitQuickRun(t, workspace, "switch", "-")
	nonAncestor := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=rebase-plan&base="+other, "")
	if nonAncestor.Code != http.StatusBadRequest {
		t.Fatalf("non-ancestor status=%d body=%s", nonAncestor.Code, nonAncestor.Body.String())
	}

	gitQuickRun(t, workspace, "checkout", "--detach", "HEAD")
	detached := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=rebase-plan&base="+base, "")
	if detached.Code != http.StatusConflict {
		t.Fatalf("detached status=%d body=%s", detached.Code, detached.Body.String())
	}
}

func TestGitRebasePlanFlagsMergeCommitRange(t *testing.T) {
	workspace, s, mainBranch := setupGitQuickRepo(t)
	base := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	gitQuickRun(t, workspace, "switch", "-c", "feature/rebase-merge")
	if err := os.WriteFile(filepath.Join(workspace, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "feature.txt")
	gitQuickRun(t, workspace, "commit", "-m", "feature commit")
	gitQuickRun(t, workspace, "switch", mainBranch)
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "main.txt")
	gitQuickRun(t, workspace, "commit", "-m", "main commit")
	gitQuickRun(t, workspace, "merge", "--no-ff", "-m", "merge feature for rebase", "feature/rebase-merge")

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=rebase-plan&base="+base, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("merge plan status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		HasMergeCommits bool                  `json:"has_merge_commits"`
		Commits         []gitRebasePlanCommit `json:"commits"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.HasMergeCommits {
		t.Fatalf("merge range not flagged: %+v", got.Commits)
	}
	found := false
	for _, row := range got.Commits {
		if row.Merge && row.Subject == "merge feature for rebase" {
			found = true
		}
	}
	if !found {
		t.Fatalf("merge commit not identified: %+v", got.Commits)
	}
}
