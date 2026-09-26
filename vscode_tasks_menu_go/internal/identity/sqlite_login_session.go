package identity

import (
	"context"
	"fmt"
	"time"
)

func (d *sqliteDatabase) CreateLoginSession(ctx context.Context, session AuthSession, projectID ID, verifiedPasswordHash string) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if session.ID == "" || session.UserID == "" || session.TokenHash == "" || projectID == "" || verifiedPasswordHash == "" ||
		session.CreatedAt.IsZero() || session.LastSeenAt.IsZero() || !session.ExpiresAt.After(session.CreatedAt) || session.RevokedAt != nil {
		return fmt.Errorf("invalid login session")
	}
	result, err := d.db.ExecContext(ctx, `
INSERT INTO auth_sessions(id, user_id, token_hash, created_at, expires_at, last_seen_at)
SELECT ?, users.id, ?, ?, ?, ?
FROM users
JOIN project_members ON project_members.user_id = users.id
JOIN projects ON projects.id = project_members.project_id
WHERE users.id = ? AND users.password_hash = ? AND users.enabled = 1
  AND projects.id = ? AND projects.enabled = 1 AND project_members.enabled = 1
`, string(session.ID), session.TokenHash, session.CreatedAt.UTC().Format(time.RFC3339Nano),
		session.ExpiresAt.UTC().Format(time.RFC3339Nano), session.LastSeenAt.UTC().Format(time.RFC3339Nano),
		string(session.UserID), verifiedPasswordHash, string(projectID))
	if err != nil {
		return fmt.Errorf("create verified login session: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrUnauthenticated
	}
	return nil
}
