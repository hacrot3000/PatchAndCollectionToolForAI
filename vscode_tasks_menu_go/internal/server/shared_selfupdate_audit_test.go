package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedSelfUpdateStartIsAudited(t *testing.T) {
	s, principal := sharedFileAuditServer(t)
	RegisterSelfUpdateStart(s, func() error { return nil })
	t.Cleanup(func() { RegisterSelfUpdateStart(s, nil) })

	req := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=start", nil)
	req = sharedAuditRequest(req, principal)
	recorder := httptest.NewRecorder()
	s.selfUpdateState(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("start status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	events, err := s.Identity.ListAudit(context.Background(), identity.AuditQuery{
		ProjectID: principal.ProjectID,
		UserID: principal.UserID,
		Action: "selfupdate.start",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Result != "success" || events[0].ResourceType != "selfupdate" {
		t.Fatalf("unexpected self-update audit: %+v", events)
	}
}
