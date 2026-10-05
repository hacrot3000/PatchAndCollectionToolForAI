package server

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type projectGitStatusEntry struct {
	Path           string `json:"path"`
	Repository     string `json:"repository"`
	Status         string `json:"status"`
	IndexStatus    string `json:"index_status"`
	WorktreeStatus string `json:"worktree_status"`
	Conflicted     bool   `json:"conflicted,omitempty"`
	Untracked      bool   `json:"untracked,omitempty"`
}

func projectGitStatusCode(change gitChange) string {
	if change.Conflicted {
		return "U"
	}
	if change.Untracked {
		return "?"
	}
	for _, status := range []byte(change.IndexStatus + change.WorktreeStatus) {
		switch status {
		case 'D':
			return "D"
		case 'A':
			return "A"
		case 'R':
			return "R"
		case 'C':
			return "C"
		case 'T':
			return "T"
		case 'M':
			return "M"
		}
	}
	return ""
}

func projectGitWorkspacePath(repo gitRepository, path string) string {
	path = filepath.ToSlash(strings.TrimPrefix(path, "./"))
	if repo.ID == "." || repo.ID == "" {
		return path
	}
	if path == "" {
		return filepath.ToSlash(repo.ID)
	}
	return filepath.ToSlash(repo.ID) + "/" + path
}

func (s *Server) projectGitStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repos, _, err := s.discoverGitRepositories(false)
	if err != nil {
		http.Error(w, "cannot discover Git repositories", http.StatusConflict)
		return
	}
	entries := make([]projectGitStatusEntry, 0)
	errorsByRepo := map[string]string{}
	for _, repo := range repos {
		out, _, _, runErr := runGitInDirectory(r.Context(), 4*time.Second, repo.Root,
			"status", "--porcelain=v1", "-z", "--untracked-files=all")
		if runErr != nil {
			errorsByRepo[repo.ID] = runErr.Error()
			continue
		}
		for _, change := range parseGitStatusZ(out) {
			status := projectGitStatusCode(change)
			if status == "" {
				continue
			}
			entries = append(entries, projectGitStatusEntry{
				Path:           projectGitWorkspacePath(repo, change.Path),
				Repository:     repo.ID,
				Status:         status,
				IndexStatus:    change.IndexStatus,
				WorktreeStatus: change.WorktreeStatus,
				Conflicted:     change.Conflicted,
				Untracked:      change.Untracked,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": len(repos) > 0,
		"changes":   entries,
		"errors":    errorsByRepo,
	})
}
