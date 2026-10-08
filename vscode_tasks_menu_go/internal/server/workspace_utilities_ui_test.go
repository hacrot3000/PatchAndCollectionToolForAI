package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func readWorkspaceUtilitiesAsset(t *testing.T, filename string) string {
	t.Helper()
	data, err := webassets.Files.ReadFile("featuremods/" + filename)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWorkspaceUtilitiesOneLauncherAndOrderedPopup(t *testing.T) {
	js := readWorkspaceUtilitiesAsset(t, "workspaceutilities.js")
	for _, want := range []string{
		"function installWorkspaceUtilitiesLauncher()",
		"if(rail.querySelector('[data-view=\"workspace-utilities\"]'))return true;",
		"for(const legacy of ['snapshots','operations','health'])",
		"launcher.dataset.view='workspace-utilities'",
		"launcher.setAttribute('aria-haspopup','menu')",
		"launcher.setAttribute('aria-expanded','false')",
		"menu.setAttribute('role','menu')",
		"item.setAttribute('role','menuitem')",
		"rail.append(launcher)",
		"document.body.append(menu)",
		"globalThis.TaskMenuWorkspaceUtilities={launcher,menu,open:openMenu,close:closeMenu",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("grouped workspace utility launcher missing %q", want)
		}
	}
	snapshots := strings.Index(js, "name:'Workspace snapshots'")
	operations := strings.Index(js, "name:'OPERATION CENTER'")
	health := strings.Index(js, "name:'PROJECT HEALTH'")
	if snapshots < 0 || operations <= snapshots || health <= operations {
		t.Fatalf("workspace tool menu order is wrong: snapshots=%d operations=%d health=%d", snapshots, operations, health)
	}
	for _, target := range []string{
		"TaskMenuWorkspaceSnapshots?.open?.()",
		"TaskMenuOperationCenter?.open?.()",
		"TaskMenuProjectHealth?.open?.()",
	} {
		if !strings.Contains(js, target) {
			t.Fatalf("grouped launcher lost existing dialog entrypoint %q", target)
		}
	}
}

func TestWorkspaceUtilitiesMenuKeyboardAndResponsivePlacement(t *testing.T) {
	js := readWorkspaceUtilitiesAsset(t, "workspaceutilities.js")
	for _, want := range []string{
		"launcher.setAttribute('aria-controls',menu.id)",
		"function positionMenu()",
		"document.body.classList.contains('task-sidebar-auto-hide')",
		"window.innerWidth-width-8",
		"window.innerHeight-height-8",
		"function closeMenu({focus=false}={})",
		"if(event.key==='Escape')",
		"if(event.key==='Tab')",
		"event.key==='ArrowDown'",
		"event.key==='ArrowUp'",
		"event.key==='Home'",
		"event.key==='End'",
		"document.addEventListener('pointerdown'",
		"window.addEventListener('resize'",
		"window.addEventListener('scroll'",
		"window.addEventListener('taskmenu:tasks'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("workspace utilities popover missing keyboard/position contract %q", want)
		}
	}
}

func TestWorkspaceUtilityDialogsKeepPublicAPIsAndHaveNoStandaloneLaunchers(t *testing.T) {
	for _, entry := range []struct{file, api string}{
		{"workspacesnapshots.js", "globalThis.TaskMenuWorkspaceSnapshots={open:openDialog"},
		{"operationcenter.js", "globalThis.TaskMenuOperationCenter={open,close,refresh,begin"},
		{"projecthealth.js", "globalThis.TaskMenuProjectHealth={open,close,refresh,collect"},
	} {
		js := readWorkspaceUtilitiesAsset(t, entry.file)
		if !strings.Contains(js, entry.api) {
			t.Errorf("%s no longer exports %q", entry.file, entry.api)
		}
		for _, forbidden := range []string{
			"function installLauncher()",
			"data-view=\"snapshots\"",
			"data-view=\"operations\"",
			"data-view=\"health\"",
		} {
			if strings.Contains(js, forbidden) {
				t.Errorf("%s still creates standalone activity launcher %q", entry.file, forbidden)
			}
		}
	}
	snapshots := readWorkspaceUtilitiesAsset(t, "workspacesnapshots.js")
	if !strings.Contains(snapshots, "event.key.toLowerCase()==='s'") {
		t.Fatal("Workspace snapshots Ctrl+Alt+S keyboard shortcut was lost")
	}
}

func TestWorkspaceUtilitiesLoadAfterAllDialogsAndFitNarrowSidebar(t *testing.T) {
	js := readWorkspaceUtilitiesAsset(t, "next.js")
	workspaceTools := strings.Index(js, "featuremods/workspaceutilities.js")
	if workspaceTools < 0 {
		t.Fatal("workspaceutilities.js not loaded")
	}
	for _, name := range []string{
		"featuremods/activitybar.js",
		"featuremods/operationcenter.js",
		"featuremods/workspacesnapshots.js",
		"featuremods/projecthealth.js",
	} {
		if at := strings.Index(js, name); at < 0 || at > workspaceTools {
			t.Fatalf("unified workspace tools must load after %s", name)
		}
	}
	activity := readWorkspaceUtilitiesAsset(t, "activitybar.js")
	for _, want := range []string{
		"body:not(.task-sidebar-auto-hide) .task-activity-bar>.task-activity-button{flex:0 1 38px;min-width:29px}",
		"Math.max(220,Math.min(650",
	} {
		if !strings.Contains(activity,want) {
			t.Fatalf("sidebar icons may overflow at narrow width: %q", want)
		}
	}
}
