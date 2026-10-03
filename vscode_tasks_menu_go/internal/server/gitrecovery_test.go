package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyGitFailureCommonCases(t *testing.T) {
	tests := []struct {
		name   string
		action string
		text   string
		want   string
	}{
		{"no upstream push", "push", "fatal: The current branch main has no upstream branch.", "no_upstream_push"},
		{"no upstream pull", "pull", "There is no tracking information for the current branch.", "no_upstream_pull"},
		{"push rejected", "push", "! [rejected] main -> main (non-fast-forward)\nUpdates were rejected because the remote contains work", "push_non_fast_forward"},
		{"pull diverged", "pull", "fatal: Not possible to fast-forward, aborting.", "pull_diverged"},
		{"dirty", "pull", "Your local changes to the following files would be overwritten by merge", "dirty_worktree"},
		{"conflict", "merge", "error: you have unmerged files. fix conflicts and then commit", "conflicts"},
		{"identity", "commit", "Author identity unknown\nPlease tell me who you are.", "identity_missing"},
		{"gpg", "commit", "error: gpg failed to sign the data", "gpg_signing"},
		{"ssh auth", "pull", "git@github.com: Permission denied (publickey).", "ssh_auth"},
		{"host key", "fetch", "Host key verification failed.", "ssh_host_key"},
		{"https auth", "fetch", "fatal: could not read Username for 'https://github.com': terminal prompts disabled", "https_auth"},
		{"permission", "push", "remote: Permission to owner/repo.git denied to user.\nfatal: unable to access: The requested URL returned error: 403", "remote_permission"},
		{"dns", "fetch", "Could not resolve host: github.com", "network_dns"},
		{"connect", "fetch", "Failed to connect to github.com port 443: Connection timed out", "network_connect"},
		{"tls", "fetch", "SSL certificate problem: unable to get local issuer certificate", "tls"},
		{"transport", "fetch", "RPC failed; HTTP/2 stream 5 was not closed cleanly", "transport"},
		{"index lock", "commit", "fatal: Unable to create '.git/index.lock': File exists.", "index_lock"},
		{"dubious", "status", "fatal: detected dubious ownership in repository at '/tmp/repo'", "dubious_ownership"},
		{"protected", "push", "remote: error: GH013: Repository rule violations found\nremote: protected branch", "protected_branch"},
		{"detached", "push", "fatal: You are not currently on a branch.", "detached_head"},
		{"unrelated", "pull", "fatal: refusing to merge unrelated histories", "unrelated_histories"},
		{"large", "push", "remote: error: GH001: Large files detected.", "file_too_large"},
		{"disk", "commit", "fatal: unable to write: No space left on device", "disk_full"},
		{"corrupt", "status", "error: object file .git/objects/aa/bb is empty\nfatal: bad object HEAD", "repository_corrupt"},
		{"timeout", "fetch", "git operation timed out", "timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyGitFailure(tt.action, tt.text, ""); got != tt.want {
				t.Fatalf("classify=%q want=%q for %q", got, tt.want, tt.text)
			}
		})
	}
}

func TestGitRecoveryPushSetUpstream(t *testing.T) {
	workspace, s, branch := setupGitQuickRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitQuickRun(t, filepath.Dir(remote), "init", "--bare", remote)
	gitQuickRun(t, workspace, "remote", "add", "origin", remote)

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"push_set_upstream"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("repair status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); got != "origin/"+branch {
		t.Fatalf("upstream=%q want origin/%s", got, branch)
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "--verify", "refs/remotes/origin/"+branch); got == "" {
		t.Fatal("remote branch was not published")
	}
}

func TestGitRecoverySetUpstreamAfterFetch(t *testing.T) {
	workspace, s, branch := setupGitQuickRepo(t)
	remoteRoot := t.TempDir()
	remote := filepath.Join(remoteRoot, "remote.git")
	gitQuickRun(t, remoteRoot, "init", "--bare", remote)
	gitQuickRun(t, workspace, "remote", "add", "origin", remote)
	gitQuickRun(t, workspace, "push", "origin", branch)
	gitQuickRun(t, workspace, "fetch", "origin")
	gitQuickRun(t, workspace, "branch", "--unset-upstream")

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"set_upstream"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("set upstream status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); got != "origin/"+branch {
		t.Fatalf("upstream=%q want origin/%s", got, branch)
	}
}

func TestGitRecoveryConfiguresRepositoryIdentity(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	gitQuickRun(t, workspace, "config", "--unset", "user.name")
	gitQuickRun(t, workspace, "config", "--unset", "user.email")

	body := `{"action":"repair","repair":"configure_identity","name":"TaskDeck User","email":"taskdeck@example.invalid"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("identity status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "config", "--get", "user.name"); got != "TaskDeck User" {
		t.Fatalf("user.name=%q", got)
	}
	if got := gitQuickRun(t, workspace, "config", "--get", "user.email"); got != "taskdeck@example.invalid" {
		t.Fatalf("user.email=%q", got)
	}
}

func TestGitRecoveryStaleIndexLockRequiresConfirmationAndAge(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	lock := filepath.Join(workspace, ".git", "index.lock")
	if err := os.WriteFile(lock, []byte{}, 0o644); err != nil { t.Fatal(err) }

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"remove_stale_index_lock"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":false`) || !strings.Contains(rr.Body.String(), "explicit confirmation") {
		t.Fatalf("unconfirmed status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"remove_stale_index_lock","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":false`) || !strings.Contains(rr.Body.String(), "possibly active") {
		t.Fatalf("recent lock status=%d body=%s", rr.Code, rr.Body.String())
	}
	old := time.Now().Add(-5 * time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil { t.Fatal(err) }

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"remove_stale_index_lock","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("stale lock status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatalf("stale lock still exists: %v", err)
	}
}

func TestGitRecoveryCreateBranchFromDetachedHead(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	gitQuickRun(t, workspace, "switch", "--detach", "HEAD")
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"create_branch_from_head","new_branch":"recovered/work"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("create branch status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "branch", "--show-current"); got != "recovered/work" {
		t.Fatalf("current branch=%q", got)
	}
}

func TestGitRecoveryRejectsUnknownRepairAndReportsFailureCode(t *testing.T) {
	_, s, _ := setupGitQuickRepo(t)
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"repair","repair":"raw_command","message":"git reset --hard"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil { t.Fatal(err) }
	if response["ok"] != false {
		t.Fatalf("response=%v", response)
	}
	if !strings.Contains(response["error"].(string), "unsupported Git repair action") {
		t.Fatalf("response=%v", response)
	}
	if response["failure_code"] == nil {
		t.Fatalf("missing failure_code: %v", response)
	}
}
