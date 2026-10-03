package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const gitMergeTimeout = 2 * time.Minute

type gitConflictFile struct {
	Path        string `json:"path"`
	ProjectPath string `json:"project_path,omitempty"`
	Status      string `json:"status,omitempty"`
	HasBase     bool   `json:"has_base"`
	HasCurrent  bool   `json:"has_current"`
	HasIncoming bool   `json:"has_incoming"`
}

type gitConflictState struct {
	Operation string            `json:"operation,omitempty"`
	Branch    string            `json:"branch,omitempty"`
	Files     []gitConflictFile `json:"files"`
}

func isGitUnmergedStatus(x, y byte) bool {
	switch string([]byte{x, y}) {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	default:
		return false
	}
}

func (s *Server) gitConflictProjectPath(ctx context.Context, repoPath string) string {
	root, err := filepath.Abs(s.Workspace)
	if err != nil {
		return ""
	}
	repo, err := filepath.Abs(s.gitDirectory(ctx))
	if err != nil {
		return ""
	}
	full := filepath.Join(repo, filepath.FromSlash(repoPath))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

func (s *Server) gitConflictState(ctx context.Context) (gitConflictState, error) {
	state := gitConflictState{Files: []gitConflictFile{}}
	if operation, err := s.gitOperationState(ctx); err == nil {
		state.Operation = operation
	}
	if branch, err := s.gitCurrentBranchName(ctx); err == nil {
		state.Branch = branch
	}

	statusByPath := map[string]string{}
	if out, _, _, err := s.runGit(ctx, 8*time.Second, "status", "--porcelain=v1", "-z", "--untracked-files=no"); err == nil {
		for _, change := range parseGitStatusZ(out) {
			if len(change.IndexStatus) == 1 && len(change.WorktreeStatus) == 1 && isGitUnmergedStatus(change.IndexStatus[0], change.WorktreeStatus[0]) {
				statusByPath[change.Path] = change.IndexStatus + change.WorktreeStatus
			}
		}
	}

	out, _, truncated, err := s.runGit(ctx, 8*time.Second, "ls-files", "-u", "-z")
	if err != nil {
		return state, err
	}
	if truncated {
		return state, fmt.Errorf("conflict file list exceeded output limit")
	}
	byPath := map[string]*gitConflictFile{}
	order := []string{}
	for _, record := range strings.Split(out, "\x00") {
		if record == "" {
			continue
		}
		tab := strings.IndexByte(record, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(record[:tab])
		if len(meta) < 3 {
			continue
		}
		stage, stageErr := strconv.Atoi(meta[2])
		if stageErr != nil || stage < 1 || stage > 3 {
			continue
		}
		path := record[tab+1:]
		item := byPath[path]
		if item == nil {
			item = &gitConflictFile{Path: path, ProjectPath: s.gitConflictProjectPath(ctx, path), Status: statusByPath[path]}
			byPath[path] = item
			order = append(order, path)
		}
		switch stage {
		case 1:
			item.HasBase = true
		case 2:
			item.HasCurrent = true
		case 3:
			item.HasIncoming = true
		}
	}
	for _, path := range order {
		state.Files = append(state.Files, *byPath[path])
	}
	return state, nil
}

func gitConflictFindFile(state gitConflictState, path string) (gitConflictFile, bool) {
	for _, item := range state.Files {
		if item.Path == path {
			return item, true
		}
	}
	return gitConflictFile{}, false
}

func (s *Server) gitResolveConflictSide(ctx context.Context, rawPath, side string) (string, bool, error) {
	path, err := validGitRelativePath(rawPath)
	if err != nil {
		return "", false, err
	}
	side = strings.ToLower(strings.TrimSpace(side))
	if side != "current" && side != "incoming" {
		return "", false, fmt.Errorf("conflict side must be current or incoming")
	}
	state, err := s.gitConflictState(ctx)
	if err != nil {
		return "", false, err
	}
	if state.Operation == "" {
		return "", false, fmt.Errorf("no merge, rebase, cherry-pick, or revert operation is in progress")
	}
	item, ok := gitConflictFindFile(state, path)
	if !ok {
		return "", false, fmt.Errorf("%s is not currently an unmerged path", path)
	}
	hasStage := item.HasCurrent
	checkoutSide := "--ours"
	if side == "incoming" {
		hasStage = item.HasIncoming
		checkoutSide = "--theirs"
	}
	var output []string
	run := func(timeout time.Duration, args ...string) error {
		stdout, stderr, _, runErr := s.runGit(ctx, timeout, args...)
		if text := joinGitOutput(stdout, stderr); text != "" {
			output = append(output, text)
		}
		return runErr
	}
	if hasStage {
		if err := run(20*time.Second, "checkout", checkoutSide, "--", path); err != nil {
			return joinGitOutput(output...), false, err
		}
		if err := run(20*time.Second, "add", "-A", "--", path); err != nil {
			return joinGitOutput(output...), false, err
		}
	} else {
		if err := run(20*time.Second, "rm", "--ignore-unmatch", "--", path); err != nil {
			return joinGitOutput(output...), false, err
		}
	}
	output = append(output, fmt.Sprintf("Resolved %s using the %s side and staged the result.", path, side))
	return joinGitOutput(output...), false, nil
}

func (s *Server) gitMarkConflictResolved(ctx context.Context, rawPath string) (string, bool, error) {
	path, err := validGitRelativePath(rawPath)
	if err != nil {
		return "", false, err
	}
	state, err := s.gitConflictState(ctx)
	if err != nil {
		return "", false, err
	}
	if state.Operation == "" {
		return "", false, fmt.Errorf("no merge, rebase, cherry-pick, or revert operation is in progress")
	}
	if _, ok := gitConflictFindFile(state, path); !ok {
		return "", false, fmt.Errorf("%s is not currently an unmerged path", path)
	}
	stdout, stderr, truncated, err := s.runGit(ctx, 20*time.Second, "add", "-A", "--", path)
	if err != nil {
		return joinGitOutput(stdout, stderr), truncated, err
	}
	return joinGitOutput(stdout, stderr, "Marked "+path+" resolved and staged the working-tree version."), truncated, nil
}

func (s *Server) gitResolveAllConflictsSide(ctx context.Context, side string) (string, bool, error) {
	state, err := s.gitConflictState(ctx)
	if err != nil {
		return "", false, err
	}
	if state.Operation == "" {
		return "", false, fmt.Errorf("no merge, rebase, cherry-pick, or revert operation is in progress")
	}
	if len(state.Files) == 0 {
		return "No unmerged paths remain.", false, nil
	}
	var output []string
	for _, item := range state.Files {
		text, _, resolveErr := s.gitResolveConflictSide(ctx, item.Path, side)
		if text != "" {
			output = append(output, text)
		}
		if resolveErr != nil {
			return joinGitOutput(output...), false, fmt.Errorf("resolve %s: %w", item.Path, resolveErr)
		}
	}
	output = append(output, fmt.Sprintf("Resolved and staged %d conflict path(s) using the %s side.", len(state.Files), side))
	return joinGitOutput(output...), false, nil
}

func (s *Server) gitMergeToPrepareResolution(ctx context.Context, req gitActionRequest) (string, bool, error) {
	source := strings.TrimSpace(req.ExpectedSourceSHA)
	target := strings.TrimSpace(req.ExpectedTargetSHA)
	branch := strings.TrimSpace(req.NewBranch)
	if source == "" || target == "" {
		return "", false, fmt.Errorf("Merge To recovery requires the original source and target commit IDs")
	}
	if err := s.validBranchName(ctx, branch); err != nil {
		return "", false, err
	}
	if s.localBranchExists(ctx, branch) {
		return "", false, fmt.Errorf("branch %s already exists", branch)
	}
	clean, err := s.gitWorktreeClean(ctx)
	if err != nil {
		return "", false, err
	}
	if !clean {
		return "", false, fmt.Errorf("working tree must be clean before creating a Merge To conflict-resolution branch")
	}
	verifyCommit := func(value, label string) (string, error) {
		out, _, _, verifyErr := s.runGit(ctx, 8*time.Second, "rev-parse", "--verify", value+"^{commit}")
		if verifyErr != nil {
			return "", fmt.Errorf("%s commit is no longer available: %w", label, verifyErr)
		}
		return strings.TrimSpace(out), nil
	}
	sourceSHA, err := verifyCommit(source, "source")
	if err != nil {
		return "", false, err
	}
	targetSHA, err := verifyCommit(target, "target")
	if err != nil {
		return "", false, err
	}
	original, _ := s.gitCurrentBranchName(ctx)
	switchOut, switchErrOut, switchTruncated, err := s.runGit(ctx, 30*time.Second, "switch", "-c", branch, targetSHA)
	output := joinGitOutput(switchOut, switchErrOut)
	if err != nil {
		return output, switchTruncated, err
	}
	mergeOut, mergeErrOut, mergeTruncated, mergeErr := s.runGit(ctx, gitMergeTimeout, "merge", "--no-edit", sourceSHA)
	output = joinGitOutput(output, mergeOut, mergeErrOut)
	truncated := switchTruncated || mergeTruncated
	if mergeErr != nil {
		return joinGitOutput(output,
			"Created local Merge To resolution branch: "+branch,
			"Original branch: "+original,
			"Resolve the conflicts in this branch, then continue the merge."),
			truncated, mergeErr
	}
	return joinGitOutput(output,
		"Created local Merge To resolution branch: "+branch,
		"Merge completed without conflicts. Review the result and push it explicitly when ready.",
		"Original branch: "+original), truncated, nil
}

func (s *Server) gitFailurePayload(ctx context.Context, action, output, errorText string, truncated bool) map[string]any {
	code := classifyGitFailure(action, output, errorText)
	payload := map[string]any{
		"ok": false, "action": action, "output": output, "error": errorText,
		"failure_code": code, "truncated": truncated,
	}
	if code == "conflicts" || code == "merge_in_progress" || code == "rebase_in_progress" || code == "cherry_pick_in_progress" {
		if state, err := s.gitConflictState(ctx); err == nil {
			payload["conflict_state"] = state
		}
	}
	return payload
}

func (s *Server) gitAttachConflictState(ctx context.Context, payload map[string]any) {
	if state, err := s.gitConflictState(ctx); err == nil {
		payload["conflict_state"] = state
	}
}
