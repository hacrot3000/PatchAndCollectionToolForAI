package server

import (
	"context"
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

func TestGitQuickMergeToFastForwardNeverNeedsWorktree(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	targetSHA := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(workspace, "ff.txt"), []byte("ff\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "ff.txt")
	gitQuickRun(t, workspace, "commit", "-m", "fast forward source")
	sourceSHA := gitQuickRun(t, workspace, "rev-parse", "HEAD")

	engine, slow := s.gitMergeToEngine(context.Background(), targetSHA, sourceSHA)
	if engine != "fast-forward" || slow {
		t.Fatalf("engine=%q slow=%v, want fast-forward without fallback", engine, slow)
	}
}

func TestGitQuickMergeToPushesCommittedHEADAndLeavesDirtyWorktreeUntouched(t *testing.T) {
	workspace, s, current := setupGitQuickRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitQuickRun(t, filepath.Dir(remote), "init", "--bare", remote)
	gitQuickRun(t, workspace, "remote", "add", "origin", remote)
	gitQuickRun(t, workspace, "push", "-u", "origin", current)

	gitQuickRun(t, workspace, "switch", "-c", "target/merge-to")
	if err := os.WriteFile(filepath.Join(workspace, "target.txt"), []byte("target only\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "target.txt")
	gitQuickRun(t, workspace, "commit", "-m", "target branch change")
	gitQuickRun(t, workspace, "push", "-u", "origin", "target/merge-to")
	targetBefore := gitQuickRun(t, workspace, "rev-parse", "target/merge-to")

	gitQuickRun(t, workspace, "switch", current)
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), []byte("source committed\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "source.txt")
	gitQuickRun(t, workspace, "commit", "-m", "source branch change")
	sourceSHA := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("dirty tracked local\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(workspace, "dirty-local.txt"), []byte("keep me local\n"), 0o644); err != nil { t.Fatal(err) }

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=merge-to-preflight&branch=target%2Fmerge-to&target_source=local", "")
	if rr.Code != http.StatusOK { t.Fatalf("Merge To preflight status=%d body=%s", rr.Code, rr.Body.String()) }
	var preflight gitMergeToPreflightResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &preflight); err != nil { t.Fatal(err) }
	if !preflight.Dirty || preflight.Current != current || preflight.CurrentSHA != sourceSHA || preflight.TargetSHA != targetBefore {
		t.Fatalf("Merge To preflight=%+v", preflight)
	}
	if preflight.MergeEngine == "" {
		t.Fatalf("Merge To preflight missing engine: %+v", preflight)
	}

	body := `{"action":"merge_to","branch":"target/merge-to","target_source":"local","expected_current":"`+current+`","expected_source_sha":"`+sourceSHA+`","expected_target_sha":"`+targetBefore+`"}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":false`) || !strings.Contains(rr.Body.String(), "uncommitted changes") {
		t.Fatalf("Merge To without dirty confirmation status=%d body=%s", rr.Code, rr.Body.String())
	}

	slowApproval := ""
	if preflight.SlowFallback {
		slowApproval = `,"allow_slow_fallback":true`
	}
	body = `{"action":"merge_to","branch":"target/merge-to","target_source":"local","expected_current":"`+current+`","expected_source_sha":"`+sourceSHA+`","expected_target_sha":"`+targetBefore+`","allow_dirty":true`+slowApproval+`}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("Merge To status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "branch", "--show-current"); got != current {
		t.Fatalf("current branch changed to %q, want %q", got, current)
	}
	trackedContent, err := os.ReadFile(filepath.Join(workspace, "tracked.txt"))
	if err != nil { t.Fatal(err) }
	if string(trackedContent) != "dirty tracked local\n" {
		t.Fatalf("dirty tracked file changed: %q", trackedContent)
	}
	dirtyContent, err := os.ReadFile(filepath.Join(workspace, "dirty-local.txt"))
	if err != nil { t.Fatal(err) }
	if string(dirtyContent) != "keep me local\n" {
		t.Fatalf("dirty untracked file changed: %q", dirtyContent)
	}
	status := gitQuickRun(t, workspace, "status", "--porcelain=v1", "--untracked-files=normal")
	if !strings.Contains(status, "M tracked.txt") || !strings.Contains(status, "?? dirty-local.txt") {
		t.Fatalf("dirty local changes disappeared after Merge To: %q", status)
	}

	remoteTarget := gitQuickRun(t, workspace, "ls-remote", remote, "refs/heads/target/merge-to")
	fields := strings.Fields(remoteTarget)
	if len(fields) != 2 {
		t.Fatalf("unexpected remote target ref: %q", remoteTarget)
	}
	resultSHA := fields[0]
	if got := gitQuickRun(t, workspace, "rev-parse", "target/merge-to"); got != resultSHA {
		t.Fatalf("local target=%s remote target=%s", got, resultSHA)
	}
	gitQuickRun(t, workspace, "merge-base", "--is-ancestor", sourceSHA, resultSHA)
	gitQuickRun(t, workspace, "merge-base", "--is-ancestor", targetBefore, resultSHA)
}

func TestGitQuickMergeToLocalTargetHonorsConfiguredUpstream(t *testing.T) {
	workspace, s, current := setupGitQuickRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitQuickRun(t, filepath.Dir(remote), "init", "--bare", remote)
	gitQuickRun(t, workspace, "remote", "add", "origin", remote)
	gitQuickRun(t, workspace, "push", "-u", "origin", current)

	gitQuickRun(t, workspace, "switch", "-c", "target/local-name")
	if err := os.WriteFile(filepath.Join(workspace, "target-upstream.txt"), []byte("target\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "target-upstream.txt")
	gitQuickRun(t, workspace, "commit", "-m", "target configured upstream")
	gitQuickRun(t, workspace, "push", "-u", "origin", "HEAD:refs/heads/release/remote-name")
	gitQuickRun(t, workspace, "switch", current)

	if err := os.WriteFile(filepath.Join(workspace, "source-upstream.txt"), []byte("source\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "source-upstream.txt")
	gitQuickRun(t, workspace, "commit", "-m", "source for configured upstream")

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=merge-to-preflight&branch=target%2Flocal-name&target_source=local", "")
	if rr.Code != http.StatusOK { t.Fatalf("configured-upstream preflight status=%d body=%s", rr.Code, rr.Body.String()) }
	var preflight gitMergeToPreflightResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &preflight); err != nil { t.Fatal(err) }
	if preflight.PushRemote != "origin" || preflight.PushBranch != "release/remote-name" || preflight.PushRemoteRef != "origin/release/remote-name" {
		t.Fatalf("configured upstream ignored: %+v", preflight)
	}
}

func TestGitQuickMergeToRemoteTargetAndRaceGuards(t *testing.T) {
	workspace, s, current := setupGitQuickRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitQuickRun(t, filepath.Dir(remote), "init", "--bare", remote)
	gitQuickRun(t, workspace, "remote", "add", "origin", remote)
	gitQuickRun(t, workspace, "push", "-u", "origin", current)

	gitQuickRun(t, workspace, "switch", "-c", "target/remote-only")
	if err := os.WriteFile(filepath.Join(workspace, "target-remote.txt"), []byte("target\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "target-remote.txt")
	gitQuickRun(t, workspace, "commit", "-m", "remote target")
	gitQuickRun(t, workspace, "push", "-u", "origin", "target/remote-only")
	gitQuickRun(t, workspace, "switch", current)
	gitQuickRun(t, workspace, "branch", "-D", "target/remote-only")

	if err := os.WriteFile(filepath.Join(workspace, "source-remote.txt"), []byte("source\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "source-remote.txt")
	gitQuickRun(t, workspace, "commit", "-m", "source for remote target")
	sourceSHA := gitQuickRun(t, workspace, "rev-parse", "HEAD")

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=merge-to-preflight&branch=origin%2Ftarget%2Fremote-only&target_source=remote", "")
	if rr.Code != http.StatusOK { t.Fatalf("remote Merge To preflight status=%d body=%s", rr.Code, rr.Body.String()) }
	var preflight gitMergeToPreflightResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &preflight); err != nil { t.Fatal(err) }
	if preflight.TargetRef != "origin/target/remote-only" || preflight.PushRemote != "origin" || preflight.PushBranch != "target/remote-only" {
		t.Fatalf("remote Merge To preflight=%+v", preflight)
	}

	staleBody := `{"action":"merge_to","branch":"origin/target/remote-only","target_source":"remote","expected_current":"`+current+`","expected_source_sha":"`+sourceSHA+`","expected_target_sha":"deadbeef"}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", staleBody)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":false`) || !strings.Contains(rr.Body.String(), "target branch changed after confirmation") {
		t.Fatalf("stale Merge To target status=%d body=%s", rr.Code, rr.Body.String())
	}

	slowApproval := ""
	if preflight.SlowFallback {
		slowApproval = `,"allow_slow_fallback":true`
	}
	body := `{"action":"merge_to","branch":"origin/target/remote-only","target_source":"remote","expected_current":"`+current+`","expected_source_sha":"`+sourceSHA+`","expected_target_sha":"`+preflight.TargetSHA+`"`+slowApproval+`}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("remote Merge To status=%d body=%s", rr.Code, rr.Body.String())
	}
	remoteTarget := gitQuickRun(t, workspace, "ls-remote", remote, "refs/heads/target/remote-only")
	fields := strings.Fields(remoteTarget)
	if len(fields) != 2 { t.Fatalf("unexpected remote target ref: %q", remoteTarget) }
	gitQuickRun(t, workspace, "merge-base", "--is-ancestor", sourceSHA, fields[0])
	if got := gitQuickRun(t, workspace, "branch", "--show-current"); got != current {
		t.Fatalf("current branch changed to %q, want %q", got, current)
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
