package server

import (
	"encoding/json"
	"fmt"
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

func TestGitSubmoduleActionsAreAsyncWithoutForcedProgressFlag(t *testing.T) {
	for _, action := range []string{
		"submodule_init",
		"submodule_update",
		"submodule_checkout_expected",
		"submodule_update_recursive",
		"submodule_sync",
	} {
		if !gitAsyncActionAllowed(action) {
			t.Fatalf("%s should support cancellable background Git jobs", action)
		}
		args := []string{"submodule", "update", "--init", "--checkout"}
		got := gitAsyncArgs(action, args)
		if strings.Join(got, " ") != strings.Join(args, " ") {
			t.Fatalf("%s unexpectedly rewrote args: %v", action, got)
		}
	}
}


func TestGitJobsAPIListsBackgroundJobsAndBoundsRetention(t *testing.T) {
	s := &Server{}
	now := time.Now()
	for i := 0; i < maxGitJobs+7; i++ {
		job := &gitJob{
			ID: fmt.Sprintf("job-%03d", i),
			RepoID: "repo",
			Action: "fetch",
			Command: "git fetch --progress",
			State: "success",
			StartedAt: now.Add(time.Duration(i)*time.Second),
			FinishedAt: now.Add(time.Duration(i)*time.Second + time.Millisecond),
		}
		s.registerGitJob(job)
	}
	running := &gitJob{
		ID: "running-job", RepoID: "repo", Action: "pull", Command: "git pull --ff-only",
		State: "running", StartedAt: now.Add(24*time.Hour),
	}
	s.registerGitJob(running)

	rr := httptest.NewRecorder()
	s.gitJobsAPI(rr, httptest.NewRequest(http.MethodGet, "/api/git/jobs", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct{ Jobs []map[string]any `json:"jobs"` }
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Jobs) > maxGitJobs {
		t.Fatalf("listed jobs=%d max=%d", len(payload.Jobs), maxGitJobs)
	}
	foundRunning := false
	for _, item := range payload.Jobs {
		if item["id"] == "running-job" { foundRunning = true }
	}
	if !foundRunning {
		t.Fatal("running Git job was evicted from bounded history")
	}
	if len(payload.Jobs) == 0 || payload.Jobs[0]["id"] != "running-job" {
		t.Fatalf("Git jobs are not newest-first: %#v", payload.Jobs)
	}
}

func TestGitJobCompactsCarriageReturnProgress(t *testing.T) {
	job := &gitJob{}
	job.appendOutput([]byte("Enumerating objects: 63, done.\nCounting objects:   1% (1/63)\rCounting objects:  50% (32/63)\r"))
	// A trailing CR does not clear the row until the replacement text arrives.
	if !strings.Contains(job.snapshot()["output"].(string), "50% (32/63)") {
		t.Fatalf("lost live progress before next frame: %q", job.snapshot()["output"])
	}
	job.appendOutput([]byte("Counting objects: 100% (63/63), done.\nDelta compression using up to 16 threads\n"))
	got := job.snapshot()["output"].(string)
	want := "Enumerating objects: 63, done.\nCounting objects: 100% (63/63), done.\nDelta compression using up to 16 threads\n"
	if got != want { t.Fatalf("compacted output= %q; want %q", got, want) }
}

func TestGitJobPreservesSplitCRLFAndDiagnostics(t *testing.T) {
	job := &gitJob{}
	job.appendOutput([]byte("warning: first\r"))
	job.appendOutput([]byte("\nfatal: connection reset\r"))
	job.appendOutput([]byte("\n"))
	want := "warning: first\nfatal: connection reset\n"
	if got := job.snapshot()["output"].(string); got != want {
		t.Fatalf("split CRLF output=%q; want %q", got, want)
	}
}

func TestGitJobProgressUpdatesDoNotConsumeCaptureLimit(t *testing.T) {
	job := &gitJob{}
	for i := 0; i < 5000; i++ {
		job.appendOutput([]byte(fmt.Sprintf("Writing objects: %3d%% (20/20)\r", i%101)))
	}
	job.appendOutput([]byte("Writing objects: 100% (20/20), done.\nTo https://example.com/repo.git\n"))
	snapshot := job.snapshot()
	got := snapshot["output"].(string)
	if snapshot["truncated"] != false || len(got) > 150 {
		t.Fatalf("progress frames must not exhaust capture buffer: truncated=%v length=%d", snapshot["truncated"], len(got))
	}
	if strings.Count(got, "Writing objects:") != 1 || !strings.Contains(got, "To https://example.com/repo.git") {
		t.Fatalf("missing terminal-style summary: %q", got)
	}
}
