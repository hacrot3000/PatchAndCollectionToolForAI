package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type gitDiffHunk struct {
	Index  int    `json:"index"`
	Header string `json:"header"`
	Text   string `json:"text"`
}

type gitDiffHunkSet struct {
	Path       string        `json:"path"`
	Mode       string        `json:"mode"`
	Diff       string        `json:"diff"`
	DiffSHA256 string        `json:"diff_sha256"`
	FileHeader string        `json:"-"`
	Hunks      []gitDiffHunk `json:"hunks"`
}

func parseGitDiffHunks(path, mode, diff string) gitDiffHunkSet {
	sum := sha256.Sum256([]byte(diff))
	set := gitDiffHunkSet{
		Path: path, Mode: mode, Diff: diff,
		DiffSHA256: hex.EncodeToString(sum[:]),
		Hunks: []gitDiffHunk{},
	}
	lines := strings.SplitAfter(diff, "\n")
	var header strings.Builder
	var current strings.Builder
	currentHeader := ""
	flush := func() {
		if current.Len() == 0 {
			return
		}
		set.Hunks = append(set.Hunks, gitDiffHunk{
			Index: len(set.Hunks), Header: currentHeader, Text: current.String(),
		})
		current.Reset()
		currentHeader = ""
	}
	for _, line := range lines {
		trimmed := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if strings.HasPrefix(trimmed, "@@ ") {
			flush()
			currentHeader = trimmed
			current.WriteString(line)
			continue
		}
		if currentHeader == "" {
			header.WriteString(line)
		} else {
			current.WriteString(line)
		}
	}
	flush()
	set.FileHeader = header.String()
	return set
}

func (s *Server) gitDiffHunkSet(ctx context.Context, path, mode string) (gitDiffHunkSet, bool, error) {
	path, err := validGitRelativePath(path)
	if err != nil {
		return gitDiffHunkSet{}, false, err
	}
	args := []string{"diff", "--no-ext-diff", "--no-color", "--unified=3"}
	switch mode {
	case "staged":
		args = append(args, "--cached")
	case "", "worktree":
		mode = "worktree"
	default:
		return gitDiffHunkSet{}, false, fmt.Errorf("invalid diff mode")
	}
	args = append(args, "--", path)
	out, _, truncated, err := s.runGit(ctx, 8*time.Second, args...)
	if err != nil {
		return gitDiffHunkSet{}, truncated, err
	}
	if truncated {
		return gitDiffHunkSet{}, true, fmt.Errorf("diff is too large for safe hunk operations")
	}
	return parseGitDiffHunks(path, mode, out), false, nil
}

func (s *Server) gitApplyPatchInput(ctx context.Context, patch string, args ...string) (string, bool, error) {
	stdout, stderr, truncated, err := runGitInputInDirectory(ctx, 20*time.Second, s.gitDirectory(ctx), patch, args...)
	return joinGitOutput(stdout, stderr), truncated, err
}

func (s *Server) gitApplyHunk(ctx context.Context, rawPath, mode string, index int, expectedSHA, operation string) (string, bool, error) {
	set, _, err := s.gitDiffHunkSet(ctx, rawPath, mode)
	if err != nil {
		return "", false, err
	}
	expectedSHA = strings.TrimSpace(expectedSHA)
	if expectedSHA == "" || expectedSHA != set.DiffSHA256 {
		return "", false, fmt.Errorf("diff changed since the hunk was displayed; refresh the diff before applying")
	}
	if index < 0 || index >= len(set.Hunks) {
		return "", false, fmt.Errorf("selected hunk is no longer available; refresh the diff")
	}
	patch := set.FileHeader + set.Hunks[index].Text
	if strings.TrimSpace(patch) == "" {
		return "", false, fmt.Errorf("selected hunk patch is empty")
	}

	var checkArgs, applyArgs []string
	switch operation {
	case "stage":
		if set.Mode != "worktree" {
			return "", false, fmt.Errorf("stage hunk requires a worktree diff")
		}
		checkArgs = []string{"apply", "--cached", "--check", "--recount", "-"}
		applyArgs = []string{"apply", "--cached", "--recount", "-"}
	case "unstage":
		if set.Mode != "staged" {
			return "", false, fmt.Errorf("unstage hunk requires a staged diff")
		}
		checkArgs = []string{"apply", "--cached", "--check", "--recount", "-R", "-"}
		applyArgs = []string{"apply", "--cached", "--recount", "-R", "-"}
	case "discard":
		if set.Mode != "worktree" {
			return "", false, fmt.Errorf("discard hunk requires a worktree diff")
		}
		checkArgs = []string{"apply", "--check", "--recount", "-R", "-"}
		applyArgs = []string{"apply", "--recount", "-R", "-"}
	default:
		return "", false, fmt.Errorf("unsupported hunk operation")
	}

	checkOut, checkTruncated, err := s.gitApplyPatchInput(ctx, patch, checkArgs...)
	if err != nil {
		return checkOut, checkTruncated, fmt.Errorf("hunk preflight failed: %w", err)
	}
	applyOut, applyTruncated, err := s.gitApplyPatchInput(ctx, patch, applyArgs...)
	if err != nil {
		return joinGitOutput(checkOut, applyOut), checkTruncated || applyTruncated, err
	}
	label := map[string]string{"stage": "Staged", "unstage": "Unstaged", "discard": "Discarded"}[operation]
	return joinGitOutput(applyOut, fmt.Sprintf("%s hunk %d of %s.", label, index+1, set.Path)), applyTruncated, nil
}
