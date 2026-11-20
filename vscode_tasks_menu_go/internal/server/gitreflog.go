package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type gitReflogEntry struct {
	SHA      string `json:"sha"`
	Short    string `json:"short"`
	Selector string `json:"selector"`
	Actor    string `json:"actor"`
	Email    string `json:"email,omitempty"`
	Subject  string `json:"subject"`
	Date     string `json:"date,omitempty"`
	Kind     string `json:"kind"`
	Ref      string `json:"ref,omitempty"`
}

func gitReflogKind(subject string) string {
	lower := strings.ToLower(strings.TrimSpace(subject))
	switch {
	case strings.HasPrefix(lower, "reset:"):
		return "reset"
	case strings.HasPrefix(lower, "checkout:"):
		return "checkout"
	case strings.HasPrefix(lower, "commit"):
		return "commit"
	case strings.HasPrefix(lower, "merge "):
		return "merge"
	case strings.HasPrefix(lower, "rebase"):
		return "rebase"
	case strings.HasPrefix(lower, "cherry-pick"):
		return "cherry-pick"
	case strings.HasPrefix(lower, "revert"):
		return "revert"
	case strings.HasPrefix(lower, "pull "):
		return "pull"
	case strings.HasPrefix(lower, "branch:"):
		return "branch"
	default:
		return "other"
	}
}

func gitReflogRef(selector string) string {
	selector = strings.TrimSpace(selector)
	if i := strings.Index(selector, "@{"); i > 0 {
		return selector[:i]
	}
	return selector
}

func parseGitReflog(raw, search, refFilter, kindFilter string, limit int) []gitReflogEntry {
	search = strings.ToLower(strings.TrimSpace(search))
	refFilter = strings.ToLower(strings.TrimSpace(refFilter))
	kindFilter = strings.ToLower(strings.TrimSpace(kindFilter))
	rows := make([]gitReflogEntry, 0, limit)
	for _, record := range strings.Split(raw, "\x1e") {
		record = strings.Trim(record, "\r\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\x00", 7)
		if len(fields) != 7 {
			continue
		}
		row := gitReflogEntry{
			SHA: fields[0], Short: fields[1], Selector: fields[2],
			Actor: fields[3], Email: fields[4], Subject: fields[5], Date: fields[6],
		}
		row.Kind = gitReflogKind(row.Subject)
		row.Ref = gitReflogRef(row.Selector)
		if kindFilter != "" && strings.ToLower(row.Kind) != kindFilter {
			continue
		}
		if refFilter != "" && !strings.Contains(strings.ToLower(row.Ref), refFilter) {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				row.SHA, row.Short, row.Selector, row.Ref, row.Actor, row.Email, row.Subject, row.Kind,
			}, " "))
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

func (s *Server) gitReflog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 200
	if value, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && value > 0 && value <= 500 {
		limit = value
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	refFilter := strings.TrimSpace(r.URL.Query().Get("ref"))
	kindFilter := strings.TrimSpace(r.URL.Query().Get("kind"))
	if len(search) > 200 || len(refFilter) > 200 || len(kindFilter) > 40 {
		http.Error(w, "Git reflog filter is too long", http.StatusBadRequest)
		return
	}
	scanLimit := limit * 4
	if scanLimit < 500 {
		scanLimit = 500
	}
	if scanLimit > 2000 {
		scanLimit = 2000
	}
	format := "%H%x00%h%x00%gD%x00%gn%x00%ge%x00%gs%x00%ci%x1e"
	out, stderr, truncated, err := s.runGit(r.Context(), 10*time.Second,
		"reflog", "show", "--all", "--date=iso-strict", "--max-count="+strconv.Itoa(scanLimit), "--format="+format)
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusConflict)
		return
	}
	rows := parseGitReflog(out, search, refFilter, kindFilter, limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": rows, "limit": limit, "truncated": truncated || len(rows) >= limit,
	})
}
