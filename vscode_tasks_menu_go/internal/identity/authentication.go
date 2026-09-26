package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

const (
	SessionLifetime    = 12 * time.Hour
	SessionIdleTimeout = 30 * time.Minute
)

var ErrUnauthenticated = errors.New("authentication required")

type AuthenticationStore interface {
	Reader
	SessionStore
}

func randomID() (ID, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return ID(hex.EncodeToString(raw[:])), nil
}

// CreateBrowserSession returns the raw token only to the caller setting the
// cookie. Neither the stored session nor its ID contains that bearer token.
func CreateBrowserSession(ctx context.Context, store SessionStore, userID ID, now time.Time) (string, AuthSession, error) {
	token, session, err := newBrowserSession(userID, now)
	if err != nil {
		return "", AuthSession{}, err
	}
	if err := store.CreateAuthSession(ctx, session); err != nil {
		return "", AuthSession{}, err
	}
	return token, session, nil
}

// CreateLoginBrowserSession atomically checks the verified credentials and
// project access again when persisting the new session, closing the window in
// which a password reset/disable could occur during the expensive verifier.
func CreateLoginBrowserSession(ctx context.Context, store SessionStore, principal Principal, verifiedPasswordHash string, now time.Time) (string, AuthSession, error) {
	token, session, err := newBrowserSession(principal.UserID, now)
	if err != nil {
		return "", AuthSession{}, err
	}
	if err := store.CreateLoginSession(ctx, session, principal.ProjectID, verifiedPasswordHash); err != nil {
		return "", AuthSession{}, err
	}
	return token, session, nil
}

func newBrowserSession(userID ID, now time.Time) (string, AuthSession, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", AuthSession{}, err
	}
	id, err := randomID()
	if err != nil {
		return "", AuthSession{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	hash, _ := sessionTokenHash(token)
	session := AuthSession{
		ID: id, UserID: userID, TokenHash: hash,
		CreatedAt: now.UTC(), LastSeenAt: now.UTC(), ExpiresAt: now.UTC().Add(SessionLifetime),
	}
	return token, session, nil
}

func sessionTokenHash(token string) (string, error) {
	if len(token) != 43 {
		return "", ErrUnauthenticated
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != 32 {
		return "", ErrUnauthenticated
	}
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:]), nil
}

func authenticationError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrUnauthenticated
	}
	return err
}

// ResolveUserPrincipal checks enabled state independently of permissions: an
// enabled member with no permissions is distinct from a disabled membership.
func ResolveUserPrincipal(ctx context.Context, store Reader, projectKey string, user User) (Principal, error) {
	if !user.Enabled || user.ID == "" {
		return Principal{}, ErrUnauthenticated
	}
	project, err := store.ProjectByKey(ctx, projectKey)
	if err != nil {
		return Principal{}, authenticationError(err)
	}
	if !project.Enabled {
		return Principal{}, ErrUnauthenticated
	}
	member, err := store.ProjectMember(ctx, project.ID, user.ID)
	if err != nil {
		return Principal{}, authenticationError(err)
	}
	if !member.Enabled {
		return Principal{}, ErrUnauthenticated
	}
	permissions, err := store.EffectivePermissions(ctx, project.ID, user.ID)
	if err != nil {
		return Principal{}, authenticationError(err)
	}
	return Principal{UserID: user.ID, Username: user.Username, ProjectID: project.ID, ProjectKey: project.Key, Permissions: permissions}, nil
}

// AuthenticateBrowserSession re-resolves project access on every request. A
// cookie from another daemon never grants membership in this daemon's project.
func AuthenticateBrowserSession(ctx context.Context, store AuthenticationStore, projectKey, token string, now time.Time) (Principal, AuthSession, error) {
	hash, err := sessionTokenHash(token)
	if err != nil {
		return Principal{}, AuthSession{}, err
	}
	session, err := store.AuthSessionByTokenHash(ctx, hash)
	if err != nil {
		return Principal{}, AuthSession{}, authenticationError(err)
	}
	if session.RevokedAt != nil || !session.ExpiresAt.After(now) ||
		session.CreatedAt.After(now) || !session.CreatedAt.Add(SessionLifetime).After(now) ||
		!session.LastSeenAt.Add(SessionIdleTimeout).After(now) {
		return Principal{}, AuthSession{}, ErrUnauthenticated
	}
	user, err := store.UserByID(ctx, session.UserID)
	if err != nil {
		return Principal{}, AuthSession{}, authenticationError(err)
	}
	if user.PasswordChangedAt != nil && !user.PasswordChangedAt.Before(session.CreatedAt) {
		return Principal{}, AuthSession{}, ErrUnauthenticated
	}
	principal, err := ResolveUserPrincipal(ctx, store, projectKey, user)
	if err != nil {
		return Principal{}, AuthSession{}, err
	}
	// Bound write traffic while still enforcing idle expiration on every request.
	if now.Sub(session.LastSeenAt) >= time.Minute {
		if err := store.TouchAuthSession(ctx, session.ID, now); err != nil {
			return Principal{}, AuthSession{}, authenticationError(err)
		}
	}
	return principal, session, nil
}

// RevokeBrowserSession also works after membership removal/idle expiration.
func RevokeBrowserSession(ctx context.Context, store SessionStore, token string, now time.Time) error {
	hash, err := sessionTokenHash(token)
	if err != nil {
		return nil
	}
	session, err := store.AuthSessionByTokenHash(ctx, hash)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return store.RevokeAuthSession(ctx, session.ID, now)
}
