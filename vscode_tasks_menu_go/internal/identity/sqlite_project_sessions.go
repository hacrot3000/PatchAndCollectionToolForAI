package identity

import (
	"context"
	"fmt"
	"time"
)

func (d *sqliteDatabase) RevokeProjectSessions(ctx context.Context, projectID ID, revokedAt time.Time) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if projectID == "" || revokedAt.IsZero() {
		return fmt.Errorf("revoke project sessions requires project_id and revoked_at")
	}
	_, err := d.db.ExecContext(ctx, `
UPDATE auth_sessions
SET revoked_at = ?
WHERE project_id = ? AND revoked_at IS NULL
`, revokedAt.UTC().Format(time.RFC3339Nano), string(projectID))
	if err != nil {
		return fmt.Errorf("revoke project auth sessions: %w", err)
	}
	return nil
}
