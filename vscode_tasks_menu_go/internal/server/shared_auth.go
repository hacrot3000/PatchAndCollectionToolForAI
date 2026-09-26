package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

type sharedPrincipalContextKey struct{}

func PrincipalFromContext(ctx context.Context) (identity.Principal, bool) {
	p, ok := ctx.Value(sharedPrincipalContextKey{}).(identity.Principal)
	return p, ok
}

func (s *Server) validateSharedIdentity(ctx context.Context) error {
	if s.Identity == nil {
		return fmt.Errorf("shared identity store is unavailable")
	}
	project, err := s.Identity.ProjectByKey(ctx, s.Config.SharedProjectID)
	if err != nil {
		return fmt.Errorf("shared project is unavailable; provision the first admin with --shared-admin-bootstrap: %w", err)
	}
	if !project.Enabled {
		return fmt.Errorf("shared project is disabled")
	}
	return nil
}

func sharedSameOrigin(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, r.Host) || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return false
		}
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead || strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		switch r.Header.Get("Sec-Fetch-Site") {
		case "cross-site", "same-site":
			return false
		}
	}
	return true
}

// Authentication precedes every project route, including WebSocket upgrades.
// Auth routes bypass the legacy single-browser lease. Project operations remain
// unavailable until module authorization and session ownership are implemented.
func (s *Server) sharedAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/health" && loopbackRemote(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if s.Identity == nil || !s.Config.TLS() || r.TLS == nil {
			http.Error(w, "shared authentication unavailable; HTTPS and identity store required", http.StatusServiceUnavailable)
			return
		}
		if !sharedSameOrigin(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/", "/index.html":
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		case "/login", "/login.js":
			sharedLoginPage(w, r)
			return
		case "/api/auth/login":
			s.sharedLogin(w, r)
			return
		case "/api/auth/logout":
			s.sharedLogout(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		principal, _, err := identity.AuthenticateBrowserSession(ctx, s.Identity, s.Config.SharedProjectID, s.sharedCookieToken(r), time.Now())
		if err != nil {
			sharedAuthError(w, err)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), sharedPrincipalContextKey{}, principal))
		if r.URL.Path == "/api/auth/me" {
			s.sharedCurrentUser(w, r)
			return
		}
		http.Error(w, "shared project APIs await module authorization and session ownership", http.StatusServiceUnavailable)
	})
}

func (s *Server) sharedCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	writeJSON(w, http.StatusOK, sharedPrincipalView(principal))
}
