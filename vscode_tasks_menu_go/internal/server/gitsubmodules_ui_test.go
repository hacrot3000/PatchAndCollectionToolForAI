package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitSubmoduleManagerUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"['submodules','Submodules']",
		"async function loadSubmodules()",
		"gitView('submodules')",
		"submoduleStatusSummary(item)",
		"submoduleProjectPath(item)",
		"openSubmoduleRepository(item)",
		"submodule_init",
		"submodule_update",
		"submodule_checkout_expected",
		"submodule_update_recursive",
		"submodule_sync",
		"Update recursive",
		"Sync URLs",
		"Checkout expected",
		"Open repo",
		"Blocked because this submodule has local changes",
		"expected_sha:item.expected_sha",
		"refreshRepositories(true)",
		"selectRepository(match.id)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Git Submodule Manager UI missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"submodule_update',{path:",
		"submodule_init',{path:",
		"submodule_checkout_expected',{path:",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("Submodule Manager must not send arbitrary path in mutation payload: %q", forbidden)
		}
	}
}

func TestGitSubmoduleActionsUseCancellableJobs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	want := "['fetch','pull','push','merge','delete_remote_branch','submodule_init','submodule_update','submodule_checkout_expected','submodule_update_recursive','submodule_sync']"
	if !strings.Contains(js, want) {
		t.Fatalf("Git async action list missing submodule operations: want %q", want)
	}
}
