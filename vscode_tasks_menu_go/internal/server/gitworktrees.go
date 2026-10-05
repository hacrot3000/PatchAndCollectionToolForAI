package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
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
