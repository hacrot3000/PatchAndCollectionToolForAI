package server

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConnectionAuditHookReceivesMetadata(t *testing.T) {
	var (
		gotRequest *http.Request
		gotEvent   ConnectionAuditEvent
	)
	s := &Server{
		ConnectionAudit: func(r *http.Request, event ConnectionAuditEvent) {
			gotRequest = r
			gotEvent = event
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/db/sessions?statement=SECRET_QUERY", strings.NewReader("top-secret"))
	s.auditConnection(req, ConnectionAuditEvent{
		Kind:      " db_session ",
		Action:    " execute ",
		ProfileID: " profile-1 ",
		SessionID: " session-1 ",
		Success:   true,
	})
	if gotRequest != req {
		t.Fatal("audit hook did not receive the original request context")
	}
	if gotEvent.Kind != "db_session" || gotEvent.Action != "execute" ||
		gotEvent.ProfileID != "profile-1" || gotEvent.SessionID != "session-1" || !gotEvent.Success {
		t.Fatalf("event=%+v", gotEvent)
	}
}

func TestDefaultConnectionAuditLogContainsNoRequestPayload(t *testing.T) {
	var buffer bytes.Buffer
	s := &Server{Log: log.New(&buffer, "", 0)}
	req := httptest.NewRequest(http.MethodPost, "/api/db/sessions?statement=SECRET_QUERY", strings.NewReader("top-secret"))
	s.auditConnection(req, ConnectionAuditEvent{
		Kind:      "db_session",
		Action:    "execute",
		ProfileID: "profile-1",
		SessionID: "session-1",
		Success:   false,
	})
	text := buffer.String()
	for _, want := range []string{
		"connection_audit",
		"kind=db_session",
		"action=execute",
		"profile=profile-1",
		"session=session-1",
		"success=false",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("audit log missing %q: %s", want, text)
		}
	}
	for _, forbidden := range []string{"top-secret", "SECRET_QUERY", "statement="} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("audit log leaked request data %q: %s", forbidden, text)
		}
	}
}
