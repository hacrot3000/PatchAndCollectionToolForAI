package server

import (
	"context"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type gitStatusResponse struct {
	Repository bool   `json:"repository"`
	Branch     string `json:"branch,omitempty"`
	Head       string `json:"head,omitempty"`
	Changed    int    `json:"changed"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
}

func (s *Server) gitStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.gitAction(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch strings.TrimSpace(r.URL.Query().Get("view")) {
	case "changes":
		s.gitChanges(w, r)
		return
	case "diff":
		s.gitDiff(w, r)
		return
	case "log":
		s.gitLog(w, r)
		return
	case "branches":
		s.gitBranches(w, r)
		return
	case "stashes":
		s.gitStashes(w, r)
		return
	case "compare":
		s.gitCompare(w, r)
		return
	case "ahead-behind":
		s.gitAheadBehind(w, r)
		return
	case "", "status":
		// Continue with compact status below.
	default:
		http.Error(w, "unknown git view", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", s.Workspace, "status", "--porcelain=v1", "--branch", "--untracked-files=normal")
	out, err := cmd.Output()
	if err != nil {
		writeJSON(w, http.StatusOK, gitStatusResponse{Repository: false})
		return
	}
	lines := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
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
	if headOut, headErr := exec.CommandContext(ctx, "git", "-C", s.Workspace, "rev-parse", "--short=8", "HEAD").Output(); headErr == nil {
		head = strings.TrimSpace(string(headOut))
	}
	writeJSON(w, http.StatusOK, gitStatusResponse{Repository: true, Branch: branch, Head: head, Changed: changed, Ahead: ahead, Behind: behind})
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
