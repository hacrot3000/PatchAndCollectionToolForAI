package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitReflogRecoveryCenterUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"['reflog','Recovery']",
		"gitReflogFilters={search:'',ref:'',kind:''}",
		"function gitReflogGuidance(entry)",
		"function gitReflogControls(run)",
		"function renderGitReflogDetail(entry,host)",
		"async function loadReflog()",
		"gitView('reflog',params)",
		"Search SHA / selector / actor / subject",
		"Ref contains, e.g. HEAD or main",
		"Create recovery branch",
		"create_branch_ref",
		"HEAD, Index and Working tree remain unchanged",
		"Checkout detached",
		"Reset current here…",
		"Tag this commit…",
		"Compare with HEAD",
		"Copy SHA",
		"Copy selector",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Git reflog Recovery Center UI missing %q", want)
		}
	}
}

func TestGitReflogBackendClassifiesRecoveryKinds(t *testing.T) {
	cases := map[string]string{
		"reset: moving to HEAD~1": "reset",
		"checkout: moving from main to feature": "checkout",
		"commit: recoverable work": "commit",
		"rebase (finish): returning to refs/heads/main": "rebase",
		"cherry-pick: abc": "cherry-pick",
		"revert: abc": "revert",
		"branch: Created from HEAD": "branch",
	}
	for subject, want := range cases {
		if got := gitReflogKind(subject); got != want {
			t.Fatalf("gitReflogKind(%q)=%q want %q", subject, got, want)
		}
	}
}

func TestGitRecoveryCenterScansLostCommits(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Unreachable / lost commits",
		"Scan lost commits",
		"gitView('lost-commits',params)",
		"git fsck --no-reflogs --unreachable",
		"not visible in reflog",
		"Use this scan after force-deleting a branch",
		"Recover branch",
		"Compare HEAD",
		"Copy SHA",
		"Recovery only creates a branch ref; it does not modify HEAD, Index, or Working tree.",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Git lost-commit Recovery Center UI missing %q", want)
		}
	}
}
