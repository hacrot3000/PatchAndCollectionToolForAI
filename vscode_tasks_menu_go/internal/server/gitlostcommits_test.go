package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupForceDeletedBranchCommit(t *testing.T) (string, *Server, string, string) {
	t.Helper()
	workspace, s, _ := setupGitQuickRepo(t)
	head := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	lost := gitQuickRun(t, workspace, "commit-tree", "HEAD^{tree}", "-p", "HEAD", "-m", "lost after force delete")
	gitQuickRun(t, workspace, "branch", "doomed/lost", lost)
	gitQuickRun(t, workspace, "branch", "-D", "doomed/lost")
	return workspace, s, head, lost
}

func TestGitLostCommitsFindsForceDeletedBranchCommitOutsideReflog(t *testing.T) {
	_, s, _, lost := setupForceDeletedBranchCommit(t)

	reflog := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=reflog&limit=500", "")
	if reflog.Code != http.StatusOK {
		t.Fatalf("reflog status=%d body=%s", reflog.Code, reflog.Body.String())
	}
	if strings.Contains(reflog.Body.String(), lost) {
		t.Fatalf("fixture lost commit unexpectedly remained visible in reflog: %s", reflog.Body.String())
	}

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=lost-commits&limit=100", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("lost commits status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Commits []gitLostCommit `json:"commits"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var found *gitLostCommit
	for i := range got.Commits {
		if got.Commits[i].SHA == lost {
			found = &got.Commits[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("lost commit %s not found: %+v", lost, got.Commits)
	}
	if found.ReflogVisible {
		t.Fatalf("lost commit incorrectly marked reflog-visible: %+v", found)
	}
	if found.Subject != "lost after force delete" {
		t.Fatalf("lost commit subject=%q", found.Subject)
	}

	search := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=lost-commits&search=force+delete", "")
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), lost) {
		t.Fatalf("lost search status=%d body=%s", search.Code, search.Body.String())
	}
}

func TestGitLostCommitCanBeRecoveredWithoutChangingHEADOrDirtyWorktree(t *testing.T) {
	workspace, s, head, lost := setupForceDeletedBranchCommit(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("dirty while recovering lost commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{"action":"create_branch_ref","branch":"recovery/force-deleted","ref":"` + lost + `","expected_sha":"` + lost + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("recover branch status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "recovery/force-deleted"); got != lost {
		t.Fatalf("recovery branch=%q want %q", got, lost)
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD moved=%q want %q", got, head)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "dirty while recovering lost commit\n" {
		t.Fatalf("worktree changed=%q", data)
	}
}

func TestParseGitLostCommitSHAsIgnoresNonCommitObjectsAndDuplicates(t *testing.T) {
	sha := strings.Repeat("a", 40)
	raw := "unreachable blob " + strings.Repeat("b", 40) + "\n" +
		"unreachable commit " + sha + "\n" +
		"dangling commit " + sha + "\n" +
		"notice: something else\n"
	got := parseGitLostCommitSHAs(raw, 10)
	if len(got) != 1 || got[0] != sha {
		t.Fatalf("parseGitLostCommitSHAs=%v", got)
	}
}
