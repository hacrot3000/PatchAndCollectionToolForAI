package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
)

func authTestServer() *Server {
	cfg := config.Default()
	cfg.AuthEnabled = true
	cfg.Username = "admin"
	cfg.Password = "correct-horse-battery-staple"
	return &Server{Config: cfg}
}

func TestRemoteHealthRequiresAuthentication(t *testing.T) {
	s := authTestServer()
	h := s.Handler()

	remote := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	remote.RemoteAddr = "192.0.2.50:41000"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, remote)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("remote health without auth status=%d body=%s", rr.Code, rr.Body.String())
	}

	authorized := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	authorized.RemoteAddr = "192.0.2.50:41001"
	authorized.SetBasicAuth("admin", "correct-horse-battery-staple")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, authorized)
	if rr.Code != http.StatusOK {
		t.Fatalf("remote health with auth status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLoopbackHealthRemainsAvailableWithoutAuthentication(t *testing.T) {
	s := authTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.RemoteAddr = "127.0.0.1:41000"
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("loopback health status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSensitiveRoutesRejectRemoteRequestsWithoutAuthentication(t *testing.T) {
	s := authTestServer()
	h := s.Handler()
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/"},
		{http.MethodGet, "/api/tasks"},
		{http.MethodGet, "/api/state/tasks"},
		{http.MethodGet, "/api/config/page-title"},
		{http.MethodGet, "/api/broadcast"},
		{http.MethodGet, "/api/command-presets"},
		{http.MethodGet, "/api/git/status"},
		{http.MethodGet, "/api/files/download?path=x"},
		{http.MethodGet, "/api/project/tree?path=."},
		{http.MethodGet, "/api/project/file?path=x"},
		{http.MethodGet, "/api/project/files/search?q=x"},
		{http.MethodGet, "/api/project/content/search?q=x"},
		{http.MethodGet, "/api/sessions"},
		{http.MethodGet, "/api/sessions/example/ws"},
		{http.MethodPost, "/api/files/upload"},
		{http.MethodPost, "/api/sessions"},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.RemoteAddr = "198.51.100.77:42000"
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d want 401 body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestBasicAuthBlocksRepeatedFailuresByRemoteIP(t *testing.T) {
	s := authTestServer()
	h := s.Handler()
	for i := 0; i < authFailureLimit; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.9:43000"
		req.SetBasicAuth("admin", "wrong")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status=%d want 401", i+1, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:43001"
	req.SetBasicAuth("admin", "correct-horse-battery-staple")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("blocked client status=%d want 429 body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("rate-limited response missing Retry-After")
	}
}

func TestSuccessfulAuthenticationClearsFailureState(t *testing.T) {
	s := authTestServer()
	h := s.Handler()
	for i := 0; i < authFailureLimit-1; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.10:44000"
		req.SetBasicAuth("admin", "wrong")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status=%d want 401", i+1, rr.Code)
		}
	}
	okReq := httptest.NewRequest(http.MethodGet, "/", nil)
	okReq.RemoteAddr = "203.0.113.10:44001"
	okReq.SetBasicAuth("admin", "correct-horse-battery-staple")
	okRR := httptest.NewRecorder()
	h.ServeHTTP(okRR, okReq)
	if okRR.Code == http.StatusUnauthorized || okRR.Code == http.StatusTooManyRequests {
		t.Fatalf("valid auth unexpectedly rejected status=%d", okRR.Code)
	}

	badAgain := httptest.NewRequest(http.MethodGet, "/", nil)
	badAgain.RemoteAddr = "203.0.113.10:44002"
	badAgain.SetBasicAuth("admin", "wrong")
	badRR := httptest.NewRecorder()
	h.ServeHTTP(badRR, badAgain)
	if badRR.Code != http.StatusUnauthorized {
		t.Fatalf("failure after successful auth status=%d want 401", badRR.Code)
	}
}

func TestCredentialComparisonDoesNotDependOnLength(t *testing.T) {
	if !constantTimeCredentialEqual("same", "same") {
		t.Fatal("equal credentials must match")
	}
	for _, tc := range [][2]string{{"short", "much-longer"}, {"much-longer", "short"}, {"same", "diff"}} {
		if constantTimeCredentialEqual(tc[0], tc[1]) {
			t.Fatalf("unexpected credential match for %q and %q", tc[0], tc[1])
		}
	}
}
