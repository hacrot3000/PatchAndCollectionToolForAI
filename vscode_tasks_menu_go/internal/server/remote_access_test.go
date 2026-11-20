package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestRemoteRouteRequiresProjectAdmin(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req := httptest.NewRequest(method, "/api/config/remote-access", nil)
		permissions := sharedRoutePermissions(req)
		if len(permissions) != 1 || permissions[0] != identity.PermissionProjectAdmin {
			t.Fatalf("%s remote config permission = %v", method, permissions)
		}
	}
}

func TestDirectSharedRemoteHandlerFailsWithoutPrincipal(t *testing.T) {
	s := &Server{Config: config.Config{SharedServerEnabled: true}, Workspace: "/tmp/project"}
	req := httptest.NewRequest(http.MethodGet, "/api/config/remote-access", nil)
	resp := httptest.NewRecorder()
	s.remoteAccessConfig(resp, req)
	if resp.Code == http.StatusOK {
		t.Fatal("untrusted request was allowed to read remote configuration")
	}
}

func TestRemoteEnrollmentDisabledByDefault(t *testing.T) {
	c := config.Default()
	c.AuthEnabled = true
	c.Password = "test-not-default"
	s := &Server{Config:c, Workspace: "/tmp/project"}
	req := httptest.NewRequest(http.MethodPost, "/api/config/remote-access",
		bytes.NewBufferString(`{"action":"enroll","name":"Demo"}`))
	resp := httptest.NewRecorder()
	s.remoteAccessConfig(resp, req)
	if resp.Code != http.StatusConflict {
		t.Fatalf("remote enrollment should be disabled by default, got %d", resp.Code)
	}
}
