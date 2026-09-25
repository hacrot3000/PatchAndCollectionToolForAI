package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	_ AdminStore = (*sqliteDatabase)(nil)
	_ Store      = (*sqliteDatabase)(nil)
)

func (d *sqliteDatabase) CreateRole(ctx context.Context, role Role) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	role.Name = strings.TrimSpace(role.Name)
	if role.ID == "" || role.Name == "" {
		return fmt.Errorf("identity role requires id and name")
	}

	result, err := d.db.ExecContext(ctx, `
INSERT INTO roles(id, name, description, system_role)
VALUES (?, ?, ?, ?)
ON CONFLICT(name) DO NOTHING
`, string(role.ID), role.Name, role.Description, encodeDBBool(role.SystemRole))
	if err != nil {
		return fmt.Errorf("create identity role: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read created identity role count: %w", err)
	}
	if affected == 0 {
		return ErrConflict
	}
	return nil
}

func (d *sqliteDatabase) SetRolePermissions(ctx context.Context, roleID ID, permissionKeys []string) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if roleID == "" {
		return fmt.Errorf("set role permissions requires role_id")
	}

	keys, err := normalizePermissionKeys(permissionKeys)
	if err != nil {
		return err
	}

	conn, err := d.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve role permission connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin role permission update: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	var exists int
	err = conn.QueryRowContext(ctx, "SELECT 1 FROM roles WHERE id = ?", string(roleID)).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read identity role: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "DELETE FROM role_permissions WHERE role_id = ?", string(roleID)); err != nil {
		return fmt.Errorf("clear identity role permissions: %w", err)
	}
	for _, key := range keys {
		result, err := conn.ExecContext(ctx, `
INSERT INTO role_permissions(role_id, permission_id)
SELECT ?, id FROM permissions WHERE permission_key = ?
`, string(roleID), key)
		if err != nil {
			return fmt.Errorf("add identity role permission %q: %w", key, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("read identity role permission %q count: %w", key, err)
		}
		if affected == 0 {
			return fmt.Errorf("%w: permission %q", ErrNotFound, key)
		}
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit role permission update: %w", err)
	}
	committed = true
	return nil
}

func (d *sqliteDatabase) UpsertProjectMember(ctx context.Context, member ProjectMember) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if member.ProjectID == "" || member.UserID == "" || member.RoleID == "" {
		return fmt.Errorf("project member requires project_id, user_id and role_id")
	}
	if member.CreatedAt.IsZero() || member.UpdatedAt.IsZero() {
		return fmt.Errorf("project member requires created_at and updated_at")
	}

	_, err := d.db.ExecContext(ctx, `
INSERT INTO project_members(
    project_id, user_id, role_id, enabled, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, user_id) DO UPDATE SET
    role_id = excluded.role_id,
    enabled = excluded.enabled,
    updated_at = excluded.updated_at
`,
		string(member.ProjectID),
		string(member.UserID),
		string(member.RoleID),
		encodeDBBool(member.Enabled),
		member.CreatedAt.UTC().Format(timeFormat),
		member.UpdatedAt.UTC().Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("upsert identity project member: %w", err)
	}
	return nil
}

func (d *sqliteDatabase) SetMemberPermission(ctx context.Context, override MemberPermission) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if override.ProjectID == "" || override.UserID == "" || override.PermissionID == "" {
		return fmt.Errorf("member permission requires project_id, user_id and permission_id")
	}
	switch override.Effect {
	case PermissionAllow, PermissionDeny:
	default:
		return fmt.Errorf("invalid member permission effect %q", override.Effect)
	}

	_, err := d.db.ExecContext(ctx, `
INSERT INTO member_permissions(project_id, user_id, permission_id, effect)
VALUES (?, ?, ?, ?)
ON CONFLICT(project_id, user_id, permission_id) DO UPDATE SET
    effect = excluded.effect
`,
		string(override.ProjectID),
		string(override.UserID),
		string(override.PermissionID),
		string(override.Effect),
	)
	if err != nil {
		return fmt.Errorf("set identity member permission: %w", err)
	}
	return nil
}

func (d *sqliteDatabase) DeleteMemberPermission(ctx context.Context, projectID, userID, permissionID ID) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if projectID == "" || userID == "" || permissionID == "" {
		return fmt.Errorf("delete member permission requires project_id, user_id and permission_id")
	}

	result, err := d.db.ExecContext(ctx, `
DELETE FROM member_permissions
WHERE project_id = ? AND user_id = ? AND permission_id = ?
`, string(projectID), string(userID), string(permissionID))
	if err != nil {
		return fmt.Errorf("delete identity member permission: %w", err)
	}
	return requireChangedRow(result, "identity member permission")
}

func normalizePermissionKeys(keys []string) ([]string, error) {
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("permission key cannot be empty")
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out, nil
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"
