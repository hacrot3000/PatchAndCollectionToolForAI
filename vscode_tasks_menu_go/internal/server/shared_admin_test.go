package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSharedAdminReadAPIsNeverExposeCredentialHashes(t *testing.T) {
	s := sharedLoginTestServer(t)
	cookie := sharedAPILogin(t, s, "alice")
	for _, path := range []string{
		"/api/admin/users",
		"/api/admin/roles",
		"/api/admin/permissions",
		"/api/admin/sessions",
		"/api/admin/audit",
	} {
		response := sharedRequest(t, s, path, cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
		body := response.Body.String()
		if strings.Contains(body, "password_hash") || strings.Contains(body, "token_hash") || strings.Contains(body, cookie.Value) {
			t.Fatalf("%s exposed credential material: %s", path, body)
		}
	}
}

func TestSharedAdminAPIsStayUnavailableInLegacyMode(t *testing.T) {
	s := authTestServer()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	request.SetBasicAuth(s.Config.Username, s.Config.Password)
	recorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("legacy admin status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
