package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type gitSemanticsPreview struct {
	Branch          string         `json:"branch,omitempty"`
	Detached        bool           `json:"detached,omitempty"`
	HeadSHA         string         `json:"head_sha"`
	TargetSHA       string         `json:"target_sha"`
	HeadEqualsTarget bool          `json:"head_equals_target"`
	Staged          int            `json:"staged"`
	Unstaged        int            `json:"unstaged"`
	Untracked       int            `json:"untracked"`
	Conflicted      int            `json:"conflicted"`
	ChangedFiles    []gitGraphFile `json:"changed_files"`
	Path            string         `json:"path,omitempty"`
	PathStaged      bool           `json:"path_staged,omitempty"`
	PathUnstaged    bool           `json:"path_unstaged,omitempty"`
	PathUntracked   bool           `json:"path_untracked,omitempty"`
	PathConflicted  bool           `json:"path_conflicted,omitempty"`
	TargetPathExists bool          `json:"target_path_exists,omitempty"`
	HeadPathExists   bool          `json:"head_path_exists,omitempty"`
}

func (s *Server) gitResolveExactCommit(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if !gitCompareCommitPattern.MatchString(ref) {
		return "", fmt.Errorf("full commit SHA is required")
	}
	out, stderr, _, err := s.runGit(ctx, 4*time.Second, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(joinGitOutput(stderr, err.Error())))
	}
	sha := strings.TrimSpace(out)
	if sha != ref || !gitCompareCommitPattern.MatchString(sha) {
		return "", fmt.Errorf("commit changed or could not be resolved exactly")
	}
	return sha, nil
}

func (s *Server) gitCurrentHeadSHA(ctx context.Context) (string, error) {
	out, stderr, _, err := s.runGit(ctx, 4*time.Second, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(joinGitOutput(stderr, err.Error())))
	}
	sha := strings.TrimSpace(out)
	if !gitCompareCommitPattern.MatchString(sha) {
		return "", fmt.Errorf("current HEAD is invalid")
	}
	return sha, nil
}

func (s *Server) gitCheckExpectedHead(ctx context.Context, expected string) (string, error) {
	head, err := s.gitCurrentHeadSHA(ctx)
	if err != nil {
		return "", err
	}
	expected = strings.TrimSpace(expected)
	if expected != "" && expected != head {
		return "", fmt.Errorf("HEAD changed after preview; refresh the Git wizard and confirm again")
	}
	return head, nil
}

func gitPathExistsAtCommit(ctx context.Context, s *Server, commit, pathValue string) bool {
	if commit == "" || pathValue == "" {
		return false
	}
	_, _, _, err := s.runGit(ctx, 4*time.Second, "cat-file", "-e", commit+":"+pathValue)
	return err == nil
}

func (s *Server) gitSemanticsPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	target, err := s.gitResolveExactCommit(r.Context(), r.URL.Query().Get("ref"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	head, err := s.gitCurrentHeadSHA(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	branchOut, _, _, _ := s.runGit(r.Context(), 3*time.Second, "branch", "--show-current")
	branch := strings.TrimSpace(branchOut)

	statusOut, _, statusTruncated, err := s.runGit(r.Context(), 5*time.Second,
		"status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	changes := parseGitStatusZ(statusOut)
	preview := gitSemanticsPreview{
		Branch: branch, Detached: branch == "", HeadSHA: head, TargetSHA: target,
		HeadEqualsTarget: head == target,
	}
	for _, change := range changes {
		if change.Staged {
			preview.Staged++
		}
		if change.Unstaged {
			preview.Unstaged++
		}
		if change.Untracked {
			preview.Untracked++
		}
		if change.Conflicted {
			preview.Conflicted++
		}
	}
	diffOut, stderr, diffTruncated, err := s.runGit(r.Context(), 8*time.Second,
		"diff", "--name-status", "-z", "--find-renames", target, head)
	if err != nil {
		http.Error(w, strings.TrimSpace(joinGitOutput(stderr, err.Error())), http.StatusConflict)
		return
	}
	preview.ChangedFiles = parseGitGraphFiles(diffOut)

	pathValue := strings.TrimSpace(r.URL.Query().Get("path"))
	if pathValue != "" {
		pathValue, err = validGitRelativePath(pathValue)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		preview.Path = pathValue
		preview.TargetPathExists = gitPathExistsAtCommit(r.Context(), s, target, pathValue)
		preview.HeadPathExists = gitPathExistsAtCommit(r.Context(), s, head, pathValue)
		for _, change := range changes {
			if change.Path != pathValue && change.OriginalPath != pathValue {
				continue
			}
			preview.PathStaged = preview.PathStaged || change.Staged
			preview.PathUnstaged = preview.PathUnstaged || change.Unstaged
			preview.PathUntracked = preview.PathUntracked || change.Untracked
			preview.PathConflicted = preview.PathConflicted || change.Conflicted
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"preview": preview,
		"truncated": statusTruncated || diffTruncated,
	})
}
