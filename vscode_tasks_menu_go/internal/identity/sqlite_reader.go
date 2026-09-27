package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var _ Reader = (*sqliteDatabase)(nil)

type rowScanner interface {
	Scan(dest ...any) error
}

func (d *sqliteDatabase) UserByID(ctx context.Context, id ID) (User, error) {
	if d == nil || d.db == nil {
		return User{}, fmt.Errorf("identity DB is not open")
	}
	return scanUser(d.db.QueryRowContext(ctx, `
SELECT id, username, display_name, password_hash, enabled,
       created_at, updated_at, last_login_at, password_changed_at
FROM users
WHERE id = ?
`, string(id)))
}

func (d *sqliteDatabase) UserByUsername(ctx context.Context, username string) (User, error) {
	if d == nil || d.db == nil {
		return User{}, fmt.Errorf("identity DB is not open")
	}
	return scanUser(d.db.QueryRowContext(ctx, `
SELECT id, username, display_name, password_hash, enabled,
       created_at, updated_at, last_login_at, password_changed_at
FROM users
WHERE username = ?
`, username))
}

func scanUser(row rowScanner) (User, error) {
	var (
		user            User
		id              string
		enabled         int64
		createdAt       string
		updatedAt       string
		lastLoginAt     sql.NullString
		passwordChanged sql.NullString
	)
	if err := row.Scan(
		&id,
		&user.Username,
		&user.DisplayName,
		&user.PasswordHash,
		&enabled,
		&createdAt,
		&updatedAt,
		&lastLoginAt,
		&passwordChanged,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("read identity user: %w", err)
	}

	var err error
	user.ID = ID(id)
	if user.Enabled, err = decodeDBBool(enabled); err != nil {
		return User{}, fmt.Errorf("read identity user enabled: %w", err)
	}
	if user.CreatedAt, err = parseDBTime(createdAt); err != nil {
		return User{}, fmt.Errorf("read identity user created_at: %w", err)
	}
	if user.UpdatedAt, err = parseDBTime(updatedAt); err != nil {
		return User{}, fmt.Errorf("read identity user updated_at: %w", err)
	}
	if user.LastLoginAt, err = parseNullableDBTime(lastLoginAt); err != nil {
		return User{}, fmt.Errorf("read identity user last_login_at: %w", err)
	}
	if user.PasswordChangedAt, err = parseNullableDBTime(passwordChanged); err != nil {
		return User{}, fmt.Errorf("read identity user password_changed_at: %w", err)
	}
	return user, nil
}

func (d *sqliteDatabase) ProjectByKey(ctx context.Context, projectKey string) (Project, error) {
	if d == nil || d.db == nil {
		return Project{}, fmt.Errorf("identity DB is not open")
	}

	var (
		project   Project
		id        string
		enabled   int64
		createdAt string
		updatedAt string
	)
	err := d.db.QueryRowContext(ctx, `
SELECT id, project_key, display_name, enabled, created_at, updated_at
FROM projects
WHERE project_key = ?
`, projectKey).Scan(
		&id,
		&project.Key,
		&project.DisplayName,
		&enabled,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("read identity project: %w", err)
	}

	project.ID = ID(id)
	if project.Enabled, err = decodeDBBool(enabled); err != nil {
		return Project{}, fmt.Errorf("read identity project enabled: %w", err)
	}
	if project.CreatedAt, err = parseDBTime(createdAt); err != nil {
		return Project{}, fmt.Errorf("read identity project created_at: %w", err)
	}
	if project.UpdatedAt, err = parseDBTime(updatedAt); err != nil {
		return Project{}, fmt.Errorf("read identity project updated_at: %w", err)
	}
	return project, nil
}

func (d *sqliteDatabase) ProjectMember(ctx context.Context, projectID, userID ID) (ProjectMember, error) {
	if d == nil || d.db == nil {
		return ProjectMember{}, fmt.Errorf("identity DB is not open")
	}

	var (
		member    ProjectMember
		project   string
		user      string
		role      string
		enabled   int64
		createdAt string
		updatedAt string
	)
	err := d.db.QueryRowContext(ctx, `
SELECT project_id, user_id, role_id, enabled, created_at, updated_at
FROM project_members
WHERE project_id = ? AND user_id = ?
`, string(projectID), string(userID)).Scan(
		&project,
		&user,
		&role,
		&enabled,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectMember{}, ErrNotFound
	}
	if err != nil {
		return ProjectMember{}, fmt.Errorf("read identity project member: %w", err)
	}

	member.ProjectID = ID(project)
	member.UserID = ID(user)
	member.RoleID = ID(role)
	if member.Enabled, err = decodeDBBool(enabled); err != nil {
		return ProjectMember{}, fmt.Errorf("read identity project member enabled: %w", err)
	}
	if member.CreatedAt, err = parseDBTime(createdAt); err != nil {
		return ProjectMember{}, fmt.Errorf("read identity project member created_at: %w", err)
	}
	if member.UpdatedAt, err = parseDBTime(updatedAt); err != nil {
		return ProjectMember{}, fmt.Errorf("read identity project member updated_at: %w", err)
	}
	return member, nil
}

func (d *sqliteDatabase) EffectivePermissions(ctx context.Context, projectID, userID ID) (map[string]bool, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("identity DB is not open")
	}

	var (
		userEnabled    int64
		projectEnabled int64
		memberEnabled  int64
		roleID         string
	)
	err := d.db.QueryRowContext(ctx, `
SELECT users.enabled, projects.enabled, project_members.enabled, project_members.role_id
FROM project_members
JOIN users ON users.id = project_members.user_id
JOIN projects ON projects.id = project_members.project_id
WHERE project_members.project_id = ? AND project_members.user_id = ?
`, string(projectID), string(userID)).Scan(
		&userEnabled,
		&projectEnabled,
		&memberEnabled,
		&roleID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolve identity membership state: %w", err)
	}

	userOK, err := decodeDBBool(userEnabled)
	if err != nil {
		return nil, fmt.Errorf("read identity user enabled: %w", err)
	}
	projectOK, err := decodeDBBool(projectEnabled)
	if err != nil {
		return nil, fmt.Errorf("read identity project enabled: %w", err)
	}
	memberOK, err := decodeDBBool(memberEnabled)
	if err != nil {
		return nil, fmt.Errorf("read identity membership enabled: %w", err)
	}
	if !userOK || !projectOK || !memberOK {
		return map[string]bool{}, nil
	}

	permissions := make(map[string]bool)

	rows, err := d.db.QueryContext(ctx, `
SELECT permissions.permission_key
FROM role_permissions
JOIN permissions ON permissions.id = role_permissions.permission_id
WHERE role_permissions.role_id = ?
ORDER BY permissions.permission_key
`, roleID)
	if err != nil {
		return nil, fmt.Errorf("read identity role permissions: %w", err)
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan identity role permission: %w", err)
		}
		permissions[key] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate identity role permissions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close identity role permissions: %w", err)
	}

	rows, err = d.db.QueryContext(ctx, `
SELECT permissions.permission_key, member_permissions.effect
FROM member_permissions
JOIN permissions ON permissions.id = member_permissions.permission_id
WHERE member_permissions.project_id = ? AND member_permissions.user_id = ?
ORDER BY permissions.permission_key
`, string(projectID), string(userID))
	if err != nil {
		return nil, fmt.Errorf("read identity member permissions: %w", err)
	}
	for rows.Next() {
		var (
			key    string
			effect string
		)
		if err := rows.Scan(&key, &effect); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan identity member permission: %w", err)
		}
		switch PermissionEffect(effect) {
		case PermissionAllow:
			permissions[key] = true
		case PermissionDeny:
			delete(permissions, key)
		default:
			rows.Close()
			return nil, fmt.Errorf("invalid identity permission effect %q", effect)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate identity member permissions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close identity member permissions: %w", err)
	}
	return permissions, nil
}

func decodeDBBool(value int64) (bool, error) {
	switch value {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("invalid boolean value %d", value)
	}
}

func parseDBTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func parseNullableDBTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseDBTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
