package server

import (
	"context"
	"net/http"
	"net/http/httptest"
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
