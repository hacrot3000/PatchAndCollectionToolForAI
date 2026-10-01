package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestSelfUpdateBranchSettingsUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/selfupdatesettings.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/config/self-update",
		"/api/config/self-update?action=branches",
		"self-update-branch-settings",
		"self-update-branch",
		"Self-update branch",
		"method:'PUT'",
		"JSON.stringify({branch,run_full_validation_tests:runFullValidationTests})",
		"self-update-full-validation",
		"Run full validation tests before self-update",
		"Current: '+currentBranch",
		"Check GitHub branch '+currentBranch",
		"if(a==='main')return -1",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("selfupdatesettings.js missing %q", want)
		}
	}
	if strings.Contains(js, "localStorage") {
		t.Fatal("self-update branch must be stored in project config, not localStorage")
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/selfupdatesettings.js';") {
		t.Fatal("next.js must load selfupdatesettings.js")
	}
}

func TestSelfUpdateBranchControlLivesInUpdateSettingsSection(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const selfUpdateBranch=document.querySelector('#self-update-branch-settings')",
		"if(selfUpdateBranch)selfUpdateBranch.hidden=false",
		"addSection(settings.pop,'UPDATE',[selfUpdateBranch,document.querySelector('#self-update-check')])",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("menus.js missing self-update branch setting placement %q", want)
		}
	}
}
