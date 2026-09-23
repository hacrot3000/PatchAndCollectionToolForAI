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
		"document.addEventListener('pointerdown'",
		"rail.contains(target)",
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
