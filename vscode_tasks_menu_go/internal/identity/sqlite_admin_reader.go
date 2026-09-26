package identity

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

var _ AdminReader = (*sqliteDatabase)(nil)

func (d *sqliteDatabase) ListProjectMembers(ctx context.Context, projectID ID) ([]ProjectMemberDetails, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("identity DB is not open")
	}
	if projectID == "" {
		return nil, fmt.Errorf("list project members requires project_id")
	}
	rows, err := d.db.QueryContext(ctx, `
SELECT users.id, users.username, users.display_name, users.enabled,
       users.created_at, users.updated_at, users.last_login_at, users.password_changed_at,
       project_members.role_id, roles.name, project_members.enabled,
       project_members.created_at, project_members.updated_at
FROM project_members
JOIN users ON users.id = project_members.user_id
JOIN roles ON roles.id = project_members.role_id
WHERE project_members.project_id = ?
ORDER BY users.username COLLATE NOCASE, users.id
`, string(projectID))
	if err != nil {
		return nil, fmt.Errorf("query project members: %w", err)
	}
	members := make([]ProjectMemberDetails, 0)
	byUser := make(map[ID]int)
	for rows.Next() {
		var member ProjectMemberDetails
		var userID, roleID string
		var userEnabled, memberEnabled int64
		var userCreated, userUpdated, memberCreated, memberUpdated string
		var lastLogin, passwordChanged sql.NullString
		if err := rows.Scan(
			&userID, &member.Username, &member.DisplayName, &userEnabled,
			&userCreated, &userUpdated, &lastLogin, &passwordChanged,
			&roleID, &member.RoleName, &memberEnabled, &memberCreated, &memberUpdated,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan project member: %w", err)
		}
		member.ProjectID = projectID
		member.UserID = ID(userID)
		member.RoleID = ID(roleID)
		member.PermissionOverrides = map[string]PermissionEffect{}
		if member.UserEnabled, err = decodeDBBool(userEnabled); err != nil {
			rows.Close()
			return nil, err
		}
		if member.MemberEnabled, err = decodeDBBool(memberEnabled); err != nil {
			rows.Close()
			return nil, err
		}
		if member.UserCreatedAt, err = parseDBTime(userCreated); err != nil {
			rows.Close()
			return nil, err
		}
		if member.UserUpdatedAt, err = parseDBTime(userUpdated); err != nil {
			rows.Close()
			return nil, err
		}
		if member.MemberCreatedAt, err = parseDBTime(memberCreated); err != nil {
			rows.Close()
			return nil, err
		}
		if member.MemberUpdatedAt, err = parseDBTime(memberUpdated); err != nil {
			rows.Close()
			return nil, err
		}
		if member.LastLoginAt, err = parseNullableDBTime(lastLogin); err != nil {
			rows.Close()
			return nil, err
		}
		if member.PasswordChangedAt, err = parseNullableDBTime(passwordChanged); err != nil {
			rows.Close()
			return nil, err
		}
		byUser[member.UserID] = len(members)
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate project members: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close project members: %w", err)
	}

	overrides, err := d.db.QueryContext(ctx, `
SELECT member_permissions.user_id, permissions.permission_key, member_permissions.effect
FROM member_permissions
JOIN permissions ON permissions.id = member_permissions.permission_id
WHERE member_permissions.project_id = ?
ORDER BY member_permissions.user_id, permissions.permission_key
`, string(projectID))
	if err != nil {
		return nil, fmt.Errorf("query project member permission overrides: %w", err)
	}
	defer overrides.Close()
	for overrides.Next() {
		var userID, key, effect string
		if err := overrides.Scan(&userID, &key, &effect); err != nil {
			return nil, fmt.Errorf("scan project member permission override: %w", err)
		}
		index, ok := byUser[ID(userID)]
		if !ok {
			continue
		}
		value := PermissionEffect(effect)
		if value != PermissionAllow && value != PermissionDeny {
			return nil, fmt.Errorf("invalid identity permission effect %q", effect)
		}
		members[index].PermissionOverrides[key] = value
	}
	if err := overrides.Err(); err != nil {
		return nil, fmt.Errorf("iterate project member permission overrides: %w", err)
	}
	return members, nil
}

func (d *sqliteDatabase) ListRoles(ctx context.Context) ([]RoleDetails, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("identity DB is not open")
	}
	rows, err := d.db.QueryContext(ctx, `
SELECT id, name, description, system_role
FROM roles
ORDER BY system_role DESC, name COLLATE NOCASE, id
`)
	if err != nil {
		return nil, fmt.Errorf("query identity roles: %w", err)
	}
	roles := make([]RoleDetails, 0)
	byID := make(map[ID]int)
	for rows.Next() {
		var role RoleDetails
		var id string
		var systemRole int64
		if err := rows.Scan(&id, &role.Name, &role.Description, &systemRole); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan identity role: %w", err)
		}
		role.ID = ID(id)
		if role.SystemRole, err = decodeDBBool(systemRole); err != nil {
			rows.Close()
			return nil, err
		}
		role.Permissions = []string{}
		byID[role.ID] = len(roles)
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate identity roles: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close identity roles: %w", err)
	}
	permissions, err := d.db.QueryContext(ctx, `
SELECT role_permissions.role_id, permissions.permission_key
FROM role_permissions
JOIN permissions ON permissions.id = role_permissions.permission_id
ORDER BY role_permissions.role_id, permissions.permission_key
`)
	if err != nil {
		return nil, fmt.Errorf("query identity role permissions: %w", err)
	}
	defer permissions.Close()
	for permissions.Next() {
		var roleID, key string
		if err := permissions.Scan(&roleID, &key); err != nil {
			return nil, fmt.Errorf("scan identity role permission: %w", err)
		}
		if index, ok := byID[ID(roleID)]; ok {
			roles[index].Permissions = append(roles[index].Permissions, key)
		}
	}
	if err := permissions.Err(); err != nil {
		return nil, fmt.Errorf("iterate identity role permissions: %w", err)
	}
	return roles, nil
}

func (d *sqliteDatabase) ListAuthSessions(ctx context.Context, query AuthSessionQuery) ([]AuthSession, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("identity DB is not open")
	}
	if query.ProjectID == "" {
		return nil, fmt.Errorf("list auth sessions requires project_id")
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	where := []string{"project_members.project_id = ?"}
	args := []any{string(query.ProjectID)}
	if query.UserID != "" {
		where = append(where, "auth_sessions.user_id = ?")
		args = append(args, string(query.UserID))
	}
	if !query.IncludeRevoked {
		where = append(where, "auth_sessions.revoked_at IS NULL")
	}
	statement := `
SELECT auth_sessions.id, auth_sessions.user_id, auth_sessions.token_hash,
       auth_sessions.created_at, auth_sessions.expires_at, auth_sessions.last_seen_at,
       auth_sessions.revoked_at, auth_sessions.client_metadata
FROM auth_sessions
JOIN project_members ON project_members.user_id = auth_sessions.user_id
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY auth_sessions.created_at DESC, auth_sessions.id DESC
LIMIT ?`
	args = append(args, limit)
	rows, err := d.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("query auth sessions: %w", err)
	}
	defer rows.Close()
	sessions := make([]AuthSession, 0, limit)
	for rows.Next() {
		var value AuthSession
		var id, userID, createdAt, expiresAt, lastSeenAt string
		var revokedAt sql.NullString
		if err := rows.Scan(&id, &userID, &value.TokenHash, &createdAt, &expiresAt, &lastSeenAt, &revokedAt, &value.ClientMetadata); err != nil {
			return nil, fmt.Errorf("scan auth session: %w", err)
		}
		value.ID = ID(id)
		value.UserID = ID(userID)
		if value.CreatedAt, err = parseDBTime(createdAt); err != nil {
			return nil, err
		}
		if value.ExpiresAt, err = parseDBTime(expiresAt); err != nil {
			return nil, err
		}
		if value.LastSeenAt, err = parseDBTime(lastSeenAt); err != nil {
			return nil, err
		}
		if value.RevokedAt, err = parseNullableDBTime(revokedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate auth sessions: %w", err)
	}
	return sessions, nil
}
