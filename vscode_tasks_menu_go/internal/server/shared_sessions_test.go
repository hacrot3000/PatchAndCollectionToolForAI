package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

type ownershipTestService struct {
	session.Service
	supported bool
	items     []session.Metadata
	started   []tasks.Execution
}

func (s *ownershipTestService) SupportsSessionOwnership() bool { return s.supported }
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
