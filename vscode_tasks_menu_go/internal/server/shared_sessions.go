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
	stampSharedSession(principal, spec, kind)
	return true
}

func stampSharedSession(principal identity.Principal, spec *tasks.Execution, kind string) {
	spec.SessionKind = kind
	spec.OwnerUserID = string(principal.UserID)
	spec.ProjectID = string(principal.ProjectID)
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

func sharedSessionViewAllowed(principal identity.Principal, meta session.Metadata) bool {
	return sharedSessionVisible(principal, meta)
}

func sharedTerminalControlAllowed(principal identity.Principal, meta session.Metadata) bool {
	if principal.Allowed(identity.PermissionTerminalControlAll) {
		return true
	}
	return meta.OwnerUserID == string(principal.UserID) && principal.Allowed(identity.PermissionTerminalControlOwn)
}

func sharedSessionControlAllowed(principal identity.Principal, meta session.Metadata) bool {
	if meta.ProjectID == "" || meta.ProjectID != string(principal.ProjectID) {
		return false
	}
	if principal.Allowed(identity.PermissionSessionsManage) {
		return true
	}
	switch meta.Kind {
	case tasks.SessionKindTerminal:
		return sharedTerminalControlAllowed(principal, meta)
	case tasks.SessionKindTask:
		return principal.Allowed(identity.PermissionTasksRun)
	case tasks.SessionKindPatch:
		return principal.Allowed(identity.PermissionPatchRun)
	default:
		return false
	}
}

func sharedSessionActionAllowed(principal identity.Principal, meta session.Metadata, method, action string) bool {
	if meta.ProjectID == "" || meta.ProjectID != string(principal.ProjectID) {
		return false
	}
	if principal.Allowed(identity.PermissionSessionsManage) {
		return true
	}
	view := (action == "" && method == http.MethodGet) || action == "protocol" || action == "ws"
	switch meta.Kind {
	case tasks.SessionKindTerminal:
		if view {
			return sharedSessionViewAllowed(principal, meta)
		}
		switch action {
		case "", "stop", "kill", "clear", "title", "resize":
			return sharedTerminalControlAllowed(principal, meta)
		default:
			return false
		}
	case tasks.SessionKindTask:
		if view {
			return principal.Allowed(identity.PermissionTasksView)
		}
		switch action {
		case "", "stop", "kill", "clear", "title", "resize":
			return principal.Allowed(identity.PermissionTasksRun)
		default:
			return false
		}
	case tasks.SessionKindPatch:
		switch action {
		case "", "protocol", "ws":
			if action == "" && method != http.MethodGet {
				return principal.Allowed(identity.PermissionPatchRun)
			}
			return principal.Allowed(identity.PermissionPatchView) || principal.Allowed(identity.PermissionPatchHistory)
		case "stop", "kill", "clear", "title", "resize", "prompt-response", "item-action", "queue-delete", "resume-action":
			return principal.Allowed(identity.PermissionPatchRun)
		case "parallel-collect":
			return principal.Allowed(identity.PermissionPatchCollect)
		case "history-detail", "history-support":
			return principal.Allowed(identity.PermissionPatchHistory)
		case "history-cleanup", "history-manage":
			return principal.Allowed(identity.PermissionPatchCleanup)
		default:
			return false
		}
	default:
		return false
	}
}

func (s *Server) authorizeSharedSessionItem(w http.ResponseWriter, r *http.Request, id, action string) bool {
	if !s.Config.SharedServerEnabled {
		return true
	}
	if !s.sharedSessionOwnershipReady(w) {
		return false
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return false
	}
	meta, ok := s.Sessions.Metadata(id)
	if !ok || meta.ProjectID == "" || meta.ProjectID != string(principal.ProjectID) {
		http.NotFound(w, r)
		return false
	}
	if !sharedSessionActionAllowed(principal, meta, r.Method, action) {
		writePermissionDenied(w)
		return false
	}
	return true
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
