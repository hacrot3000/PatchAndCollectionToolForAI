package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type gitRebasePlanCommit struct {
	SHA        string `json:"sha"`
	Short      string `json:"short"`
	Date       string `json:"date"`
	Author     string `json:"author"`
	Subject    string `json:"subject"`
	Parents    []string `json:"parents"`
	Merge      bool   `json:"merge,omitempty"`
}

func (s *Server) gitResolveRebaseBase(ctx context.Context, raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("rebase base is required")
	}
	ref := raw
	switch {
	case gitCompareCommitPattern.MatchString(raw):
	case raw == "HEAD":
	default:
		if err := s.validBranchName(ctx, raw); err != nil {
			return "", "", fmt.Errorf("rebase base must be HEAD, a full commit SHA, or an existing branch")
		}
		if !s.localBranchExists(ctx, raw) && !s.remoteBranchExists(ctx, raw) {
			return "", "", fmt.Errorf("rebase base branch not found")
		}
	}
	out, stderr, _, err := s.runGit(ctx, 4*time.Second, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("%s", strings.TrimSpace(joinGitOutput(stderr, err.Error())))
	}
	sha := strings.TrimSpace(out)
	if !gitCompareCommitPattern.MatchString(sha) {
		return "", "", fmt.Errorf("resolved rebase base commit is invalid")
	}
	return ref, sha, nil
}

func parseGitRebasePlan(raw string) []gitRebasePlanCommit {
	rows := []gitRebasePlanCommit{}
	for _, record := range strings.Split(raw, "\x1e") {
		record = strings.Trim(record, "\r\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\x00", 6)
		if len(fields) != 6 || !gitCompareCommitPattern.MatchString(fields[0]) {
			continue
		}
		parents := strings.Fields(fields[5])
		rows = append(rows, gitRebasePlanCommit{
			SHA: fields[0], Short: fields[1], Date: fields[2], Author: fields[3],
			Subject: fields[4], Parents: parents, Merge: len(parents) > 1,
		})
	}
	return rows
}

func (s *Server) gitRebasePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	branch, err := s.gitCurrentBranch(r.Context())
	if err != nil {
		http.Error(w, "interactive rebase requires a named current branch", http.StatusConflict)
		return
	}
	baseRef, baseSHA, err := s.gitResolveRebaseBase(r.Context(), r.URL.Query().Get("base"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	headOut, stderr, _, err := s.runGit(r.Context(), 4*time.Second, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusConflict)
		return
	}
	headSHA := strings.TrimSpace(headOut)
	if baseSHA == headSHA {
		http.Error(w, "rebase base equals HEAD; choose an earlier commit or branch", http.StatusBadRequest)
		return
	}
	if _, stderr, _, err := s.runGit(r.Context(), 4*time.Second, "merge-base", "--is-ancestor", baseSHA, headSHA); err != nil {
		message := strings.TrimSpace(stderr)
		if message == "" {
			message = "rebase base must be an ancestor of HEAD"
		}
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	limit := 100
	if value, convErr := strconv.Atoi(r.URL.Query().Get("limit")); convErr == nil && value > 0 && value <= 200 {
		limit = value
	}
	format := "%H%x00%h%x00%ad%x00%an%x00%s%x00%P%x1e"
	out, stderr, truncated, err := s.runGit(r.Context(), 10*time.Second,
		"log", "--reverse", "--topo-order", "--date=iso-strict",
		"--max-count="+strconv.Itoa(limit+1), "--pretty=format:"+format, baseSHA+".."+headSHA)
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusConflict)
		return
	}
	rows := parseGitRebasePlan(out)
	rangeTruncated := truncated || len(rows) > limit
	if len(rows) > limit {
		rows = rows[:limit]
	}
	clean, cleanErr := s.gitWorktreeClean(r.Context())
	if cleanErr != nil {
		http.Error(w, cleanErr.Error(), http.StatusConflict)
		return
	}
	hasMerge := false
	for _, row := range rows {
		if row.Merge {
			hasMerge = true
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"branch": branch, "base_ref": baseRef, "base_sha": baseSHA, "head_sha": headSHA,
		"clean": clean, "commits": rows, "truncated": rangeTruncated,
		"has_merge_commits": hasMerge,
	})
}
