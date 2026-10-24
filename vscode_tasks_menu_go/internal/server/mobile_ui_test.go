package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestMobileUIProfileIsIsolatedFromDesktopPresentation(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/mobile.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"if(app.layoutProfile==='mobile')",
		"mobile-drawer-open",
		"mobile-actions-open",
		"mobile-terminal-keys",
		"--taskmenu-header-height:50px",
		"window.visualViewport",
		"env(safe-area-inset-top)",
		"env(safe-area-inset-bottom)",
		"['Ctrl+C','\\x03']",
		"['↑','\\x1b[A']",
		"['Enter','\\r']",
		"app.browserLeaseLost",
		".header-action-menus>.taskmenu-direct-action",
		"if(target?.closest('.taskmenu-direct-action'))setTimeout(closePanels,0)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("mobile.js missing %q", want)
		}
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/mobile.js';") {
		t.Fatal("next.js must load mobile.js")
	}
}

func TestCoreUIDetectsStableLayoutProfileOncePerPage(t *testing.T) {
	for _, want := range []string{
		"const layoutProfile=(matchMedia('(pointer: coarse)').matches||matchMedia('(max-width: 900px)').matches)?'mobile':'desktop';",
		"document.documentElement.dataset.taskmenuLayout=layoutProfile;",
		"get layoutProfile(){return layoutProfile;}",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("app JS missing layout-profile behavior %q", want)
		}
	}
}


func TestMobileHeaderActionsStayAboveBackdrop(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/mobile.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		`body>header{height:50px!important;min-height:50px!important;max-height:50px!important;padding:0 8px!important;gap:8px!important;z-index:1300`,
		`body.mobile-actions-open>header{z-index:1600}`,
		`.header-action-menus{display:none;position:fixed;z-index:1550`,
		`.mobile-backdrop{display:none;position:fixed;inset:0;z-index:1400`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("mobile stacking contract missing %q", want)
		}
	}
}

func TestMobileWorkspaceToolNavigationReachesDesktopCapabilities(t *testing.T) {
 read := func(name string) string {
  t.Helper()
  data,err:=webassets.Files.ReadFile("featuremods/"+name+".js")
  if err!=nil{t.Fatal(err)}
  return string(data)
 }
 mobile:=read("mobile")
 for _,name:=range []string{
  "mobile-workspace-tools",
  "globalThis.TaskMenuMobileTools={open:openMobileTool",
  "globalThis.TaskMenuPatchPanel?.open?.()",
  "globalThis.TaskMenuConnections?.open?.()",
  "globalThis.TaskMenuSchedules?.open?.('','overview')",
  "globalThis.TaskMenuSchedules?.openPanel?.()",
  "globalThis.TaskMenuExplorer?.open?.()",
  "globalThis.TaskMenuGitFiles?.open?.()",
  "globalThis.TaskMenuOperationCenter?.open?.()",
  "globalThis.TaskMenuWorkspaceSnapshots?.open?.()",
  "globalThis.TaskMenuProjectHealth?.open?.()",
  "globalThis.TaskMenuProjectProfiles?.open?.()",
  "globalThis.TaskMenuRemoteWorkspace?.open?.()",
  "globalThis.TaskMenuProjectSearch?.open?.()",
  "canAny('patch.view','patch.run')",
  "canAny('schedules.view','schedules.manage','schedules.run')",
  "button.hidden=!allowed()",
  "if(entry&&!entry[3]())throw new Error",
  "globalThis.TaskMenuConnections?.close?.()",
  "globalThis.TaskMenuExplorer?.close?.()",
  "globalThis.TaskMenuSchedules?.closePanel?.()",
  "globalThis.TaskMenuGitFiles?.close?.()",
  "menu?.prepend(toolsNav)",
  "body.mobile-drawer-open>main>aside{z-index:2050}",
  ".task-patch-panel-body",
  ".task-connections-panel",
  ".task-connection-form{grid-template-columns:minmax(0,1fr)}",
  ".schedule-nav",
  ".operation-center-row",
  ".workspace-snapshot-row",
 } {
  if !strings.Contains(mobile,name){t.Errorf("mobile feature unreachable or not touch-responsive: %q",name)}
 }
 for _,name:=range []string{"patchpanel","connections"} {
  js:=read(name)
  if strings.Contains(js,"app.layoutProfile==='mobile'||!app.taskData?.workspace") {
   t.Errorf("%s is still disabled on mobile",name)
  }
 }
 // The desktop sidebar is intentionally not installed on small screens:
 // mobile.js provides its own equivalent navigation rather than a second rail.
 if !strings.Contains(read("activitybar"),"app.layoutProfile==='mobile'") {
  t.Fatal("mobile nav must not also install desktop Activity Bar")
 }
}

func TestMobileTabContextCanBeOpenedWithoutRightClick(t *testing.T) {
 data,err:=webassets.Files.ReadFile("featuremods/tabcontext.js")
 if err!=nil{t.Fatal(err)}
 js:=string(data)
 for _,part:=range []string{
  "function decorateMobileTabActions(descriptor)",
  "button.className='mobile-tab-actions'",
  "button.setAttribute('aria-haspopup','menu')",
  "openContextMenu(current.view,rect.left,rect.bottom+4)",
  "openEditorContextMenu(current.view,rect.left,rect.bottom+4)",
  "openFeatureContextMenu(current,rect.left,rect.bottom+4)",
  "decorateMobileTabActions(descriptor)",
 } {
  if !strings.Contains(js,part){t.Errorf("mobile tab context affordance missing: %q",part)}
 }
}
