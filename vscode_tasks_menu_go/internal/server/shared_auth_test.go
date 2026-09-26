package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
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

func sharedRequest(t *testing.T, s *Server, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "https://taskdeck.test"+path, nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func sharedAPILogin(t *testing.T, s *Server, username string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": "private-admin-password"})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, loginRequest(string(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("login API: %d %s", w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}

func TestSharedHTTPConcurrentUsersAndMembershipIsolation(t *testing.T) {
	s := sharedLoginTestServer(t)
	ctx := context.Background()
	alice, err := s.Identity.UserByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	project, err := s.Identity.ProjectByKey(ctx, s.Config.SharedProjectID)
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.Identity.ProjectMember(ctx, project.ID, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := s.Identity.CreateUser(ctx, identity.User{ID: "bob", Username: "bob", PasswordHash: alice.PasswordHash, Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	member.UserID = "bob"
	if err := s.Identity.UpsertProjectMember(ctx, member); err != nil {
		t.Fatal(err)
	}
	// Auth must not be gated by the pre-existing single-browser controller.
	if _, err := s.browserLeaseState().acquire(); err != nil {
		t.Fatal(err)
	}
	a := sharedAPILogin(t, s, "alice")
	b := sharedAPILogin(t, s, "bob")
	for _, cookie := range []*http.Cookie{a, b} {
		if w := sharedRequest(t, s, "/api/auth/me", cookie); w.Code != http.StatusOK {
			t.Fatalf("concurrent user rejected: %d %s", w.Code, w.Body.String())
		}
		if w := sharedRequest(t, s, "/api/tasks", cookie); w.Code != http.StatusOK {
			t.Fatalf("authorized task list rejected: %d %s", w.Code, w.Body.String())
		}
		if w := sharedRequest(t, s, "/api/sessions", cookie); w.Code != http.StatusOK {
			t.Fatalf("authorized session list rejected: %d %s", w.Code, w.Body.String())
		}
		if w := sharedRequest(t, s, "/api/state/tasks", cookie); w.Code != http.StatusOK {
			t.Fatalf("authorized task state rejected: %d %s", w.Code, w.Body.String())
		}
		if w := sharedRequest(t, s, "/api/sessions/x/ws", cookie); w.Code != http.StatusForbidden {
			t.Fatalf("unmapped session item route: %d", w.Code)
		}
	}
	member.Enabled = false
	if err := s.Identity.UpsertProjectMember(ctx, member); err != nil {
		t.Fatal(err)
	}
	if w := sharedRequest(t, s, "/api/auth/me", b); w.Code != http.StatusUnauthorized {
		t.Fatal("disabled membership still authenticates")
	}
	if w := sharedRequest(t, s, "/api/auth/me", a); w.Code != http.StatusOK {
		t.Fatal("one user's change affected another")
	}
	other := &Server{Config: s.Config, Identity: s.Identity}
	other.Config.SharedProjectID = "other-project"
	wrongProject := *a
	wrongProject.Name = other.sharedCookieName()
	if w := sharedRequest(t, other, "/api/auth/me", &wrongProject); w.Code != http.StatusUnauthorized {
		t.Fatal("token grants another project")
	}
	if err := s.Identity.Close(); err != nil {
		t.Fatal(err)
	}
	if w := sharedRequest(t, s, "/api/auth/me", a); w.Code != http.StatusServiceUnavailable {
		t.Fatal("DB outage did not fail closed")
	}
}

func TestSharedHTTPRejectsBasicAuthCSRFAndForwardedTLS(t *testing.T) {
	s := sharedLoginTestServer(t)
	r := httptest.NewRequest(http.MethodGet, "https://taskdeck.test/api/auth/me", nil)
	r.SetBasicAuth(s.Config.Username, s.Config.Password)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") != "" {
		t.Fatal("legacy fallback")
	}
	for _, origin := range []string{"https://evil.test", "http://taskdeck.test", "https://taskdeck.test:444", "null", "https://taskdeck.test/path"} {
		r := loginRequest(`{"username":"alice","password":"private-admin-password"}`)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("origin %q accepted: %d", origin, w.Code)
		}
	}
	r = loginRequest(`{"username":"alice","password":"private-admin-password"}`)
	r.TLS = nil
	r.Header.Set("X-Forwarded-Proto", "https")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("forwarded TLS header trusted")
	}
	r = loginRequest(`{"username":"alice","password":"private-admin-password"}`)
	r.Header.Set("Sec-Fetch-Site", "same-site")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("cross-port same-site mutation accepted")
	}
}

func TestSharedIdentityRequiresProvisionedEnabledProject(t *testing.T) {
	s := sharedLoginTestServer(t)
	if err := s.validateSharedIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, err := s.Identity.ProjectByKey(context.Background(), s.Config.SharedProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Identity.SetProjectEnabled(context.Background(), p.ID, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.validateSharedIdentity(context.Background()); err == nil {
		t.Fatal("disabled project ready")
	}
	s.Config.SharedProjectID = "missing"
	if err := s.validateSharedIdentity(context.Background()); err == nil {
		t.Fatal("missing project ready")
	}
}
