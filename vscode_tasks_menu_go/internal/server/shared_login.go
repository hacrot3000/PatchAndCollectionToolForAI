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
		s.appendSharedAudit(r, nil, nil, "auth.login", "user", "", "denied", map[string]any{"reason": "rate_limited"})
		writeAuthRateLimit(w, retry)
		return
	}
	if !s.beginSharedLogin() {
		s.appendSharedAudit(r, nil, nil, "auth.login", "user", "", "denied", map[string]any{"reason": "login_capacity"})
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
		s.appendSharedAudit(r, nil, nil, "auth.login", "user", "", "denied", map[string]any{"reason": "invalid_request"})
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
		s.appendSharedAudit(r, nil, nil, "auth.login", "user", "", "error", map[string]any{"reason": "identity_lookup"})
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
		s.appendSharedAudit(r, nil, nil, "auth.login", "user", "", "error", map[string]any{"reason": "password_verifier"})
		sharedAuthError(w, err)
		return
	}
	if lookupErr != nil || !verified || !user.Enabled {
		var auditUserID *identity.ID
		if lookupErr == nil {
			value := user.ID
			auditUserID = &value
		}
		s.appendSharedAudit(r, nil, auditUserID, "auth.login", "user", "", "denied", map[string]any{"reason": "invalid_credentials"})
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}
	principal, err := identity.ResolveUserPrincipal(ctx, s.Identity, s.Config.SharedProjectID, user)
	if err != nil {
		if errors.Is(err, identity.ErrUnauthenticated) {
			value := user.ID
			s.appendSharedAudit(r, nil, &value, "auth.login", "user", string(user.ID), "denied", map[string]any{"reason": "project_access"})
			http.Error(w, "invalid username or password", http.StatusUnauthorized)
		} else {
			sharedAuthError(w, err)
		}
		return
	}
	if err := identity.RevokeBrowserSession(ctx, s.Identity, s.sharedCookieToken(r), time.Now()); err != nil {
		s.appendSharedAudit(r, &principal, nil, "auth.login", "user", string(user.ID), "error", map[string]any{"reason": "session_rotation"})
		sharedAuthError(w, err)
		return
	}
	token, session, err := identity.CreateLoginBrowserSession(ctx, s.Identity, principal, user.PasswordHash, time.Now())
	if err != nil {
		s.appendSharedAudit(r, &principal, nil, "auth.login", "user", string(user.ID), "error", map[string]any{"reason": "session_create"})
		sharedAuthError(w, err)
		return
	}
	s.authRecordSuccess(r.RemoteAddr)
	s.appendSharedAudit(r, &principal, nil, "auth.login", "user", string(user.ID), "success", nil)
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
	var principal *identity.Principal
	if resolved, _, authErr := identity.AuthenticateBrowserSession(ctx, s.Identity, s.Config.SharedProjectID, s.sharedCookieToken(r), time.Now()); authErr == nil {
		principal = &resolved
	}
	if err := identity.RevokeBrowserSession(ctx, s.Identity, s.sharedCookieToken(r), time.Now()); err != nil {
		s.appendSharedAudit(r, principal, nil, "auth.logout", "session", "", "error", nil)
		sharedAuthError(w, err)
		return
	}
	s.setSharedCookie(w, "", time.Unix(1, 0))
	s.appendSharedAudit(r, principal, nil, "auth.logout", "session", "", "success", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sharedPasswordChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireSharedJSON(w, r) {
		return
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	rateKey := "password-change:" + string(principal.UserID) + ":" + authRemoteKey(r.RemoteAddr)
	if blocked, retry := s.authBlockedKey(rateKey, time.Now()); blocked {
		s.appendSharedAudit(r, &principal, nil, "auth.password_change", "user", string(principal.UserID), "denied", map[string]any{"reason": "rate_limited"})
		writeAuthRateLimit(w, retry)
		return
	}
	if !s.beginSharedLogin() {
		s.appendSharedAudit(r, &principal, nil, "auth.password_change", "user", string(principal.UserID), "denied", map[string]any{"reason": "scrypt_capacity"})
		writeAuthRateLimit(w, time.Second)
		return
	}
	defer s.endSharedLogin()

	var request struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&request)
	if err != nil || decoder.Decode(new(any)) != io.EOF || request.CurrentPassword == "" {
		request.CurrentPassword = ""
		request.NewPassword = ""
		http.Error(w, "invalid password change request", http.StatusBadRequest)
		return
	}
	if err := identity.ValidateNewPassword(request.NewPassword); err != nil {
		request.CurrentPassword = ""
		request.NewPassword = ""
		http.Error(w, "invalid new password", http.StatusBadRequest)
		return
	}
	if request.CurrentPassword == request.NewPassword {
		request.CurrentPassword = ""
		request.NewPassword = ""
		http.Error(w, "new password must differ from current password", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	user, err := s.Identity.UserByID(ctx, principal.UserID)
	if err != nil {
		request.CurrentPassword = ""
		request.NewPassword = ""
		s.appendSharedAudit(r, &principal, nil, "auth.password_change", "user", string(principal.UserID), "error", map[string]any{"reason": "identity_lookup"})
		sharedAuthError(w, err)
		return
	}
	s.authRecordFailureKey(rateKey, time.Now())
	verified, err := identity.VerifyPassword(ctx, request.CurrentPassword, user.PasswordHash)
	request.CurrentPassword = ""
	if err != nil {
		request.NewPassword = ""
		s.appendSharedAudit(r, &principal, nil, "auth.password_change", "user", string(principal.UserID), "error", map[string]any{"reason": "password_verifier"})
		sharedAuthError(w, err)
		return
	}
	if !verified {
		request.NewPassword = ""
		s.appendSharedAudit(r, &principal, nil, "auth.password_change", "user", string(principal.UserID), "denied", map[string]any{"reason": "invalid_current_password"})
		http.Error(w, "current password is incorrect", http.StatusUnauthorized)
		return
	}
	s.authRecordSuccessKey(rateKey)

	nextHash, err := identity.HashPassword(ctx, request.NewPassword)
	request.NewPassword = ""
	if err != nil {
		s.appendSharedAudit(r, &principal, nil, "auth.password_change", "user", string(principal.UserID), "error", map[string]any{"reason": "password_hash"})
		sharedAuthError(w, err)
		return
	}
	changedAt := time.Now().UTC()
	if err := s.Identity.ChangeUserPasswordHash(ctx, principal.UserID, user.PasswordHash, nextHash, changedAt); err != nil {
		if errors.Is(err, identity.ErrConflict) {
			s.setSharedCookie(w, "", time.Unix(1, 0))
			s.appendSharedAudit(r, &principal, nil, "auth.password_change", "user", string(principal.UserID), "denied", map[string]any{"reason": "concurrent_change"})
			http.Error(w, "password changed concurrently; sign in again", http.StatusConflict)
			return
		}
		s.appendSharedAudit(r, &principal, nil, "auth.password_change", "user", string(principal.UserID), "error", map[string]any{"reason": "identity_store"})
		sharedAuthError(w, err)
		return
	}

	s.appendSharedAudit(r, &principal, nil, "auth.password_change", "user", string(principal.UserID), "success", nil)
	s.setSharedCookie(w, "", time.Unix(1, 0))
	w.WriteHeader(http.StatusNoContent)
}
