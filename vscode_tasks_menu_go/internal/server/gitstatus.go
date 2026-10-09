package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type gitStatusResponse struct {
	Repository bool   `json:"repository"`
	RepoID     string `json:"repo_id,omitempty"`
	RepoName   string `json:"repo_name,omitempty"`
	RepoPath   string `json:"repo_path,omitempty"`
	Branch     string `json:"branch,omitempty"`
	Head       string `json:"head,omitempty"`
	Changed    int    `json:"changed"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
}

func (s *Server) gitStatus(w http.ResponseWriter, r *http.Request) {
	view := strings.TrimSpace(r.URL.Query().Get("view"))
	if r.Method == http.MethodGet && view == "repositories" {
		s.gitRepositories(w, r)
		return
	}

	repo, err := s.resolveGitRepository(r.URL.Query().Get("repo"))
	if err != nil {
		if r.Method == http.MethodGet && (view == "" || view == "status") {
			writeJSON(w, http.StatusOK, gitStatusResponse{Repository: false})
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r = r.WithContext(withGitRepository(r.Context(), repo))

	if r.Method == http.MethodPost {
		s.gitAction(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch view {
	case "changes":
		s.gitChanges(w, r)
		return
	case "diff":
		s.gitDiff(w, r)
		return
	case "ignore-suggestions":
		rows, err := s.gitIgnoreSuggestions(r.Context(), r.URL.Query().Get("path"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": r.URL.Query().Get("path"), "suggestions": rows})
		return
	case "ignore-preview":
		item, err := s.gitIgnoreCustomSuggestion(r.Context(), r.URL.Query().Get("path"), r.URL.Query().Get("pattern"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": r.URL.Query().Get("path"), "suggestion": item})
		return
	case "log":
		s.gitLog(w, r)
		return
	case "graph":
		s.gitGraph(w, r)
		return
	case "reflog":
		s.gitReflog(w, r)
		return
	case "lost-commits":
		s.gitLostCommits(w, r)
		return
	case "worktrees":
		s.gitWorktrees(w, r)
		return
	case "submodules":
		s.gitSubmodules(w, r)
		return
	case "rebase-plan":
		s.gitRebasePlan(w, r)
		return
	case "semantics-preview":
		s.gitSemanticsPreview(w, r)
		return
	case "commit-files":
		s.gitCommitFiles(w, r)
		return
	case "graph-compare-head":
		s.gitGraphCompareHead(w, r)
		return
	case "file-history":
		s.gitFileHistory(w, r)
		return
	case "file-content":
		s.gitFileContent(w, r)
		return
	case "blame":
		s.gitFileBlame(w, r)
		return
	case "branches":
		s.gitBranches(w, r)
		return
	case "tags":
		s.gitTags(w, r)
		return
	case "stashes":
		s.gitStashes(w, r)
		return
	case "compare":
		s.gitCompare(w, r)
		return
	case "compare-tree":
		s.gitCompareTree(w, r)
		return
	case "merge-preflight":
		s.gitMergePreflight(w, r)
		return
	case "merge-to-preflight":
		s.gitMergeToPreflight(w, r)
		return
	case "ahead-behind":
		s.gitAheadBehind(w, r)
		return
	case "", "status":
		writeJSON(w, http.StatusOK, s.gitCompactStatus(r.Context(), repo))
	default:
		http.Error(w, "unknown git view", http.StatusBadRequest)
	}
}

func (s *Server) gitCompactStatus(ctx context.Context, repo gitRepository) gitStatusResponse {
	ctx = withGitRepository(ctx, repo)
	out, _, _, err := s.runGit(ctx, 3*time.Second, "status", "--porcelain=v1", "--branch", "--untracked-files=normal")
	if err != nil {
		return gitStatusResponse{Repository: false, RepoID: repo.ID, RepoName: repo.Name, RepoPath: repo.Path}
	}
	lines := strings.Split(strings.TrimRight(out, "\r\n"), "\n")
	branch, ahead, behind := "", 0, 0
	changed := 0
	if len(lines) > 0 && strings.HasPrefix(lines[0], "## ") {
		branch, ahead, behind = parseGitBranchHeader(lines[0])
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) != "" {
				changed++
			}
		}
	} else {
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				changed++
			}
		}
	}
	head := ""
	if headOut, _, _, headErr := s.runGit(ctx, 3*time.Second, "rev-parse", "--short=8", "HEAD"); headErr == nil {
		head = strings.TrimSpace(headOut)
	}
	return gitStatusResponse{
		Repository: true,
		RepoID: repo.ID, RepoName: repo.Name, RepoPath: repo.Path,
		Branch: branch, Head: head, Changed: changed, Ahead: ahead, Behind: behind,
	}
}

func (s *Server) gitRepositories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	force := strings.TrimSpace(r.URL.Query().Get("refresh")) == "1"
	repos, settings, err := s.discoverGitRepositories(force)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.gitReposMu.Lock()
	scanWarnings := append([]string(nil), s.gitScanWarnings...)
	defaultRepository := s.gitDefaultRepo
	s.gitReposMu.Unlock()
	rows := make([]gitRepositoryStatus, 0, len(repos))
	for _, repo := range repos {
		status := s.gitCompactStatus(r.Context(), repo)
		row := gitRepositoryStatus{
			ID: repo.ID, Name: repo.Name, Path: repo.Path, Explicit: repo.Explicit, Default: repo.Default,
			Branch: status.Branch, Head: status.Head, Changed: status.Changed,
			Ahead: status.Ahead, Behind: status.Behind,
		}
		if !status.Repository {
			row.Error = "Git status unavailable"
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repositories": rows,
		"default_repository": defaultRepository,
		"scan_enabled": settings.ScanEnabled,
		"scan_warnings": scanWarnings,
		"scan_depth": settings.ScanDepth,
		"auto_select_from_terminal_cwd": settings.AutoSelectFromTerminalCWD,
	})
}

func parseGitBranchHeader(header string) (string, int, int) {
	body := strings.TrimSpace(strings.TrimPrefix(header, "## "))
	if strings.HasPrefix(body, "No commits yet on ") {
		return strings.TrimSpace(strings.TrimPrefix(body, "No commits yet on ")), 0, 0
	}
	if strings.HasPrefix(body, "Initial commit on ") {
		return strings.TrimSpace(strings.TrimPrefix(body, "Initial commit on ")), 0, 0
	}
	if strings.HasPrefix(body, "HEAD (no branch)") {
		return "DETACHED", 0, 0
	}
	status := ""
	if i := strings.Index(body, " ["); i >= 0 {
		status = strings.TrimSuffix(body[i+2:], "]")
		body = body[:i]
	}
	branch := body
	if i := strings.Index(branch, "..."); i >= 0 {
		branch = branch[:i]
	}
	ahead, behind := 0, 0
	for _, item := range strings.Split(status, ",") {
		fields := strings.Fields(strings.TrimSpace(item))
		if len(fields) != 2 {
			continue
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "ahead":
			ahead = n
		case "behind":
			behind = n
		}
	}
	return strings.TrimSpace(branch), ahead, behind
}
