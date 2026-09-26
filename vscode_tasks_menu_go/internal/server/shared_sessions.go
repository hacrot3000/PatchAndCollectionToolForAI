package server

import (
	"net/http"

	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func (s *Server) sharedSessionOwnershipReady(w http.ResponseWriter) bool {
	capability, ok := s.Sessions.(session.OwnershipCapability)
	if !ok || !capability.SupportsSessionOwnership() {
		http.Error(w, "session ownership is unavailable; restart the session broker", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func sharedSessionCreatePermission(kind string) string {
	switch kind {
	case tasks.SessionKindTerminal:
		return identity.PermissionTerminalCreate
	case tasks.SessionKindPatch:
		return identity.PermissionPatchRun
	case tasks.SessionKindTask:
		return identity.PermissionTasksRun
	default:
		return ""
	}
}

func (s *Server) prepareSharedSession(w http.ResponseWriter, r *http.Request, spec *tasks.Execution, kind string) bool {
	if !s.Config.SharedServerEnabled {
		return true
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return false
	}
	permission := sharedSessionCreatePermission(kind)
	if permission == "" || !principal.Allowed(permission) {
		writePermissionDenied(w)
		return false
	}
	spec.SessionKind = kind
	spec.OwnerUserID = string(principal.UserID)
	spec.ProjectID = string(principal.ProjectID)
	return true
}

func sharedSessionVisible(principal identity.Principal, meta session.Metadata) bool {
	if meta.ProjectID == "" || meta.ProjectID != string(principal.ProjectID) {
		return false
	}
	if principal.Allowed(identity.PermissionSessionsManage) {
		return true
	}
	switch meta.Kind {
	case tasks.SessionKindTask:
		return principal.Allowed(identity.PermissionTasksView)
	case tasks.SessionKindPatch:
		return principal.Allowed(identity.PermissionPatchView) || principal.Allowed(identity.PermissionPatchHistory)
	case tasks.SessionKindTerminal:
		if principal.Allowed(identity.PermissionTerminalViewAll) {
			return true
		}
		return meta.OwnerUserID == string(principal.UserID) && principal.Allowed(identity.PermissionTerminalViewOwn)
	default:
		return false
	}
}

func filterSharedSessions(principal identity.Principal, items []session.Metadata) []session.Metadata {
	filtered := make([]session.Metadata, 0, len(items))
	for _, item := range items {
		if sharedSessionVisible(principal, item) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
