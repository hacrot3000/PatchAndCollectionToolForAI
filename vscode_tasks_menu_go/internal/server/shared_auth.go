package server

import "net/http"

// Keep intermediate shared-server revisions closed until session authentication
// is wired. Shared mode must never fall through to the legacy Basic Auth path.
func (s *Server) sharedAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/health" && loopbackRemote(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "shared authentication is not ready", http.StatusServiceUnavailable)
	})
}
