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
		"git-repo-bar",
		"repoSelect",
		"repo_name",
		"repo_path",
		"refreshRepositories(false)",
		"const selectedBefore=activeRepoID",
		"await refreshRepositories(true)",
		"'/api/sessions/'+encodeURIComponent(String(id))+'/cwd'",
		"if(live?.local===false)return",
		"if(live?.cwd)cwd=live.cwd",
		"target_type||'').toLowerCase()==='ssh'",
		"view.meta?.target_profile_id",
		".git-panel{display:none;position:fixed;z-index:1500;top:calc(var(--taskmenu-header-height,30px) + 6px);",
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
