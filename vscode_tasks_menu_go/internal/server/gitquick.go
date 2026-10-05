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

func runGitInDirectory(parent context.Context, timeout time.Duration, dir string, args ...string) (string, string, bool, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
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

func (s *Server) runGit(parent context.Context, timeout time.Duration, args ...string) (string, string, bool, error) {
	return runGitInDirectory(parent, timeout, s.gitDirectory(parent), args...)
}

type gitChange struct {
	Path          string `json:"path"`
	OriginalPath  string `json:"original_path,omitempty"`
	IndexStatus   string `json:"index_status"`
	WorktreeStatus string `json:"worktree_status"`
	Staged        bool   `json:"staged"`
	Unstaged      bool   `json:"unstaged"`
	Untracked     bool   `json:"untracked"`
	Conflicted    bool   `json:"conflicted,omitempty"`
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
			Conflicted:     isGitUnmergedStatus(x, y),
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
	changes := parseGitStatusZ(out)
	payload := map[string]any{"changes": changes, "truncated": truncated}
	for _, change := range changes {
		if change.Conflicted {
			if state, stateErr := s.gitConflictState(r.Context()); stateErr == nil {
				payload["conflict_state"] = state
			}
			break
		}
	}
	writeJSON(w, http.StatusOK, payload)
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
	set, truncated, runErr := s.gitDiffHunkSet(r.Context(), path, mode)
	if runErr != nil {
		http.Error(w, runErr.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path": set.Path, "mode": set.Mode, "diff": set.Diff,
		"diff_sha256": set.DiffSHA256, "hunks": set.Hunks, "truncated": truncated,
	})
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

func (s *Server) gitRemoteBranchParts(ctx context.Context, remoteRef string) (string, string, error) {
	remoteRef = strings.TrimSpace(remoteRef)
	if remoteRef == "" || strings.ContainsAny(remoteRef, "\r\n\x00") {
		return "", "", fmt.Errorf("invalid remote branch")
	}
	remote := s.gitRemoteNameForRef(ctx, remoteRef)
	if remote == "" {
		return "", "", fmt.Errorf("cannot determine remote for %s", remoteRef)
	}
	branch := strings.TrimPrefix(remoteRef, remote+"/")
	if branch == "" || branch == remoteRef {
		return "", "", fmt.Errorf("invalid remote branch")
	}
	if err := s.validBranchName(ctx, branch); err != nil {
		return "", "", err
	}
	return remote, branch, nil
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

type gitMergeToPreflightResponse struct {
	Current       string `json:"current"`
	CurrentSHA    string `json:"current_sha"`
	Dirty         bool   `json:"dirty"`
	TargetSource  string `json:"target_source"`
	TargetBranch  string `json:"target_branch"`
	TargetRef     string `json:"target_ref"`
	TargetSHA     string `json:"target_sha"`
	PushRemote    string `json:"push_remote"`
	PushBranch    string `json:"push_branch"`
	PushRemoteRef string `json:"push_remote_ref,omitempty"`
	PushRemoteSHA string `json:"push_remote_sha,omitempty"`
	MergeEngine   string `json:"merge_engine"`
	SlowFallback  bool   `json:"slow_fallback"`
}

var gitVersionPattern = regexp.MustCompile(`git version ([0-9]+)\.([0-9]+)`)

func (s *Server) gitSupportsMergeTreeWriteTree(ctx context.Context) bool {
	out, _, _, err := s.runGit(ctx, 3*time.Second, "--version")
	if err != nil {
		return false
	}
	match := gitVersionPattern.FindStringSubmatch(out)
	if len(match) != 3 {
		return false
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	return major > 2 || (major == 2 && minor >= 38)
}

func (s *Server) gitMergeToEngine(ctx context.Context, targetSHA, sourceSHA string) (string, bool) {
	if targetSHA == sourceSHA || s.gitCommitIsAncestor(ctx, targetSHA, sourceSHA) || s.gitCommitIsAncestor(ctx, sourceSHA, targetSHA) {
		return "fast-forward", false
	}
	if s.gitSupportsMergeTreeWriteTree(ctx) {
		return "merge-tree", false
	}
	return "worktree", true
}

func (s *Server) gitRemoteNames(ctx context.Context) []string {
	out, _, _, err := s.runGit(ctx, 3*time.Second, "remote")
	if err != nil {
		return nil
	}
	names := []string{}
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (s *Server) gitConfiguredBranchUpstream(ctx context.Context, branch string) string {
	out, _, _, err := s.runGit(ctx, 3*time.Second, "for-each-ref", "--format=%(upstream:short)", "refs/heads/"+branch)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (s *Server) gitMergeToPushDestination(ctx context.Context, targetSource, branch string) (string, string, error) {
	switch targetSource {
	case "remote":
		remote := s.gitRemoteNameForRef(ctx, branch)
		if remote == "" {
			return "", "", fmt.Errorf("cannot determine remote for target branch")
		}
		pushBranch := strings.TrimPrefix(branch, remote+"/")
		if pushBranch == "" || pushBranch == branch {
			return "", "", fmt.Errorf("invalid remote target branch")
		}
		return remote, pushBranch, nil
	case "local":
		upstream := s.gitConfiguredBranchUpstream(ctx, branch)
		if upstream != "" {
			remote := s.gitRemoteNameForRef(ctx, upstream)
			if remote != "" {
				pushBranch := strings.TrimPrefix(upstream, remote+"/")
				if pushBranch != "" && pushBranch != upstream {
					return remote, pushBranch, nil
				}
			}
		}
		names := s.gitRemoteNames(ctx)
		for _, name := range names {
			if name == "origin" {
				return "origin", branch, nil
			}
		}
		if len(names) == 1 {
			return names[0], branch, nil
		}
		if len(names) == 0 {
			return "", "", fmt.Errorf("Merge To requires a Git remote so the target branch can be pushed")
		}
		return "", "", fmt.Errorf("target branch has no upstream and repository has multiple remotes; configure an upstream first")
	default:
		return "", "", fmt.Errorf("invalid Merge To target source")
	}
}

func (s *Server) gitCommitIsAncestor(ctx context.Context, ancestor, descendant string) bool {
	if ancestor == "" || descendant == "" {
		return false
	}
	_, _, _, err := s.runGit(ctx, 4*time.Second, "merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}

func (s *Server) gitMergeToPreflightData(ctx context.Context, branch, targetSource string) (gitMergeToPreflightResponse, error) {
	branch = strings.TrimSpace(branch)
	targetSource = strings.ToLower(strings.TrimSpace(targetSource))
	if targetSource != "local" && targetSource != "remote" {
		return gitMergeToPreflightResponse{}, fmt.Errorf("invalid Merge To target source")
	}
	if err := s.validBranchName(ctx, branch); err != nil {
		return gitMergeToPreflightResponse{}, err
	}
	current, err := s.gitCurrentBranch(ctx)
	if err != nil {
		return gitMergeToPreflightResponse{}, err
	}
	currentSHA := s.gitBranchSHA(ctx, "HEAD")
	if currentSHA == "" {
		return gitMergeToPreflightResponse{}, fmt.Errorf("cannot resolve current HEAD")
	}
	clean, err := s.gitWorktreeClean(ctx)
	if err != nil {
		return gitMergeToPreflightResponse{}, err
	}

	targetRef := ""
	switch targetSource {
	case "local":
		if !s.localBranchExists(ctx, branch) {
			return gitMergeToPreflightResponse{}, fmt.Errorf("local target branch not found")
		}
		if branch == current {
			return gitMergeToPreflightResponse{}, fmt.Errorf("cannot Merge To the current branch")
		}
		targetRef = branch
	case "remote":
		if !s.remoteBranchExists(ctx, branch) {
			return gitMergeToPreflightResponse{}, fmt.Errorf("remote target branch not found")
		}
		targetRef = branch
	}

	pushRemote, pushBranch, err := s.gitMergeToPushDestination(ctx, targetSource, branch)
	if err != nil {
		return gitMergeToPreflightResponse{}, err
	}
	if pushBranch == current {
		return gitMergeToPreflightResponse{}, fmt.Errorf("cannot Merge To the current branch")
	}
	if _, _, _, err := s.runGit(ctx, 15*time.Second, "fetch", "--prune", pushRemote); err != nil {
		return gitMergeToPreflightResponse{}, fmt.Errorf("cannot refresh remote %s before Merge To: %w", pushRemote, err)
	}

	if targetSource == "remote" {
		targetRef = pushRemote + "/" + pushBranch
		if !s.remoteBranchExists(ctx, targetRef) {
			return gitMergeToPreflightResponse{}, fmt.Errorf("remote target branch not found after refresh")
		}
	}
	targetSHA := s.gitBranchSHA(ctx, targetRef)
	if targetSHA == "" {
		return gitMergeToPreflightResponse{}, fmt.Errorf("cannot resolve target branch revision")
	}
	pushRemoteRef := pushRemote + "/" + pushBranch
	pushRemoteSHA := s.gitBranchSHA(ctx, pushRemoteRef)
	if targetSource == "local" && pushRemoteSHA != "" && !s.gitCommitIsAncestor(ctx, pushRemoteSHA, targetSHA) {
		return gitMergeToPreflightResponse{}, fmt.Errorf("target local branch %s is behind or diverged from %s; sync the target branch before Merge To", branch, pushRemoteRef)
	}

	engine, slowFallback := s.gitMergeToEngine(ctx, targetSHA, currentSHA)
	return gitMergeToPreflightResponse{
		Current: current,
		CurrentSHA: currentSHA,
		Dirty: !clean,
		TargetSource: targetSource,
		TargetBranch: branch,
		TargetRef: targetRef,
		TargetSHA: targetSHA,
		PushRemote: pushRemote,
		PushBranch: pushBranch,
		PushRemoteRef: pushRemoteRef,
		PushRemoteSHA: pushRemoteSHA,
		MergeEngine: engine,
		SlowFallback: slowFallback,
	}, nil
}

func (s *Server) gitMergeToPreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := s.gitMergeToPreflightData(
		r.Context(),
		r.URL.Query().Get("branch"),
		r.URL.Query().Get("target_source"),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
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
	Action            string `json:"action"`
	Path              string `json:"path,omitempty"`
	Message           string `json:"message,omitempty"`
	Branch            string `json:"branch,omitempty"`
	Ref               string `json:"ref,omitempty"`
	Source            string `json:"source,omitempty"`
	TargetSource      string `json:"target_source,omitempty"`
	ExpectedSHA       string `json:"expected_sha,omitempty"`
	ExpectedHeadSHA   string `json:"expected_head_sha,omitempty"`
	ExpectedCurrent   string `json:"expected_current,omitempty"`
	ExpectedSourceSHA string `json:"expected_source_sha,omitempty"`
	ExpectedTargetSHA string `json:"expected_target_sha,omitempty"`
	AllowDirty        bool   `json:"allow_dirty,omitempty"`
	AllowSlowFallback bool   `json:"allow_slow_fallback,omitempty"`
	Repair            string `json:"repair,omitempty"`
	OriginalAction    string `json:"original_action,omitempty"`
	Remote            string `json:"remote,omitempty"`
	RemoteURL         string `json:"remote_url,omitempty"`
	Name              string `json:"name,omitempty"`
	Email             string `json:"email,omitempty"`
	NewBranch         string `json:"new_branch,omitempty"`
	IgnoreID          string `json:"ignore_id,omitempty"`
	LargePath         string `json:"large_path,omitempty"`
	LFSPattern        string `json:"lfs_pattern,omitempty"`
	ConflictSide      string `json:"conflict_side,omitempty"`
	Mode              string `json:"mode,omitempty"`
	WorktreeID        string `json:"worktree_id,omitempty"`
	DirectoryName     string                `json:"directory_name,omitempty"`
	RebasePlan        []gitRebaseActionItem `json:"rebase_plan,omitempty"`
	HunkIndex         int    `json:"hunk_index,omitempty"`
	ExpectedDiffSHA   string `json:"expected_diff_sha,omitempty"`
	Async             bool   `json:"async,omitempty"`
	Confirmed         bool   `json:"confirmed,omitempty"`
}

func joinGitOutput(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		if text := strings.TrimSpace(part); text != "" {
			nonEmpty = append(nonEmpty, text)
		}
	}
	return strings.Join(nonEmpty, "\n")
}

func (s *Server) gitMergeToResult(ctx context.Context, data gitMergeToPreflightResponse) (string, string, bool, error) {
	if data.TargetSHA == data.CurrentSHA {
		return data.TargetSHA, "Target already contains the current HEAD.", false, nil
	}
	if s.gitCommitIsAncestor(ctx, data.TargetSHA, data.CurrentSHA) {
		return data.CurrentSHA, "Fast-forward target to current HEAD.", false, nil
	}
	if s.gitCommitIsAncestor(ctx, data.CurrentSHA, data.TargetSHA) {
		return data.TargetSHA, "Target already contains all commits from current HEAD.", false, nil
	}

	if data.MergeEngine == "merge-tree" {
		stdout, stderr, truncated, err := s.runGit(ctx, gitMergeTimeout, "merge-tree", "--write-tree", data.TargetSHA, data.CurrentSHA)
		if err != nil {
			return "", joinGitOutput(stdout, stderr), truncated, fmt.Errorf("Merge To has conflicts or could not compute a merge tree: %w", err)
		}
		treeSHA := strings.TrimSpace(strings.SplitN(stdout, "\n", 2)[0])
		if treeSHA == "" {
			return "", joinGitOutput(stdout, stderr), truncated, fmt.Errorf("Merge To did not produce a merge tree")
		}
		message := fmt.Sprintf("Merge %s into %s", data.Current, data.PushBranch)
		commitOut, commitErrOut, commitTruncated, commitErr := s.runGit(
			ctx,
			10*time.Second,
			"commit-tree", treeSHA,
			"-p", data.TargetSHA,
			"-p", data.CurrentSHA,
			"-m", message,
		)
		truncated = truncated || commitTruncated
		if commitErr != nil {
			return "", joinGitOutput(stdout, stderr, commitOut, commitErrOut), truncated, fmt.Errorf("cannot create Merge To commit: %w", commitErr)
		}
		resultSHA := strings.TrimSpace(commitOut)
		if resultSHA == "" {
			return "", joinGitOutput(stdout, stderr, commitOut, commitErrOut), truncated, fmt.Errorf("Merge To commit SHA is empty")
		}
		return resultSHA, joinGitOutput("Merged without checkout using git merge-tree.", stderr), truncated, nil
	}

	tmp, err := os.MkdirTemp("", "taskdeck-merge-to-*")
	if err != nil {
		return "", "", false, fmt.Errorf("cannot create temporary Merge To worktree: %w", err)
	}
	if err := os.Remove(tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return "", "", false, fmt.Errorf("cannot prepare temporary Merge To worktree: %w", err)
	}
	defer func() {
		_, _, _, _ = s.runGit(ctx, 8*time.Second, "worktree", "remove", "--force", tmp)
		_ = os.RemoveAll(tmp)
	}()

	addOut, addErrOut, addTruncated, err := s.runGit(ctx, 30*time.Second, "worktree", "add", "--detach", tmp, data.TargetSHA)
	if err != nil {
		return "", joinGitOutput(addOut, addErrOut), addTruncated, fmt.Errorf("cannot prepare temporary target worktree: %w", err)
	}
	mergeOut, mergeErrOut, mergeTruncated, err := runGitInDirectory(ctx, gitMergeTimeout, tmp, "merge", "--no-edit", data.CurrentSHA)
	truncated := addTruncated || mergeTruncated
	output := joinGitOutput("Used temporary worktree fallback because this Git version lacks merge-tree --write-tree.", addOut, addErrOut, mergeOut, mergeErrOut)
	if err != nil {
		return "", output, truncated, fmt.Errorf("Merge To failed in temporary worktree; nothing was pushed: %w", err)
	}
	resultOut, resultErrOut, resultTruncated, err := runGitInDirectory(ctx, 5*time.Second, tmp, "rev-parse", "HEAD")
	truncated = truncated || resultTruncated
	if err != nil {
		return "", joinGitOutput(output, resultOut, resultErrOut), truncated, fmt.Errorf("cannot resolve Merge To result commit: %w", err)
	}
	resultSHA := strings.TrimSpace(resultOut)
	if resultSHA == "" {
		return "", output, truncated, fmt.Errorf("Merge To result commit SHA is empty")
	}
	return resultSHA, output, truncated, nil
}

func (s *Server) gitMergeToAction(ctx context.Context, req gitActionRequest) (string, bool, error) {
	data, err := s.gitMergeToPreflightData(ctx, req.Branch, req.TargetSource)
	if err != nil {
		return "", false, err
	}
	if expectedCurrent := strings.TrimSpace(req.ExpectedCurrent); expectedCurrent != "" && data.Current != expectedCurrent {
		return "", false, fmt.Errorf("current branch changed after confirmation; refresh branches and confirm Merge To again")
	}
	if expectedSource := strings.TrimSpace(req.ExpectedSourceSHA); expectedSource != "" && data.CurrentSHA != expectedSource {
		return "", false, fmt.Errorf("current branch HEAD changed after confirmation; refresh branches and confirm Merge To again")
	}
	if expectedTarget := strings.TrimSpace(req.ExpectedTargetSHA); expectedTarget != "" && data.TargetSHA != expectedTarget {
		return "", false, fmt.Errorf("target branch changed after confirmation; refresh branches and confirm Merge To again")
	}
	if data.Dirty && !req.AllowDirty {
		return "", false, fmt.Errorf("working tree has uncommitted changes; confirm Merge To to continue with committed HEAD only")
	}
	if data.SlowFallback && !req.AllowSlowFallback {
		return "", false, fmt.Errorf("this Git version requires a temporary worktree for Merge To; explicit confirmation is required")
	}

	resultSHA, mergeOutput, truncated, err := s.gitMergeToResult(ctx, data)
	if err != nil {
		return mergeOutput, truncated, err
	}

	pushSpec := resultSHA + ":refs/heads/" + data.PushBranch
	pushOut, pushErrOut, pushTruncated, err := s.runGit(ctx, gitNetworkPushTimeout, "push", data.PushRemote, pushSpec)
	truncated = truncated || pushTruncated
	output := joinGitOutput(mergeOutput, pushOut, pushErrOut)
	if err != nil {
		return output, truncated, fmt.Errorf("Merge To result %s was created locally but push to %s/%s failed: %w", resultSHA, data.PushRemote, data.PushBranch, err)
	}

	if data.TargetSource == "local" && resultSHA != data.TargetSHA {
		updateOut, updateErrOut, updateTruncated, updateErr := s.runGit(ctx, 8*time.Second, "branch", "-f", data.TargetBranch, resultSHA)
		truncated = truncated || updateTruncated
		if updateErr != nil {
			output = joinGitOutput(output, updateOut, updateErrOut, "WARNING: push succeeded, but the local target branch could not be moved; it may be checked out in another worktree.")
		}
	}
	output = joinGitOutput(output, fmt.Sprintf("Merge To complete: %s @ %s -> %s/%s", data.Current, data.CurrentSHA[:12], data.PushRemote, data.PushBranch))
	return output, truncated, nil
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
	case "repair":
		output, truncated, err := s.gitRepairAction(r.Context(), req)
		if err != nil {
			payload := s.gitFailurePayload(r.Context(), action, output, err.Error(), truncated)
			payload["repair"] = req.Repair
			writeJSON(w, http.StatusOK, payload)
			return
		}
		payload := map[string]any{"ok": true, "action": action, "repair": req.Repair, "output": output, "truncated": truncated}
		if strings.HasPrefix(strings.TrimSpace(req.Repair), "conflict_") || strings.TrimSpace(req.Repair) == "continue_in_progress" || strings.TrimSpace(req.Repair) == "merge_to_prepare_resolution" {
			s.gitAttachConflictState(r.Context(), payload)
		}
		writeJSON(w, http.StatusOK, payload)
		return
	case "interactive_rebase":
		output, truncated, err := s.gitInteractiveRebaseExecute(r.Context(), req)
		if err != nil {
			payload := s.gitFailurePayload(r.Context(), action, output, err.Error(), truncated)
			s.gitAttachConflictState(r.Context(), payload)
			writeJSON(w, http.StatusOK, payload)
			return
		}
		payload := map[string]any{"ok": true, "action": action, "output": output, "truncated": truncated}
		s.gitAttachConflictState(r.Context(), payload)
		writeJSON(w, http.StatusOK, payload)
		return
	case "fetch":
		timeout = gitNetworkReadTimeout
		args = []string{"fetch", "--prune"}
	case "pull":
		timeout = gitNetworkReadTimeout
		args = []string{"pull", "--ff-only"}
	case "push":
		timeout = gitNetworkPushTimeout
		if large, inspected := s.gitHubPushLargeBlobPreflight(r.Context()); inspected && len(large) > 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": false, "action": action,
				"output": formatGitLargeBlobOutput("github", large),
				"error": "GitHub push blocked before upload because outgoing commits contain files larger than 100 MiB",
				"failure_code": "file_too_large",
				"provider": "github",
				"limit_bytes": gitHubPushBlobLimit,
				"large_files": large,
				"lfs_plan": s.gitLFSMigrationPlan(r.Context()),
			})
			return
		}
		args = []string{"push"}
	case "stage":
		path, err := validGitRelativePath(req.Path)
		if err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		args = []string{"add", "--", path}
	case "unstage":
		path, err := validGitRelativePath(req.Path)
		if err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		args = []string{"restore", "--staged", "--", path}
	case "stage_hunk", "unstage_hunk", "discard_hunk":
		operation := "stage"
		mode := "worktree"
		if action == "unstage_hunk" {
			operation, mode = "unstage", "staged"
		} else if action == "discard_hunk" {
			operation = "discard"
			if !req.Confirmed {
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": "discard hunk requires explicit confirmation", "failure_code": "confirmation_required"})
				return
			}
		}
		output, truncated, err := s.gitApplyHunk(r.Context(), req.Path, mode, req.HunkIndex, req.ExpectedDiffSHA, operation)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "output": output, "error": err.Error(), "truncated": truncated})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": action, "output": output, "truncated": truncated})
		return
	case "ignore":
		item, added, err := s.gitIgnoreApply(r.Context(), req.Path, req.IgnoreID)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": err.Error()})
			return
		}
		message := "Ignore rule already exists in .gitignore."
		if added {
			message = "Added ignore rule to .gitignore."
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": action, "output": message, "suggestion": item, "added": added})
		return
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
		// Let Git decide whether local changes can move safely with the switch.
		// Non-conflicting staged/unstaged/untracked changes are preserved; Git
		// itself refuses the switch if checkout would overwrite local work.
		args = []string{"switch", branch}
	case "create_branch":
		branch := strings.TrimSpace(req.Branch)
		if err := s.validBranchName(r.Context(), branch); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		// Creating a branch at the current HEAD does not require a clean tree.
		// Keep local changes attached to the new branch exactly as git switch -c
		// normally does.
		args = []string{"switch", "-c", branch}
	case "checkout_commit", "create_branch_at", "create_branch_ref", "reset_commit":
		ref := strings.TrimSpace(req.Ref)
		if !gitCompareCommitPattern.MatchString(ref) {
			http.Error(w, "full commit SHA is required", http.StatusBadRequest)
			return
		}
		resolved, _, _, resolveErr := s.runGit(r.Context(), 4*time.Second, "rev-parse", "--verify", ref+"^{commit}")
		if resolveErr != nil {
			http.Error(w, "commit ref not found", http.StatusNotFound)
			return
		}
		sha := strings.TrimSpace(resolved)
		if !gitCompareCommitPattern.MatchString(sha) {
			http.Error(w, "resolved commit is invalid", http.StatusConflict)
			return
		}
		if expected := strings.TrimSpace(req.ExpectedSHA); expected != "" && expected != sha {
			http.Error(w, "commit changed after confirmation; refresh the graph and confirm again", http.StatusConflict)
			return
		}
		switch action {
		case "checkout_commit":
			clean, cleanErr := s.gitWorktreeClean(r.Context())
			if cleanErr != nil { http.Error(w, cleanErr.Error(), http.StatusConflict); return }
			if !clean {
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": "working tree must be clean before checking out a detached commit", "failure_code": "dirty_worktree"})
				return
			}
			args = []string{"switch", "--detach", sha}
		case "create_branch_at":
			branch := strings.TrimSpace(req.Branch)
			if err := s.validBranchName(r.Context(), branch); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
			if s.localBranchExists(r.Context(), branch) { http.Error(w, "local branch already exists", http.StatusConflict); return }
			clean, cleanErr := s.gitWorktreeClean(r.Context())
			if cleanErr != nil { http.Error(w, cleanErr.Error(), http.StatusConflict); return }
			if !clean {
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": "working tree must be clean before creating a branch at another commit", "failure_code": "dirty_worktree"})
				return
			}
			args = []string{"switch", "-c", branch, sha}
		case "create_branch_ref":
			branch := strings.TrimSpace(req.Branch)
			if err := s.validBranchName(r.Context(), branch); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
			if s.localBranchExists(r.Context(), branch) { http.Error(w, "local branch already exists", http.StatusConflict); return }
			args = []string{"branch", branch, sha}
		case "reset_commit":
			if !req.Confirmed {
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": "reset requires explicit confirmation", "failure_code": "confirmation_required"})
				return
			}
			mode := strings.ToLower(strings.TrimSpace(req.Mode))
			if mode != "soft" && mode != "mixed" && mode != "hard" {
				http.Error(w, "reset mode must be soft, mixed, or hard", http.StatusBadRequest)
				return
			}
			args = []string{"reset", "--" + mode, sha}
		}
	case "worktree_add_branch":
		repo, ok := gitRepositoryFromContext(r.Context())
		if !ok { http.Error(w, "Git repository context unavailable", http.StatusConflict); return }
		branch := strings.TrimSpace(req.Branch)
		if err := s.validBranchName(r.Context(), branch); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		if !s.localBranchExists(r.Context(), branch) { http.Error(w, "local branch not found", http.StatusNotFound); return }
		rows, rowsErr := s.gitWorktreeRows(r.Context())
		if rowsErr != nil { http.Error(w, rowsErr.Error(), http.StatusConflict); return }
		if location := gitWorktreeBranchLocation(rows, branch); location != "" {
			http.Error(w, "branch is already checked out in worktree "+location, http.StatusConflict)
			return
		}
		target, targetErr := gitWorktreeTargetPath(repo, req.DirectoryName)
		if targetErr != nil { http.Error(w, targetErr.Error(), http.StatusBadRequest); return }
		timeout = gitMergeTimeout
		args = []string{"worktree", "add", "--", target, branch}
	case "worktree_add_new_branch":
		repo, ok := gitRepositoryFromContext(r.Context())
		if !ok { http.Error(w, "Git repository context unavailable", http.StatusConflict); return }
		branch := strings.TrimSpace(req.Branch)
		if err := s.validBranchName(r.Context(), branch); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		if s.localBranchExists(r.Context(), branch) { http.Error(w, "local branch already exists", http.StatusConflict); return }
		target, targetErr := gitWorktreeTargetPath(repo, req.DirectoryName)
		if targetErr != nil { http.Error(w, targetErr.Error(), http.StatusBadRequest); return }
		sha, sourceErr := s.gitWorktreeSourceSHA(r.Context(), req.Ref)
		if sourceErr != nil { http.Error(w, sourceErr.Error(), http.StatusBadRequest); return }
		if expected := strings.TrimSpace(req.ExpectedSHA); expected != "" && expected != sha {
			http.Error(w, "worktree source changed after confirmation; refresh and confirm again", http.StatusConflict)
			return
		}
		timeout = gitMergeTimeout
		args = []string{"worktree", "add", "-b", branch, "--", target, sha}
	case "worktree_open":
		if s.OpenWorkspace == nil { http.Error(w, "TaskDeck workspace launcher is unavailable", http.StatusConflict); return }
		rows, rowsErr := s.gitWorktreeRows(r.Context())
		if rowsErr != nil { http.Error(w, rowsErr.Error(), http.StatusConflict); return }
		item, found := gitWorktreeByID(rows, req.WorktreeID)
		if !found { http.Error(w, "worktree identity is stale or unknown", http.StatusNotFound); return }
		if item.Bare { http.Error(w, "cannot open a bare worktree as a TaskDeck project", http.StatusConflict); return }
		if err := s.OpenWorkspace(item.path); err != nil { http.Error(w, err.Error(), http.StatusConflict); return }
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": action, "output": "TaskDeck launch requested for "+item.DisplayPath})
		return
	case "worktree_remove":
		rows, rowsErr := s.gitWorktreeRows(r.Context())
		if rowsErr != nil { http.Error(w, rowsErr.Error(), http.StatusConflict); return }
		item, found := gitWorktreeByID(rows, req.WorktreeID)
		if !found { http.Error(w, "worktree identity is stale or unknown", http.StatusNotFound); return }
		if item.Current || item.Primary { http.Error(w, "cannot remove the active primary worktree", http.StatusConflict); return }
		if !req.Confirmed {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": "worktree removal requires explicit confirmation", "failure_code": "confirmation_required"})
			return
		}
		timeout = gitMergeTimeout
		args = []string{"worktree", "remove", item.path}
	case "worktree_prune":
		if !req.Confirmed {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": "worktree prune requires explicit confirmation", "failure_code": "confirmation_required"})
			return
		}
		args = []string{"worktree", "prune", "--verbose", "--expire", "now"}
	case "delete_branch", "force_delete_branch":
		if !req.Confirmed {
			message := "deleting a local branch requires explicit confirmation"
			if action == "force_delete_branch" { message = "force deleting a local branch requires explicit confirmation" }
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": message, "failure_code": "confirmation_required"})
			return
		}
		branch := strings.TrimSpace(req.Branch)
		if err := s.validBranchName(r.Context(), branch); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		if !s.localBranchExists(r.Context(), branch) { http.Error(w, "local branch not found", http.StatusNotFound); return }
		current, err := s.gitCurrentBranch(r.Context())
		if err != nil { http.Error(w, err.Error(), http.StatusConflict); return }
		if current == branch { http.Error(w, "cannot delete the current branch", http.StatusConflict); return }
		flag := "-d"
		if action == "force_delete_branch" { flag = "-D" }
		args = []string{"branch", flag, branch}
	case "delete_remote_tracking":
		if !req.Confirmed {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": "deleting a local remote-tracking ref requires explicit confirmation", "failure_code": "confirmation_required"})
			return
		}
		remoteRef := strings.TrimSpace(req.Branch)
		if !s.remoteBranchExists(r.Context(), remoteRef) { http.Error(w, "remote-tracking branch not found", http.StatusNotFound); return }
		if _, _, err := s.gitRemoteBranchParts(r.Context(), remoteRef); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		args = []string{"branch", "-dr", remoteRef}
	case "delete_remote_branch":
		if !req.Confirmed {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": "deleting a branch on a Git remote requires explicit confirmation", "failure_code": "confirmation_required"})
			return
		}
		remoteRef := strings.TrimSpace(req.Branch)
		remote, branch, err := s.gitRemoteBranchParts(r.Context(), remoteRef)
		if err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		timeout = gitNetworkPushTimeout
		args = []string{"push", remote, "--delete", branch}
	case "create_tag":
		tagArgs, err := s.gitCreateTagArgs(r.Context(), req.Name, req.Message, req.Ref)
		if err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		args = tagArgs
	case "cherry_pick", "revert_commit":
		clean, err := s.gitWorktreeClean(r.Context())
		if err != nil { http.Error(w, err.Error(), http.StatusConflict); return }
		if !clean {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": "working tree must be clean before "+action, "failure_code": "dirty_worktree"})
			return
		}
		if _, err := s.gitCurrentBranch(r.Context()); err != nil { http.Error(w, err.Error(), http.StatusConflict); return }
		ref := strings.TrimSpace(req.Ref)
		if ref == "" || strings.ContainsAny(ref, "\x00\r\n") { http.Error(w, "commit ref is required", http.StatusBadRequest); return }
		resolved, _, _, err := s.runGit(r.Context(), 4*time.Second, "rev-parse", "--verify", ref+"^{commit}")
		if err != nil || strings.TrimSpace(resolved) == "" { http.Error(w, "commit ref not found", http.StatusNotFound); return }
		sha := strings.TrimSpace(resolved)
		if expected := strings.TrimSpace(req.ExpectedSHA); expected != "" && expected != sha {
			http.Error(w, "commit changed after confirmation; refresh the log and confirm again", http.StatusConflict)
			return
		}
		timeout = gitMergeTimeout
		if action == "cherry_pick" {
			args = []string{"cherry-pick", sha}
		} else {
			args = []string{"revert", "--no-edit", sha}
		}
	case "delete_tag":
		if !req.Confirmed {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": "deleting a Git tag requires explicit confirmation", "failure_code": "confirmation_required"})
			return
		}
		tag, err := validGitTagName(r.Context(), s, req.Name)
		if err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		if _, _, _, err := s.runGit(r.Context(), 3*time.Second, "show-ref", "--verify", "--quiet", "refs/tags/"+tag); err != nil {
			http.Error(w, "Git tag not found", http.StatusNotFound)
			return
		}
		args = []string{"tag", "-d", tag}
	case "merge":
		branch := strings.TrimSpace(req.Branch)
		data, err := s.gitMergePreflightData(r.Context(), branch)
		if err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		if !data.Clean { http.Error(w, "working tree must be clean before merging; commit or stash local changes first", http.StatusConflict); return }
		if expectedCurrent := strings.TrimSpace(req.ExpectedCurrent); expectedCurrent != "" && data.Current != expectedCurrent {
			http.Error(w, "current branch changed after confirmation; refresh branches and confirm again", http.StatusConflict)
			return
		}
		source := strings.ToLower(strings.TrimSpace(req.Source))
		if data.RequiresChoice && source != "local" && source != "remote" {
			http.Error(w, "local and remote branch differ; choose merge source: local or remote", http.StatusBadRequest)
			return
		}
		if source == "" { source = data.DefaultSource }
		mergeRef := ""
		mergeSHA := ""
		switch source {
		case "local":
			mergeRef, mergeSHA = data.LocalRef, data.LocalSHA
		case "remote":
			mergeRef, mergeSHA = data.RemoteRef, data.RemoteSHA
		default:
			http.Error(w, "invalid merge source", http.StatusBadRequest)
			return
		}
		if mergeRef == "" { http.Error(w, "selected merge source is unavailable", http.StatusBadRequest); return }
		if expected := strings.TrimSpace(req.ExpectedSHA); expected != "" && mergeSHA != expected {
			http.Error(w, "merge source changed after confirmation; refresh branches and confirm again", http.StatusConflict)
			return
		}
		if mergeRef == data.Current {
			http.Error(w, "cannot merge the current branch into itself", http.StatusBadRequest)
			return
		}
		timeout = gitMergeTimeout
		args = []string{"merge", "--no-edit", mergeRef}
	case "merge_to":
		output, truncated, err := s.gitMergeToAction(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusOK, s.gitFailurePayload(r.Context(), action, output, err.Error(), truncated))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": action, "output": output, "truncated": truncated})
		return
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
	if req.Async && gitAsyncActionAllowed(action) {
		repo, ok := gitRepositoryFromContext(r.Context())
		if !ok {
			http.Error(w, "Git repository context unavailable", http.StatusConflict)
			return
		}
		job, err := s.startGitCommandJob(repo, action, args, timeout)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "action": action, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"ok": true, "action": action, "async": true,
			"job_id": job.ID, "state": "running", "command": job.Command,
		})
		return
	}
	stdout, stderr, truncated, err := s.runGit(r.Context(), timeout, args...)
	output := strings.TrimSpace(strings.TrimSpace(stdout) + "\n" + strings.TrimSpace(stderr))
	if err != nil {
		writeJSON(w, http.StatusOK, s.gitFailurePayload(r.Context(), action, output, err.Error(), truncated))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": action, "output": output, "truncated": truncated})
}
