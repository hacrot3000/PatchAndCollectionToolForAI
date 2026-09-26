package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteSSHTerminalRejectsLocalCWDAndEnvironmentOverrides(t *testing.T) {
	s := &Server{Workspace: t.TempDir()}
	for _, body := range []string{
		`{"kind":"terminal","ssh_profile_id":"prod","cwd":"tools"}`,
		`{"kind":"terminal","ssh_profile_id":"prod","env":{"TASKDECK_SSH_ASKPASS_TOKEN":"attacker"}}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d want 400 response=%s", body, rr.Code, rr.Body.String())
		}
	}
}
