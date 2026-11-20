package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitSemanticsRepo(t *testing.T) (string, *Server, string, string) {
	t.Helper()
	workspace, s, _ := setupGitQuickRepo(t)
	base := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "history.txt"), []byte("history\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "-A")
	gitQuickRun(t, workspace, "commit", "-m", "second")
	head := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	return workspace, s, base, head
}

func TestGitSemanticsPreviewReportsHeadTargetAndThreeStateChanges(t *testing.T) {
	workspace, s, base, head := setupGitSemanticsRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("worktree local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "staged.txt")
	if err := os.WriteFile(filepath.Join(workspace, "untracked.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rr := callGitStatusHandler(t, s, http.MethodGet,
		"/api/git/status?view=semantics-preview&ref="+base+"&path=tracked.txt", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Preview gitSemanticsPreview `json:"preview"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	p := got.Preview
	if p.HeadSHA != head || p.TargetSHA != base || p.HeadEqualsTarget {
		t.Fatalf("preview refs=%+v", p)
	}
	if p.Branch == "" || p.Detached {
		t.Fatalf("preview branch=%+v", p)
	}
	if p.Staged != 1 || p.Unstaged != 1 || p.Untracked != 1 || p.Conflicted != 0 {
		t.Fatalf("preview worktree counts=%+v", p)
	}
	if p.Path != "tracked.txt" || p.PathStaged || !p.PathUnstaged || p.PathUntracked || p.PathConflicted {
		t.Fatalf("preview path state=%+v", p)
	}
	if !p.TargetPathExists || !p.HeadPathExists {
		t.Fatalf("tracked path existence=%+v", p)
	}
	foundTracked, foundHistory := false, false
	for _, file := range p.ChangedFiles {
		if file.Path == "tracked.txt" {
			foundTracked = true
		}
		if file.Path == "history.txt" {
			foundHistory = true
		}
	}
	if !foundTracked || !foundHistory {
		t.Fatalf("changed files=%+v", p.ChangedFiles)
	}
}

func TestGitResetRejectsStaleHeadGuard(t *testing.T) {
	workspace, s, base, previewHead := setupGitSemanticsRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "third.txt"), []byte("third\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "third.txt")
	gitQuickRun(t, workspace, "commit", "-m", "third")
	actualHead := gitQuickRun(t, workspace, "rev-parse", "HEAD")

	body := `{"action":"reset_commit","mode":"mixed","confirmed":true,"ref":"` + base +
		`","expected_sha":"` + base + `","expected_head_sha":"` + previewHead + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "HEAD changed after preview") {
		t.Fatalf("stale reset status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != actualHead {
		t.Fatalf("stale reset moved HEAD=%q want=%q", got, actualHead)
	}
}

func TestGitRestoreFileFromCommitChangesWorktreeOnly(t *testing.T) {
	workspace, s, base, head := setupGitSemanticsRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"action":"restore_file_commit","path":"tracked.txt","confirmed":true,"ref":"` + base +
		`","expected_sha":"` + base + `","expected_head_sha":"` + head + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("restore worktree status=%d body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(workspace, "tracked.txt"))
	if err != nil || string(data) != "one\n" {
		t.Fatalf("worktree=%q err=%v", data, err)
	}
	if got := gitQuickRun(t, workspace, "show", ":tracked.txt"); got != "two" {
		t.Fatalf("index changed=%q want two", got)
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD changed=%q", got)
	}
}

func TestGitRestoreStagedFromCommitChangesIndexOnly(t *testing.T) {
	workspace, s, base, head := setupGitSemanticsRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("worktree three\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "tracked.txt")
	body := `{"action":"restore_staged_commit","path":"tracked.txt","confirmed":true,"ref":"` + base +
		`","expected_sha":"` + base + `","expected_head_sha":"` + head + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("restore staged status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "show", ":tracked.txt"); got != "one" {
		t.Fatalf("index=%q want one", got)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "tracked.txt"))
	if err != nil || string(data) != "worktree three\n" {
		t.Fatalf("worktree changed=%q err=%v", data, err)
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD changed=%q", got)
	}
}

func TestGitRestoreRequiresConfirmationAndRejectsUnsafePath(t *testing.T) {
	_, s, base, head := setupGitSemanticsRepo(t)
	body := `{"action":"restore_file_commit","path":"tracked.txt","ref":"` + base +
		`","expected_sha":"` + base + `","expected_head_sha":"` + head + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "confirmation_required") {
		t.Fatalf("unconfirmed restore status=%d body=%s", rr.Code, rr.Body.String())
	}
	body = `{"action":"restore_file_commit","path":"../outside","confirmed":true,"ref":"` + base +
		`","expected_sha":"` + base + `","expected_head_sha":"` + head + `"}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unsafe restore status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGitRevertRejectsStaleHeadGuard(t *testing.T) {
	workspace, s, base, previewHead := setupGitSemanticsRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "third.txt"), []byte("third\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "third.txt")
	gitQuickRun(t, workspace, "commit", "-m", "third")
	actualHead := gitQuickRun(t, workspace, "rev-parse", "HEAD")

	body := `{"action":"revert_commit","ref":"` + previewHead + `","expected_sha":"` + previewHead +
		`","expected_head_sha":"` + previewHead + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "HEAD changed after preview") {
		t.Fatalf("stale revert status=%d body=%s base=%s", rr.Code, rr.Body.String(), base)
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != actualHead {
		t.Fatalf("stale revert moved HEAD=%q want=%q", got, actualHead)
	}
}
