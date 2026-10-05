package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestAuthenticationModeWizardUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/authmode.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/config/auth-mode",
		"Authentication migration wizard",
		"1 · Mode",
		"2 · Setup",
		"3 · Review",
		"Multi-user shared-server",
		"Single authentication",
		"legacy_credential_reusable",
		"resolved_identity_db",
		"Current single-auth credential",
		"all browser sessions for this project will be revoked",
		"shared password hashes are never decrypted or exported",
		"method:'POST'",
		"target_mode:selectedMode",
		"project_id=saved.project_id",
		"identity_db=saved.identity_db||''",
		"setTimeout(()=>window.location.assign(target),1800)",
		"saved.password='';saved.confirm=''",
		"let setup={}",
		"setup.password='';setup.confirm=''",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("authmode.js missing %q", want)
		}
	}
	if strings.Contains(js, "__taskdeckAuthModeSetup") || strings.Contains(js, "status.password") || strings.Contains(js, "current_password") {
		t.Fatal("authentication mode status UI must never request or display the current plaintext password")
	}
}

func TestAuthenticationModeWizardLoadsBeforeSettingsMenu(t *testing.T) {
	nextData, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	next := string(nextData)
	auth := strings.Index(next, "import '/featuremods/authmode.js';")
	menus := strings.Index(next, "import '/featuremods/menus.js';")
	if auth < 0 || menus < 0 || auth > menus {
		t.Fatalf("auth mode wizard must load before settings menu: auth=%d menus=%d", auth, menus)
	}

	menuData, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil {
		t.Fatal(err)
	}
	menu := string(menuData)
	for _, want := range []string{
		"document.querySelector('#auth-mode-settings')",
		"!app.sharedMode||app.hasPermission('project.admin')",
		"addSection(settings.pop,'SECURITY',[authModeSettings])",
	} {
		if !strings.Contains(menu, want) {
			t.Fatalf("settings menu missing auth migration integration %q", want)
		}
	}
}
