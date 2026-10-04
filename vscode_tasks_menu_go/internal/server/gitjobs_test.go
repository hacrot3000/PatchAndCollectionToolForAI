package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func installFakeGitJobBinary(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake POSIX Git process-group test")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "git")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func waitGitJobState(t *testing.T, job *gitJob, states ...string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := job.snapshot()
		state, _ := snapshot["state"].(string)
		for _, wanted := range states {
			if state == wanted {
				return snapshot
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("Git job did not reach states %v; snapshot=%+v", states, job.snapshot())
	return nil
}

func TestGitAsyncArgsForcesProgressForNetworkActions(t *testing.T) {
	for _, tc := range []struct {
		action string
		args   []string
	}{
		{"fetch", []string{"fetch", "--prune"}},
		{"pull", []string{"pull", "--ff-only"}},
		{"push", []string{"push"}},
	} {
		got := gitAsyncArgs(tc.action, tc.args)
		if len(got) < 2 || got[1] != "--progress" {
			t.Fatalf("%s args=%v", tc.action, got)
		}
	}
	got := gitAsyncArgs("merge", []string{"merge", "--no-edit", "feature"})
	if strings.Join(got, " ") != "merge --no-edit feature" {
		t.Fatalf("merge args changed: %v", got)
	}
}

func TestGitJobStreamsOutputBeforeCompletion(t *testing.T) {
	installFakeGitJobBinary(t, "printf 'counting objects... 25%%\\r' >&2\nsleep 0.25\nprintf 'counting objects... 100%%\\n' >&2\nexit 0")
	s := &Server{}
	repo := gitRepository{ID: ".", Root: t.TempDir()}
	job, err := s.startGitCommandJob(repo, "push", []string{"push"}, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	sawLiveOutput := false
	for time.Now().Before(deadline) {
		snapshot := job.snapshot()
		if snapshot["state"] == "running" && strings.Contains(snapshot["output"].(string), "25%") {
			sawLiveOutput = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !sawLiveOutput {
		t.Fatalf("did not observe live output: %+v", job.snapshot())
	}
	final := waitGitJobState(t, job, "success")
	if final["ok"] != true || !strings.Contains(final["output"].(string), "100%") {
		t.Fatalf("final=%+v", final)
	}
}

func TestGitJobCancelStopsProcessGroup(t *testing.T) {
	installFakeGitJobBinary(t, "trap 'exit 130' TERM INT\nprintf 'started\\n' >&2\nwhile :; do sleep 1; done")
	s := &Server{}
	repo := gitRepository{ID: ".", Root: t.TempDir()}
	job, err := s.startGitCommandJob(repo, "fetch", []string{"fetch"}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(job.snapshot()["output"].(string), "started") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	job.mu.Lock()
	cancel := job.cancel
	job.mu.Unlock()
	if cancel == nil {
		t.Fatal("missing cancel function")
	}
	cancel()
	final := waitGitJobState(t, job, "canceled")
	if final["failure_code"] != "canceled" || final["error"] != "git operation canceled" {
		t.Fatalf("final=%+v", final)
	}
}

func TestGitJobsAPIAndCancelControl(t *testing.T) {
	installFakeGitJobBinary(t, "trap 'exit 130' TERM INT\nprintf 'ready\\n' >&2\nwhile :; do sleep 1; done")
	s := &Server{}
	job, err := s.startGitCommandJob(gitRepository{ID: "repo", Root: t.TempDir()}, "fetch", []string{"fetch"}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	get := httptest.NewRecorder()
	s.gitJobsAPI(get, httptest.NewRequest(http.MethodGet, "/api/git/jobs?id="+job.ID, nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"state":"running"`) {
		t.Fatalf("GET status=%d body=%s", get.Code, get.Body.String())
	}

	control := httptest.NewRecorder()
	body := strings.NewReader(`{"id":"`+job.ID+`","action":"cancel"}`)
	s.gitJobsControl(control, httptest.NewRequest(http.MethodPost, "/api/git/jobs/control", body))
	if control.Code != http.StatusAccepted {
		t.Fatalf("control status=%d body=%s", control.Code, control.Body.String())
	}
	waitGitJobState(t, job, "canceled")
}

func TestStartGitJobRejectsNonLongAction(t *testing.T) {
	s := &Server{}
	_, err := s.startGitCommandJob(gitRepository{ID: ".", Root: t.TempDir()}, "stage", []string{"add", "-A"}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("err=%v", err)
	}
}

func TestGitJobTimeoutClassified(t *testing.T) {
	installFakeGitJobBinary(t, "trap 'exit 143' TERM\nsleep 5")
	s := &Server{}
	job, err := s.startGitCommandJob(gitRepository{ID: ".", Root: t.TempDir()}, "fetch", []string{"fetch"}, 80*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	final := waitGitJobState(t, job, "failed")
	if final["failure_code"] != "timeout" || !strings.Contains(final["error"].(string), "timed out") {
		t.Fatalf("final=%+v", final)
	}
}

func TestGitJobFailureKeepsRecoveryClassification(t *testing.T) {
	installFakeGitJobBinary(t, "printf 'fatal: could not resolve host: github.com\\n' >&2\nexit 1")
	s := &Server{}
	job, err := s.startGitCommandJob(gitRepository{ID: ".", Root: t.TempDir()}, "push", []string{"push"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	final := waitGitJobState(t, job, "failed")
	if final["failure_code"] != "network_dns" {
		t.Fatalf("final=%+v", final)
	}
}

var _ = context.Background
