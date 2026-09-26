package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSharedAuthNeverFallsBackToLegacyCredentials(t *testing.T) {
	s := authTestServer()
	s.Config.SharedServerEnabled = true
	s.Config.SharedProjectID = "test-project"
	for _, path := range []string{"/", "/api/tasks", "/api/sessions", "/api/sessions/example/ws", "/api/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "192.0.2.50:41000"
		req.SetBasicAuth(s.Config.Username, s.Config.Password)
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusServiceUnavailable || rr.Header().Get("WWW-Authenticate") != "" {
			t.Fatalf("shared route %s accepted legacy credentials: status=%d", path, rr.Code)
		}
	}
}
