package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type gitLostCommit struct {
	SHA            string `json:"sha"`
	Short          string `json:"short"`
	Date           string `json:"date,omitempty"`
	Author         string `json:"author,omitempty"`
	Subject        string `json:"subject,omitempty"`
	ReflogVisible  bool   `json:"reflog_visible"`
}

func parseGitLostCommitSHAs(raw string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	seen := map[string]bool{}
	rows := make([]string, 0, limit)
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 3 {
			continue
		}
		if fields[0] != "unreachable" && fields[0] != "dangling" {
			continue
		}
		if fields[1] != "commit" || !gitCompareCommitPattern.MatchString(fields[2]) || seen[fields[2]] {
			continue
		}
		seen[fields[2]] = true
		rows = append(rows, fields[2])
		if len(rows) >= limit {
			break
		}
	}
	return rows
}

func parseGitLostCommitDetails(raw string, reflog map[string]bool, search string, limit int) []gitLostCommit {
	search = strings.ToLower(strings.TrimSpace(search))
	rows := make([]gitLostCommit, 0, limit)
	for _, record := range strings.Split(raw, "\x1e") {
		record = strings.Trim(record, "\r\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\x00", 5)
		if len(fields) != 5 || !gitCompareCommitPattern.MatchString(fields[0]) {
			continue
		}
		row := gitLostCommit{
			SHA: fields[0], Short: fields[1], Date: fields[2],
			Author: fields[3], Subject: fields[4], ReflogVisible: reflog[fields[0]],
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{row.SHA, row.Short, row.Author, row.Subject}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Date == rows[j].Date {
			return rows[i].SHA < rows[j].SHA
		}
		return rows[i].Date > rows[j].Date
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func (s *Server) gitLostCommits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if value, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && value > 0 && value <= 300 {
		limit = value
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len(search) > 200 {
		http.Error(w, "Git lost-commit search is too long", http.StatusBadRequest)
		return
	}

	fsckOut, fsckErr, truncated, err := s.runGit(r.Context(), 20*time.Second,
		"fsck", "--no-reflogs", "--unreachable", "--no-progress")
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(fsckErr, err.Error())), http.StatusConflict)
		return
	}
	shas := parseGitLostCommitSHAs(joinGitOutput(fsckOut, fsckErr), 300)
	if len(shas) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"commits": []gitLostCommit{}, "limit": limit, "truncated": truncated,
		})
		return
	}

	reflogOut, _, _, _ := s.runGit(r.Context(), 6*time.Second, "reflog", "show", "--all", "--format=%H")
	reflog := map[string]bool{}
	for _, sha := range strings.Fields(reflogOut) {
		if gitCompareCommitPattern.MatchString(sha) {
			reflog[sha] = true
		}
	}

	format := "%H%x00%h%x00%ad%x00%an%x00%s%x1e"
	args := []string{"show", "--no-patch", "--date=iso-strict", "--pretty=format:" + format}
	args = append(args, shas...)
	detailsOut, detailsErr, detailsTruncated, err := s.runGit(r.Context(), 12*time.Second, args...)
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(detailsErr, err.Error())), http.StatusConflict)
		return
	}
	rows := parseGitLostCommitDetails(detailsOut, reflog, search, limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"commits": rows, "limit": limit,
		"truncated": truncated || detailsTruncated || len(shas) >= 300 || len(rows) >= limit,
	})
}
