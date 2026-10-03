package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const gitRecoveryRecentLockAge = 2 * time.Minute

func classifyGitFailure(action, output, errorText string) string {
	text := strings.ToLower(strings.TrimSpace(output + "\n" + errorText))
	switch {
	case strings.Contains(text, "detected dubious ownership"):
		return "dubious_ownership"
	case strings.Contains(text, "another git process seems to be running") || strings.Contains(text, "index.lock") || strings.Contains(text, "unable to create") && strings.Contains(text, ".git/") && strings.Contains(text, ".lock"):
		return "index_lock"
	case strings.Contains(text, "author identity unknown") || strings.Contains(text, "please tell me who you are") || strings.Contains(text, "unable to auto-detect email address"):
		return "identity_missing"
	case strings.Contains(text, "gpg failed to sign") || strings.Contains(text, "failed to sign the data") || strings.Contains(text, "signing failed"):
		return "gpg_signing"
	case strings.Contains(text, "no configured push destination") || strings.Contains(text, "does not appear to be a git repository") || strings.Contains(text, "no such remote") || strings.Contains(text, "repository has no configured git remote"):
		return "remote_missing"
	case strings.Contains(text, "permission denied (publickey") || strings.Contains(text, "could not read from remote repository") && strings.Contains(text, "permission denied"):
		return "ssh_auth"
	case strings.Contains(text, "host key verification failed") || strings.Contains(text, "remote host identification has changed"):
		return "ssh_host_key"
	case strings.Contains(text, "authentication failed") || strings.Contains(text, "could not read username") || strings.Contains(text, "could not read password") || strings.Contains(text, "http basic: access denied") || strings.Contains(text, "password authentication was removed"):
		return "https_auth"
	case strings.Contains(text, "repository not found") || strings.Contains(text, "the requested url returned error: 403") || strings.Contains(text, "permission to") && strings.Contains(text, "denied") || strings.Contains(text, "write access to repository not granted"):
		return "remote_permission"
	case strings.Contains(text, "git operation timed out"):
		return "timeout"
	case strings.Contains(text, "could not resolve host") || strings.Contains(text, "could not resolve hostname") || strings.Contains(text, "could not resolve proxy") || strings.Contains(text, "temporary failure in name resolution") || strings.Contains(text, "name or service not known"):
		return "network_dns"
	case strings.Contains(text, "failed to connect") || strings.Contains(text, "connection timed out") || strings.Contains(text, "operation timed out") || strings.Contains(text, "connection refused") || strings.Contains(text, "network is unreachable"):
		return "network_connect"
	case strings.Contains(text, "ssl certificate problem") || strings.Contains(text, "certificate verify failed") || strings.Contains(text, "server certificate verification failed") || strings.Contains(text, "tls"):
		return "tls"
	case strings.Contains(text, "429") && strings.Contains(text, "rate") || strings.Contains(text, "rate limit exceeded"):
		return "rate_limited"
	case strings.Contains(text, "rpc failed") || strings.Contains(text, "http/2 stream") || strings.Contains(text, "early eof") || strings.Contains(text, "remote end hung up unexpectedly") || strings.Contains(text, "requested url returned error: 502") || strings.Contains(text, "requested url returned error: 503"):
		return "transport"
	case strings.Contains(text, "has no upstream branch"):
		return "no_upstream_push"
	case strings.Contains(text, "there is no tracking information for the current branch") || strings.Contains(text, "no upstream configured"):
		return "no_upstream_pull"
	case strings.Contains(text, "non-fast-forward") || strings.Contains(text, "fetch first") || strings.Contains(text, "updates were rejected because the remote contains work"):
		return "push_non_fast_forward"
	case strings.Contains(text, "not possible to fast-forward") || strings.Contains(text, "divergent branches") || strings.Contains(text, "need to specify how to reconcile"):
		return "pull_diverged"
	case strings.Contains(text, "would be overwritten by merge") || strings.Contains(text, "would be overwritten by checkout") || strings.Contains(text, "please commit your changes or stash them") || strings.Contains(text, "cannot pull with rebase") || strings.Contains(text, "you have unstaged changes") || strings.Contains(text, "index contains uncommitted changes") || strings.Contains(text, "working tree must be clean"):
		return "dirty_worktree"
	case strings.Contains(text, "merge_head exists") || strings.Contains(text, "you have not concluded your merge"):
		return "merge_in_progress"
	case strings.Contains(text, "rebase in progress") || strings.Contains(text, "rebase-merge") || strings.Contains(text, "rebase-apply") || strings.Contains(text, "already a rebase-merge directory"):
		return "rebase_in_progress"
	case strings.Contains(text, "cherry-pick is currently in progress") || strings.Contains(text, "cherry_pick_head"):
		return "cherry_pick_in_progress"
	case action == "merge_to" && (strings.Contains(text, "conflict") || strings.Contains(text, "automatic merge failed")):
		return "merge_to_conflicts"
	case strings.Contains(text, "you have unmerged files") || strings.Contains(text, "needs merge") || strings.Contains(text, "fix conflicts and then commit") || strings.Contains(text, "resolve all conflicts manually") || strings.Contains(text, "unresolved conflict") || strings.Contains(text, "automatic merge failed"):
		return "conflicts"
	case strings.Contains(text, "exceeds github's file size limit") || strings.Contains(text, "gh001") || strings.Contains(text, "large files detected") || strings.Contains(text, "oversized file") || strings.Contains(text, "oversized blob"):
		return "file_too_large"
	case strings.Contains(text, "protected branch") || strings.Contains(text, "protected branch hook declined") || strings.Contains(text, "gh013") || strings.Contains(text, "pre-receive hook declined"):
		return "protected_branch"
	case strings.Contains(text, "src refspec") && strings.Contains(text, "does not match any"):
		return "refspec_missing"
	case strings.Contains(text, "couldn't find remote ref") || strings.Contains(text, "remote ref does not exist"):
		return "remote_ref_missing"
	case strings.Contains(text, "already exists") && strings.Contains(text, "branch"):
		return "branch_exists"
	case strings.Contains(text, "pathspec") && strings.Contains(text, "did not match"):
		return "pathspec_missing"
	case strings.Contains(text, "local branch not found") || strings.Contains(text, "no such branch") || strings.Contains(text, "invalid reference") || strings.Contains(text, "unknown revision"):
		return "branch_missing"
	case strings.Contains(text, "cannot lock ref") || strings.Contains(text, "is at") && strings.Contains(text, "but expected"):
		return "ref_lock"
	case strings.Contains(text, "nothing to commit") || strings.Contains(text, "no changes added to commit"):
		return "nothing_to_commit"
	case strings.Contains(text, "you are not currently on a branch") || strings.Contains(text, "detached head"):
		return "detached_head"
	case strings.Contains(text, "refusing to merge unrelated histories"):
		return "unrelated_histories"
	case strings.Contains(text, "no space left on device") || strings.Contains(text, "disk quota exceeded"):
		return "disk_full"
	case strings.Contains(text, "bad object") || strings.Contains(text, "object file") && strings.Contains(text, "is empty") || strings.Contains(text, "corrupt loose object") || strings.Contains(text, "invalid object"):
		return "repository_corrupt"
	case strings.Contains(text, "hook") && (strings.Contains(text, "failed") || strings.Contains(text, "declined") || strings.Contains(text, "exit code")):
		return "hook_failed"
	case strings.Contains(text, "not a git repository"):
		return "not_git_repository"
	case strings.Contains(text, "permission denied") || strings.Contains(text, "operation not permitted"):
		return "filesystem_permission"
	}
	_ = action
	return "generic"
}

func (s *Server) gitCurrentBranchName(ctx context.Context) (string, error) {
	out, _, _, err := s.runGit(ctx, 5*time.Second, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("current HEAD is detached; create or switch to a branch first")
	}
	return strings.TrimSpace(out), nil
}

func stringInList(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func (s *Server) gitPreferredRemote(ctx context.Context, requested string) (string, error) {
	remotes := s.gitRemoteNames(ctx)
	if len(remotes) == 0 {
		return "", fmt.Errorf("repository has no configured Git remote")
	}
	requested = strings.TrimSpace(requested)
	if requested != "" {
		if !stringInList(remotes, requested) {
			return "", fmt.Errorf("Git remote %q does not exist", requested)
		}
		return requested, nil
	}
	if branch, branchErr := s.gitCurrentBranchName(ctx); branchErr == nil {
		if out, _, _, configErr := s.runGit(ctx, 3*time.Second, "config", "--get", "branch."+branch+".remote"); configErr == nil {
			if remote := strings.TrimSpace(out); remote != "" && remote != "." && stringInList(remotes, remote) {
				return remote, nil
			}
		}
	}
	if stringInList(remotes, "origin") {
		return "origin", nil
	}
	if len(remotes) == 1 {
		return remotes[0], nil
	}
	return "", fmt.Errorf("repository has multiple remotes; select a remote explicitly")
}

func (s *Server) gitOperationState(ctx context.Context) (string, error) {
	out, _, _, err := s.runGit(ctx, 5*time.Second, "rev-parse", "--git-dir")
	if err != nil {
		return "", err
	}
	gitDir := strings.TrimSpace(out)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(s.gitDirectory(ctx), gitDir)
	}
	exists := func(name string) bool {
		_, statErr := os.Stat(filepath.Join(gitDir, name))
		return statErr == nil
	}
	switch {
	case exists("MERGE_HEAD"):
		return "merge", nil
	case exists("rebase-merge") || exists("rebase-apply"):
		return "rebase", nil
	case exists("CHERRY_PICK_HEAD"):
		return "cherry-pick", nil
	case exists("REVERT_HEAD"):
		return "revert", nil
	default:
		return "", nil
	}
}

func gitRecoveryNeedsConfirmation(repair string) bool {
	switch repair {
	case "push_force_with_lease", "trust_repository", "remove_stale_index_lock", "pull_allow_unrelated", "commit_no_verify", "push_no_verify", "large_file_remove_latest", "large_file_prepare_recommit", "conflict_resolve_all", "merge_to_prepare_resolution":
		return true
	default:
		return false
	}
}

func (s *Server) gitRepairAction(ctx context.Context, req gitActionRequest) (string, bool, error) {
	repair := strings.TrimSpace(req.Repair)
	if repair == "" {
		return "", false, fmt.Errorf("Git repair action is required")
	}
	if gitRecoveryNeedsConfirmation(repair) && !req.Confirmed {
		return "", false, fmt.Errorf("explicit confirmation is required for repair %q", repair)
	}

	run := func(timeout time.Duration, args ...string) (string, bool, error) {
		stdout, stderr, truncated, err := s.runGit(ctx, timeout, args...)
		return joinGitOutput(stdout, stderr), truncated, err
	}

	switch repair {
	case "status":
		return run(10*time.Second, "status", "--short", "--branch")
	case "fetch":
		remote := strings.TrimSpace(req.Remote)
		if remote == "" {
			return run(30*time.Second, "fetch", "--prune")
		}
		if _, err := s.gitPreferredRemote(ctx, remote); err != nil {
			return "", false, err
		}
		return run(30*time.Second, "fetch", "--prune", remote)
	case "remote_check":
		remote, err := s.gitPreferredRemote(ctx, req.Remote)
		if err != nil {
			return "", false, err
		}
		return run(30*time.Second, "ls-remote", "--heads", remote)
	case "set_upstream":
		branch, err := s.gitCurrentBranchName(ctx)
		if err != nil {
			return "", false, err
		}
		remote, err := s.gitPreferredRemote(ctx, req.Remote)
		if err != nil {
			return "", false, err
		}
		ref := "refs/remotes/" + remote + "/" + branch
		if _, _, _, err := s.runGit(ctx, 5*time.Second, "rev-parse", "--verify", ref); err != nil {
			return "", false, fmt.Errorf("remote branch %s/%s is not available locally; fetch it first or publish the branch", remote, branch)
		}
		return run(10*time.Second, "branch", "--set-upstream-to="+remote+"/"+branch, branch)
	case "push_set_upstream":
		branch, err := s.gitCurrentBranchName(ctx)
		if err != nil {
			return "", false, err
		}
		remote, err := s.gitPreferredRemote(ctx, req.Remote)
		if err != nil {
			return "", false, err
		}
		return run(gitNetworkPushTimeout, "push", "--set-upstream", remote, branch)
	case "pull_rebase":
		return run(60*time.Second, "pull", "--rebase")
	case "pull_merge":
		return run(60*time.Second, "-c", "core.editor=true", "pull", "--no-rebase")
	case "pull_allow_unrelated":
		return run(60*time.Second, "-c", "core.editor=true", "pull", "--no-rebase", "--allow-unrelated-histories")
	case "push_force_with_lease":
		return run(gitNetworkPushTimeout, "push", "--force-with-lease")
	case "retry_http1":
		var args []string
		timeout := gitNetworkReadTimeout
		switch strings.TrimSpace(req.OriginalAction) {
		case "fetch":
			args = []string{"-c", "http.version=HTTP/1.1", "fetch", "--prune"}
		case "pull":
			args = []string{"-c", "http.version=HTTP/1.1", "pull", "--ff-only"}
		case "push":
			timeout = gitNetworkPushTimeout
			args = []string{"-c", "http.version=HTTP/1.1", "push"}
		default:
			return "", false, fmt.Errorf("HTTP/1.1 retry is only supported for fetch, pull, or push")
		}
		return run(timeout, args...)
	case "large_file_remove_latest":
		return s.gitLargeFileRemoveFromLatest(ctx, req.LargePath, gitHubPushBlobLimit)
	case "large_file_prepare_recommit":
		return s.gitLargeFilePrepareRecommit(ctx, req.LargePath, gitHubPushBlobLimit)
	case "conflict_take_side":
		return s.gitResolveConflictSide(ctx, req.Path, req.ConflictSide)
	case "conflict_mark_resolved":
		return s.gitMarkConflictResolved(ctx, req.Path)
	case "conflict_resolve_all":
		return s.gitResolveAllConflictsSide(ctx, req.ConflictSide)
	case "merge_to_prepare_resolution":
		return s.gitMergeToPrepareResolution(ctx, req)
	case "abort_in_progress":
		state, err := s.gitOperationState(ctx)
		if err != nil {
			return "", false, err
		}
		switch state {
		case "merge":
			return run(20*time.Second, "merge", "--abort")
		case "rebase":
			return run(20*time.Second, "rebase", "--abort")
		case "cherry-pick":
			return run(20*time.Second, "cherry-pick", "--abort")
		case "revert":
			return run(20*time.Second, "revert", "--abort")
		default:
			return "", false, fmt.Errorf("no merge, rebase, cherry-pick, or revert operation is in progress")
		}
	case "continue_in_progress":
		state, err := s.gitOperationState(ctx)
		if err != nil {
			return "", false, err
		}
		switch state {
		case "merge":
			return run(gitMergeTimeout, "-c", "core.editor=true", "merge", "--continue")
		case "rebase":
			return run(gitMergeTimeout, "-c", "core.editor=true", "rebase", "--continue")
		case "cherry-pick":
			return run(gitMergeTimeout, "-c", "core.editor=true", "cherry-pick", "--continue")
		case "revert":
			return run(gitMergeTimeout, "-c", "core.editor=true", "revert", "--continue")
		default:
			return "", false, fmt.Errorf("no merge, rebase, cherry-pick, or revert operation is in progress")
		}
	case "skip_rebase":
		state, err := s.gitOperationState(ctx)
		if err != nil {
			return "", false, err
		}
		if state != "rebase" {
			return "", false, fmt.Errorf("no rebase operation is in progress")
		}
		return run(30*time.Second, "rebase", "--skip")
	case "configure_identity":
		name := strings.TrimSpace(req.Name)
		email := strings.TrimSpace(req.Email)
		if name == "" || email == "" || len(name) > 300 || len(email) > 500 || strings.ContainsAny(name+email, "\r\n\x00") {
			return "", false, fmt.Errorf("valid Git user name and email are required")
		}
		out1, trunc1, err := run(10*time.Second, "config", "user.name", name)
		if err != nil {
			return out1, trunc1, err
		}
		out2, trunc2, err := run(10*time.Second, "config", "user.email", email)
		return joinGitOutput(out1, out2, "Configured repository-local Git identity."), trunc1 || trunc2, err
	case "commit_no_sign":
		message := strings.TrimSpace(req.Message)
		if message == "" || len(message) > 2000 || strings.ContainsRune(message, '\x00') {
			return "", false, fmt.Errorf("commit message required")
		}
		return run(30*time.Second, "-c", "commit.gpgSign=false", "commit", "-m", message)
	case "commit_no_verify":
		message := strings.TrimSpace(req.Message)
		if message == "" || len(message) > 2000 || strings.ContainsRune(message, '\x00') {
			return "", false, fmt.Errorf("commit message required")
		}
		return run(30*time.Second, "commit", "--no-verify", "-m", message)
	case "push_no_verify":
		return run(gitNetworkPushTimeout, "push", "--no-verify")
	case "create_branch_from_head":
		branch := strings.TrimSpace(req.NewBranch)
		if err := s.validBranchName(ctx, branch); err != nil {
			return "", false, err
		}
		return run(20*time.Second, "switch", "-c", branch)
	case "push_new_branch":
		branch := strings.TrimSpace(req.NewBranch)
		if err := s.validBranchName(ctx, branch); err != nil {
			return "", false, err
		}
		remote, err := s.gitPreferredRemote(ctx, req.Remote)
		if err != nil {
			return "", false, err
		}
		if _, branchErr := s.gitCurrentBranchName(ctx); branchErr == nil {
			return run(gitNetworkPushTimeout, "push", "--set-upstream", remote, "HEAD:refs/heads/"+branch)
		}
		return run(gitNetworkPushTimeout, "push", remote, "HEAD:refs/heads/"+branch)
	case "add_remote":
		remote := strings.TrimSpace(req.Remote)
		if remote == "" {
			remote = "origin"
		}
		if len(remote) > 200 || strings.HasPrefix(remote, "-") || strings.ContainsAny(remote, " \t\r\n\x00") {
			return "", false, fmt.Errorf("invalid Git remote name")
		}
		if stringInList(s.gitRemoteNames(ctx), remote) {
			return "", false, fmt.Errorf("Git remote %q already exists; use Change remote URL instead", remote)
		}
		if _, _, _, err := s.runGit(ctx, 5*time.Second, "check-ref-format", "refs/remotes/"+remote+"/placeholder"); err != nil {
			return "", false, fmt.Errorf("invalid Git remote name %q", remote)
		}
		value := strings.TrimSpace(req.RemoteURL)
		if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
			return "", false, fmt.Errorf("valid remote URL is required")
		}
		return run(10*time.Second, "remote", "add", remote, value)
	case "set_remote_url":
		remote, err := s.gitPreferredRemote(ctx, req.Remote)
		if err != nil {
			return "", false, err
		}
		value := strings.TrimSpace(req.RemoteURL)
		if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
			return "", false, fmt.Errorf("valid remote URL is required")
		}
		return run(10*time.Second, "remote", "set-url", remote, value)
	case "trust_repository":
		dir := s.gitDirectory(ctx)
		stdout, stderr, truncated, err := runGitInDirectory(ctx, 10*time.Second, dir, "config", "--global", "--add", "safe.directory", dir)
		return joinGitOutput(stdout, stderr, "Trusted Git repository: "+dir), truncated, err
	case "remove_stale_index_lock":
		out, _, _, err := s.runGit(ctx, 5*time.Second, "rev-parse", "--git-dir")
		if err != nil {
			return "", false, err
		}
		gitDir := strings.TrimSpace(out)
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(s.gitDirectory(ctx), gitDir)
		}
		lockPath := filepath.Join(gitDir, "index.lock")
		info, statErr := os.Stat(lockPath)
		if os.IsNotExist(statErr) {
			return "No .git/index.lock file exists.", false, nil
		}
		if statErr != nil {
			return "", false, statErr
		}
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("%s is not a regular file", lockPath)
		}
		age := time.Since(info.ModTime())
		if age < gitRecoveryRecentLockAge {
			return "", false, fmt.Errorf("index.lock is only %s old; refusing to remove a possibly active Git lock", age.Round(time.Second))
		}
		if err := os.Remove(lockPath); err != nil {
			return "", false, err
		}
		return fmt.Sprintf("Removed stale Git index lock (%s old): %s", age.Round(time.Second), lockPath), false, nil
	case "fsck":
		return run(90*time.Second, "fsck", "--full")
	default:
		return "", false, fmt.Errorf("unsupported Git repair action %q", repair)
	}
}
