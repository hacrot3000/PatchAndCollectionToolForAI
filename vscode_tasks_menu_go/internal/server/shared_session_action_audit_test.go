package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestSharedSessionControlSuccessesAreAudited(t *testing.T) {
	s, principal := sharedFileAuditServer(t)
	principal.Permissions = map[string]bool{identity.PermissionTerminalControlOwn: true}
	service := &ownershipTestService{supported: true, items: []session.Metadata{{
		ID: "terminal-1",
		Kind: tasks.SessionKindTerminal,
		OwnerUserID: string(principal.UserID),
		ProjectID: string(principal.ProjectID),
		Status: "running",
	}}}
	s.Sessions = service

	stop := sharedSessionRequest(http.MethodPost, "/api/sessions/terminal-1/stop", "", principal)
	stopRecorder := httptest.NewRecorder()
	s.sessionItem(stopRecorder, stop)
	if stopRecorder.Code != http.StatusOK {
		t.Fatalf("stop status=%d body=%s", stopRecorder.Code, stopRecorder.Body.String())
	}

	kill := sharedSessionRequest(http.MethodPost, "/api/sessions/force-kill", `{"id":"terminal-1"}`, principal)
	killRecorder := httptest.NewRecorder()
	s.sessionForceKill(killRecorder, kill)
	if killRecorder.Code != http.StatusOK {
		t.Fatalf("kill status=%d body=%s", killRecorder.Code, killRecorder.Body.String())
	}

	clear := sharedSessionRequest(http.MethodPost, "/api/sessions/clear-console", `{"id":"terminal-1"}`, principal)
	clearRecorder := httptest.NewRecorder()
	s.sessionClearConsole(clearRecorder, clear)
	if clearRecorder.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", clearRecorder.Code, clearRecorder.Body.String())
	}

	events, err := s.Identity.ListAudit(context.Background(), identity.AuditQuery{
		ProjectID: principal.ProjectID,
		UserID: principal.UserID,
		Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, event := range events {
		if event.Result == "success" && event.ResourceID == "terminal-1" {
			got[event.Action] = true
		}
	}
	for _, action := range []string{"session.stop", "session.kill", "session.clear"} {
		if !got[action] {
			t.Fatalf("missing %s audit in %+v", action, events)
		}
	}
}
