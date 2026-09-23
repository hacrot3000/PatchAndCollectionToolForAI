package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestPatchPanelUsesBuiltinSessionAPI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function installPatchPanel()",
		"task-patch-panel",
		"['queue','Queue'",
		"['resume','Resume'",
		"['history','History'",
		"['plan','Plan'",
		"JSON.stringify({kind:'patch',patch_mode:mode})",
		"app.attachSession(meta,true)",
		"TaskMenuPatchPanel={open,close,toggle,start",
		"Patch panel enhancement disabled:",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("patchpanel.js missing %q", want)
		}
	}
	if strings.Contains(js, "run_python_patches.sh") {
		t.Fatal("Patch panel must use the built-in session API, not a project launcher path")
	}
}

func TestPatchPanelLoadsBeforeActivityBar(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	panel := strings.Index(js, "patchpanel.js")
	activity := strings.Index(js, "activitybar.js")
	if panel < 0 || activity < 0 || panel > activity {
		t.Fatal("Patch panel must initialize before the Activity Bar binds its Patch view")
	}
}
