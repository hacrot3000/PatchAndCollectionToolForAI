package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedAdminUIRequiresAdministrationCapability(t *testing.T) {
	cfg := config.Default()
	cfg.SharedServerEnabled = true
	s := &Server{Config: cfg}
	allowed := identity.Principal{Permissions: map[string]bool{identity.PermissionUsersView: true}}
	request := httptest.NewRequest(http.MethodGet, "/admin/access", nil)
	request = request.WithContext(context.WithValue(request.Context(), sharedPrincipalContextKey{}, allowed))
	recorder := httptest.NewRecorder()
	s.sharedAdminUI(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("allowed admin UI status=%d type=%q", recorder.Code, recorder.Header().Get("Content-Type"))
	}

	denied := httptest.NewRequest(http.MethodGet, "/admin/access", nil)
	denied = denied.WithContext(context.WithValue(denied.Context(), sharedPrincipalContextKey{}, identity.Principal{}))
	deniedRecorder := httptest.NewRecorder()
	s.sharedAdminUI(deniedRecorder, denied)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied admin UI status=%d", deniedRecorder.Code)
	}
}

func TestSharedAdminUIIsAbsentInLegacyMode(t *testing.T) {
	s := &Server{Config: config.Default()}
	request := httptest.NewRequest(http.MethodGet, "/admin/access", nil)
	recorder := httptest.NewRecorder()
	s.sharedAdminUI(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("legacy admin UI status=%d", recorder.Code)
	}
}

func TestSharedAdminUIUsersScriptUsesPermissionGatedAPIs(t *testing.T) {
	for _, path := range []string{
		"/api/admin/users",
		"/api/admin/roles",
		"/api/admin/users/access",
		"/api/admin/permissions",
		"/api/admin/users/permission",
		"/api/admin/sessions",
		"/api/admin/audit",
	} {
		if !strings.Contains(sharedAdminJS, path) {
			t.Fatalf("admin UI script missing %s", path)
		}
	}
	if strings.Contains(sharedAdminJS, "innerHTML") {
		t.Fatal("admin UI should build user-controlled content with DOM text nodes")
	}
	if strings.Contains(sharedAdminJS, "npm") {
		t.Fatal("admin UI unexpectedly references npm")
	}
}

func TestSharedAdminUIIncludesRoleAdministrationView(t *testing.T) {
	if !strings.Contains(sharedAdminHTML, `data-view="roles"`) {
		t.Fatal("admin UI missing Roles navigation")
	}
	for _, fragment := range []string{"renderRoles", "Create custom role", "Save permissions", "System roles are read-only"} {
		if !strings.Contains(sharedAdminJS, fragment) {
			t.Fatalf("admin role UI missing %q", fragment)
		}
	}
}
