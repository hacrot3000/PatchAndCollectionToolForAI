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

func TestGitQuickSwitchAllowsNonConflictingDirtyChanges(t *testing.T) {
	workspace, s, original := setupGitQuickRepo(t)

	gitQuickRun(t, workspace, "switch", "-c", "feature/safe-switch")
	if err := os.WriteFile(filepath.Join(workspace, "branch-only.txt"), []byte("branch\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "branch-only.txt")
	gitQuickRun(t, workspace, "commit", "-m", "branch-only change")
	gitQuickRun(t, workspace, "switch", original)

	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("dirty stays\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(workspace, "untracked-stays.txt"), []byte("untracked stays\n"), 0o644); err != nil { t.Fatal(err) }

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"switch","branch":"feature/safe-switch"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("safe dirty switch status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "branch", "--show-current"); got != "feature/safe-switch" {
		t.Fatalf("branch=%q after safe dirty switch", got)
	}
	if content, err := os.ReadFile(filepath.Join(workspace, "tracked.txt")); err != nil || string(content) != "dirty stays\n" {
		t.Fatalf("tracked dirty change not preserved: content=%q err=%v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(workspace, "untracked-stays.txt")); err != nil || string(content) != "untracked stays\n" {
		t.Fatalf("untracked dirty change not preserved: content=%q err=%v", content, err)
	}
}

func TestGitQuickCreateBranchAllowsDirtyChanges(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("dirty before create\n"), 0o644); err != nil { t.Fatal(err) }

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"create_branch","branch":"feature/dirty-create"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("dirty create branch status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "branch", "--show-current"); got != "feature/dirty-create" {
		t.Fatalf("branch=%q after dirty create", got)
	}
	if content, err := os.ReadFile(filepath.Join(workspace, "tracked.txt")); err != nil || string(content) != "dirty before create\n" {
		t.Fatalf("dirty change not preserved on create: content=%q err=%v", content, err)
	}
}

func TestGitQuickSwitchLetsGitRejectConflictingDirtyChanges(t *testing.T) {
	workspace, s, original := setupGitQuickRepo(t)

	gitQuickRun(t, workspace, "switch", "-c", "feature/conflicting-switch")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("branch version\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "tracked.txt")
	gitQuickRun(t, workspace, "commit", "-m", "conflicting branch change")
	gitQuickRun(t, workspace, "switch", original)

	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("local dirty version\n"), 0o644); err != nil { t.Fatal(err) }
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"switch","branch":"feature/conflicting-switch"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":false`) {
		t.Fatalf("conflicting dirty switch status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "branch", "--show-current"); got != original {
		t.Fatalf("branch changed despite conflicting switch: got=%q want=%q", got, original)
	}
	if content, err := os.ReadFile(filepath.Join(workspace, "tracked.txt")); err != nil || string(content) != "local dirty version\n" {
		t.Fatalf("conflicting dirty change altered: content=%q err=%v", content, err)
	}
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

func TestGitTagsCreateListDelete(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"create_tag","name":"v1.0.0","message":"Release 1.0","ref":"HEAD"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("create annotated tag status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"create_tag","name":"snapshot","ref":"HEAD"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("create lightweight tag status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=tags", "")
	if rr.Code != http.StatusOK { t.Fatalf("tags status=%d body=%s", rr.Code, rr.Body.String()) }
	var response struct { Tags []gitTagRow `json:"tags"` }
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil { t.Fatal(err) }
	if len(response.Tags) != 2 { t.Fatalf("tags=%+v", response.Tags) }
	byName := map[string]gitTagRow{}
	for _, tag := range response.Tags { byName[tag.Name] = tag }
	if !byName["v1.0.0"].Annotated || byName["snapshot"].Annotated {
		t.Fatalf("tags=%+v", response.Tags)
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"delete_tag","name":"v1.0.0"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "confirmation_required") {
		t.Fatalf("unconfirmed delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"delete_tag","name":"v1.0.0","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("delete tag status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "tag", "--list", "v1.0.0"); got != "" {
		t.Fatalf("tag still exists: %q", got)
	}
}

func TestGitCreateTagRejectsInvalidNameAndDuplicate(t *testing.T) {
	_, s, _ := setupGitQuickRepo(t)
	if _, err := validGitTagName(context.Background(), s, "bad tag"); err == nil {
		t.Fatal("invalid tag name accepted")
	}
	args, err := s.gitCreateTagArgs(context.Background(), "v2.0.0", "message", "HEAD")
	if err != nil || len(args) < 5 || args[0] != "tag" || args[1] != "-a" {
		t.Fatalf("args=%v err=%v", args, err)
	}
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"create_tag","name":"v2.0.0","message":"message"}`)
	if rr.Code != http.StatusOK { t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String()) }
	if _, err := s.gitCreateTagArgs(context.Background(), "v2.0.0", "", "HEAD"); err == nil {
		t.Fatal("duplicate tag accepted")
	}
}

func TestGitCherryPickAndRevertActions(t *testing.T) {
	workspace, s, mainBranch := setupGitQuickRepo(t)
	gitQuickRun(t, workspace, "switch", "-c", "feature/cherry")
	if err := os.WriteFile(filepath.Join(workspace, "cherry.txt"), []byte("picked\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "cherry.txt")
	gitQuickRun(t, workspace, "commit", "-m", "add cherry file")
	sourceSHA := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	gitQuickRun(t, workspace, "switch", mainBranch)

	body := `{"action":"cherry_pick","ref":"`+sourceSHA+`","expected_sha":"`+sourceSHA+`"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("cherry-pick status=%d body=%s", rr.Code, rr.Body.String())
	}
	if raw, err := os.ReadFile(filepath.Join(workspace, "cherry.txt")); err != nil || string(raw) != "picked\n" {
		t.Fatalf("cherry-picked file raw=%q err=%v", raw, err)
	}

	body = `{"action":"revert_commit","ref":"`+sourceSHA+`","expected_sha":"`+sourceSHA+`"}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("revert status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, "cherry.txt")); !os.IsNotExist(err) {
		t.Fatalf("revert did not remove cherry.txt: err=%v", err)
	}
}

func TestGitCherryPickConflictUsesRecoveryState(t *testing.T) {
	workspace, s, mainBranch := setupGitQuickRepo(t)
	gitQuickRun(t, workspace, "switch", "-c", "feature/conflicting-pick")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("feature\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "commit", "-am", "feature conflict")
	sourceSHA := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	gitQuickRun(t, workspace, "switch", mainBranch)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("main\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "commit", "-am", "main conflict")

	body := `{"action":"cherry_pick","ref":"`+sourceSHA+`","expected_sha":"`+sourceSHA+`"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
	var response struct {
		OK bool `json:"ok"`
		FailureCode string `json:"failure_code"`
		ConflictState gitConflictState `json:"conflict_state"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil { t.Fatal(err) }
	if response.OK || response.FailureCode != "conflicts" || response.ConflictState.Operation != "cherry-pick" || len(response.ConflictState.Files) != 1 {
		t.Fatalf("response=%+v body=%s", response, rr.Body.String())
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"abort_in_progress"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("abort status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "status", "--porcelain"); got != "" {
		t.Fatalf("worktree dirty after abort: %q", got)
	}
}

func TestGitCherryPickRequiresCleanWorktree(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	sha := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("dirty\n"), 0o644); err != nil { t.Fatal(err) }
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"cherry_pick","ref":"`+sha+`","expected_sha":"`+sha+`"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "dirty_worktree") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGitSafeLocalBranchDelete(t *testing.T) {
	workspace, s, mainBranch := setupGitQuickRepo(t)
	gitQuickRun(t, workspace, "branch", "feature/merged")

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"delete_branch","branch":"feature/merged"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "confirmation_required") {
		t.Fatalf("unconfirmed delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"delete_branch","branch":"feature/merged","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("delete merged branch status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "branch", "--list", "feature/merged"); got != "" {
		t.Fatalf("branch remains: %q", got)
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"delete_branch","branch":"`+mainBranch+`","confirmed":true}`)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "cannot delete the current branch") {
		t.Fatalf("current branch delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"force_delete_branch","branch":"`+mainBranch+`","confirmed":true}`)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "cannot delete the current branch") {
		t.Fatalf("current branch force-delete status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGitBranchDeleteDistinguishesLocalTrackingAndRemote(t *testing.T) {
	workspace, s, current := setupGitQuickRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitQuickRun(t, filepath.Dir(remote), "init", "--bare", remote)
	gitQuickRun(t, workspace, "remote", "add", "origin", remote)
	gitQuickRun(t, workspace, "push", "-u", "origin", current)

	gitQuickRun(t, workspace, "switch", "-c", "feature/delete-scope")
	if err := os.WriteFile(filepath.Join(workspace, "delete-scope.txt"), []byte("scope\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "delete-scope.txt")
	gitQuickRun(t, workspace, "commit", "-m", "delete scope")
	gitQuickRun(t, workspace, "push", "-u", "origin", "feature/delete-scope")
	gitQuickRun(t, workspace, "switch", current)
	gitQuickRun(t, workspace, "merge", "--ff-only", "feature/delete-scope")

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"delete_branch","branch":"feature/delete-scope","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("local delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "ls-remote", remote, "refs/heads/feature/delete-scope"); got == "" {
		t.Fatal("deleting local branch unexpectedly deleted the remote branch")
	}
	if got := gitQuickRun(t, workspace, "branch", "-r", "--list", "origin/feature/delete-scope"); !strings.Contains(got, "origin/feature/delete-scope") {
		t.Fatalf("remote-tracking ref missing before explicit tracking delete: %q", got)
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"delete_remote_tracking","branch":"origin/feature/delete-scope","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("remote-tracking delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "branch", "-r", "--list", "origin/feature/delete-scope"); got != "" {
		t.Fatalf("remote-tracking ref remains after local forget: %q", got)
	}
	if got := gitQuickRun(t, workspace, "ls-remote", remote, "refs/heads/feature/delete-scope"); got == "" {
		t.Fatal("forgetting remote-tracking ref unexpectedly deleted remote branch")
	}

	gitQuickRun(t, workspace, "fetch", "origin")
	if got := gitQuickRun(t, workspace, "branch", "-r", "--list", "origin/feature/delete-scope"); !strings.Contains(got, "origin/feature/delete-scope") {
		t.Fatalf("fetch did not recreate remote-tracking ref: %q", got)
	}

	gitQuickRun(t, workspace, "branch", "feature/local-survives", "origin/feature/delete-scope")
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"delete_remote_branch","branch":"origin/feature/delete-scope","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("remote delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "ls-remote", remote, "refs/heads/feature/delete-scope"); got != "" {
		t.Fatalf("remote branch still exists after remote delete: %q", got)
	}
	if got := gitQuickRun(t, workspace, "branch", "--list", "feature/local-survives"); !strings.Contains(got, "feature/local-survives") {
		t.Fatalf("remote deletion unexpectedly removed same-history local branch: %q", got)
	}
}

func TestGitRemoteDeleteWorksAfterTrackingRefWasForgotten(t *testing.T) {
	workspace, s, current := setupGitQuickRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitQuickRun(t, filepath.Dir(remote), "init", "--bare", remote)
	gitQuickRun(t, workspace, "remote", "add", "origin", remote)
	gitQuickRun(t, workspace, "push", "-u", "origin", current)

	gitQuickRun(t, workspace, "switch", "-c", "feature/stale-tracking")
	if err := os.WriteFile(filepath.Join(workspace, "stale-tracking.txt"), []byte("keep local branch\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "stale-tracking.txt")
	gitQuickRun(t, workspace, "commit", "-m", "stale tracking")
	gitQuickRun(t, workspace, "push", "-u", "origin", "feature/stale-tracking")
	gitQuickRun(t, workspace, "switch", current)

	gitQuickRun(t, workspace, "branch", "-dr", "origin/feature/stale-tracking")
	if got := gitQuickRun(t, workspace, "branch", "-r", "--list", "origin/feature/stale-tracking"); got != "" {
		t.Fatalf("remote-tracking ref still present before stale-ref delete test: %q", got)
	}
	if got := gitQuickRun(t, workspace, "branch", "--list", "feature/stale-tracking"); !strings.Contains(got, "feature/stale-tracking") {
		t.Fatalf("same-named local branch missing before remote delete: %q", got)
	}

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"delete_remote_branch","branch":"origin/feature/stale-tracking","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("remote delete without tracking ref status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "ls-remote", remote, "refs/heads/feature/stale-tracking"); got != "" {
		t.Fatalf("remote branch still exists after stale-ref remote delete: %q", got)
	}
	if got := gitQuickRun(t, workspace, "branch", "--list", "feature/stale-tracking"); !strings.Contains(got, "feature/stale-tracking") {
		t.Fatalf("remote delete unexpectedly removed same-named local branch: %q", got)
	}
}

func TestGitRemoteBranchDeleteRequiresConfirmation(t *testing.T) {
	workspace, s, current := setupGitQuickRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitQuickRun(t, filepath.Dir(remote), "init", "--bare", remote)
	gitQuickRun(t, workspace, "remote", "add", "origin", remote)
	gitQuickRun(t, workspace, "push", "-u", "origin", current)

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"delete_remote_branch","branch":"origin/`+current+`"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "confirmation_required") {
		t.Fatalf("unconfirmed remote delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "ls-remote", remote, "refs/heads/"+current); got == "" {
		t.Fatal("unconfirmed remote delete changed the remote")
	}
}

func TestGitSafeBranchDeleteRefusesUnmergedBranch(t *testing.T) {
	workspace, s, mainBranch := setupGitQuickRepo(t)
	gitQuickRun(t, workspace, "switch", "-c", "feature/unmerged")
	if err := os.WriteFile(filepath.Join(workspace, "only-feature.txt"), []byte("feature\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "only-feature.txt")
	gitQuickRun(t, workspace, "commit", "-m", "unmerged work")
	gitQuickRun(t, workspace, "switch", mainBranch)

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"delete_branch","branch":"feature/unmerged","confirmed":true}`)
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("unmerged delete unexpectedly succeeded status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "branch", "--list", "feature/unmerged"); !strings.Contains(got, "feature/unmerged") {
		t.Fatalf("unmerged branch disappeared: %q", got)
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"force_delete_branch","branch":"feature/unmerged"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "confirmation_required") {
		t.Fatalf("unconfirmed force delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"force_delete_branch","branch":"feature/unmerged","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("force delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "branch", "--list", "feature/unmerged"); got != "" {
		t.Fatalf("force-deleted branch remains: %q", got)
	}
}
