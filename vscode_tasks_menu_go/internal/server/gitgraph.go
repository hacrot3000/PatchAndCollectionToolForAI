package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type gitGraphRef struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type gitGraphCommit struct {
	SHA     string        `json:"sha"`
	Short   string        `json:"short"`
	Parents []string      `json:"parents"`
	Date    string        `json:"date"`
	Author  string        `json:"author"`
	Subject string        `json:"subject"`
	Refs    []gitGraphRef `json:"refs"`
}

type gitGraphFile struct {
	Status  string `json:"status"`
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
}

func parseGitGraphDecoration(value string) []gitGraphRef {
	rows := []gitGraphRef{}
	seen := map[string]bool{}
	appendRef := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		kind := "other"
		name := raw
		switch {
		case raw == "HEAD":
			kind = "head"
		case strings.HasPrefix(raw, "tag: refs/tags/"):
			kind, name = "tag", strings.TrimPrefix(raw, "tag: refs/tags/")
		case strings.HasPrefix(raw, "refs/tags/"):
			kind, name = "tag", strings.TrimPrefix(raw, "refs/tags/")
		case strings.HasPrefix(raw, "refs/heads/"):
			kind, name = "local", strings.TrimPrefix(raw, "refs/heads/")
		case strings.HasPrefix(raw, "refs/remotes/"):
			kind, name = "remote", strings.TrimPrefix(raw, "refs/remotes/")
		}
		key := kind + "\x00" + name
		if name == "" || seen[key] {
			return
		}
		seen[key] = true
		rows = append(rows, gitGraphRef{Name: name, Kind: kind})
	}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if left, right, ok := strings.Cut(part, " -> "); ok {
			appendRef(left)
			appendRef(right)
			continue
		}
		appendRef(part)
	}
	return rows
}

func parseGitGraphCommits(raw, search string, limit int) []gitGraphCommit {
	search = strings.ToLower(strings.TrimSpace(search))
	rows := make([]gitGraphCommit, 0, limit)
	for _, record := range strings.Split(raw, "\x1e") {
		record = strings.Trim(record, "\r\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\x00", 7)
		if len(fields) != 7 {
			continue
		}
		row := gitGraphCommit{
			SHA: fields[0], Short: fields[1],
			Parents: strings.Fields(fields[2]),
			Date: fields[3], Author: fields[4], Subject: fields[5],
			Refs: parseGitGraphDecoration(fields[6]),
		}
		if search != "" {
			var refText strings.Builder
			for _, ref := range row.Refs {
				refText.WriteByte(' ')
				refText.WriteString(ref.Name)
			}
			haystack := strings.ToLower(row.SHA + " " + row.Short + " " + row.Author + " " + row.Subject + refText.String())
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		rows = append(rows, row)
		if len(rows) >= limit {
			break
		}
	}
	return rows
}

func gitGraphDateArg(raw, label string, endOfDay bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return "", fmt.Errorf("%s must use YYYY-MM-DD", label)
	}
	if endOfDay {
		parsed = parsed.Add(24*time.Hour - time.Nanosecond)
	}
	return parsed.Format(time.RFC3339), nil
}

func (s *Server) gitGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 200
	if value, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && value > 0 && value <= 500 {
		limit = value
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	author := strings.TrimSpace(r.URL.Query().Get("author"))
	message := strings.TrimSpace(r.URL.Query().Get("message"))
	if len(search) > 200 || len(author) > 200 || len(message) > 200 {
		http.Error(w, "Git graph filter is too long", http.StatusBadRequest)
		return
	}
	since, err := gitGraphDateArg(r.URL.Query().Get("since"), "since", false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	until, err := gitGraphDateArg(r.URL.Query().Get("until"), "until", true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pathValue := strings.TrimSpace(r.URL.Query().Get("path"))
	if pathValue != "" {
		pathValue, err = validGitRelativePath(pathValue)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	scanLimit := limit
	if search != "" {
		scanLimit = limit * 5
		if scanLimit < 500 {
			scanLimit = 500
		}
		if scanLimit > 2000 {
			scanLimit = 2000
		}
	}
	format := "%H%x00%h%x00%P%x00%ad%x00%an%x00%s%x00%D%x1e"
	args := []string{
		"log", "--all", "--topo-order", "--date=iso-strict", "--decorate=full",
		"--max-count=" + strconv.Itoa(scanLimit), "--pretty=format:" + format,
	}
	if author != "" {
		args = append(args, "--regexp-ignore-case", "--author="+author)
	}
	if message != "" {
		args = append(args, "--regexp-ignore-case", "--fixed-strings", "--grep="+message)
	}
	if since != "" {
		args = append(args, "--since="+since)
	}
	if until != "" {
		args = append(args, "--until="+until)
	}
	if pathValue != "" {
		args = append(args, "--", pathValue)
	}
	out, stderr, truncated, err := s.runGit(r.Context(), 12*time.Second, args...)
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusConflict)
		return
	}
	rows := parseGitGraphCommits(out, search, limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"commits": rows, "truncated": truncated || len(rows) >= limit,
		"limit": limit, "path": pathValue,
	})
}

func (s *Server) gitCommitFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if !gitCompareCommitPattern.MatchString(ref) {
		http.Error(w, "commit ref must be a full commit SHA", http.StatusBadRequest)
		return
	}
	resolved, stderr, _, err := s.runGit(r.Context(), 4*time.Second, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusNotFound)
		return
	}
	commit := strings.TrimSpace(resolved)
	if !gitCompareCommitPattern.MatchString(commit) {
		http.Error(w, "resolved commit is invalid", http.StatusConflict)
		return
	}
	out, stderr, truncated, err := s.runGit(r.Context(), 8*time.Second,
		"diff-tree", "--root", "--no-commit-id", "--name-status", "-r", "-z", "--find-renames", commit)
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusConflict)
		return
	}
	files := parseGitGraphFiles(out)
	writeJSON(w, http.StatusOK, map[string]any{
		"commit": commit, "files": files, "truncated": truncated,
	})
}

func parseGitGraphFiles(raw string) []gitGraphFile {
	tokens := strings.Split(raw, "\x00")
	files := []gitGraphFile{}
	for i := 0; i < len(tokens); {
		status := strings.TrimSpace(tokens[i])
		i++
		if status == "" {
			continue
		}
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if i+1 >= len(tokens) {
				break
			}
			oldPath, newPath := tokens[i], tokens[i+1]
			i += 2
			if oldPath != "" && newPath != "" {
				files = append(files, gitGraphFile{Status: status, Path: newPath, OldPath: oldPath})
			}
			continue
		}
		if i >= len(tokens) {
			break
		}
		pathValue := tokens[i]
		i++
		if pathValue != "" {
			files = append(files, gitGraphFile{Status: status, Path: pathValue})
		}
	}
	return files
}

func (s *Server) gitGraphCompareHead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if !gitCompareCommitPattern.MatchString(ref) {
		http.Error(w, "commit ref must be a full commit SHA", http.StatusBadRequest)
		return
	}
	resolved, stderr, _, err := s.runGit(r.Context(), 4*time.Second, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusNotFound)
		return
	}
	left := strings.TrimSpace(resolved)
	headOut, stderr, _, err := s.runGit(r.Context(), 4*time.Second, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusNotFound)
		return
	}
	right := strings.TrimSpace(headOut)
	out, stderr, truncated, err := s.runGit(r.Context(), 8*time.Second,
		"diff", "--name-status", "-z", "--find-renames", left, right)
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"left": left, "right": right, "files": parseGitGraphFiles(out), "truncated": truncated,
	})
}
