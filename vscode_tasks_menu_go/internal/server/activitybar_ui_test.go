package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestActivityBarIsAdditiveAndFailureIsolated(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function installActivityBar()",
		"if(!app||app.layoutProfile==='mobile'||!app.taskData?.workspace)return false",
		"window.addEventListener('taskmenu:tasks',safeInstallActivityBar,{once:true})",
		"console.warn('Auto sidebar enhancement disabled:'",
		"sidebar-auto-hide:",
		"sidebar-width:",
		"makeButton('tasks','Tasks'",
		"makeButton('explorer','Explorer'",
		"makeButton('patch','Patch Tool'",
		"makeButton('history','History'",
		"task-history-panel",
		"adoptHistoryFromTasks",
		"restoreHistoryToTasks",
		"menu.querySelector('.history-section')",
		"appearance.append(toggle)",
		"TaskMenuExplorer?.open()",
		"TaskMenuExplorer?.close()",
		"TaskMenuPatchPanel?.deactivate()",
		"document.addEventListener('pointerdown'",
		"rail.contains(target)",
		"const headerMenus=document.querySelector('.header-action-menus')",
		"if(headerMenus?.contains(target))return",
		"activeView==='tasks'&&menu.contains(target)",
		"activeView==='explorer'&&explorerPanel?.contains(target)",
		"activeView==='patch'&&globalThis.TaskMenuPatchPanel?.panel?.contains(target)",
		"activeView==='history'&&historyPanel.contains(target)",
		"},true)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("activitybar.js missing %q", want)
		}
	}
}

func TestActivityBarLoadsAfterAllExistingFeatureModules(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	activity := strings.LastIndex(js, "activitybar.js")
	for _, existing := range []string{
		"menus.js",
		"broadcast.js",
		"commandpresets.js",
		"tabcontext.js",
		"mobile.js",
		"viewportfix.js",
		"patchpanel.js",
	} {
		pos := strings.Index(js, existing)
		if pos < 0 || activity < 0 || pos > activity {
			t.Fatalf("activity bar must load after existing module %q", existing)
		}
	}
}


func TestActivityBarDoesNotClosePatchTabWhenSwitchingViews(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function deactivatePatchPanel()",
		"globalThis.TaskMenuPatchPanel?.deactivate()",
		"if(previous==='patch')deactivatePatchPanel()",
		"showTasks()",
		"showExplorer()",
		"showHistory()",
	} {
		if !strings.Contains(js,want) {
			t.Fatalf("activity bar Patch tab persistence missing %q",want)
		}
	}
	if strings.Contains(js,"TaskMenuPatchPanel?.close()") {
		t.Fatal("activity bar must not hard-close the native Patch tab when switching views")
	}
}

func TestAlwaysVisibleSidebarSwitchKeepsPatchWorkspaceVisible(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function showTasks(){",
		"function showExplorer(){",
		"function showHistory(){",
		"if(enabled())deactivatePatchPanel()",
	} {
		if !strings.Contains(js,want) {
			t.Fatalf("always-visible Patch persistence missing %q",want)
		}
	}
	for _, name := range []string{"showTasks","showExplorer","showHistory"} {
		start:=strings.Index(js,"function "+name+"(){")
		if start<0 { t.Fatalf("%s unavailable",name) }
		endRel:=strings.Index(js[start:],"\n  function ")
		if endRel<0 { endRel=len(js)-start }
		block:=js[start:start+endRel]
		if strings.Contains(block,"\n    deactivatePatchPanel();") {
			t.Fatalf("%s must not unconditionally deactivate Patch in Always visible mode",name)
		}
		if !strings.Contains(block,"if(enabled())deactivatePatchPanel()") {
			t.Fatalf("%s must deactivate Patch only in Auto-hide mode",name)
		}
	}
}


func TestActivityBarPatchIconIsIdempotentWorkspaceNavigation(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"if(activeView===view){",
		"if(view==='patch'){showPatch();return;}",
		"Patch Tool is a workspace tab with its own close button",
		"if(!enabled()){",
		"if(view==='tasks')showTasks()",
		"if(view==='explorer')showExplorer()",
		"if(view==='history')showHistory()",
		"globalThis.TaskMenuPatchPanel?.open()",
	} {
		if !strings.Contains(js,want) {
			t.Fatalf("Patch Activity Bar idempotent navigation missing %q",want)
		}
	}
	blockStart:=strings.Index(js,"function activate(view){")
	blockEndRel:=strings.Index(js[blockStart:],"tasksButton.onclick")
	if blockStart<0||blockEndRel<0 { t.Fatal("activitybar activate() bounds unavailable") }
	block:=js[blockStart:blockStart+blockEndRel]
	if strings.Contains(block,"if(activeView===view){closeActive();return;}") {
		t.Fatal("Patch Activity Bar icon must not use the generic toggle-close path")
	}
}


func TestActivityBarAlwaysVisibleUsesHorizontalNavigationRow(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"body:not(.task-sidebar-auto-hide) .task-activity-bar{display:flex;position:fixed;top:52px;left:0",
		"width:var(--taskmenu-sidebar-inline-width,310px)",
		"height:46px",
		"flex-direction:row",
		"body:not(.task-sidebar-auto-hide) #menu{padding-top:56px}",
		"body.task-sidebar-auto-hide .task-activity-bar{display:flex}",
		"flex-direction:column",
	} {
		if !strings.Contains(js,want) {
			t.Fatalf("always-visible horizontal sidebar navigation missing %q",want)
		}
	}
}

func TestActivityBarAlwaysVisibleViewsStayInFixedSidebar(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"body:not(.task-sidebar-auto-hide) .project-explorer{top:98px!important;left:0!important",
		"body:not(.task-sidebar-auto-hide) .task-history-panel{top:98px;left:0",
		"activeView=on?'':'tasks'",
		"restoreHistoryToTasks()",
		"const active=button.dataset.view===activeView",
		"if(!enabled()){",
		"showTasks()",
		"if(!enabled())restoreHistoryToTasks()",
	} {
		if !strings.Contains(js,want) {
			t.Fatalf("always-visible fixed sidebar view missing %q",want)
		}
	}
}

func TestActivityBarFixedSidebarWidthTracksResizableMenu(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function syncInlineSidebarWidth()",
		"menu.getBoundingClientRect().width",
		"new ResizeObserver(syncInlineSidebarWidth)",
		"sidebarResizeObserver.observe(menu)",
		"document.documentElement.style.setProperty('--taskmenu-sidebar-inline-width'",
		"document.documentElement.style.setProperty('--taskmenu-sidebar-panel-width'",
	} {
		if !strings.Contains(js,want) {
			t.Fatalf("fixed sidebar width synchronization missing %q",want)
		}
	}
}
