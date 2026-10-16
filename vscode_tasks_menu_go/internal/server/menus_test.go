package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGroupedMenusFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"header-action-menus",
		"makeMenu('Files'",
		"Quick Open…  Ctrl+P",
		"explorer.textContent='Explorer'",
		"TaskMenuExplorer?.open()",
		"const terminal=document.querySelector('#open-terminal')",
		"terminal.classList.add('taskmenu-menu-trigger','taskmenu-direct-action')",
		"terminal.textContent='Terminal'",
		"terminal.title='Open a new terminal · Ctrl+Shift+`'",
		"addSection(settings.pop,'TERMINAL',[document.querySelector('#terminal-cwd')])",
		"makeMenu('Settings'",
		"addSection(settings.pop,'PATCH TOOL',[patchUIControl])",
		"patchUISelect.id='patch-ui-mode'",
		"Native UI",
		"Terminal (legacy)",
		"vscode-tasks-menu:patch-ui-mode:",
		"taskmenu:patch-ui-mode",
		"TaskMenuPatchUISettings",
		"addSection(files.pop,'FILES',[document.querySelector('#upload-workspace')])",
		"addSection(settings.pop,'APPEARANCE',[document.querySelector('.appearance-controls'),document.querySelector('#running-indicator-settings')])",
		"const selfUpdateBranch=document.querySelector('#self-update-branch-settings')",
		"if(selfUpdateBranch)selfUpdateBranch.hidden=false",
		"addSection(settings.pop,'UPDATE',[selfUpdateBranch,document.querySelector('#self-update-check')])",
		"addSection(settings.pop,'WORKSPACE',[document.querySelector('#config-backup-settings'),document.querySelector('#tasks-json-editor'),document.querySelector('#reload'),document.querySelector('#edit-title')])",
		".git-status-pill",
		"pane-action-menus",
		"paneMenu(view,'Session'",
		"paneMenu(view,'Console'",
		".session-force-restart",
		"['SPLIT'",
		".session-split-vertical",
		".session-split-horizontal",
		".session-merge-vertical",
		".session-merge-horizontal",
		".session-swap-split",
		".session-unsplit",
		".console-search-btn",
		".copy-console",
		".taskmenu-menu-popover>.copy-console{margin-left:0;max-width:100%}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("menus.js missing grouped control behavior %q", want)
		}
	}
	if strings.Contains(js, "makeMenu('Workspace'") {
		t.Fatal("Workspace header menu must be removed")
	}
	if strings.Contains(js, "makeMenu('Terminal'") {
		t.Fatal("Terminal header action must open a terminal directly instead of a dropdown")
	}
	if !strings.Contains(indexHTML, `/featuremods/next.js`) {
		t.Fatal("index must load featuremods/next.js")
	}
}


func TestGroupedMenusDefersWorkspaceSettingsUntilTasksLoaded(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const projectWorkspace=String(app.taskData?.workspace||'').trim();",
		"if(!projectWorkspace)return false;",
		"vscode-tasks-menu:patch-ui-mode:'+projectWorkspace",
		"if(!installHeaderMenus()){",
		"window.addEventListener('taskmenu:tasks',()=>installHeaderMenus(),{once:true})",
	} {
		if !strings.Contains(js,want) {
			t.Fatalf("menus bootstrap guard missing %q",want)
		}
	}
	if strings.Contains(js,"app.taskData.workspace") {
		t.Fatal("menus.js must not dereference taskData.workspace before /api/tasks has loaded")
	}
}


func TestGroupedMenusPlaceGitBesideWorkspaceOnDesktop(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"if(app.layoutProfile==='mobile'){",
		"host.append(git)",
		"git.classList.add('header-left-git')",
		"workspace.after(git)",
		"margin-left:auto",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("menus.js missing desktop Git header placement %q", want)
		}
	}
}

func TestGroupedMenusUseCompactTextActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		".taskmenu-menu-trigger{white-space:nowrap;padding:2px 4px;border:0!important",
		"background:transparent!important;font-size:11px",
		".taskmenu-menu-popover>button{width:100%;text-align:left;border:0;background:transparent",
		".header-action-menus{display:flex;align-items:center;gap:1px;white-space:nowrap;margin-left:auto}",
		".header-left-git,.header-action-menus .git-status-pill{margin:0;border:0!important;background:transparent!important",
		".pane-action-menus{display:flex;align-items:center;gap:1px",
		".pane-action-menus .taskmenu-menu-trigger{padding:1px 4px;font-size:10px}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("menus.js missing compact text-action styling %q", want)
		}
	}
}

func TestGitPanelHasPermanentFilesMenuEntryForNestedRepositories(t *testing.T) {
	data,err:=webassets.Files.ReadFile("featuremods/menus.js")
	if err!=nil { t.Fatal(err) }
	js:=string(data)
	for _,want:=range []string{
		"gitRepositories.textContent='Git repositories…'",
		"gitRepositories.disabled=Boolean(app.sharedMode&&!app.hasPermission?.('git.status')&&!app.hasPermission?.('project.admin'))",
		"gitRepositories.onclick=()=>{closeAll();globalThis.TaskMenuGitFiles?.open?.();}",
		"addSection(files.pop,'GIT',[gitRepositories])",
	} {
		if !strings.Contains(js,want) { t.Errorf("permanent nested Git entry missing %q",want) }
	}
}

func TestHeaderBrowserMenuBetweenFilesAndTerminal(t *testing.T) {
    data,err:=webassets.Files.ReadFile("featuremods/menus.js")
    if err!=nil {t.Fatal(err)}
    js:=string(data)
    from:=strings.Index(js,"function installHeaderMenus(){")
    end:=strings.Index(js,"function paneMenu(")
    if from<0||end<=from {t.Fatal("header menu installer is missing")}
    header:=js[from:end]
    files:=strings.Index(header,"host.append(files.menu)")
    browser:=strings.Index(header,"host.append(browser.menu)")
    terminal:=strings.Index(header,"host.append(terminal)")
    if files<0||browser<=files||terminal<=browser {
        t.Fatalf("Browser header entry must be between Files and direct Terminal: files=%d browser=%d terminal=%d",files,browser,terminal)
    }
    for _,want:=range []string{
        "makeMenu('Browser'",
        "headerMenusByName.set('browser',browser.menu)",
        "openBrowser.textContent='Open browser…'",
        "browserAvailability.textContent='Check availability…'",
        "browserSettings.textContent='Browser settings…'",
        "browserGuide.textContent='Installation / setup guide…'",
        "TaskMenuSemanticBrowser?.checkAvailability?.()",
        "TaskMenuSemanticBrowser?.openSettings?.()",
        "TaskMenuSemanticBrowser?.openSetupGuide?.()",
    } {
        if !strings.Contains(header,want) {t.Errorf("Browser menu missing %q",want)}
    }
    if strings.Contains(header,"openBrowser.disabled=")||strings.Contains(header,"browser.menu.hidden="){
        t.Fatal("Browser menu and setup guide must remain discoverable before worker installation")
    }
    if !strings.Contains(header,"terminal.textContent='Terminal'"){
        t.Fatal("Terminal direct action must be preserved")
    }
}

func TestSemanticBrowserSettingsAndSetupAreAvailableWithoutWorker(t *testing.T) {
    data,err:=webassets.Files.ReadFile("featuremods/semantic_browser.js")
    if err!=nil {t.Fatal(err)}
    js:=string(data)
    for _,want:=range []string{
        "async function openBrowser()",
        "if(!status.enabled){openSetupGuide();return null;}",
        "function openSetupGuide()",
        "step3Tab:'auto'",
        "semantic-browser-install-tabs",
        "semantic-browser-installer-logs",
        "updateInstallerConsole(",
        "onlyLogsChanged",
        "Theo dõi log: Tắt",
        "selectInstallTab('manual')",
        "function openSettings()",
        "async function checkAvailability()",
        "/api/semantic-browser/setup",
        "Generate and save socket/token",
        "allowed_hosts:hosts.value.trim()",
        "node_supported",
        "worker_connected",
        "browser.use",
        "return {configured:null,permitted:false,enabled:false};",
        "if(!hasPermission){",
        "validateBrowserHome",
        "parsed.protocol!=='https:'",
        "browserPreferencesKey()",
        "autoRefresh",
        "if(!browserPreferences().autoRefresh)return;",
        "globalThis.TaskMenuSemanticBrowser={views,open:openBrowser,openSetupGuide,openSettings,checkAvailability,refreshAvailability,ready}",
    } {
        if !strings.Contains(js,want) {t.Errorf("Browser setup/settings integration missing %q",want)}
    }
    if strings.Contains(js,"document.querySelector('#open-terminal')?.after(openButton)"){
        t.Fatal("outdated Browser button must not be inserted after Terminal")
    }
}
