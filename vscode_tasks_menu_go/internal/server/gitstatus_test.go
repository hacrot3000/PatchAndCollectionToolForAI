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
		"let requestedRepo=activeRepoID",
		"const selectedBefore=requestedRepo",
		"await refreshRepositories(true)",
		"'/api/sessions/'+encodeURIComponent(String(id))+'/cwd'",
		"if(live?.local===false)return",
		"if(live?.cwd)cwd=live.cwd",
		"target_type||'').toLowerCase()==='ssh'",
		"view.meta?.target_profile_id",
		".git-panel{display:none;position:fixed;z-index:2100;top:calc(var(--taskmenu-header-height,30px) + 6px);",
		"body.task-sidebar-auto-hide .git-panel{left:60px;width:min(1080px,calc(100vw - 72px))}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("gitstatus.js missing behavior %q", want)
		}
	}
}

func TestGitPanelRemainsAccessibleInSharedServerWithGitReadPermission(t *testing.T) {
  data,err:=webassets.Files.ReadFile("featuremods/gitstatus.js")
  if err!=nil {t.Fatal(err)}
  js:=string(data)
  for _,want:=range []string{
    "function canViewGitPanel(){return !app.sharedMode||Boolean(app.hasPermission?.('git.status')||app.hasPermission?.('project.admin'));}",
    "if(canViewGitPanel())showGitFallback('Git','Open Git Quick Actions')",
    "if(!data?.repository){showGitFallback('Git · No repository'",
    "showGitFallback('Git · Unavailable'",
    "if(!canViewGitPanel())return;",
    "panel.classList.toggle('visible')",
    "Cannot load Git repositories:",
    "async function refresh(){",
    "setInterval(refresh,5000)",
  }{
    if !strings.Contains(js,want){t.Errorf("shared-server Git launcher missing %q",want)}
  }
  // A missing repository or temporary API failure should no longer hide
  // the only entry point into the repository scanner.
  if strings.Contains(js,"if(!data?.repository){pill.className='git-status-pill';") {
    t.Fatal("Git status without an active repository must not hide the launcher")
  }
  if strings.Contains(js,"catch(e){if(seq===refreshSeq){pill.className='git-status-pill';") {
    t.Fatal("Git status errors must remain visible for diagnostics")
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

func TestGitPanelNotifiesExplorerAfterRefresh(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"taskmenu:git-status-refreshed",
		"repo_id:activeRepoID",
		"changed:Number(data?.changed||0)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Git panel Explorer notification missing %q", want)
		}
	}
}

func TestGitPanelRescansChildRepositoriesWhenWorkspaceRootIsNotGit(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"if(canViewGitPanel())showGitFallback('Git','Open Git Quick Actions')",
		"await refreshRepositories(!repositories.length)",
		"if(!repositories.length)currentView='repositories'",
		"if(!repositories.some(item=>item.id===wanted))wanted=data.default_repository||repositories[0]?.id||''",
		"repoSelect.disabled=!repositories.length",
		"Git markers found but verification failed:",
		"Git repository scan is disabled.",
		"gitScanWarnings=Array.isArray(data.scan_warnings)",
		"repoRescan.onclick=async()=>{try{await refreshRepositories(true)",
	} {
		if !strings.Contains(js, want) { t.Errorf("nested repository Git panel missing %q", want) }
	}
	if strings.Contains(js,"if(!data?.repository){pill.className='git-status-pill';") {
		t.Fatal("Git launcher must remain visible when workspace root is not Git")
	}
}
