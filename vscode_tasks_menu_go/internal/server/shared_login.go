package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

// A syntactically valid non-secret hash ensures unknown users still perform a
// full scrypt verification. Its result is never accepted as authentication.
const sharedDummyPasswordHash = "$scrypt$v=1,ln=17,r=8,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func (s *Server) sharedCookieName() string {
	hash := sha256.Sum256([]byte(s.Config.SharedProjectID))
	return "__Host-taskdeck-" + hex.EncodeToString(hash[:8])
}

func (s *Server) sharedCookieToken(r *http.Request) string {
	var token string
	for _, cookie := range r.Cookies() {
		if cookie.Name == s.sharedCookieName() {
			if token != "" {
				return ""
			} // Reject ambiguous cookies.
			token = cookie.Value
		}
	}
	return token
}

func (s *Server) setSharedCookie(w http.ResponseWriter, token string, expires time.Time) {
	cookie := &http.Cookie{Name: s.sharedCookieName(), Value: token, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, Expires: expires}
	if token == "" {
		cookie.MaxAge = -1
	}
	http.SetCookie(w, cookie)
}

type sharedPrincipalResponse struct {
	UserID             identity.ID `json:"user_id"`
	Username           string      `json:"username"`
	ProjectID          identity.ID `json:"project_id"`
	ProjectKey         string      `json:"project_key"`
	Permissions        []string    `json:"permissions"`
	ProjectAccessReady bool        `json:"project_access_ready"`
}

func sharedPrincipalView(p identity.Principal) sharedPrincipalResponse {
	keys := make([]string, 0, len(p.Permissions))
	for key, allowed := range p.Permissions {
		if allowed {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return sharedPrincipalResponse{UserID: p.UserID, Username: p.Username, ProjectID: p.ProjectID, ProjectKey: p.ProjectKey, Permissions: keys, ProjectAccessReady: true}
}

func sharedAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, identity.ErrUnauthenticated) || errors.Is(err, identity.ErrNotFound) {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	http.Error(w, "shared authentication unavailable", http.StatusServiceUnavailable)
}

func (s *Server) beginSharedLogin() bool {
	s.sharedLoginMu.Lock()
	defer s.sharedLoginMu.Unlock()
	// Each scrypt helper needs roughly 128 MiB. Do not let parallel clients
	// create an unbounded number of helpers or queued password-bearing requests.
	if s.sharedLogins >= 2 {
		return false
	}
	s.sharedLogins++
	return true
}

func (s *Server) endSharedLogin() {
	s.sharedLoginMu.Lock()
	s.sharedLogins--
	s.sharedLoginMu.Unlock()
}

func requireSharedJSON(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

func (s *Server) sharedLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireSharedJSON(w, r) {
		return
	}
	if blocked, retry := s.authBlocked(r.RemoteAddr, time.Now()); blocked {
		writeAuthRateLimit(w, retry)
		return
	}
	if !s.beginSharedLogin() {
		writeAuthRateLimit(w, time.Second)
		return
	}
	defer s.endSharedLogin()
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&request)
	request.Username = strings.TrimSpace(request.Username)
	if err != nil || decoder.Decode(new(any)) != io.EOF || request.Username == "" || len(request.Username) > 128 || len(request.Password) == 0 || len(request.Password) > 4096 {
		s.authRecordFailure(r.RemoteAddr, time.Now())
		http.Error(w, "invalid login request", http.StatusBadRequest)
		return
	}
	// Reserve the attempt before the expensive verification, so parallel
	// requests cannot all pass the failure limiter before any are recorded.
	s.authRecordFailure(r.RemoteAddr, time.Now())
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	user, lookupErr := s.Identity.UserByUsername(ctx, request.Username)
	if lookupErr != nil && !errors.Is(lookupErr, identity.ErrNotFound) {
		sharedAuthError(w, lookupErr)
		return
	}
	hash := sharedDummyPasswordHash
	if lookupErr == nil {
		hash = user.PasswordHash
	}
	verified, err := identity.VerifyPassword(ctx, request.Password, hash)
	request.Password = ""
	if err != nil {
		sharedAuthError(w, err)
		return
	}
	if lookupErr != nil || !verified || !user.Enabled {
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}
	principal, err := identity.ResolveUserPrincipal(ctx, s.Identity, s.Config.SharedProjectID, user)
	if err != nil {
		if errors.Is(err, identity.ErrUnauthenticated) {
			http.Error(w, "invalid username or password", http.StatusUnauthorized)
		} else {
			sharedAuthError(w, err)
		}
		return
	}
	if err := identity.RevokeBrowserSession(ctx, s.Identity, s.sharedCookieToken(r), time.Now()); err != nil {
		sharedAuthError(w, err)
		return
	}
	token, session, err := identity.CreateLoginBrowserSession(ctx, s.Identity, principal, user.PasswordHash, time.Now())
	if err != nil {
		sharedAuthError(w, err)
		return
	}
	s.authRecordSuccess(r.RemoteAddr)
	s.setSharedCookie(w, token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, sharedPrincipalView(principal))
}

func (s *Server) sharedLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireSharedJSON(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := identity.RevokeBrowserSession(ctx, s.Identity, s.sharedCookieToken(r), time.Now()); err != nil {
		sharedAuthError(w, err)
		return
	}
	s.setSharedCookie(w, "", time.Unix(1, 0))
	w.WriteHeader(http.StatusNoContent)
}
