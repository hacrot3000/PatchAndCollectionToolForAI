package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	gitHubDefaultPushBlobLimit = int64(100 << 20)
	gitNetworkPushTimeout      = 10 * time.Minute
	gitNetworkReadTimeout      = 2 * time.Minute
)

var gitHubPushBlobLimit = gitHubDefaultPushBlobLimit

type gitLargeBlobInfo struct {
	Path       string  `json:"path"`
	SizeBytes  int64   `json:"size_bytes"`
	SizeMiB    float64 `json:"size_mib"`
	LimitBytes int64   `json:"limit_bytes"`
	LimitMiB   float64 `json:"limit_mib"`
}

type gitLFSMigrationPlan struct {
	Available       bool   `json:"available"`
	Version         string `json:"version,omitempty"`
	Branch          string `json:"branch,omitempty"`
	Upstream        string `json:"upstream,omitempty"`
	Range           string `json:"range,omitempty"`
	OutgoingCommits int    `json:"outgoing_commits,omitempty"`
	Head            string `json:"head,omitempty"`
}

var gitHubRemotePattern = regexp.MustCompile(`(?i)(^|[@/:])github\.com([/:]|$)`)

func isGitHubRemoteURL(value string) bool {
	return gitHubRemotePattern.MatchString(strings.TrimSpace(value))
}

func runGitInputInDirectory(parent context.Context, timeout time.Duration, dir, input string, args ...string) (string, string, bool, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "LC_ALL=C")
	cmd.Stdin = strings.NewReader(input)
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

func (s *Server) gitPushUpstream(ctx context.Context) (branch, upstream, remote, remoteURL string, err error) {
	branch, err = s.gitCurrentBranchName(ctx)
	if err != nil {
		return "", "", "", "", err
	}
	out, _, _, upstreamErr := s.runGit(ctx, 5*time.Second, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if upstreamErr != nil || strings.TrimSpace(out) == "" {
		return branch, "", "", "", fmt.Errorf("current branch has no upstream")
	}
	upstream = strings.TrimSpace(out)
	out, _, _, remoteErr := s.runGit(ctx, 3*time.Second, "config", "--get", "branch."+branch+".remote")
	if remoteErr != nil || strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "." {
		return branch, upstream, "", "", fmt.Errorf("upstream remote is unavailable")
	}
	remote = strings.TrimSpace(out)
	out, _, _, urlErr := s.runGit(ctx, 5*time.Second, "remote", "get-url", "--push", remote)
	if urlErr != nil || strings.TrimSpace(out) == "" {
		return branch, upstream, remote, "", fmt.Errorf("push remote URL is unavailable")
	}
	remoteURL = strings.TrimSpace(out)
	return branch, upstream, remote, remoteURL, nil
}

func (s *Server) gitLFSVersion(ctx context.Context) (string, bool) {
	stdout, stderr, _, err := s.runGit(ctx, 5*time.Second, "lfs", "version")
	if err != nil {
		return "", false
	}
	version := strings.TrimSpace(joinGitOutput(stdout, stderr))
	if version == "" {
		return "", false
	}
	if len(version) > 512 {
		version = version[:512]
	}
	return version, true
}

func (s *Server) gitLFSMigrationPlan(ctx context.Context) gitLFSMigrationPlan {
	version, available := s.gitLFSVersion(ctx)
	plan := gitLFSMigrationPlan{Available: available, Version: version}
	if !available {
		return plan
	}
	branch, upstream, _, _, err := s.gitPushUpstream(ctx)
	if err != nil {
		return plan
	}
	if _, _, _, err := s.runGit(ctx, 8*time.Second, "merge-base", "--is-ancestor", upstream, "HEAD"); err != nil {
		return plan
	}
	countOut, _, _, err := s.runGit(ctx, 8*time.Second, "rev-list", "--count", upstream+"..HEAD")
	if err != nil {
		return plan
	}
	count, err := strconv.Atoi(strings.TrimSpace(countOut))
	if err != nil || count < 0 {
		return plan
	}
	headOut, _, _, err := s.runGit(ctx, 5*time.Second, "rev-parse", "HEAD")
	if err != nil {
		return plan
	}
	plan.Branch = branch
	plan.Upstream = upstream
	plan.Range = upstream + "..HEAD"
	plan.OutgoingCommits = count
	plan.Head = strings.TrimSpace(headOut)
	return plan
}

func validGitLFSPattern(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("Git LFS include pattern is required")
	}
	if len(value) > 512 || strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("Git LFS include pattern is invalid")
	}
	if strings.Contains(value, ",") {
		return "", fmt.Errorf("Git LFS include pattern must contain exactly one pattern")
	}
	return value, nil
}

func gitLFSMigrateArgs(pattern string) []string {
	return []string{"lfs", "migrate", "import", "--yes", "--skip-fetch", "--include=" + pattern}
}

func (s *Server) gitLargeFileMigrateLFS(ctx context.Context, rawPath, rawPattern string, limit int64) (string, bool, error) {
	path, err := validGitRelativePath(rawPath)
	if err != nil {
		return "", false, err
	}
	item, upstream, err := s.gitLargePathIsOutgoing(ctx, path, limit)
	if err != nil {
		return "", false, err
	}
	version, available := s.gitLFSVersion(ctx)
	if !available {
		return "", false, fmt.Errorf("Git LFS is not installed or git lfs version failed")
	}
	clean, err := s.gitWorktreeClean(ctx)
	if err != nil {
		return "", false, err
	}
	if !clean {
		return "", false, fmt.Errorf("working tree and index must be clean before Git LFS history migration")
	}
	if _, _, _, err := s.runGit(ctx, 8*time.Second, "merge-base", "--is-ancestor", upstream, "HEAD"); err != nil {
		return "", false, fmt.Errorf("upstream is not an ancestor of HEAD; integrate remote history before Git LFS migration")
	}
	pattern := strings.TrimSpace(rawPattern)
	if pattern == "" {
		pattern = path
	}
	pattern, err = validGitLFSPattern(pattern)
	if err != nil {
		return "", false, err
	}
	headOut, _, _, err := s.runGit(ctx, 5*time.Second, "rev-parse", "HEAD")
	if err != nil {
		return "", false, err
	}
	originalHEAD := strings.TrimSpace(headOut)
	countOut, _, _, err := s.runGit(ctx, 8*time.Second, "rev-list", "--count", upstream+"..HEAD")
	if err != nil {
		return "", false, err
	}
	count := strings.TrimSpace(countOut)

	var output []string
	run := func(timeout time.Duration, args ...string) error {
		stdout, stderr, truncated, runErr := s.runGit(ctx, timeout, args...)
		if text := joinGitOutput(stdout, stderr); text != "" {
			output = append(output, text)
		}
		if truncated {
			output = append(output, "Git output was truncated.")
		}
		return runErr
	}
	if err := run(20*time.Second, "lfs", "install", "--local"); err != nil {
		return joinGitOutput(output...), false, fmt.Errorf("initialize Git LFS for this repository: %w", err)
	}
	if err := run(10*time.Minute, gitLFSMigrateArgs(pattern)...); err != nil {
		return joinGitOutput(output...), false, fmt.Errorf("migrate unpushed history to Git LFS: %w", err)
	}
	remaining, err := s.gitOutgoingLargeBlobs(ctx, upstream, limit)
	if err != nil {
		return joinGitOutput(output...), false, fmt.Errorf("verify Git LFS migration: %w", err)
	}
	for _, candidate := range remaining {
		if filepath.ToSlash(candidate.Path) == path {
			return joinGitOutput(output...), false, fmt.Errorf("Git LFS migration completed but %s is still an oversized outgoing Git blob", path)
		}
	}
	newHeadOut, _, _, err := s.runGit(ctx, 5*time.Second, "rev-parse", "HEAD")
	if err != nil {
		return joinGitOutput(output...), false, err
	}
	output = append(output,
		fmt.Sprintf("Migrated %s (%.2f MiB) to Git LFS using pattern %q.", path, item.SizeMiB, pattern),
		"Git LFS: "+version,
		fmt.Sprintf("Rewrote %s unpushed commit(s) in %s.", count, upstream+"..HEAD"),
		"Original HEAD: "+originalHEAD+" (recoverable via Git reflog).",
		"New HEAD: "+strings.TrimSpace(newHeadOut)+".",
		"No remote refs were modified. Review the rewritten commits, then retry the normal push.",
	)
	return joinGitOutput(output...), false, nil
}

func parseGitLargeBlobBatch(raw string, limit int64) []gitLargeBlobInfo {
	byPath := map[string]gitLargeBlobInfo{}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 4)
		if len(fields) != 4 || fields[1] != "blob" {
			continue
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size <= limit {
			continue
		}
		path := strings.TrimSpace(fields[3])
		if path == "" {
			continue
		}
		item := gitLargeBlobInfo{
			Path: path, SizeBytes: size, SizeMiB: float64(size) / float64(1<<20),
			LimitBytes: limit, LimitMiB: float64(limit) / float64(1<<20),
		}
		if previous, exists := byPath[path]; !exists || item.SizeBytes > previous.SizeBytes {
			byPath[path] = item
		}
	}
	items := make([]gitLargeBlobInfo, 0, len(byPath))
	for _, item := range byPath {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SizeBytes != items[j].SizeBytes {
			return items[i].SizeBytes > items[j].SizeBytes
		}
		return items[i].Path < items[j].Path
	})
	return items
}

func (s *Server) gitOutgoingLargeBlobs(ctx context.Context, upstream string, limit int64) ([]gitLargeBlobInfo, error) {
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return nil, fmt.Errorf("upstream is required")
	}
	objects, _, truncated, err := s.runGit(ctx, 20*time.Second, "rev-list", "--objects", upstream+"..HEAD")
	if err != nil {
		return nil, fmt.Errorf("enumerate outgoing Git objects: %w", err)
	}
	if truncated {
		return nil, fmt.Errorf("outgoing Git object list exceeded inspection limit")
	}
	if strings.TrimSpace(objects) == "" {
		return nil, nil
	}
	batch, batchErr, batchTruncated, err := runGitInputInDirectory(
		ctx, 30*time.Second, s.gitDirectory(ctx), objects,
		"cat-file", "--batch-check=%(objectname) %(objecttype) %(objectsize) %(rest)",
	)
	if err != nil {
		return nil, fmt.Errorf("inspect outgoing Git object sizes: %w: %s", err, strings.TrimSpace(batchErr))
	}
	if batchTruncated {
		return nil, fmt.Errorf("outgoing Git size inspection exceeded output limit")
	}
	return parseGitLargeBlobBatch(batch, limit), nil
}

func (s *Server) gitHubPushLargeBlobPreflight(ctx context.Context) ([]gitLargeBlobInfo, bool) {
	_, upstream, _, remoteURL, err := s.gitPushUpstream(ctx)
	if err != nil || !isGitHubRemoteURL(remoteURL) {
		return nil, false
	}
	items, err := s.gitOutgoingLargeBlobs(ctx, upstream, gitHubPushBlobLimit)
	if err != nil {
		// Fail open: a diagnostic preflight must not block a push when the
		// normal Git command can still produce authoritative remote output.
		return nil, false
	}
	return items, true
}

func formatGitLargeBlobOutput(provider string, items []gitLargeBlobInfo) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	if provider == "github" {
		b.WriteString("TaskDeck preflight: GitHub rejects individual Git blobs larger than 100 MiB.\n")
	} else {
		b.WriteString("TaskDeck detected oversized Git blobs in outgoing commits.\n")
	}
	for _, item := range items {
		fmt.Fprintf(&b, "- %s — %.2f MiB (limit %.2f MiB)\n", item.Path, item.SizeMiB, item.LimitMiB)
	}
	b.WriteString("The push was stopped before uploading the pack.")
	return strings.TrimSpace(b.String())
}

func (s *Server) gitRequireCleanTrackedTree(ctx context.Context) error {
	out, _, _, err := s.runGit(ctx, 8*time.Second, "status", "--porcelain=v1", "--untracked-files=no")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("tracked working tree/index must be clean before rewriting local commits")
	}
	return nil
}

func (s *Server) gitLargePathIsOutgoing(ctx context.Context, path string, limit int64) (gitLargeBlobInfo, string, error) {
	path, err := validGitRelativePath(path)
	if err != nil {
		return gitLargeBlobInfo{}, "", err
	}
	_, upstream, _, _, err := s.gitPushUpstream(ctx)
	if err != nil {
		return gitLargeBlobInfo{}, "", err
	}
	items, err := s.gitOutgoingLargeBlobs(ctx, upstream, limit)
	if err != nil {
		return gitLargeBlobInfo{}, upstream, err
	}
	for _, item := range items {
		if filepath.ToSlash(item.Path) == path {
			return item, upstream, nil
		}
	}
	return gitLargeBlobInfo{}, upstream, fmt.Errorf("oversized blob %s is not currently present in outgoing commits", path)
}

func (s *Server) gitLargeFileRemoveFromLatest(ctx context.Context, rawPath string, limit int64) (string, bool, error) {
	path, err := validGitRelativePath(rawPath)
	if err != nil {
		return "", false, err
	}
	item, _, err := s.gitLargePathIsOutgoing(ctx, path, limit)
	if err != nil {
		return "", false, err
	}
	if err := s.gitRequireCleanTrackedTree(ctx); err != nil {
		return "", false, err
	}
	if out, _, _, parentErr := s.runGit(ctx, 5*time.Second, "rev-parse", "HEAD^"); parentErr == nil {
		parent := strings.TrimSpace(out)
		if treeOut, _, _, treeErr := s.runGit(ctx, 8*time.Second, "ls-tree", "-r", "--name-only", parent, "--", path); treeErr != nil {
			return "", false, treeErr
		} else if strings.TrimSpace(treeOut) != "" {
			return "", false, fmt.Errorf("oversized file %s already exists before the latest commit; use the multi-commit cleanup option instead", path)
		}
	}
	if headOut, _, _, err := s.runGit(ctx, 8*time.Second, "ls-tree", "-r", "--name-only", "HEAD", "--", path); err != nil {
		return "", false, err
	} else if strings.TrimSpace(headOut) == "" {
		return "", false, fmt.Errorf("oversized file %s is not present in the latest commit; use the multi-commit cleanup option instead", path)
	}

	pattern := "/" + escapeGitIgnoreLiteral(path)
	added, err := appendGitIgnoreRule(s.gitDirectory(ctx), pattern)
	if err != nil {
		return "", false, err
	}
	var output []string
	if added {
		output = append(output, "Added "+pattern+" to .gitignore.")
	}
	run := func(timeout time.Duration, args ...string) error {
		stdout, stderr, _, runErr := s.runGit(ctx, timeout, args...)
		if text := joinGitOutput(stdout, stderr); text != "" {
			output = append(output, text)
		}
		return runErr
	}
	if err := run(20*time.Second, "rm", "--cached", "--", path); err != nil {
		return joinGitOutput(output...), false, err
	}
	if err := run(10*time.Second, "add", "--", ".gitignore"); err != nil {
		return joinGitOutput(output...), false, err
	}
	if err := run(60*time.Second, "commit", "--amend", "--no-edit"); err != nil {
		return joinGitOutput(output...), false, err
	}
	output = append(output,
		fmt.Sprintf("Removed %s (%.2f MiB) from the latest commit while keeping the working-tree file.", path, item.SizeMiB),
		"The latest local commit was rewritten. The file is now ignored.",
	)
	return joinGitOutput(output...), false, nil
}

func (s *Server) gitLargeFilePrepareRecommit(ctx context.Context, rawPath string, limit int64) (string, bool, error) {
	path, err := validGitRelativePath(rawPath)
	if err != nil {
		return "", false, err
	}
	item, upstream, err := s.gitLargePathIsOutgoing(ctx, path, limit)
	if err != nil {
		return "", false, err
	}
	if err := s.gitRequireCleanTrackedTree(ctx); err != nil {
		return "", false, err
	}
	if _, _, _, err := s.runGit(ctx, 8*time.Second, "merge-base", "--is-ancestor", upstream, "HEAD"); err != nil {
		return "", false, fmt.Errorf("upstream is not an ancestor of HEAD; integrate remote history before rewriting unpushed commits")
	}
	headOut, _, _, err := s.runGit(ctx, 5*time.Second, "rev-parse", "HEAD")
	if err != nil {
		return "", false, err
	}
	originalHEAD := strings.TrimSpace(headOut)
	countOut, _, _, err := s.runGit(ctx, 8*time.Second, "rev-list", "--count", upstream+"..HEAD")
	if err != nil {
		return "", false, err
	}
	count := strings.TrimSpace(countOut)

	pattern := "/" + escapeGitIgnoreLiteral(path)
	added, err := appendGitIgnoreRule(s.gitDirectory(ctx), pattern)
	if err != nil {
		return "", false, err
	}
	var output []string
	if added {
		output = append(output, "Added "+pattern+" to .gitignore.")
	}
	run := func(timeout time.Duration, args ...string) error {
		stdout, stderr, _, runErr := s.runGit(ctx, timeout, args...)
		if text := joinGitOutput(stdout, stderr); text != "" {
			output = append(output, text)
		}
		return runErr
	}
	if err := run(10*time.Second, "add", "--", ".gitignore"); err != nil {
		return joinGitOutput(output...), false, err
	}
	if err := run(20*time.Second, "reset", "--soft", upstream); err != nil {
		return joinGitOutput(output...), false, err
	}
	if err := run(20*time.Second, "rm", "--cached", "--ignore-unmatch", "--", path); err != nil {
		return joinGitOutput(output...), false, err
	}
	if err := run(10*time.Second, "add", "--", ".gitignore"); err != nil {
		return joinGitOutput(output...), false, err
	}
	statusOut, statusErr, _, statusRunErr := s.runGit(ctx, 10*time.Second, "status", "--short", "--branch")
	if statusRunErr == nil {
		output = append(output, joinGitOutput(statusOut, statusErr))
	}
	output = append(output,
		fmt.Sprintf("Prepared cleanup for %s (%.2f MiB).", path, item.SizeMiB),
		fmt.Sprintf("Moved HEAD back to %s and converted %s unpushed commit(s) into staged changes.", upstream, count),
		"The oversized file remains in the working tree but is untracked and ignored.",
		"Review the staged changes and create a new commit before pushing.",
		"Original HEAD: "+originalHEAD+" (recoverable via Git reflog).",
	)
	return joinGitOutput(output...), false, nil
}
