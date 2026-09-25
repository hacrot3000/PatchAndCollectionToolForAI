package identity

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (d *sqliteDatabase) CreateUser(ctx context.Context, user User) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if user.ID == "" || strings.TrimSpace(user.Username) == "" || user.PasswordHash == "" {
		return fmt.Errorf("identity user requires id, username and password_hash")
	}
	if user.CreatedAt.IsZero() || user.UpdatedAt.IsZero() {
		return fmt.Errorf("identity user requires created_at and updated_at")
	}

	var lastLoginAt any
	if user.LastLoginAt != nil {
		lastLoginAt = user.LastLoginAt.UTC().Format(time.RFC3339Nano)
	}
	var passwordChangedAt any
	if user.PasswordChangedAt != nil {
		passwordChangedAt = user.PasswordChangedAt.UTC().Format(time.RFC3339Nano)
	}

	result, err := d.db.ExecContext(ctx, `
INSERT INTO users(
    id, username, display_name, password_hash, enabled,
    created_at, updated_at, last_login_at, password_changed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(username) DO NOTHING
`,
		string(user.ID),
		strings.TrimSpace(user.Username),
		user.DisplayName,
		user.PasswordHash,
		encodeDBBool(user.Enabled),
		user.CreatedAt.UTC().Format(time.RFC3339Nano),
		user.UpdatedAt.UTC().Format(time.RFC3339Nano),
		lastLoginAt,
		passwordChangedAt,
	)
	if err != nil {
		return fmt.Errorf("create identity user: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read created identity user count: %w", err)
	}
	if affected == 0 {
		return ErrConflict
	}
	return nil
}

func (d *sqliteDatabase) SetUserEnabled(ctx context.Context, userID ID, enabled bool, updatedAt time.Time) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if userID == "" || updatedAt.IsZero() {
		return fmt.Errorf("set identity user enabled requires user_id and updated_at")
	}
	result, err := d.db.ExecContext(ctx, `
UPDATE users
SET enabled = ?, updated_at = ?
WHERE id = ?
`, encodeDBBool(enabled), updatedAt.UTC().Format(time.RFC3339Nano), string(userID))
	if err != nil {
		return fmt.Errorf("set identity user enabled: %w", err)
	}
	return requireChangedRow(result, "identity user")
}

func (d *sqliteDatabase) SetUserPasswordHash(ctx context.Context, userID ID, passwordHash string, changedAt time.Time) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if userID == "" || passwordHash == "" || changedAt.IsZero() {
		return fmt.Errorf("set identity user password requires user_id, password_hash and changed_at")
	}
	result, err := d.db.ExecContext(ctx, `
UPDATE users
SET password_hash = ?, password_changed_at = ?, updated_at = ?
WHERE id = ?
`,
		passwordHash,
		changedAt.UTC().Format(time.RFC3339Nano),
		changedAt.UTC().Format(time.RFC3339Nano),
		string(userID),
	)
	if err != nil {
		return fmt.Errorf("set identity user password: %w", err)
	}
	return requireChangedRow(result, "identity user")
}

func (d *sqliteDatabase) EnsureProject(ctx context.Context, project Project) (Project, error) {
	if d == nil || d.db == nil {
		return Project{}, fmt.Errorf("identity DB is not open")
	}
	project.Key = strings.TrimSpace(project.Key)
	if project.ID == "" || project.Key == "" {
		return Project{}, fmt.Errorf("identity project requires id and project_key")
	}
	if project.CreatedAt.IsZero() || project.UpdatedAt.IsZero() {
		return Project{}, fmt.Errorf("identity project requires created_at and updated_at")
	}

	_, err := d.db.ExecContext(ctx, `
INSERT INTO projects(id, project_key, display_name, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(project_key) DO NOTHING
`,
		string(project.ID),
		project.Key,
		project.DisplayName,
		encodeDBBool(project.Enabled),
		project.CreatedAt.UTC().Format(time.RFC3339Nano),
		project.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return Project{}, fmt.Errorf("ensure identity project: %w", err)
	}
	return d.ProjectByKey(ctx, project.Key)
}

func (d *sqliteDatabase) SetProjectEnabled(ctx context.Context, projectID ID, enabled bool, updatedAt time.Time) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if projectID == "" || updatedAt.IsZero() {
		return fmt.Errorf("set identity project enabled requires project_id and updated_at")
	}
	result, err := d.db.ExecContext(ctx, `
UPDATE projects
SET enabled = ?, updated_at = ?
WHERE id = ?
`, encodeDBBool(enabled), updatedAt.UTC().Format(time.RFC3339Nano), string(projectID))
	if err != nil {
		return fmt.Errorf("set identity project enabled: %w", err)
	}
	return requireChangedRow(result, "identity project")
}

func encodeDBBool(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
