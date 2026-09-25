package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var _ SessionStore = (*sqliteDatabase)(nil)

func (d *sqliteDatabase) CreateAuthSession(ctx context.Context, session AuthSession) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if session.ID == "" || session.UserID == "" || session.TokenHash == "" {
		return fmt.Errorf("auth session requires id, user_id and token_hash")
	}
	if session.CreatedAt.IsZero() || session.ExpiresAt.IsZero() || session.LastSeenAt.IsZero() {
		return fmt.Errorf("auth session requires created_at, expires_at and last_seen_at")
	}
	if !session.ExpiresAt.After(session.CreatedAt) {
		return fmt.Errorf("auth session expires_at must be after created_at")
	}

	var revokedAt any
	if session.RevokedAt != nil {
		revokedAt = session.RevokedAt.UTC().Format(time.RFC3339Nano)
	}

	_, err := d.db.ExecContext(ctx, `
INSERT INTO auth_sessions(
    id, user_id, token_hash, created_at, expires_at, last_seen_at, revoked_at, client_metadata
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`,
		string(session.ID),
		string(session.UserID),
		session.TokenHash,
		session.CreatedAt.UTC().Format(time.RFC3339Nano),
		session.ExpiresAt.UTC().Format(time.RFC3339Nano),
		session.LastSeenAt.UTC().Format(time.RFC3339Nano),
		revokedAt,
		session.ClientMetadata,
	)
	if err != nil {
		return fmt.Errorf("create identity auth session: %w", err)
	}
	return nil
}

func (d *sqliteDatabase) AuthSessionByTokenHash(ctx context.Context, tokenHash string) (AuthSession, error) {
	if d == nil || d.db == nil {
		return AuthSession{}, fmt.Errorf("identity DB is not open")
	}
	if tokenHash == "" {
		return AuthSession{}, ErrNotFound
	}

	var (
		session    AuthSession
		id         string
		userID     string
		createdAt  string
		expiresAt  string
		lastSeenAt string
		revokedAt  sql.NullString
	)
	err := d.db.QueryRowContext(ctx, `
SELECT id, user_id, token_hash, created_at, expires_at, last_seen_at, revoked_at, client_metadata
FROM auth_sessions
WHERE token_hash = ?
`, tokenHash).Scan(
		&id,
		&userID,
		&session.TokenHash,
		&createdAt,
		&expiresAt,
		&lastSeenAt,
		&revokedAt,
		&session.ClientMetadata,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthSession{}, ErrNotFound
	}
	if err != nil {
		return AuthSession{}, fmt.Errorf("read identity auth session: %w", err)
	}

	session.ID = ID(id)
	session.UserID = ID(userID)
	if session.CreatedAt, err = parseDBTime(createdAt); err != nil {
		return AuthSession{}, fmt.Errorf("read auth session created_at: %w", err)
	}
	if session.ExpiresAt, err = parseDBTime(expiresAt); err != nil {
		return AuthSession{}, fmt.Errorf("read auth session expires_at: %w", err)
	}
	if session.LastSeenAt, err = parseDBTime(lastSeenAt); err != nil {
		return AuthSession{}, fmt.Errorf("read auth session last_seen_at: %w", err)
	}
	if session.RevokedAt, err = parseNullableDBTime(revokedAt); err != nil {
		return AuthSession{}, fmt.Errorf("read auth session revoked_at: %w", err)
	}
	return session, nil
}

func (d *sqliteDatabase) TouchAuthSession(ctx context.Context, sessionID ID, seenAt time.Time) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if sessionID == "" || seenAt.IsZero() {
		return fmt.Errorf("touch auth session requires id and seen_at")
	}

	result, err := d.db.ExecContext(ctx, `
UPDATE auth_sessions
SET last_seen_at = ?
WHERE id = ? AND revoked_at IS NULL
`, seenAt.UTC().Format(time.RFC3339Nano), string(sessionID))
	if err != nil {
		return fmt.Errorf("touch identity auth session: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read touched auth session count: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *sqliteDatabase) RevokeAuthSession(ctx context.Context, sessionID ID, revokedAt time.Time) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if sessionID == "" || revokedAt.IsZero() {
		return fmt.Errorf("revoke auth session requires id and revoked_at")
	}

	_, err := d.db.ExecContext(ctx, `
UPDATE auth_sessions
SET revoked_at = ?
WHERE id = ? AND revoked_at IS NULL
`, revokedAt.UTC().Format(time.RFC3339Nano), string(sessionID))
	if err != nil {
		return fmt.Errorf("revoke identity auth session: %w", err)
	}
	return nil
}

func (d *sqliteDatabase) RevokeUserSessions(ctx context.Context, userID ID, revokedAt time.Time) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if userID == "" || revokedAt.IsZero() {
		return fmt.Errorf("revoke user sessions requires user_id and revoked_at")
	}

	_, err := d.db.ExecContext(ctx, `
UPDATE auth_sessions
SET revoked_at = ?
WHERE user_id = ? AND revoked_at IS NULL
`, revokedAt.UTC().Format(time.RFC3339Nano), string(userID))
	if err != nil {
		return fmt.Errorf("revoke identity user sessions: %w", err)
	}
	return nil
}
