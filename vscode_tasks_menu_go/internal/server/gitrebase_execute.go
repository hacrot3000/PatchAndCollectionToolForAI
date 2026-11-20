package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/gitrebaseeditor"
)

type gitRebaseActionItem struct {
	SHA     string `json:"sha"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
}

type validatedGitRebasePlan struct {
	Branch    string
	BaseSHA   string
	HeadSHA   string
	Todo      string
	Reword    map[string]string
	CommitSet map[string]gitRebasePlanCommit
}

func gitEditorCommand(path string) string {
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}

func (s *Server) runGitWithEnv(parent context.Context, timeout time.Duration, extraEnv map[string]string, args ...string) (string, string, bool, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.gitDirectory(parent)
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "LC_ALL=C")
	for key, value := range extraEnv {
		env = append(env, key+"="+value)
	}
	cmd.Env = env
	stdout := &cappedGitBuffer{limit: gitOutputLimit}
	stderr := &cappedGitBuffer{limit: gitOutputLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return stdout.String(), stderr.String(), stdout.truncated || stderr.truncated, fmt.Errorf("git operation timed out")
	}
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), stderr.String(), stdout.truncated || stderr.truncated, fmt.Errorf("%s", message)
	}
	return stdout.String(), stderr.String(), stdout.truncated || stderr.truncated, nil
}

func (s *Server) gitAbsoluteDir(ctx context.Context) (string, error) {
	out, stderr, _, err := s.runGit(ctx, 4*time.Second, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(joinGitOutput(stderr, err.Error())))
	}
	path := filepath.Clean(strings.TrimSpace(out))
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("Git directory could not be resolved")
	}
	return path, nil
}

func gitRebaseTodoLine(action string, commit gitRebasePlanCommit) string {
	subject := strings.ReplaceAll(strings.ReplaceAll(commit.Subject, "\r", " "), "\n", " ")
	return action + " " + commit.SHA + " " + subject
}

func (s *Server) gitValidateInteractiveRebase(ctx context.Context, req gitActionRequest) (validatedGitRebasePlan, error) {
	var result validatedGitRebasePlan
	if state, err := s.gitOperationState(ctx); err == nil && state != "" {
		return result, fmt.Errorf("cannot start interactive rebase while Git %s is already in progress", state)
	}

	branch, err := s.gitCurrentBranch(ctx)
	if err != nil {
		return result, fmt.Errorf("interactive rebase requires a named current branch")
	}
	expectedBranch := strings.TrimSpace(req.ExpectedCurrent)
	if expectedBranch == "" || expectedBranch != branch {
		return result, fmt.Errorf("current branch changed after planning; reload the rebase plan")
	}

	headOut, stderr, _, err := s.runGit(ctx, 4*time.Second, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return result, fmt.Errorf("%s", strings.TrimSpace(joinGitOutput(stderr, err.Error())))
	}
	headSHA := strings.TrimSpace(headOut)
	if !gitCompareCommitPattern.MatchString(headSHA) {
		return result, fmt.Errorf("current HEAD is invalid")
	}
	if strings.TrimSpace(req.ExpectedSHA) != headSHA {
		return result, fmt.Errorf("HEAD changed after planning; reload the rebase plan")
	}

	baseSHA := strings.TrimSpace(req.Ref)
	if !gitCompareCommitPattern.MatchString(baseSHA) {
		return result, fmt.Errorf("full base commit SHA is required")
	}
	resolvedBase, stderr, _, err := s.runGit(ctx, 4*time.Second, "rev-parse", "--verify", baseSHA+"^{commit}")
	if err != nil {
		return result, fmt.Errorf("%s", strings.TrimSpace(joinGitOutput(stderr, err.Error())))
	}
	if strings.TrimSpace(resolvedBase) != baseSHA {
		return result, fmt.Errorf("base commit changed after planning")
	}
	if baseSHA == headSHA {
		return result, fmt.Errorf("rebase base equals HEAD")
	}
	if _, stderr, _, err := s.runGit(ctx, 4*time.Second, "merge-base", "--is-ancestor", baseSHA, headSHA); err != nil {
		message := strings.TrimSpace(stderr)
		if message == "" {
			message = "rebase base must remain an ancestor of HEAD"
		}
		return result, fmt.Errorf("%s", message)
	}
	clean, err := s.gitWorktreeClean(ctx)
	if err != nil {
		return result, err
	}
	if !clean {
		return result, fmt.Errorf("working tree and index must be clean before interactive rebase")
	}

	format := "%H%x00%h%x00%ad%x00%an%x00%s%x00%P%x1e"
	out, stderr, truncated, err := s.runGit(ctx, 10*time.Second,
		"log", "--reverse", "--topo-order", "--date=iso-strict",
		"--max-count=201", "--pretty=format:"+format, baseSHA+".."+headSHA)
	if err != nil {
		return result, fmt.Errorf("%s", strings.TrimSpace(joinGitOutput(stderr, err.Error())))
	}
	commits := parseGitRebasePlan(out)
	if truncated || len(commits) > 200 {
		return result, fmt.Errorf("rebase range is too large; choose a closer base")
	}
	if len(commits) == 0 {
		return result, fmt.Errorf("rebase range is empty")
	}
	commitSet := make(map[string]gitRebasePlanCommit, len(commits))
	for _, commit := range commits {
		if commit.Merge {
			return result, fmt.Errorf("interactive rebase execution does not yet support merge commits in the selected range")
		}
		commitSet[commit.SHA] = commit
	}
	if len(req.RebasePlan) != len(commits) {
		return result, fmt.Errorf("rebase plan no longer matches the selected commit range")
	}

	seen := make(map[string]bool, len(req.RebasePlan))
	reword := map[string]string{}
	todo := make([]string, 0, len(req.RebasePlan))
	kept := 0
	for index, item := range req.RebasePlan {
		sha := strings.TrimSpace(item.SHA)
		commit, ok := commitSet[sha]
		if !ok || seen[sha] {
			return result, fmt.Errorf("rebase plan contains a stale, duplicate, or unknown commit at row %d", index+1)
		}
		seen[sha] = true
		action := strings.ToLower(strings.TrimSpace(item.Action))
		switch action {
		case "pick", "reword", "edit", "squash", "fixup", "drop":
		default:
			return result, fmt.Errorf("unsupported rebase action %q at row %d", action, index+1)
		}
		if (action == "squash" || action == "fixup") && kept == 0 {
			return result, fmt.Errorf("%s cannot be the first non-dropped commit", action)
		}
		if action != "drop" {
			kept++
		}
		if action == "reword" {
			message := strings.TrimSpace(item.Message)
			if message == "" || len(message) > 4000 || strings.ContainsRune(message, '\x00') {
				return result, fmt.Errorf("reword commit %s requires a valid non-empty message", commit.Short)
			}
			reword[sha] = message
		}
		todo = append(todo, gitRebaseTodoLine(action, commit))
	}
	if len(seen) != len(commitSet) {
		return result, fmt.Errorf("rebase plan does not contain every commit in the selected range")
	}
	if kept == 0 {
		return result, fmt.Errorf("interactive rebase cannot drop every commit")
	}

	result.Branch = branch
	result.BaseSHA = baseSHA
	result.HeadSHA = headSHA
	result.Todo = strings.Join(todo, "\n") + "\n"
	result.Reword = reword
	result.CommitSet = commitSet
	return result, nil
}

func (s *Server) gitRebaseEditorPaths(ctx context.Context) (string, string, string, error) {
	gitDir, err := s.gitAbsoluteDir(ctx)
	if err != nil {
		return "", "", "", err
	}
	return gitDir,
		filepath.Join(gitDir, "taskdeck-rebase-todo"),
		filepath.Join(gitDir, "taskdeck-rebase-editor-state.json"),
		nil
}

func (s *Server) gitRebaseEditorEnv(ctx context.Context) (map[string]string, string, string, error) {
	gitDir, todoPath, statePath, err := s.gitRebaseEditorPaths(ctx)
	if err != nil {
		return nil, "", "", err
	}
	command := strings.TrimSpace(s.GitRebaseEditorCommand)
	if command == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, "", "", fmt.Errorf("resolve TaskDeck executable: %w", err)
		}
		command = gitEditorCommand(exe)
	}
	env := map[string]string{
		"GIT_SEQUENCE_EDITOR":                  command,
		"GIT_EDITOR":                           command,
		gitrebaseeditor.EditorModeEnv:           "1",
		gitrebaseeditor.TodoPathEnv:             todoPath,
		gitrebaseeditor.StatePathEnv:            statePath,
	}
	return env, gitDir, statePath, nil
}

func (s *Server) gitCleanupRebaseEditorState(ctx context.Context) {
	_, todoPath, statePath, err := s.gitRebaseEditorPaths(ctx)
	if err != nil {
		return
	}
	_ = os.Remove(todoPath)
	_ = os.Remove(statePath)
}

func (s *Server) gitInteractiveRebaseExecute(ctx context.Context, req gitActionRequest) (string, bool, error) {
	plan, err := s.gitValidateInteractiveRebase(ctx, req)
	if err != nil {
		return "", false, err
	}
	env, gitDir, statePath, err := s.gitRebaseEditorEnv(ctx)
	if err != nil {
		return "", false, err
	}
	_, todoPath, _, _ := s.gitRebaseEditorPaths(ctx)
	if err := os.WriteFile(todoPath, []byte(plan.Todo), 0o600); err != nil {
		return "", false, fmt.Errorf("write interactive rebase todo: %w", err)
	}
	if err := gitrebaseeditor.WriteState(statePath, gitrebaseeditor.State{GitDir: gitDir, Reword: plan.Reword}); err != nil {
		_ = os.Remove(todoPath)
		return "", false, fmt.Errorf("write interactive rebase editor state: %w", err)
	}

	stdout, stderr, truncated, runErr := s.runGitWithEnv(ctx, gitMergeTimeout, env,
		"-c", "rebase.autosquash=false", "rebase", "-i", "--no-autostash", plan.BaseSHA)
	output := strings.TrimSpace(joinGitOutput(stdout, stderr))
	state, stateErr := s.gitOperationState(ctx)
	if stateErr != nil || state != "rebase" {
		s.gitCleanupRebaseEditorState(ctx)
	}
	if runErr != nil {
		return output, truncated, runErr
	}
	if state == "rebase" {
		output = strings.TrimSpace(joinGitOutput(output, "Interactive rebase paused; use the existing Git recovery/merge editor controls to continue, skip, or abort."))
	}
	return output, truncated, nil
}

func (s *Server) gitRunRebaseContinuation(ctx context.Context, args ...string) (string, bool, error) {
	env, _, statePath, err := s.gitRebaseEditorEnv(ctx)
	if err != nil {
		return "", false, err
	}
	if _, err := os.Stat(statePath); err != nil {
		stdout, stderr, truncated, runErr := s.runGit(ctx, gitMergeTimeout, args...)
		return joinGitOutput(stdout, stderr), truncated, runErr
	}
	stdout, stderr, truncated, runErr := s.runGitWithEnv(ctx, gitMergeTimeout, env, args...)
	state, stateErr := s.gitOperationState(ctx)
	if stateErr != nil || state != "rebase" {
		s.gitCleanupRebaseEditorState(ctx)
	}
	return joinGitOutput(stdout, stderr), truncated, runErr
}
