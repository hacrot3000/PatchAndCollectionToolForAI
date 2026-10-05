package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type gitWorktreeRow struct {
	ID          string `json:"id"`
	DisplayPath string `json:"display_path"`
	Branch      string `json:"branch,omitempty"`
	Head        string `json:"head,omitempty"`
	Current     bool   `json:"current,omitempty"`
	Primary     bool   `json:"primary,omitempty"`
	Detached    bool   `json:"detached,omitempty"`
	Bare        bool   `json:"bare,omitempty"`
	Locked      bool   `json:"locked,omitempty"`
	LockReason  string `json:"lock_reason,omitempty"`
	Prunable    bool   `json:"prunable,omitempty"`
	PruneReason string `json:"prune_reason,omitempty"`
	path        string
}

func gitWorktreeID(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:])
}

func sameGitWorktreePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func gitWorktreeDisplayPath(repoRoot, worktreePath string) string {
	if sameGitWorktreePath(repoRoot, worktreePath) {
		return "."
	}
	parent := filepath.Dir(filepath.Clean(repoRoot))
	if rel, err := filepath.Rel(parent, filepath.Clean(worktreePath)); err == nil {
		rel = filepath.Clean(rel)
		if rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
	}
	base := filepath.Base(filepath.Clean(worktreePath))
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "worktree"
	}
	return base + " (external)"
}

func parseGitWorktreePorcelain(raw, repoRoot string) []gitWorktreeRow {
	rows := []gitWorktreeRow{}
	var row gitWorktreeRow
	hasRow := false
	flush := func() {
		if !hasRow || strings.TrimSpace(row.path) == "" {
			row = gitWorktreeRow{}
			hasRow = false
			return
		}
		row.ID = gitWorktreeID(row.path)
		row.DisplayPath = gitWorktreeDisplayPath(repoRoot, row.path)
		row.Current = sameGitWorktreePath(repoRoot, row.path)
		row.Primary = len(rows) == 0
		rows = append(rows, row)
		row = gitWorktreeRow{}
		hasRow = false
	}
	for _, token := range strings.Split(raw, "\x00") {
		if token == "" {
			flush()
			continue
		}
		key, value, _ := strings.Cut(token, " ")
		value = strings.TrimSpace(value)
		switch key {
		case "worktree":
			if hasRow && row.path != "" {
				flush()
			}
			row.path = value
			hasRow = true
		case "HEAD":
			row.Head = value
		case "branch":
			row.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "detached":
			row.Detached = true
		case "bare":
			row.Bare = true
		case "locked":
			row.Locked = true
			row.LockReason = value
		case "prunable":
			row.Prunable = true
			row.PruneReason = value
		}
	}
	flush()
	return rows
}

func (s *Server) gitWorktrees(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo, ok := gitRepositoryFromContext(r.Context())
	if !ok {
		http.Error(w, "Git repository context unavailable", http.StatusConflict)
		return
	}
	out, stderr, truncated, err := s.runGit(r.Context(), 8*time.Second,
		"worktree", "list", "--porcelain", "-z")
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusConflict)
		return
	}
	rows := parseGitWorktreePorcelain(out, repo.Root)
	writeJSON(w, http.StatusOK, map[string]any{
		"worktrees": rows, "count": len(rows), "truncated": truncated,
	})
}

func (s *Server) gitWorktreeRows(ctx context.Context) ([]gitWorktreeRow, error) {
	repo, ok := gitRepositoryFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("Git repository context unavailable")
	}
	out, stderr, _, err := s.runGit(ctx, 8*time.Second, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(joinGitOutput(stderr, err.Error())))
	}
	return parseGitWorktreePorcelain(out, repo.Root), nil
}

func validGitWorktreeDirectoryName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || value == "." || value == ".." ||
		strings.ContainsAny(value, "/\\\x00\r\n") {
		return "", fmt.Errorf("worktree directory name must be one safe path segment")
	}
	return value, nil
}

func gitWorktreeTargetPath(repo gitRepository, directoryName string) (string, error) {
	name, err := validGitWorktreeDirectoryName(directoryName)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(filepath.Clean(repo.Root))
	target := filepath.Join(parent, name)
	if filepath.Dir(filepath.Clean(target)) != parent {
		return "", fmt.Errorf("worktree target escapes repository parent")
	}
	if _, err := os.Lstat(target); err == nil {
		return "", fmt.Errorf("worktree target already exists")
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect worktree target: %w", err)
	}
	return target, nil
}

func gitWorktreeBranchLocation(rows []gitWorktreeRow, branch string) string {
	branch = strings.TrimSpace(branch)
	for _, row := range rows {
		if row.Branch == branch {
			return row.DisplayPath
		}
	}
	return ""
}

func (s *Server) gitWorktreeSourceSHA(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "HEAD" {
		ref = "HEAD"
	} else if !gitCompareCommitPattern.MatchString(ref) {
		if err := s.validBranchName(ctx, ref); err != nil {
			return "", fmt.Errorf("worktree source must be HEAD, full commit SHA, or existing branch")
		}
		if !s.localBranchExists(ctx, ref) && !s.remoteBranchExists(ctx, ref) {
			return "", fmt.Errorf("worktree source branch not found")
		}
	}
	out, stderr, _, err := s.runGit(ctx, 4*time.Second, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(joinGitOutput(stderr, err.Error())))
	}
	sha := strings.TrimSpace(out)
	if !gitCompareCommitPattern.MatchString(sha) {
		return "", fmt.Errorf("resolved worktree source commit is invalid")
	}
	return sha, nil
}

func gitWorktreeByID(rows []gitWorktreeRow, id string) (gitWorktreeRow, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return gitWorktreeRow{}, false
	}
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return gitWorktreeRow{}, false
}
