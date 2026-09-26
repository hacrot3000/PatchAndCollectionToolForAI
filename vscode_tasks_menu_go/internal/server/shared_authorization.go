package server

import (
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func requirePermission(permission string, next http.Handler) http.Handler {
	if !identity.KnownPermission(permission) {
		panic("unknown TaskDeck permission: " + permission)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			sharedAuthError(w, identity.ErrUnauthenticated)
			return
		}
		if !principal.Allowed(permission) {
			writePermissionDenied(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sharedAuthorize(next http.Handler) http.Handler {
	tasks := requirePermission(identity.PermissionTasksView, next)
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
		case r.URL.Path == "/api/tasks":
			tasks.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/"):
			writePermissionDenied(w)
		default:
			next.ServeHTTP(w, r)
		}
	})
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
