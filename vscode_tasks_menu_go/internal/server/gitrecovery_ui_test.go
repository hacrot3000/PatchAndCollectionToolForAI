package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitRecoveryWizardModule(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitrecovery.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"Git recovery wizard",
		"TaskDeckGitRecovery",
		"function classifyLocal(ctx)",
		"no_upstream_push",
		"no_upstream_pull",
		"push_non_fast_forward",
		"pull_diverged",
		"dirty_worktree",
		"conflicts",
		"identity_missing",
		"gpg_signing",
		"ssh_auth",
		"ssh_host_key",
		"https_auth",
		"remote_permission",
		"network_dns",
		"network_connect",
		"tls",
		"transport",
		"protected_branch",
		"detached_head",
		"unrelated_histories",
		"index_lock",
		"dubious_ownership",
		"repository_corrupt",
		"file_too_large",
		"function largeFilesFromContext(ctx)",
		"large_file_remove_latest",
		"large_file_prepare_recommit",
		"Remove from latest commit, ignore, then retry push",
		"Prepare all unpushed commits for recommit",
		"Oversized file path",
		"remote_missing",
		"pathspec_missing",
		"branch_missing",
		"ref_lock",
		"nothing_to_commit",
		"rate_limited",
		"filesystem_permission",
		"add_remote",
		"push_set_upstream",
		"set_upstream",
		"pull_rebase",
		"pull_merge",
		"push_force_with_lease",
		"abort_in_progress",
		"continue_in_progress",
		"configure_identity",
		"commit_no_sign",
		"push_new_branch",
		"set_remote_url",
		"remove_stale_index_lock",
		"retry_http1",
		"Run",
		"Copy details",
		"Retry original",
		"window.confirm",
		"function copyText(value)",
		"Git panel refreshed.",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("gitrecovery.js missing recovery behavior %q", want)
		}
	}
	for _, forbidden := range []string{"innerHTML=", "eval(", "new Function("} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("gitrecovery.js must not use unsafe dynamic rendering/execution %q", forbidden)
		}
	}
}

func TestGitStatusRoutesFailuresIntoRecoveryWizard(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const gitRecovery=globalThis.TaskDeckGitRecovery",
		"function gitFailureError(message,data={})",
		"function openGitRecovery(",
		"async function repairAction(",
		"action:'repair',repair",
		"failure_code",
		"gitFailureCode",
		"gitDetails=data&&typeof data==='object'?data:{}",
		"details:failure.gitDetails",
		"gitWizardShown",
		"recovery:false",
		"runRepair:",
		"runAction:",
		"rescan:",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("gitstatus.js missing Git recovery integration %q", want)
		}
	}
}

func TestGitRecoveryLoadsBeforeGitStatus(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	recovery := strings.Index(js, "featuremods/gitrecovery.js")
	status := strings.Index(js, "featuremods/gitstatus.js")
	if recovery < 0 || status < 0 || recovery > status {
		t.Fatalf("gitrecovery.js must load before gitstatus.js: recovery=%d status=%d", recovery, status)
	}
}
