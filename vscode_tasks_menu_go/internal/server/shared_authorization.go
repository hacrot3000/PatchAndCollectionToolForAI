package server

import (
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func requirePermission(permission string, next http.Handler) http.Handler {
	return requireAllPermissions([]string{permission}, next)
}

func requireAllPermissions(permissions []string, next http.Handler) http.Handler {
	if len(permissions) == 0 {
		panic("TaskDeck permission policy cannot be empty")
	}
	for _, permission := range permissions {
		if !identity.KnownPermission(permission) {
			panic("unknown TaskDeck permission: " + permission)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			sharedAuthError(w, identity.ErrUnauthenticated)
			return
		}
		for _, permission := range permissions {
			if !principal.Allowed(permission) {
				writePermissionDenied(w)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sharedAuthorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case sharedStaticPath(r.URL.Path):
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			next.ServeHTTP(w, r)
		case r.URL.Path == "/api/browser/lease":
			next.ServeHTTP(w, r)
		case r.URL.Path == "/api/mutation-lock":
			if _, ok := PrincipalFromContext(r.Context()); !ok {
				sharedAuthError(w, identity.ErrUnauthenticated)
				return
			}
			next.ServeHTTP(w, r)
		case r.URL.Path == "/api/sessions":
			// Session list/create authorization depends on session metadata or
			// the requested kind and is enforced inside sessionsRoot.
			next.ServeHTTP(w, r)
		case sharedSessionItemPath(r.URL.Path):
			// Item authorization depends on persisted ownership and is enforced
			// before sessionItem performs any operation.
			next.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/"):
			permissions := sharedRoutePermissions(r)
			principal, ok := PrincipalFromContext(r.Context())
			if !ok {
				sharedAuthError(w, identity.ErrUnauthenticated)
				return
			}
			if len(permissions) == 0 {
				s.appendSharedAudit(r, &principal, nil, "authorization.denied", "http_route", r.URL.Path, "denied", map[string]any{
					"method": r.Method,
					"reason": "unmapped_route",
				})
				writePermissionDenied(w)
				return
			}
			for _, permission := range permissions {
				if !principal.Allowed(permission) {
					s.appendSharedAudit(r, &principal, nil, "authorization.denied", "http_route", r.URL.Path, "denied", map[string]any{
						"method":              r.Method,
						"required_permission": permission,
					})
					writePermissionDenied(w)
					return
				}
			}
			next.ServeHTTP(w, r)
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func sharedSessionItemPath(path string) bool {
	return strings.HasPrefix(path, "/api/sessions/")
}

func sharedRoutePermissions(r *http.Request) []string {
	switch r.URL.Path {
	case "/api/admin/users", "/api/admin/users/access", "/api/admin/users/permission":
		if r.Method == http.MethodGet {
			return []string{identity.PermissionUsersView}
		}
		return []string{identity.PermissionUsersManage}
	case "/api/admin/roles", "/api/admin/permissions":
		if r.Method == http.MethodGet {
			return []string{identity.PermissionRolesView}
		}
		return []string{identity.PermissionRolesManage}
	case "/api/admin/sessions":
		return []string{identity.PermissionSessionsManage}
	case "/api/admin/audit":
		return []string{identity.PermissionAuditView}
	case "/api/tasks":
		return []string{identity.PermissionTasksView}
	case "/api/config/page-title", "/api/config/terminal-cwds", "/api/command-presets":
		if r.Method == http.MethodGet {
			return []string{identity.PermissionSettingsRead}
		}
		return []string{identity.PermissionSettingsWrite}
	case "/api/git/status":
		if r.Method != http.MethodGet {
			return nil // Shared mode has no Git mutation capability.
		}
		switch strings.TrimSpace(r.URL.Query().Get("view")) {
		case "log":
			return []string{identity.PermissionGitLog}
		case "diff", "compare":
			return []string{identity.PermissionGitDiff}
		default:
			return []string{identity.PermissionGitStatus}
		}
	case "/api/patch/ai-pack":
		return []string{identity.PermissionPatchView}
	case "/api/files/selection":
		return []string{identity.PermissionFilesRead}
	case "/api/files/download":
		return []string{identity.PermissionFilesDownload}
	case "/api/files/upload":
		permissions := []string{identity.PermissionFilesUpload}
		if r.URL.Query().Get("overwrite") == "1" {
			permissions = append(permissions, identity.PermissionFilesWrite)
		}
		return permissions
	case "/api/project/tree", "/api/project/files/search", "/api/project/content/search":
		return []string{identity.PermissionFilesRead}
	case "/api/project/file":
		if r.Method == http.MethodGet {
			return []string{identity.PermissionFilesRead}
		}
		return []string{identity.PermissionFilesWrite}
	case "/api/state/tasks":
		switch r.URL.Query().Get("scope") {
		case "self-update":
			if r.Method == http.MethodGet {
				return []string{identity.PermissionSelfupdateCheck}
			}
			return []string{identity.PermissionSelfupdateRun}
		case "terminals":
			return nil // Session ownership is required before this shared state opens.
		default:
			if r.Method == http.MethodGet {
				return []string{identity.PermissionTasksView}
			}
			return []string{identity.PermissionSettingsWrite}
		}
	default:
		return nil
	}
}

func sharedStaticPath(path string) bool {
	switch path {
	case "/", "/index.html", "/app.css", "/app.js", "/features.js", "/vendor/xterm.js", "/vendor/codemirror6-all.min.js", "/vendor/addon-fit.js", "/vendor/xterm.css":
		return true
	default:
		return strings.HasPrefix(path, "/featuremods/")
	}
}

func writePermissionDenied(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "permission denied", http.StatusForbidden)
}
