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


func TestActivityBarPatchIconIsIdempotentWorkspaceNavigation(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"if(activeView===view){",
		"if(view==='patch'){showPatch();return;}",
		"Patch Tool is a workspace tab with its own close button",
		"if(view==='patch'){",
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
