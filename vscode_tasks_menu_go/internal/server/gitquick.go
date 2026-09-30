package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const gitOutputLimit = 512 << 10

type cappedGitBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (w *cappedGitBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := w.limit - w.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = w.buf.Write(p[:remaining])
			w.truncated = true
		} else {
			_, _ = w.buf.Write(p)
		}
	} else if len(p) > 0 {
		w.truncated = true
	}
	return original, nil
}

func (w *cappedGitBuffer) String() string { return w.buf.String() }

func (s *Server) runGit(parent context.Context, timeout time.Duration, args ...string) (string, string, bool, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.Workspace
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "LC_ALL=C")
	stdout := &cappedGitBuffer{limit: gitOutputLimit}
	stderr := &cappedGitBuffer{limit: gitOutputLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return stdout.String(), stderr.String(), stdout.truncated || stderr.truncated, fmt.Errorf("git operation timed out")
	}
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), stderr.String(), stdout.truncated || stderr.truncated, fmt.Errorf("%s", message)
	}
	return stdout.String(), stderr.String(), stdout.truncated || stderr.truncated, nil
}

type gitChange struct {
	Path          string `json:"path"`
	OriginalPath  string `json:"original_path,omitempty"`
	IndexStatus   string `json:"index_status"`
	WorktreeStatus string `json:"worktree_status"`
	Staged        bool   `json:"staged"`
	Unstaged      bool   `json:"unstaged"`
	Untracked     bool   `json:"untracked"`
}

func parseGitStatusZ(raw string) []gitChange {
	parts := strings.Split(raw, "\x00")
	changes := make([]gitChange, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		record := parts[i]
		if len(record) < 4 || record[2] != ' ' {
			continue
		}
		x, y := record[0], record[1]
		change := gitChange{
			Path:           record[3:],
			IndexStatus:    string(x),
			WorktreeStatus: string(y),
			Staged:         x != ' ' && x != '?',
			Unstaged:       y != ' ' && y != '?',
			Untracked:      x == '?' && y == '?',
		}
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			if i+1 < len(parts) && parts[i+1] != "" {
				change.OriginalPath = parts[i+1]
				i++
			}
		}
		changes = append(changes, change)
	}
	return changes
}

func (s *Server) gitChanges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	out, _, truncated, err := s.runGit(r.Context(), 4*time.Second, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": parseGitStatusZ(out), "truncated": truncated})
}

func validGitRelativePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, '\x00') || filepath.IsAbs(value) {
		return "", fmt.Errorf("invalid repository path")
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid repository path")
	}
	return filepath.ToSlash(clean), nil
}

func (s *Server) gitDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := validGitRelativePath(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	args := []string{"diff", "--no-ext-diff", "--no-color", "--unified=3"}
	if mode == "staged" {
		args = append(args, "--cached")
	} else if mode != "" && mode != "worktree" {
		http.Error(w, "invalid diff mode", http.StatusBadRequest)
		return
	}
	args = append(args, "--", path)
	out, _, truncated, runErr := s.runGit(r.Context(), 5*time.Second, args...)
	if runErr != nil {
		http.Error(w, runErr.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "mode": mode, "diff": out, "truncated": truncated})
}

type gitCommitRow struct {
	SHA     string `json:"sha"`
	Short   string `json:"short"`
	Date    string `json:"date"`
	Author  string `json:"author"`
	Subject string `json:"subject"`
}

func (s *Server) gitLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 30
	if value, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && value > 0 && value <= 100 {
		limit = value
	}
	format := "%H%x09%h%x09%ad%x09%an%x09%s"
	out, _, truncated, err := s.runGit(r.Context(), 5*time.Second, "log", "-n", strconv.Itoa(limit), "--date=iso-strict", "--pretty=format:"+format)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"commits": []gitCommitRow{}, "truncated": false})
		return
	}
	rows := make([]gitCommitRow, 0, limit)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) != 5 {
			continue
		}
		rows = append(rows, gitCommitRow{SHA: fields[0], Short: fields[1], Date: fields[2], Author: fields[3], Subject: fields[4]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"commits": rows, "truncated": truncated})
}

type gitBranchRow struct {
	Name     string `json:"name"`
	Current  bool   `json:"current,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Remote   bool   `json:"remote,omitempty"`
}

func parseBranchRows(out string, remote bool) []gitBranchRow {
	rows := make([]gitBranchRow, 0)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		row := gitBranchRow{Name: strings.TrimSpace(fields[0]), Remote: remote}
		if row.Name == "" || strings.HasSuffix(row.Name, "/HEAD") {
			continue
		}
		if len(fields) > 1 {
			row.Current = strings.TrimSpace(fields[1]) == "*"
		}
		if len(fields) > 2 {
			row.Upstream = strings.TrimSpace(fields[2])
		}
		rows = append(rows, row)
	}
	return rows
}

func (s *Server) gitBranches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	format := "%(refname:short)%09%(HEAD)%09%(upstream:short)"
	localOut, _, _, localErr := s.runGit(r.Context(), 4*time.Second, "for-each-ref", "--format="+format, "refs/heads")
	if localErr != nil {
		http.Error(w, localErr.Error(), http.StatusConflict)
		return
	}
	remoteOut, _, _, _ := s.runGit(r.Context(), 4*time.Second, "for-each-ref", "--format="+format, "refs/remotes")
	writeJSON(w, http.StatusOK, map[string]any{"local": parseBranchRows(localOut, false), "remote": parseBranchRows(remoteOut, true)})
}

func (s *Server) gitStashes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	out, _, truncated, _ := s.runGit(r.Context(), 4*time.Second, "stash", "list", "--format=%gd%x09%H%x09%cr%x09%s")
	type stashRow struct {
		Ref     string `json:"ref"`
		SHA     string `json:"sha"`
		When    string `json:"when"`
		Subject string `json:"subject"`
	}
	rows := []stashRow{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) == 4 {
			rows = append(rows, stashRow{Ref: fields[0], SHA: fields[1], When: fields[2], Subject: fields[3]})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"stashes": rows, "truncated": truncated})
}

func (s *Server) localBranchExists(ctx context.Context, branch string) bool {
	_, _, _, err := s.runGit(ctx, 3*time.Second, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func (s *Server) remoteBranchExists(ctx context.Context, branch string) bool {
	_, _, _, err := s.runGit(ctx, 3*time.Second, "show-ref", "--verify", "--quiet", "refs/remotes/"+branch)
	return err == nil
}

func (s *Server) validBranchName(ctx context.Context, branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" || strings.ContainsAny(branch, "\r\n\x00") {
		return fmt.Errorf("invalid branch name")
	}
	_, _, _, err := s.runGit(ctx, 3*time.Second, "check-ref-format", "--branch", branch)
	if err != nil {
		return fmt.Errorf("invalid branch name")
	}
	return nil
}

type gitMergePreflightResponse struct {
	Branch         string `json:"branch"`
	Current        string `json:"current"`
	Clean          bool   `json:"clean"`
	LocalRef       string `json:"local_ref,omitempty"`
	RemoteRef      string `json:"remote_ref,omitempty"`
	LocalSHA       string `json:"local_sha,omitempty"`
	RemoteSHA      string `json:"remote_sha,omitempty"`
	Same           bool   `json:"same"`
	RequiresChoice bool   `json:"requires_choice"`
	DefaultSource  string `json:"default_source,omitempty"`
}

func (s *Server) gitCurrentBranch(ctx context.Context) (string, error) {
	out, _, _, err := s.runGit(ctx, 3*time.Second, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(out)
	if branch == "" {
		return "", fmt.Errorf("cannot merge while HEAD is detached")
	}
	return branch, nil
}

func (s *Server) gitBranchSHA(ctx context.Context, ref string) string {
	if strings.TrimSpace(ref) == "" {
		return ""
	}
	out, _, _, err := s.runGit(ctx, 3*time.Second, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (s *Server) gitLocalBranchUpstream(ctx context.Context, branch string) string {
	out, _, _, err := s.runGit(ctx, 3*time.Second, "for-each-ref", "--format=%(upstream:short)", "refs/heads/"+branch)
	if err != nil {
		return ""
	}
	upstream := strings.TrimSpace(out)
	if upstream != "" && s.remoteBranchExists(ctx, upstream) {
		return upstream
	}
	candidate := "origin/" + branch
	if s.remoteBranchExists(ctx, candidate) {
		return candidate
	}
	return ""
}

func (s *Server) gitLocalBranchTrackingRemote(ctx context.Context, remoteRef string) string {
	out, _, _, err := s.runGit(ctx, 3*time.Second, "for-each-ref", "--format=%(refname:short)%09%(upstream:short)", "refs/heads")
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			fields := strings.SplitN(line, "\t", 2)
			if len(fields) == 2 && strings.TrimSpace(fields[1]) == remoteRef {
				local := strings.TrimSpace(fields[0])
				if local != "" {
					return local
				}
			}
		}
	}
	if slash := strings.Index(remoteRef, "/"); slash > 0 && slash+1 < len(remoteRef) {
		candidate := remoteRef[slash+1:]
		if s.localBranchExists(ctx, candidate) {
			return candidate
		}
	}
	return ""
}

func (s *Server) gitMergeRefs(ctx context.Context, branch string) (string, string, error) {
	branch = strings.TrimSpace(branch)
	if s.localBranchExists(ctx, branch) {
		return branch, s.gitLocalBranchUpstream(ctx, branch), nil
	}
	if s.remoteBranchExists(ctx, branch) {
		return s.gitLocalBranchTrackingRemote(ctx, branch), branch, nil
	}
	return "", "", fmt.Errorf("branch not found")
}

func (s *Server) gitRemoteNameForRef(ctx context.Context, remoteRef string) string {
	remoteRef = strings.TrimSpace(remoteRef)
	if remoteRef == "" {
		return ""
	}
	out, _, _, err := s.runGit(ctx, 3*time.Second, "remote")
	if err != nil {
		return ""
	}
	best := ""
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name != "" && strings.HasPrefix(remoteRef, name+"/") && len(name) > len(best) {
			best = name
		}
	}
	return best
}

func (s *Server) gitRefreshMergeRemote(ctx context.Context, remoteRef string) error {
	name := s.gitRemoteNameForRef(ctx, remoteRef)
	if name == "" {
		return nil
	}
	_, _, _, err := s.runGit(ctx, 15*time.Second, "fetch", "--prune", name)
	if err != nil {
		return fmt.Errorf("cannot refresh remote %s before merge: %w", name, err)
	}
	return nil
}

func (s *Server) gitMergePreflightData(ctx context.Context, branch string) (gitMergePreflightResponse, error) {
	if err := s.validBranchName(ctx, branch); err != nil {
		return gitMergePreflightResponse{}, err
	}
	current, err := s.gitCurrentBranch(ctx)
	if err != nil {
		return gitMergePreflightResponse{}, err
	}
	clean, err := s.gitWorktreeClean(ctx)
	if err != nil {
		return gitMergePreflightResponse{}, err
	}
	localRef, remoteRef, err := s.gitMergeRefs(ctx, branch)
	if err != nil {
		return gitMergePreflightResponse{}, err
	}
	if remoteRef != "" {
		if err := s.gitRefreshMergeRemote(ctx, remoteRef); err != nil {
			return gitMergePreflightResponse{}, err
		}
		localRef, remoteRef, err = s.gitMergeRefs(ctx, branch)
		if err != nil {
			return gitMergePreflightResponse{}, err
		}
	}
	localSHA := s.gitBranchSHA(ctx, localRef)
	remoteSHA := s.gitBranchSHA(ctx, remoteRef)
	same := localSHA != "" && remoteSHA != "" && localSHA == remoteSHA
	defaultSource := ""
	switch {
	case localRef != "" && remoteRef == "":
		defaultSource = "local"
	case remoteRef != "" && localRef == "":
		defaultSource = "remote"
	case same && localRef != "":
		defaultSource = "local"
	}
	return gitMergePreflightResponse{
		Branch: branch, Current: current, Clean: clean,
		LocalRef: localRef, RemoteRef: remoteRef,
		LocalSHA: localSHA, RemoteSHA: remoteSHA,
		Same: same,
		RequiresChoice: localRef != "" && remoteRef != "" && !same,
		DefaultSource: defaultSource,
	}, nil
}

func (s *Server) gitMergePreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	data, err := s.gitMergePreflightData(r.Context(), branch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !data.Clean {
		http.Error(w, "working tree must be clean before merging; commit or stash local changes first", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) gitCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base := strings.TrimSpace(r.URL.Query().Get("base"))
	if err := s.validBranchName(r.Context(), base); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.localBranchExists(r.Context(), base) && !s.remoteBranchExists(r.Context(), base) {
		http.Error(w, "branch not found", http.StatusNotFound)
		return
	}
	stat, _, truncated, err := s.runGit(r.Context(), 6*time.Second, "diff", "--no-color", "--stat", base+"...HEAD")
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	names, _, namesTruncated, _ := s.runGit(r.Context(), 6*time.Second, "diff", "--no-color", "--name-status", base+"...HEAD")
	writeJSON(w, http.StatusOK, map[string]any{"base": base, "stat": stat, "files": names, "truncated": truncated || namesTruncated})
}

func (s *Server) gitAheadBehind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	upstreamOut, _, _, err := s.runGit(r.Context(), 3*time.Second, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"upstream": "", "ahead": 0, "behind": 0, "commits": []map[string]string{}})
		return
	}
	upstream := strings.TrimSpace(upstreamOut)
	countOut, _, _, err := s.runGit(r.Context(), 4*time.Second, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	fields := strings.Fields(countOut)
	ahead, behind := 0, 0
	if len(fields) >= 2 {
		ahead, _ = strconv.Atoi(fields[0])
		behind, _ = strconv.Atoi(fields[1])
	}
	detailOut, _, truncated, _ := s.runGit(r.Context(), 5*time.Second, "log", "--left-right", "--cherry-pick", "--pretty=format:%m%x09%h%x09%s", "HEAD...@{upstream}")
	commits := []map[string]string{}
	for _, line := range strings.Split(detailOut, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) == 3 {
			direction := "ahead"
			if strings.TrimSpace(parts[0]) == ">" {
				direction = "behind"
			}
			commits = append(commits, map[string]string{"direction": direction, "sha": parts[1], "subject": parts[2]})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"upstream": upstream, "ahead": ahead, "behind": behind, "commits": commits, "truncated": truncated})
}

func (s *Server) gitWorktreeClean(ctx context.Context) (bool, error) {
	out, _, _, err := s.runGit(ctx, 4*time.Second, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

var stashRefPattern = regexp.MustCompile(`^stash@\{[0-9]+\}$`)

type gitActionRequest struct {
	Action  string `json:"action"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Source  string `json:"source,omitempty"`
}

func (s *Server) gitAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req gitActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	action := strings.TrimSpace(req.Action)
	args := []string{}
	timeout := 20 * time.Second
	switch action {
	case "fetch":
		args = []string{"fetch", "--prune"}
	case "pull":
		args = []string{"pull", "--ff-only"}
	case "push":
		args = []string{"push"}
	case "stage":
		path, err := validGitRelativePath(req.Path)
		if err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		args = []string{"add", "--", path}
	case "unstage":
		path, err := validGitRelativePath(req.Path)
		if err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		args = []string{"restore", "--staged", "--", path}
	case "stage_all":
		args = []string{"add", "-A"}
	case "commit":
		message := strings.TrimSpace(req.Message)
		if message == "" || len(message) > 2000 || strings.ContainsRune(message, '\x00') {
			http.Error(w, "commit message required", http.StatusBadRequest)
			return
		}
		args = []string{"commit", "-m", message}
	case "switch":
		branch := strings.TrimSpace(req.Branch)
		if err := s.validBranchName(r.Context(), branch); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		if !s.localBranchExists(r.Context(), branch) { http.Error(w, "local branch not found", http.StatusNotFound); return }
		clean, err := s.gitWorktreeClean(r.Context()); if err != nil { http.Error(w, err.Error(), http.StatusConflict); return }
		if !clean { http.Error(w, "working tree must be clean before switching branch; commit or stash changes first", http.StatusConflict); return }
		args = []string{"switch", branch}
	case "create_branch":
		branch := strings.TrimSpace(req.Branch)
		if err := s.validBranchName(r.Context(), branch); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		clean, err := s.gitWorktreeClean(r.Context()); if err != nil { http.Error(w, err.Error(), http.StatusConflict); return }
		if !clean { http.Error(w, "working tree must be clean before creating/switching branch; commit or stash changes first", http.StatusConflict); return }
		args = []string{"switch", "-c", branch}
	case "merge":
		branch := strings.TrimSpace(req.Branch)
		data, err := s.gitMergePreflightData(r.Context(), branch)
		if err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		if !data.Clean { http.Error(w, "working tree must be clean before merging; commit or stash local changes first", http.StatusConflict); return }
		source := strings.ToLower(strings.TrimSpace(req.Source))
		if data.RequiresChoice && source != "local" && source != "remote" {
			http.Error(w, "local and remote branch differ; choose merge source: local or remote", http.StatusBadRequest)
			return
		}
		if source == "" { source = data.DefaultSource }
		mergeRef := ""
		switch source {
		case "local":
			mergeRef = data.LocalRef
		case "remote":
			mergeRef = data.RemoteRef
		default:
			http.Error(w, "invalid merge source", http.StatusBadRequest)
			return
		}
		if mergeRef == "" { http.Error(w, "selected merge source is unavailable", http.StatusBadRequest); return }
		if mergeRef == data.Current {
			http.Error(w, "cannot merge the current branch into itself", http.StatusBadRequest)
			return
		}
		args = []string{"merge", "--no-edit", mergeRef}
	case "stash_push":
		message := strings.TrimSpace(req.Message)
		if message == "" { message = "vscode_tasks_menu " + time.Now().Format("2006-01-02 15:04:05") }
		if len(message) > 500 || strings.ContainsRune(message, '\x00') { http.Error(w, "invalid stash message", http.StatusBadRequest); return }
		args = []string{"stash", "push", "-u", "-m", message}
	case "stash_pop":
		ref := strings.TrimSpace(req.Ref)
		if ref != "" && !stashRefPattern.MatchString(ref) { http.Error(w, "invalid stash ref", http.StatusBadRequest); return }
		args = []string{"stash", "pop"}; if ref != "" { args = append(args, ref) }
	default:
		http.Error(w, "unsupported git action", http.StatusBadRequest)
		return
	}
	stdout, stderr, truncated, err := s.runGit(r.Context(), timeout, args...)
	output := strings.TrimSpace(strings.TrimSpace(stdout) + "\n" + strings.TrimSpace(stderr))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "output": output, "error": err.Error(), "truncated": truncated})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": action, "output": output, "truncated": truncated})
}
