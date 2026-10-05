package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const gitJobOutputLimit = 2 << 20

type gitJob struct {
	mu sync.Mutex

	ID         string
	RepoID     string
	Action     string
	Command    string
	State      string
	Output     string
	Error      string
	Truncated  bool
	StartedAt  time.Time
	FinishedAt time.Time
	Result     map[string]any

	cancel context.CancelFunc
}

func newGitJobID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("git-%d", time.Now().UnixNano())
}

func (j *gitJob) appendOutput(data []byte) {
	if len(data) == 0 {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	remaining := gitJobOutputLimit - len(j.Output)
	if remaining <= 0 {
		j.Truncated = true
		return
	}
	if len(data) > remaining {
		data = data[:remaining]
		j.Truncated = true
	}
	j.Output += string(data)
}

func (j *gitJob) snapshot() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	payload := map[string]any{
		"id":         j.ID,
		"repo_id":    j.RepoID,
		"action":     j.Action,
		"command":    j.Command,
		"state":      j.State,
		"output":     j.Output,
		"truncated":  j.Truncated,
		"started_at": j.StartedAt.UTC().Format(time.RFC3339Nano),
	}
	if !j.FinishedAt.IsZero() {
		payload["finished_at"] = j.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	if j.Error != "" {
		payload["error"] = j.Error
	}
	for key, value := range j.Result {
		payload[key] = value
	}
	return payload
}

type gitJobWriter struct{ job *gitJob }

func (w gitJobWriter) Write(p []byte) (int, error) {
	w.job.appendOutput(p)
	return len(p), nil
}

func (s *Server) registerGitJob(job *gitJob) {
	s.gitJobsMu.Lock()
	if s.gitJobs == nil {
		s.gitJobs = map[string]*gitJob{}
	}
	s.gitJobs[job.ID] = job
	s.gitJobsMu.Unlock()
}

func (s *Server) findGitJob(id string) (*gitJob, bool) {
	s.gitJobsMu.Lock()
	defer s.gitJobsMu.Unlock()
	job, ok := s.gitJobs[id]
	return job, ok
}

func gitAsyncActionAllowed(action string) bool {
	switch strings.TrimSpace(action) {
	case "fetch", "pull", "push", "merge", "delete_remote_branch", "submodule_init", "submodule_update", "submodule_checkout_expected", "submodule_update_recursive", "submodule_sync":
		return true
	default:
		return false
	}
}

func gitAsyncArgs(action string, args []string) []string {
	out := append([]string(nil), args...)
	switch action {
	case "fetch", "pull", "push":
		for _, arg := range out {
			if arg == "--progress" || arg == "--no-progress" {
				return out
			}
		}
		if len(out) > 0 {
			out = append(out[:1], append([]string{"--progress"}, out[1:]...)...)
		}
	}
	return out
}

func gitCommandDisplay(args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "git")
	for _, arg := range args {
		if arg == "" || strings.ContainsAny(arg, " \t\r\n'\"\\$\x60") {
			parts = append(parts, quoteGitDisplayArg(arg))
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

func quoteGitDisplayArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (s *Server) startGitCommandJob(repo gitRepository, action string, args []string, timeout time.Duration) (*gitJob, error) {
	if !gitAsyncActionAllowed(action) {
		return nil, fmt.Errorf("Git action %s does not support background jobs", action)
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	job := &gitJob{
		ID: newGitJobID(), RepoID: repo.ID, Action: action,
		Command: gitCommandDisplay(args), State: "running",
		StartedAt: time.Now(), cancel: cancel,
	}
	s.registerGitJob(job)
	go s.runGitCommandJob(ctx, repo, job, gitAsyncArgs(action, args))
	return job, nil
}

func (s *Server) runGitCommandJob(ctx context.Context, repo gitRepository, job *gitJob, args []string) {
	defer job.cancel()
	command := exec.Command("git", args...)
	command.Dir = repo.Root
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "LC_ALL=C")
	prepareGitJobProcess(command)
	writer := gitJobWriter{job: job}
	command.Stdout = writer
	command.Stderr = writer

	if err := command.Start(); err != nil {
		job.mu.Lock()
		job.State = "failed"
		job.Error = err.Error()
		job.FinishedAt = time.Now()
		job.Result = map[string]any{"ok": false, "failure_code": classifyGitFailure(job.Action, job.Output, err.Error())}
		job.mu.Unlock()
		return
	}

	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	var err error
	select {
	case err = <-wait:
	case <-ctx.Done():
		terminateGitJobProcess(command)
		select {
		case err = <-wait:
		case <-time.After(4 * time.Second):
			killGitJobProcess(command)
			err = <-wait
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("git operation timed out")
		} else {
			err = fmt.Errorf("git operation canceled")
		}
	}

	job.mu.Lock()
	output := strings.TrimSpace(job.Output)
	truncated := job.Truncated
	job.mu.Unlock()

	repoCtx := withGitRepository(context.Background(), repo)
	job.mu.Lock()
	defer job.mu.Unlock()
	job.FinishedAt = time.Now()
	if err == nil {
		job.State = "success"
		job.Result = map[string]any{"ok": true, "action": job.Action}
		return
	}
	if strings.Contains(err.Error(), "canceled") {
		job.State = "canceled"
		job.Error = "git operation canceled"
		job.Result = map[string]any{"ok": false, "action": job.Action, "failure_code": "canceled"}
		return
	}
	job.State = "failed"
	job.Error = err.Error()
	payload := s.gitFailurePayload(repoCtx, job.Action, output, err.Error(), truncated)
	delete(payload, "output")
	delete(payload, "truncated")
	job.Result = payload
}

func (s *Server) gitJobsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Error(w, "Git job id is required", http.StatusBadRequest)
		return
	}
	job, ok := s.findGitJob(id)
	if !ok {
		http.Error(w, "Git job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, job.snapshot())
}

func (s *Server) gitJobsControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Action) != "cancel" {
		http.Error(w, "unsupported Git job control action", http.StatusBadRequest)
		return
	}
	job, ok := s.findGitJob(strings.TrimSpace(req.ID))
	if !ok {
		http.Error(w, "Git job not found", http.StatusNotFound)
		return
	}
	job.mu.Lock()
	if job.State != "running" {
		state := job.State
		job.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": job.ID, "state": state})
		return
	}
	cancel := job.cancel
	job.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "id": job.ID, "state": "canceling"})
}
