package server

import (
	"net/http"
	"strings"
)

type ConnectionAuditEvent struct {
	Kind      string
	Action    string
	ProfileID string
	SessionID string
	Success   bool
}

type ConnectionAuditFunc func(*http.Request, ConnectionAuditEvent)

func (s *Server) auditConnection(r *http.Request, event ConnectionAuditEvent) {
	event.Kind = strings.TrimSpace(event.Kind)
	event.Action = strings.TrimSpace(event.Action)
	event.ProfileID = strings.TrimSpace(event.ProfileID)
	event.SessionID = strings.TrimSpace(event.SessionID)
	if event.Kind == "" || event.Action == "" {
		return
	}
	if s.ConnectionAudit != nil {
		s.ConnectionAudit(r, event)
	}
	if s.Config.SharedServerEnabled {
		if principal, ok := PrincipalFromContext(r.Context()); ok {
			resourceID := event.SessionID
			if resourceID == "" {
				resourceID = event.ProfileID
			}
			result := "error"
			if event.Success {
				result = "success"
			}
			s.appendSharedAudit(r, &principal, nil, event.Kind+"."+event.Action, event.Kind, resourceID, result, map[string]any{
				"profile_id": event.ProfileID,
				"session_id": event.SessionID,
			})
		}
	}
	if s.Log != nil {
		s.Log.Printf(
			"connection_audit kind=%s action=%s profile=%s session=%s success=%t",
			event.Kind,
			event.Action,
			event.ProfileID,
			event.SessionID,
			event.Success,
		)
	}
}
