package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
	"github.com/coder/websocket"
)

type ownershipTestService struct {
	session.Service
	supported bool
	items     []session.Metadata
	started   []tasks.Execution
	stopped   []string
	killed    []string
	cleared   []string
	currentCwds    map[string]string
	cloneSupported bool
	clonedFrom     []string
	cloneOptions   []session.TerminalCloneOptions
}

type sharedWebSocketTestService struct {
	*ownershipTestService
	input chan []byte
}

func (s *sharedWebSocketTestService) Subscribe(string) ([]byte, <-chan []byte, func(), error) {
	return []byte("ready"), make(chan []byte), func() {}, nil
}

func (s *sharedWebSocketTestService) Input(_ string, data []byte) error {
	s.input <- append([]byte(nil), data...)
	return nil
}

func (s *ownershipTestService) Metadata(id string) (session.Metadata, bool) {
	for _, item := range s.items {
		if item.ID == id {
			return item, true
		}
	}
	return session.Metadata{}, false
}

func (s *ownershipTestService) Stop(id string) error {
	s.stopped = append(s.stopped, id)
	return nil
}

func (s *ownershipTestService) Kill(id string) error {
	s.killed = append(s.killed, id)
	return nil
}

func (s *ownershipTestService) Clear(id string) error {
	s.cleared = append(s.cleared, id)
	return nil
}

func (s *ownershipTestService) CurrentCwd(id string) (string, error) {
	if cwd, ok := s.currentCwds[id]; ok {
		return cwd, nil
	}
	if meta, ok := s.Metadata(id); ok && strings.TrimSpace(meta.Cwd) != "" {
		return meta.Cwd, nil
	}
	return "", context.Canceled
}

func (s *ownershipTestService) SupportsSessionOwnership() bool { return s.supported }
func (s *ownershipTestService) SupportsTerminalClone() bool { return s.cloneSupported }
func (s *ownershipTestService) CloneTerminal(id string, options session.TerminalCloneOptions) (session.Metadata, error) {
	s.clonedFrom = append(s.clonedFrom, id)
	s.cloneOptions = append(s.cloneOptions, options)
	return session.Metadata{
		ID: "clone-" + id, Kind: tasks.SessionKindTerminal, OwnerUserID: options.OwnerUserID,
		ProjectID: options.ProjectID, Label: "Terminal", Status: "running", Cwd: "/clone",
		TargetType: "local",
	}, nil
}
func (s *ownershipTestService) List() []session.Metadata {
	return append([]session.Metadata(nil), s.items...)
}
func (s *ownershipTestService) Start(spec tasks.Execution) (session.Metadata, error) {
	s.started = append(s.started, spec)
	return session.Metadata{
		ID: "new-session", Kind: spec.SessionKind, OwnerUserID: spec.OwnerUserID,
		ProjectID: spec.ProjectID, Label: spec.Label, Status: "running",
	}, nil
}

func sharedSessionTestServer(t *testing.T, service session.Service) *Server {
	t.Helper()
	cfg := config.Default()
	cfg.SharedServerEnabled = true
	cfg.SharedProjectID = "project-key"
	return &Server{Workspace: t.TempDir(), Config: cfg, Sessions: service}
}

func sharedSessionRequest(method, target, body string, principal identity.Principal) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	return r.WithContext(context.WithValue(r.Context(), sharedPrincipalContextKey{}, principal))
}

func TestSharedSessionListFiltersByProjectKindAndOwner(t *testing.T) {
	service := &ownershipTestService{supported: true, items: []session.Metadata{
		{ID: "own-terminal", Kind: tasks.SessionKindTerminal, OwnerUserID: "alice", ProjectID: "project-1"},
		{ID: "other-terminal", Kind: tasks.SessionKindTerminal, OwnerUserID: "bob", ProjectID: "project-1"},
		{ID: "task", Kind: tasks.SessionKindTask, OwnerUserID: "bob", ProjectID: "project-1"},
		{ID: "patch", Kind: tasks.SessionKindPatch, OwnerUserID: "bob", ProjectID: "project-1"},
		{ID: "other-project", Kind: tasks.SessionKindTask, OwnerUserID: "alice", ProjectID: "project-2"},
		{ID: "legacy", Status: "running"},
	}}
	s := sharedSessionTestServer(t, service)
	principal := identity.Principal{UserID: "alice", ProjectID: "project-1", Permissions: map[string]bool{
		identity.PermissionTerminalViewOwn: true,
		identity.PermissionTasksView:       true,
		identity.PermissionPatchView:       true,
	}}
	rr := httptest.NewRecorder()
	s.sessionsRoot(rr, sharedSessionRequest(http.MethodGet, "/api/sessions", "", principal))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Sessions []session.Metadata `json:"sessions"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Sessions) != 3 || payload.Sessions[0].ID != "own-terminal" || payload.Sessions[1].ID != "task" || payload.Sessions[2].ID != "patch" {
		t.Fatalf("unexpected visible sessions: %#v", payload.Sessions)
	}
}

func TestSharedSessionCreateRequiresKindPermissionAndStampsOwner(t *testing.T) {
	service := &ownershipTestService{supported: true}
	s := sharedSessionTestServer(t, service)
	principal := identity.Principal{UserID: "alice", ProjectID: "project-1", Permissions: map[string]bool{identity.PermissionTerminalCreate: true}}
	rr := httptest.NewRecorder()
	s.sessionsRoot(rr, sharedSessionRequest(http.MethodPost, "/api/sessions", `{"kind":"terminal"}`, principal))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(service.started) != 1 {
		t.Fatalf("start calls=%d", len(service.started))
	}
	spec := service.started[0]
	if spec.SessionKind != tasks.SessionKindTerminal || spec.OwnerUserID != "alice" || spec.ProjectID != "project-1" {
		t.Fatalf("unstamped execution: %#v", spec)
	}

	denied := identity.Principal{UserID: "bob", ProjectID: "project-1", Permissions: map[string]bool{identity.PermissionTasksRun: true}}
	rr = httptest.NewRecorder()
	s.sessionsRoot(rr, sharedSessionRequest(http.MethodPost, "/api/sessions", `{"kind":"terminal"}`, denied))
	if rr.Code != http.StatusForbidden || len(service.started) != 1 {
		t.Fatalf("terminal create without permission: status=%d starts=%d", rr.Code, len(service.started))
	}
}

func TestSharedSessionRoutesFailClosedWithoutOwnershipCapability(t *testing.T) {
	service := &ownershipTestService{supported: false}
	s := sharedSessionTestServer(t, service)
	rr := httptest.NewRecorder()
	s.sessionsRoot(rr, sharedSessionRequest(http.MethodGet, "/api/sessions", "", identity.Principal{}))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestSharedSessionItemUsesOwnAndAllTerminalPermissions(t *testing.T) {
	service := &ownershipTestService{supported: true, items: []session.Metadata{
		{ID: "own", Kind: tasks.SessionKindTerminal, OwnerUserID: "alice", ProjectID: "project-1", Status: "running"},
		{ID: "other", Kind: tasks.SessionKindTerminal, OwnerUserID: "bob", ProjectID: "project-1", Status: "running"},
		{ID: "foreign", Kind: tasks.SessionKindTerminal, OwnerUserID: "alice", ProjectID: "project-2", Status: "running"},
	}}
	s := sharedSessionTestServer(t, service)
	viewer := identity.Principal{UserID: "alice", ProjectID: "project-1", Permissions: map[string]bool{identity.PermissionTerminalViewOwn: true}}

	rr := httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodGet, "/api/sessions/own", "", viewer))
	if rr.Code != http.StatusOK {
		t.Fatalf("own terminal view status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodGet, "/api/sessions/other", "", viewer))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("other terminal view status=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodGet, "/api/sessions/foreign", "", viewer))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-project terminal status=%d", rr.Code)
	}

	controller := identity.Principal{UserID: "alice", ProjectID: "project-1", Permissions: map[string]bool{identity.PermissionTerminalControlOwn: true}}
	rr = httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodPost, "/api/sessions/own/stop", "", controller))
	if rr.Code != http.StatusOK || len(service.stopped) != 1 || service.stopped[0] != "own" {
		t.Fatalf("own terminal control status=%d stopped=%v", rr.Code, service.stopped)
	}
	rr = httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodPost, "/api/sessions/other/stop", "", controller))
	if rr.Code != http.StatusForbidden || len(service.stopped) != 1 {
		t.Fatalf("other terminal control status=%d stopped=%v", rr.Code, service.stopped)
	}
}

func TestSharedTerminalLiveCWDUsesViewPermission(t *testing.T) {
	service := &ownershipTestService{
		supported: true,
		items: []session.Metadata{
			{ID: "local", Kind: tasks.SessionKindTerminal, TaskID: 0, OwnerUserID: "alice", ProjectID: "project-1", Status: "running", Cwd: "/initial", TargetType: "local"},
			{ID: "ssh", Kind: tasks.SessionKindTerminal, TaskID: 0, OwnerUserID: "alice", ProjectID: "project-1", Status: "running", Cwd: "/host", TargetType: "ssh", TargetProfileID: "ssh-profile"},
			{ID: "other", Kind: tasks.SessionKindTerminal, TaskID: 0, OwnerUserID: "bob", ProjectID: "project-1", Status: "running", Cwd: "/other", TargetType: "local"},
		},
		currentCwds: map[string]string{"local": "/live/repo"},
	}
	s := sharedSessionTestServer(t, service)
	viewer := identity.Principal{UserID: "alice", ProjectID: "project-1", Permissions: map[string]bool{identity.PermissionTerminalViewOwn: true}}

	rr := httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodGet, "/api/sessions/local/cwd", "", viewer))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"local":true`) || !strings.Contains(rr.Body.String(), `"/live/repo"`) {
		t.Fatalf("local live cwd status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodGet, "/api/sessions/ssh/cwd", "", viewer))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"local":false`) {
		t.Fatalf("ssh cwd status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodGet, "/api/sessions/other/cwd", "", viewer))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("other-user cwd status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodPost, "/api/sessions/local/cwd", "", viewer))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("POST cwd status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSharedForceKillAndClearRequireSessionControl(t *testing.T) {
	service := &ownershipTestService{supported: true, items: []session.Metadata{{
		ID: "terminal-1", Kind: tasks.SessionKindTerminal, OwnerUserID: "alice", ProjectID: "project-1", Status: "running",
	}}}
	s := sharedSessionTestServer(t, service)
	viewer := identity.Principal{UserID: "alice", ProjectID: "project-1", Permissions: map[string]bool{identity.PermissionTerminalViewOwn: true}}
	controller := identity.Principal{UserID: "alice", ProjectID: "project-1", Permissions: map[string]bool{identity.PermissionTerminalControlOwn: true}}

	rr := httptest.NewRecorder()
	s.sessionForceKill(rr, sharedSessionRequest(http.MethodPost, "/api/sessions/force-kill", `{"id":"terminal-1"}`, viewer))
	if rr.Code != http.StatusForbidden || len(service.killed) != 0 {
		t.Fatalf("viewer force-kill status=%d calls=%v", rr.Code, service.killed)
	}
	rr = httptest.NewRecorder()
	s.sessionForceKill(rr, sharedSessionRequest(http.MethodPost, "/api/sessions/force-kill", `{"id":"terminal-1"}`, controller))
	if rr.Code != http.StatusOK || len(service.killed) != 1 {
		t.Fatalf("controller force-kill status=%d calls=%v", rr.Code, service.killed)
	}
	rr = httptest.NewRecorder()
	s.sessionClearConsole(rr, sharedSessionRequest(http.MethodPost, "/api/sessions/clear-console", `{"id":"terminal-1"}`, controller))
	if rr.Code != http.StatusOK || len(service.cleared) != 1 {
		t.Fatalf("controller clear status=%d calls=%v", rr.Code, service.cleared)
	}
}

func TestSharedPatchActionPermissionsAreSpecific(t *testing.T) {
	meta := session.Metadata{Kind: tasks.SessionKindPatch, OwnerUserID: "alice", ProjectID: "project-1"}
	base := identity.Principal{UserID: "alice", ProjectID: "project-1"}
	tests := []struct {
		action     string
		permission string
	}{
		{action: "item-action", permission: identity.PermissionPatchRun},
		{action: "parallel-collect", permission: identity.PermissionPatchCollect},
		{action: "history-support", permission: identity.PermissionPatchHistory},
		{action: "history-cleanup", permission: identity.PermissionPatchCleanup},
	}
	for _, test := range tests {
		principal := base
		principal.Permissions = map[string]bool{test.permission: true}
		if !sharedSessionActionAllowed(principal, meta, http.MethodPost, test.action) {
			t.Fatalf("%s denied its exact permission %s", test.action, test.permission)
		}
		principal.Permissions = map[string]bool{identity.PermissionPatchView: true}
		if sharedSessionActionAllowed(principal, meta, http.MethodPost, test.action) {
			t.Fatalf("%s accepted read-only Patch permission", test.action)
		}
	}
}

func TestSharedSessionWebSocketSeparatesViewAndControl(t *testing.T) {
	service := &sharedWebSocketTestService{
		ownershipTestService: &ownershipTestService{supported: true, items: []session.Metadata{{
			ID: "task-1", Kind: tasks.SessionKindTask, OwnerUserID: "alice", ProjectID: "project-1", Status: "running",
		}}},
		input: make(chan []byte, 1),
	}
	s := sharedSessionTestServer(t, service)
	serve := func(principal identity.Principal) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(context.WithValue(r.Context(), sharedPrincipalContextKey{}, principal))
			s.sessionItem(w, r)
		}))
	}

	viewer := identity.Principal{UserID: "alice", ProjectID: "project-1", Permissions: map[string]bool{identity.PermissionTasksView: true}}
	viewerServer := serve(viewer)
	defer viewerServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	viewerConn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(viewerServer.URL, "http")+"/api/sessions/task-1/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, backlog, err := viewerConn.Read(ctx)
	if err != nil || string(backlog) != "ready" {
		t.Fatalf("viewer backlog=%q err=%v", backlog, err)
	}
	if err := viewerConn.Write(ctx, websocket.MessageText, []byte("blocked")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := viewerConn.Read(ctx); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("read-only input close status=%v err=%v", websocket.CloseStatus(err), err)
	}
	select {
	case input := <-service.input:
		t.Fatalf("viewer input reached session: %q", input)
	default:
	}

	controller := identity.Principal{UserID: "alice", ProjectID: "project-1", Permissions: map[string]bool{
		identity.PermissionTasksView: true,
		identity.PermissionTasksRun:  true,
	}}
	controllerServer := serve(controller)
	defer controllerServer.Close()
	controllerConn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(controllerServer.URL, "http")+"/api/sessions/task-1/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer controllerConn.Close(websocket.StatusNormalClosure, "test complete")
	if _, _, err := controllerConn.Read(ctx); err != nil {
		t.Fatal(err)
	}
	if err := controllerConn.Write(ctx, websocket.MessageText, []byte("allowed")); err != nil {
		t.Fatal(err)
	}
	select {
	case input := <-service.input:
		if string(input) != "allowed" {
			t.Fatalf("controller input=%q", input)
		}
	case <-ctx.Done():
		t.Fatal("controller input did not reach session")
	}
}

func TestSharedSessionAuthorizationDenialsAreAudited(t *testing.T) {
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
	principal, err := store.BootstrapFirstAdmin(ctx, "project-key", "alice", hash, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	service := &ownershipTestService{supported: true, items: []session.Metadata{{
		ID: "terminal-1", Kind: tasks.SessionKindTerminal, OwnerUserID: "bob",
		ProjectID: string(principal.ProjectID), Status: "running",
	}}}
	s := sharedSessionTestServer(t, service)
	s.Identity = store
	principal.Permissions = map[string]bool{identity.PermissionTasksRun: true}

	create := sharedSessionRequest(http.MethodPost, "/api/sessions", `{"kind":"terminal"}`, principal)
	create.RemoteAddr = "127.0.0.1:12345"
	createRecorder := httptest.NewRecorder()
	s.sessionsRoot(createRecorder, create)
	if createRecorder.Code != http.StatusForbidden {
		t.Fatalf("create denial status=%d body=%s", createRecorder.Code, createRecorder.Body.String())
	}

	view := sharedSessionRequest(http.MethodGet, "/api/sessions/terminal-1", "", principal)
	view.RemoteAddr = "127.0.0.1:12345"
	viewRecorder := httptest.NewRecorder()
	s.sessionItem(viewRecorder, view)
	if viewRecorder.Code != http.StatusForbidden {
		t.Fatalf("session denial status=%d body=%s", viewRecorder.Code, viewRecorder.Body.String())
	}

	events, err := store.ListAudit(ctx, identity.AuditQuery{
		ProjectID: principal.ProjectID,
		UserID: principal.UserID,
		Action: "authorization.denied",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	var createAudit, sessionAudit bool
	for _, event := range events {
		switch {
		case event.ResourceType == "session_create" && event.ResourceID == tasks.SessionKindTerminal:
			createAudit = strings.Contains(event.Details, identity.PermissionTerminalCreate)
		case event.ResourceType == "session" && event.ResourceID == "terminal-1":
			sessionAudit = strings.Contains(event.Details, `"kind":"terminal"`)
		}
	}
	if !createAudit || !sessionAudit {
		t.Fatalf("missing session authorization audits: %+v", events)
	}
}

func TestSharedSessionStartAuditUsesKindSpecificActionsWithoutSecrets(t *testing.T) {
	ctx := context.Background()
	store, err := identity.OpenSQLiteStore(ctx, filepath.Join(t.TempDir(), "identity", "identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	principal, err := store.BootstrapFirstAdmin(ctx, "project-key", "alice", "$scrypt$v=1,ln=17,r=8,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	s := sharedSessionTestServer(t, &ownershipTestService{supported: true})
	s.Identity = store
	request := sharedSessionRequest(http.MethodPost, "/api/sessions", "", principal)
	request.RemoteAddr = "127.0.0.1:12345"

	for _, item := range []struct {
		kind   string
		id     string
		action string
		detail map[string]any
	}{
		{kind: tasks.SessionKindTerminal, id: "terminal-audit", action: "terminal.create", detail: map[string]any{"cwd": "subdir"}},
		{kind: tasks.SessionKindTask, id: "task-audit", action: "task.run", detail: map[string]any{"task_id": 7}},
		{kind: tasks.SessionKindPatch, id: "patch-audit", action: "patch.run", detail: map[string]any{"patch_mode": "queue"}},
	} {
		s.auditSharedSessionStart(request, session.Metadata{ID: item.id, Kind: item.kind}, item.detail)
		events, err := store.ListAudit(ctx, identity.AuditQuery{ProjectID: principal.ProjectID, UserID: principal.UserID, Action: item.action, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 1 || events[0].ResourceID != item.id || events[0].Result != "success" {
			t.Fatalf("%s audit=%+v", item.action, events)
		}
		if strings.Contains(events[0].Details, "password") || strings.Contains(events[0].Details, "secret") {
			t.Fatalf("%s audit leaked secret-looking detail: %s", item.action, events[0].Details)
		}
	}
}

func TestSharedSessionWebSocketRejectsCrossOriginHandshake(t *testing.T) {
	service := &sharedWebSocketTestService{
		ownershipTestService: &ownershipTestService{supported: true, items: []session.Metadata{{
			ID: "task-origin", Kind: tasks.SessionKindTask,
			OwnerUserID: "alice", ProjectID: "project-1", Status: "running",
		}}},
		input: make(chan []byte, 1),
	}
	s := sharedSessionTestServer(t, service)
	principal := identity.Principal{
		UserID: "alice", ProjectID: "project-1",
		Permissions: map[string]bool{identity.PermissionTasksView: true},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), sharedPrincipalContextKey{}, principal))
		s.sessionItem(w, r)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(
		ctx,
		"ws"+strings.TrimPrefix(server.URL, "http")+"/api/sessions/task-origin/ws",
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}}},
	)
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "unexpected connection")
	}
	if err == nil {
		t.Fatal("cross-origin WebSocket handshake unexpectedly succeeded")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("cross-origin WebSocket status=%d err=%v", status, err)
	}
}

func TestSharedTerminalCloneRequiresControlAndCreatePermission(t *testing.T) {
	service := &ownershipTestService{
		supported: true,
		cloneSupported: true,
		items: []session.Metadata{{
			ID: "terminal-1", Kind: tasks.SessionKindTerminal, TaskID: 0,
			OwnerUserID: "alice", ProjectID: "project-1", Label: "Terminal",
			Status: "running", Cwd: "/work", TargetType: "local",
		}},
	}
	s := sharedSessionTestServer(t, service)

	allowed := identity.Principal{
		UserID: "alice", ProjectID: "project-1",
		Permissions: map[string]bool{
			identity.PermissionTerminalControlOwn: true,
			identity.PermissionTerminalCreate: true,
		},
	}
	rr := httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodPost, "/api/sessions/terminal-1/clone", "", allowed))
	if rr.Code != http.StatusCreated {
		t.Fatalf("clone status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(service.clonedFrom) != 1 || service.clonedFrom[0] != "terminal-1" {
		t.Fatalf("clone sources=%v", service.clonedFrom)
	}
	if len(service.cloneOptions) != 1 || service.cloneOptions[0].OwnerUserID != "alice" || service.cloneOptions[0].ProjectID != "project-1" {
		t.Fatalf("clone options=%+v", service.cloneOptions)
	}
	if !strings.Contains(rr.Body.String(), "\"owner_user_id\":\"alice\"") || !strings.Contains(rr.Body.String(), "\"project_id\":\"project-1\"") {
		t.Fatalf("clone response=%s", rr.Body.String())
	}

	noCreate := identity.Principal{
		UserID: "alice", ProjectID: "project-1",
		Permissions: map[string]bool{identity.PermissionTerminalControlOwn: true},
	}
	rr = httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodPost, "/api/sessions/terminal-1/clone", "", noCreate))
	if rr.Code != http.StatusForbidden || len(service.clonedFrom) != 1 {
		t.Fatalf("clone without create status=%d calls=%v", rr.Code, service.clonedFrom)
	}

	noControl := identity.Principal{
		UserID: "alice", ProjectID: "project-1",
		Permissions: map[string]bool{identity.PermissionTerminalCreate: true},
	}
	rr = httptest.NewRecorder()
	s.sessionItem(rr, sharedSessionRequest(http.MethodPost, "/api/sessions/terminal-1/clone", "", noControl))
	if rr.Code != http.StatusForbidden || len(service.clonedFrom) != 1 {
		t.Fatalf("clone without control status=%d calls=%v", rr.Code, service.clonedFrom)
	}
}
