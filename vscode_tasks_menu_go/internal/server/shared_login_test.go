package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/session"
)

func sharedLoginTestServer(t *testing.T) *Server {
	t.Helper()
	s := authTestServer()
	s.Workspace = t.TempDir()
	if err := os.MkdirAll(filepath.Join(s.Workspace, ".vscode"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Workspace, ".vscode", "tasks.json"), []byte(`{"version":"2.0.0","tasks":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s.Config.SharedServerEnabled = true
	s.Config.SharedProjectID = "test-project"
	store, err := identity.OpenSQLiteStore(context.Background(), filepath.Join(t.TempDir(), "identity", "identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.Identity = store
	sessions := session.NewManager(64 << 10)
	s.Sessions = sessions
	t.Cleanup(func() { sessions.Shutdown(2 * time.Second) })
	hash, err := identity.HashPassword(context.Background(), "private-admin-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BootstrapFirstAdmin(context.Background(), s.Config.SharedProjectID, "alice", hash, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.validateSharedIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func loginRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "https://taskdeck.test/api/auth/login", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestSharedLoginCookieRotationAndLogout(t *testing.T) {
	s := sharedLoginTestServer(t)
	var previous *http.Cookie
	for attempt := 0; attempt < 2; attempt++ {
		r := loginRequest(`{"username":"alice","password":"private-admin-password"}`)
		if previous != nil {
			r.AddCookie(previous)
		}
		w := httptest.NewRecorder()
		s.sharedLogin(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("login: %d %s", w.Code, w.Body.String())
		}
		cookie := w.Result().Cookies()[0]
		if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" {
			t.Fatalf("unsafe cookie: %+v", cookie)
		}
		if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), cookie.Value) {
			t.Fatal("credentials disclosed")
		}
		if previous != nil {
			if previous.Value == cookie.Value {
				t.Fatal("token not rotated")
			}
			if _, _, err := identity.AuthenticateBrowserSession(context.Background(), s.Identity, s.Config.SharedProjectID, previous.Value, time.Now()); err == nil {
				t.Fatal("previous token still valid")
			}
		}
		previous = cookie
	}
	r := httptest.NewRequest(http.MethodPost, "https://taskdeck.test/api/auth/logout", nil)
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(previous)
	w := httptest.NewRecorder()
	s.sharedLogout(w, r)
	if w.Code != http.StatusNoContent || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatalf("logout: %d", w.Code)
	}
	if _, _, err := identity.AuthenticateBrowserSession(context.Background(), s.Identity, s.Config.SharedProjectID, previous.Value, time.Now()); err == nil {
		t.Fatal("logout failed to revoke server session")
	}
}

func TestSharedLoginGenericFailureAndRateLimit(t *testing.T) {
	s := sharedLoginTestServer(t)
	for i := 0; i < authFailureLimit; i++ {
		username := "alice"
		if i%2 == 0 {
			username = "missing"
		}
		body, _ := json.Marshal(map[string]string{"username": username, "password": "wrong-password"})
		w := httptest.NewRecorder()
		s.sharedLogin(w, loginRequest(string(body)))
		if w.Code != http.StatusUnauthorized || strings.TrimSpace(w.Body.String()) != "invalid username or password" {
			t.Fatalf("attempt %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	s.sharedLogin(w, loginRequest(`{"username":"alice","password":"private-admin-password"}`))
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit: %d", w.Code)
	}
}

func TestSharedLoginBoundsConcurrentScryptAndJSON(t *testing.T) {
	s := &Server{}
	if !s.beginSharedLogin() || !s.beginSharedLogin() || s.beginSharedLogin() {
		t.Fatal("unbounded verifier concurrency")
	}
	s.endSharedLogin()
	s.endSharedLogin()
	for _, body := range []string{`{}`, `{"username":"a","password":"b"} {}`, `{"username":"a","password":"b","extra":true}`, strings.Repeat("x", 33<<10)} {
		w := httptest.NewRecorder()
		s.sharedLogin(w, loginRequest(body))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid JSON: %d", w.Code)
		}
	}
}
