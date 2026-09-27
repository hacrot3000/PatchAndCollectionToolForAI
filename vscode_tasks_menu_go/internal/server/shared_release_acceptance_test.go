package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedReleaseAcceptanceTwoDaemonsOneIdentityDB(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "identity", "identity.db")

	storeA, err := identity.OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer storeA.Close()
	storeB, err := identity.OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer storeB.Close()

	password := "private-admin-password"
	hash, err := identity.HashPassword(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	alice, err := storeA.BootstrapFirstAdmin(ctx, "project-a", "alice", hash, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := storeA.SeedSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	projectB, err := storeB.EnsureProject(ctx, identity.Project{
		ID: "project-b-id", Key: "project-b", DisplayName: "Project B",
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := storeB.UpsertProjectMember(ctx, identity.ProjectMember{
		ProjectID: projectB.ID, UserID: alice.UserID, RoleID: "system:admin",
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	serverA := authTestServer()
	serverA.Config.SharedServerEnabled = true
	serverA.Config.SharedProjectID = "project-a"
	serverA.Identity = storeA
	serverB := authTestServer()
	serverB.Config.SharedServerEnabled = true
	serverB.Config.SharedProjectID = "project-b"
	serverB.Identity = storeB

	if err := serverA.validateSharedIdentity(ctx); err != nil {
		t.Fatalf("project A validation: %v", err)
	}
	if err := serverB.validateSharedIdentity(ctx); err != nil {
		t.Fatalf("project B validation: %v", err)
	}

	login := func(t *testing.T, s *Server) *http.Cookie {
		t.Helper()
		req := loginRequest(`{"username":"alice","password":"private-admin-password"}`)
		recorder := httptest.NewRecorder()
		s.Handler().ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("login status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		cookies := recorder.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Value == "" {
			t.Fatalf("login did not issue session cookie: %+v", cookies)
		}
		return cookies[0]
	}

	cookieA := login(t, serverA)
	cookieB := login(t, serverB)
	if cookieA.Name == cookieB.Name {
		t.Fatalf("project-scoped cookie names collided: %q", cookieA.Name)
	}

	if got := sharedRequest(t, serverA, "/api/auth/me", cookieA); got.Code != http.StatusOK {
		t.Fatalf("project A session rejected: %d %s", got.Code, got.Body.String())
	}
	if got := sharedRequest(t, serverB, "/api/auth/me", cookieB); got.Code != http.StatusOK {
		t.Fatalf("project B session rejected: %d %s", got.Code, got.Body.String())
	}

	crossProjectCookie := *cookieA
	crossProjectCookie.Name = serverB.sharedCookieName()
	if got := sharedRequest(t, serverB, "/api/auth/me", &crossProjectCookie); got.Code != http.StatusUnauthorized {
		t.Fatalf("project A token crossed into project B: %d %s", got.Code, got.Body.String())
	}

	memberB, err := storeA.ProjectMember(ctx, projectB.ID, alice.UserID)
	if err != nil {
		t.Fatal(err)
	}
	memberB.Enabled = false
	memberB.UpdatedAt = time.Now().UTC()
	if err := storeA.UpsertProjectMember(ctx, memberB); err != nil {
		t.Fatal(err)
	}
	if got := sharedRequest(t, serverB, "/api/auth/me", cookieB); got.Code != http.StatusUnauthorized {
		t.Fatalf("disabled project B membership still authorized: %d %s", got.Code, got.Body.String())
	}
	if got := sharedRequest(t, serverA, "/api/auth/me", cookieA); got.Code != http.StatusOK {
		t.Fatalf("project B membership change affected project A: %d %s", got.Code, got.Body.String())
	}

	userFromB, err := storeB.UserByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if userFromB.ID != alice.UserID {
		t.Fatalf("global user identity diverged across stores: A=%q B=%q", alice.UserID, userFromB.ID)
	}
	if _, err := storeB.ProjectMember(ctx, alice.ProjectID, alice.UserID); err != nil {
		t.Fatalf("store B cannot see project A membership: %v", err)
	}
}
