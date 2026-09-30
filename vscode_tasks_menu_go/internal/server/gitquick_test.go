package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitQuickRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func setupGitQuickRepo(t *testing.T) (string, *Server, string) {
	t.Helper()
	workspace := t.TempDir()
	gitQuickRun(t, workspace, "init")
	gitQuickRun(t, workspace, "config", "user.name", "Task Menu Test")
	gitQuickRun(t, workspace, "config", "user.email", "task-menu@example.invalid")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "tracked.txt")
	gitQuickRun(t, workspace, "commit", "-m", "initial")
	branch := gitQuickRun(t, workspace, "branch", "--show-current")
	return workspace, &Server{Workspace: workspace}, branch
}

func callGitStatusHandler(t *testing.T, s *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" { reader = strings.NewReader("") } else { reader = strings.NewReader(body) }
	req := httptest.NewRequest(method, target, reader)
	if body != "" { req.Header.Set("Content-Type", "application/json") }
	rr := httptest.NewRecorder()
	s.gitStatus(rr, req)
	return rr
}

func TestGitQuickChangesDiffStageCommitBranchAndStash(t *testing.T) {
	workspace, s, originalBranch := setupGitQuickRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("two\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(workspace, "new file.txt"), []byte("new\n"), 0o644); err != nil { t.Fatal(err) }

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=changes", "")
	if rr.Code != http.StatusOK { t.Fatalf("changes status=%d body=%s", rr.Code, rr.Body.String()) }
	var changes struct { Changes []gitChange `json:"changes"` }
	if err := json.Unmarshal(rr.Body.Bytes(), &changes); err != nil { t.Fatal(err) }
	if len(changes.Changes) != 2 { t.Fatalf("changes=%#v, want tracked + untracked", changes.Changes) }

	rr = callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=diff&mode=worktree&path=tracked.txt", "")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "two") { t.Fatalf("diff status=%d body=%s", rr.Code, rr.Body.String()) }

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"stage_all"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) { t.Fatalf("stage_all status=%d body=%s", rr.Code, rr.Body.String()) }
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"commit","message":"quick commit"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) { t.Fatalf("commit status=%d body=%s", rr.Code, rr.Body.String()) }

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"create_branch","branch":"feature/quick-ui"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) { t.Fatalf("create branch status=%d body=%s", rr.Code, rr.Body.String()) }
	if got := gitQuickRun(t, workspace, "branch", "--show-current"); got != "feature/quick-ui" { t.Fatalf("branch=%q", got) }

	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("stashed\n"), 0o644); err != nil { t.Fatal(err) }
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"stash_push","message":"quick stash"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) { t.Fatalf("stash push status=%d body=%s", rr.Code, rr.Body.String()) }
	rr = callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=stashes", "")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "stash@{0}") { t.Fatalf("stashes status=%d body=%s", rr.Code, rr.Body.String()) }
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"stash_pop","ref":"stash@{0}"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) { t.Fatalf("stash pop status=%d body=%s", rr.Code, rr.Body.String()) }

	gitQuickRun(t, workspace, "restore", "tracked.txt")
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"switch","branch":"`+originalBranch+`"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) { t.Fatalf("switch status=%d body=%s", rr.Code, rr.Body.String()) }
}

func TestGitQuickReadViewsAndValidation(t *testing.T) {
	_, s, _ := setupGitQuickRepo(t)
	for _, target := range []string{
		"/api/git/status?view=log",
		"/api/git/status?view=branches",
		"/api/git/status?view=ahead-behind",
	} {
		rr := callGitStatusHandler(t, s, http.MethodGet, target, "")
		if rr.Code != http.StatusOK { t.Fatalf("%s status=%d body=%s", target, rr.Code, rr.Body.String()) }
	}
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"stage","path":"../outside.txt"}`)
	if rr.Code != http.StatusBadRequest { t.Fatalf("unsafe path status=%d body=%s", rr.Code, rr.Body.String()) }
}


func TestGitQuickMergeRejectsDirtyWorktree(t *testing.T) {
	workspace, s, originalBranch := setupGitQuickRepo(t)
	gitQuickRun(t, workspace, "switch", "-c", "feature/merge-dirty")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("feature\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "tracked.txt")
	gitQuickRun(t, workspace, "commit", "-m", "feature change")
	gitQuickRun(t, workspace, "switch", originalBranch)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("local modify\n"), 0o644); err != nil { t.Fatal(err) }

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=merge-preflight&branch=feature%2Fmerge-dirty", "")
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "working tree must be clean before merging") {
		t.Fatalf("dirty merge preflight status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"merge","branch":"feature/merge-dirty","source":"local"}`)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "working tree must be clean before merging") {
		t.Fatalf("dirty merge action status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGitQuickMergePreflightChoosesDivergedLocalOrRemote(t *testing.T) {
	workspace, s, originalBranch := setupGitQuickRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitQuickRun(t, filepath.Dir(remote), "init", "--bare", remote)
	gitQuickRun(t, workspace, "remote", "add", "origin", remote)
	gitQuickRun(t, workspace, "push", "-u", "origin", originalBranch)

	gitQuickRun(t, workspace, "switch", "-c", "feature/merge-source")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("remote version\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "tracked.txt")
	gitQuickRun(t, workspace, "commit", "-m", "remote feature")
	gitQuickRun(t, workspace, "push", "-u", "origin", "feature/merge-source")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("local version\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "tracked.txt")
	gitQuickRun(t, workspace, "commit", "-m", "local-only feature")
	gitQuickRun(t, workspace, "switch", originalBranch)

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=merge-preflight&branch=feature%2Fmerge-source", "")
	if rr.Code != http.StatusOK { t.Fatalf("merge preflight status=%d body=%s", rr.Code, rr.Body.String()) }
	var preflight gitMergePreflightResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &preflight); err != nil { t.Fatal(err) }
	if !preflight.RequiresChoice || preflight.Same {
		t.Fatalf("preflight=%+v, want diverged local/remote choice", preflight)
	}
	if preflight.LocalRef != "feature/merge-source" || preflight.RemoteRef != "origin/feature/merge-source" {
		t.Fatalf("preflight refs=%+v", preflight)
	}
	if preflight.LocalSHA == "" || preflight.RemoteSHA == "" || preflight.LocalSHA == preflight.RemoteSHA {
		t.Fatalf("preflight SHAs=%+v", preflight)
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"merge","branch":"feature/merge-source"}`)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "choose merge source") {
		t.Fatalf("merge without source status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"merge","branch":"feature/merge-source","source":"remote","expected_sha":"deadbeef","expected_current":"`+preflight.Current+`"}`)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "merge source changed after confirmation") {
		t.Fatalf("stale confirmed SHA status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"merge","branch":"feature/merge-source","source":"remote","expected_sha":"`+preflight.RemoteSHA+`","expected_current":"other-branch"}`)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "current branch changed after confirmation") {
		t.Fatalf("stale current branch status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"merge","branch":"feature/merge-source","source":"remote","expected_sha":"`+preflight.RemoteSHA+`","expected_current":"`+preflight.Current+`"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("remote merge status=%d body=%s", rr.Code, rr.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(workspace, "tracked.txt"))
	if err != nil { t.Fatal(err) }
	if string(content) != "remote version\n" {
		t.Fatalf("merged content=%q, want remote branch version", content)
	}
}
