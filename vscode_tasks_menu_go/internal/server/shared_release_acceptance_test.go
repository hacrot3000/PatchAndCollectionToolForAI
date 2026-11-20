package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
	"github.com/coder/websocket"
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

	crossLogout := httptest.NewRequest(http.MethodPost, "https://taskdeck.test/api/auth/logout", strings.NewReader(`{}`))
	crossLogout.Header.Set("Content-Type", "application/json")
	crossLogout.AddCookie(&crossProjectCookie)
	crossLogoutRecorder := httptest.NewRecorder()
	serverB.Handler().ServeHTTP(crossLogoutRecorder, crossLogout)
	if crossLogoutRecorder.Code != http.StatusNoContent {
		t.Fatalf("cross-project logout status=%d body=%s", crossLogoutRecorder.Code, crossLogoutRecorder.Body.String())
	}
	if got := sharedRequest(t, serverA, "/api/auth/me", cookieA); got.Code != http.StatusOK {
		t.Fatalf("project B logout revoked project A session: %d %s", got.Code, got.Body.String())
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

func TestSharedReleaseAcceptanceHTTPSLoginAndWSSAuthorization(t *testing.T) {
	ctx := context.Background()
	store, err := identity.OpenSQLiteStore(ctx, filepath.Join(t.TempDir(), "identity", "identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	hash, err := identity.HashPassword(ctx, "private-admin-password")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := store.BootstrapFirstAdmin(ctx, "wss-project", "alice", hash, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	member, err := store.ProjectMember(ctx, principal.ProjectID, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	member.RoleID = "system:developer"
	member.UpdatedAt = time.Now().UTC()
	if err := store.UpsertProjectMember(ctx, member); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMemberPermission(ctx, identity.MemberPermission{
		ProjectID: principal.ProjectID,
		UserID: principal.UserID,
		PermissionID: identity.PermissionTerminalControlOwn,
		Effect: identity.PermissionDeny,
	}); err != nil {
		t.Fatal(err)
	}

	service := &sharedWebSocketTestService{
		ownershipTestService: &ownershipTestService{supported: true, items: []session.Metadata{{
			ID: "terminal-a",
			Kind: tasks.SessionKindTerminal,
			OwnerUserID: string(principal.UserID),
			ProjectID: string(principal.ProjectID),
			Status: "running",
		}}},
		input: make(chan []byte, 1),
	}
	s := authTestServer()
	s.Config.SharedServerEnabled = true
	s.Config.SharedProjectID = "wss-project"
	s.Identity = store
	s.Sessions = service
	if err := s.validateSharedIdentity(ctx); err != nil {
		t.Fatal(err)
	}

	tlsServer := httptest.NewTLSServer(s.Handler())
	defer tlsServer.Close()
	client := tlsServer.Client()

	loginReq, err := http.NewRequest(
		http.MethodPost,
		tlsServer.URL+"/api/auth/login",
		strings.NewReader(`{"username":"alice","password":"private-admin-password"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := client.Do(loginReq)
	if err != nil {
		t.Fatal(err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("HTTPS login status=%d", loginResp.StatusCode)
	}
	cookies := loginResp.Cookies()
	if len(cookies) != 1 || cookies[0].Value == "" {
		t.Fatalf("HTTPS login cookie=%+v", cookies)
	}
	cookieHeader := cookies[0].Name + "=" + cookies[0].Value
	wsURL := "wss" + strings.TrimPrefix(tlsServer.URL, "https") + "/api/sessions/terminal-a/ws"

	dial := func() *websocket.Conn {
		t.Helper()
		dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, response, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
			HTTPClient: client,
			HTTPHeader: http.Header{"Cookie": []string{cookieHeader}},
		})
		if err != nil {
			status := 0
			if response != nil {
				status = response.StatusCode
			}
			t.Fatalf("WSS dial status=%d err=%v", status, err)
		}
		return conn
	}

	viewerConn := dial()
	viewerCtx, cancelViewer := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelViewer()
	_, backlog, err := viewerConn.Read(viewerCtx)
	if err != nil || string(backlog) != "ready" {
		t.Fatalf("viewer WSS backlog=%q err=%v", backlog, err)
	}
	if err := viewerConn.Write(viewerCtx, websocket.MessageText, []byte("blocked")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := viewerConn.Read(viewerCtx); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("read-only WSS input close status=%v err=%v", websocket.CloseStatus(err), err)
	}
	select {
	case input := <-service.input:
		t.Fatalf("read-only WSS input reached terminal: %q", input)
	default:
	}

	if err := store.DeleteMemberPermission(
		ctx, principal.ProjectID, principal.UserID, identity.PermissionTerminalControlOwn,
	); err != nil {
		t.Fatal(err)
	}

	controllerConn := dial()
	defer controllerConn.Close(websocket.StatusNormalClosure, "test complete")
	controllerCtx, cancelController := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelController()
	if _, _, err := controllerConn.Read(controllerCtx); err != nil {
		t.Fatal(err)
	}
	if err := controllerConn.Write(controllerCtx, websocket.MessageText, []byte("allowed")); err != nil {
		t.Fatal(err)
	}
	select {
	case input := <-service.input:
		if string(input) != "allowed" {
			t.Fatalf("controller WSS input=%q", input)
		}
	case <-controllerCtx.Done():
		t.Fatal("authorized WSS input did not reach terminal")
	}
}
