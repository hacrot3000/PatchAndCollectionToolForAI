package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitStatusFeatureModule(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"git-status-pill",
		"/api/git/status",
		"changed",
		"↑",
		"↓",
		"setInterval(refresh,5000)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("gitstatus.js missing behavior %q", want)
		}
	}
}

func TestParseGitBranchHeader(t *testing.T) {
	branch, ahead, behind := parseGitBranchHeader("## main...origin/main [ahead 2, behind 3]")
	if branch != "main" || ahead != 2 || behind != 3 {
		t.Fatalf("got branch=%q ahead=%d behind=%d", branch, ahead, behind)
	}
	branch, ahead, behind = parseGitBranchHeader("## feature/test")
	if branch != "feature/test" || ahead != 0 || behind != 0 {
		t.Fatalf("unexpected plain branch parse: %q %d %d", branch, ahead, behind)
	}
}
