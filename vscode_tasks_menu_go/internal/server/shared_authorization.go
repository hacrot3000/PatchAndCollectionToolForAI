package server

import (
	"net/http"

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

func writePermissionDenied(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "permission denied", http.StatusForbidden)
}
