package server

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type gitBlameRow struct {
	SHA        string `json:"sha"`
	Short      string `json:"short"`
	Line       int    `json:"line"`
	Author     string `json:"author"`
	AuthorTime int64  `json:"author_time,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Text       string `json:"text"`
	Committed  bool   `json:"committed"`
}

func (s *Server) gitFileHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := validGitRelativePath(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	limit := 50
	if value, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && value > 0 && value <= 200 {
		limit = value
	}
	format := "%H%x09%h%x09%ad%x09%an%x09%s"
	out, stderr, truncated, err := s.runGit(r.Context(), 8*time.Second,
		"log", "--follow", "-n", strconv.Itoa(limit), "--date=iso-strict", "--pretty=format:"+format, "--", path)
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusConflict)
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
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "commits": rows, "truncated": truncated})
}

func parseGitBlamePorcelain(raw string) ([]gitBlameRow, error) {
	rows := []gitBlameRow{}
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	meta := map[string]string{}
	sha := ""
	finalLine := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "\t") {
			if sha == "" || finalLine < 1 {
				continue
			}
			authorTime, _ := strconv.ParseInt(meta["author-time"], 10, 64)
			short := sha
			if len(short) > 8 {
				short = short[:8]
			}
			committed := strings.Trim(sha, "0") != ""
			rows = append(rows, gitBlameRow{
				SHA: sha, Short: short, Line: finalLine,
				Author: meta["author"], AuthorTime: authorTime,
				Summary: meta["summary"], Text: strings.TrimPrefix(line, "\t"),
				Committed: committed,
			})
			meta = map[string]string{}
			sha = ""
			finalLine = 0
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 && len(fields[0]) >= 8 {
			if candidate, err := strconv.Atoi(fields[2]); err == nil && candidate > 0 {
				sha = fields[0]
				finalLine = candidate
				meta = map[string]string{}
				continue
			}
		}
		if sha == "" {
			continue
		}
		if i := strings.IndexByte(line, ' '); i > 0 {
			key := line[:i]
			switch key {
			case "author", "author-time", "summary":
				meta[key] = line[i+1:]
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse git blame output: %w", err)
	}
	return rows, nil
}

func (s *Server) gitFileBlame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := validGitRelativePath(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out, stderr, truncated, err := s.runGit(r.Context(), 12*time.Second, "blame", "--line-porcelain", "--", path)
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusConflict)
		return
	}
	rows, err := parseGitBlamePorcelain(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "lines": rows, "truncated": truncated})
}

var gitCompareCommitPattern = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)

func (s *Server) gitFileContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pathValue, err := validGitRelativePath(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if !gitCompareCommitPattern.MatchString(ref) {
		http.Error(w, "Git compare ref must be a full commit SHA", http.StatusBadRequest)
		return
	}
	repo, ok := gitRepositoryFromContext(r.Context())
	if !ok {
		http.Error(w, "Git repository context unavailable", http.StatusConflict)
		return
	}
	commit, stderr, _, err := s.runGit(r.Context(), 4*time.Second, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusNotFound)
		return
	}
	commit = strings.TrimSpace(commit)
	if !gitCompareCommitPattern.MatchString(commit) {
		http.Error(w, "resolved Git commit is invalid", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "show", "--no-ext-diff", commit+":"+pathValue)
	cmd.Dir = repo.Root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "LC_ALL=C")
	stdout := &cappedGitBuffer{limit: int(projectEditableLimit) + 1}
	stderrBuffer := &cappedGitBuffer{limit: 64 << 10}
	cmd.Stdout = stdout
	cmd.Stderr = stderrBuffer
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderrBuffer.String())
		if ctx.Err() == context.DeadlineExceeded {
			message = "Git file content lookup timed out"
		}
		if message == "" {
			message = err.Error()
		}
		http.Error(w, message, http.StatusNotFound)
		return
	}
	content := []byte(stdout.String())
	if stdout.truncated || int64(len(content)) > projectEditableLimit {
		http.Error(w, "Git file content exceeds compare limit", http.StatusRequestEntityTooLarge)
		return
	}
	if !projectTextBytesValid(content) {
		http.Error(w, "Git file content is binary or unsupported text", http.StatusUnsupportedMediaType)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repo_id": repo.ID, "path": pathValue, "ref": ref, "commit": commit, "content": string(content),
	})
}
