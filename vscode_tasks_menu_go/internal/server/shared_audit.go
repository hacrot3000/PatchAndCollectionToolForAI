package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func directClientIP(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	if len(remoteAddr) > 128 {
		return remoteAddr[:128]
	}
	return remoteAddr
}

func (s *Server) appendSharedAudit(r *http.Request, principal *identity.Principal, userID *identity.ID, action, resourceType, resourceID, result string, details map[string]any) {
	if !s.Config.SharedServerEnabled || s.Identity == nil {
		return
	}
	id, err := identity.NewID()
	if err != nil {
		if s.Log != nil {
			s.Log.Printf("audit id warning: %v", err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var projectID *identity.ID
	if principal != nil {
		principalUser := principal.UserID
		principalProject := principal.ProjectID
		userID = &principalUser
		projectID = &principalProject
	} else if project, lookupErr := s.Identity.ProjectByKey(ctx, s.Config.SharedProjectID); lookupErr == nil {
		value := project.ID
		projectID = &value
	}
	detailJSON := "{}"
	if len(details) > 0 {
		if encoded, encodeErr := json.Marshal(details); encodeErr == nil {
			detailJSON = string(encoded)
		}
	}
	event := identity.AuditEvent{
		ID: id, Timestamp: time.Now().UTC(), UserID: userID, ProjectID: projectID,
		Action: action, ResourceType: resourceType, ResourceID: resourceID,
		Result: result, ClientIP: directClientIP(r.RemoteAddr), Details: detailJSON,
	}
	if err := s.Identity.AppendAudit(ctx, event); err != nil && s.Log != nil {
		s.Log.Printf("audit append warning action=%s result=%s: %v", action, result, err)
	}
}
