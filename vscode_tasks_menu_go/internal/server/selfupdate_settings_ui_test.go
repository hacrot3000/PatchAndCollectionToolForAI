package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestSelfUpdateValidationSettingsUIUsesProjectConfig(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/selfupdatesettings.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/config/self-update",
		"method:'PUT'",
		"run_full_validation_tests",
		"self-update-validation-settings",
		"self-update-full-validation",
		"Run full validation tests before self-update",
		"Developer mode · slower updates",
		"checkbox.checked=value",
		"control.hidden=true",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("selfupdatesettings.js missing %q", want)
		}
	}
	if strings.Contains(js, "localStorage") {
		t.Fatal("self-update validation setting must be persisted in project config, not localStorage")
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/selfupdatesettings.js';") {
		t.Fatal("next.js must load selfupdatesettings.js")
	}
}

func TestSelfUpdateControlsHaveDedicatedSettingsSection(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const selfUpdateValidation=document.querySelector('#self-update-validation-settings')",
		"if(selfUpdateValidation)selfUpdateValidation.hidden=false",
		"addSection(settings.pop,'UPDATE',[selfUpdateValidation,document.querySelector('#self-update-check')])",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("menus.js missing UPDATE settings section behavior %q", want)
		}
	}
}
