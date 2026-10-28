package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestRemoteSettingsMenuIsVisibleOnlyToAdmin(t *testing.T) {
	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	ui, err := webassets.Files.ReadFile("featuremods/remoteaccess.js")
	if err != nil {
		t.Fatal(err)
	}
	menus, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil {
		t.Fatal(err)
	}
	for label, source := range map[string]string{
		"next": string(next), "UI": string(ui), "menus": string(menus),
	} {
		if !strings.Contains(source, "remoteaccess.js") && label == "next" {
			t.Fatal("Remote Access module is not loaded")
		}
		if !strings.Contains(source, "remote-access-settings") && label != "next" {
			t.Fatalf("%s missing Remote Access menu entry", label)
		}
	}
	if !strings.Contains(string(menus), "only(true,remoteSettings)") ||
		!strings.Contains(string(menus), "only(false,remoteSettings)") ||
		!strings.Contains(string(menus), "has('project.admin')") {
		t.Fatal("Remote Access settings are not gated to project admin")
	}
	if !strings.Contains(string(ui), "method:'PUT'") ||
		!strings.Contains(string(ui), "method:'POST'") ||
		!strings.Contains(string(ui), "Remote control is not implemented yet") {
		t.Fatal("Remote UI missing state/save/pairing behavior or security warning")
	}
}

func TestRemoteSettingsRouteAuthorizationByMethod(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		request := httptest.NewRequest(method, "/api/config/remote-access", nil)
		perms := sharedRoutePermissions(request)
		if len(perms) != 1 || perms[0] != identity.PermissionProjectAdmin {
			t.Fatalf("remote settings %s must require project.admin: %v", method, perms)
		}
	}
}
