package identity

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"
)

const PermissionProjectAdmin = "project.admin"

var ErrBootstrapUnavailable = errors.New("first-admin bootstrap requires an identity DB with no users")
var bootstrapNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type BootstrapStore interface {
	BootstrapFirstAdmin(ctx context.Context, projectKey, username, passwordHash string, now time.Time) (Principal, error)
}

// BootstrapFirstAdmin is a local provisioning operation, never an HTTP API.
// It deliberately requires an empty user table, even if all existing users are
// disabled. Account recovery and granting additional projects are admin flows.
// BEGIN IMMEDIATE makes the empty-store check atomic across project daemons.
func (d *sqliteDatabase) BootstrapFirstAdmin(ctx context.Context, projectKey, username, passwordHash string, now time.Time) (Principal, error) {
	if !bootstrapNamePattern.MatchString(projectKey) || !bootstrapNamePattern.MatchString(username) || now.IsZero() {
		return Principal{}, fmt.Errorf("bootstrap requires valid project key, username and timestamp")
	}
	if _, _, err := parsePasswordScryptHash(passwordHash); err != nil {
		return Principal{}, err
	}
	userID, err := randomID()
	if err != nil {
		return Principal{}, err
	}
	projectID, err := randomID()
	if err != nil {
		return Principal{}, err
	}
	auditID, err := randomID()
	if err != nil {
		return Principal{}, err
	}
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return Principal{}, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return Principal{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	var count int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return Principal{}, err
	}
	if count != 0 {
		return Principal{}, ErrBootstrapUnavailable
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	// Existing incompatible/partial provisioning fails without replacing it.
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users(id, username, password_hash, enabled, created_at, updated_at, password_changed_at) VALUES (?, ?, ?, 1, ?, ?, ?)`, []any{string(userID), username, passwordHash, stamp, stamp, stamp}},
		{`INSERT INTO projects(id, project_key, enabled, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, []any{string(projectID), projectKey, stamp, stamp}},
		{`INSERT INTO roles(id, name, description, system_role) VALUES ('system:admin', 'admin', 'Project administrator', 1)`, nil},
		{`INSERT INTO permissions(id, permission_key, description) VALUES ('project.admin', ?, 'Project administration')`, []any{PermissionProjectAdmin}},
		{`INSERT INTO role_permissions(role_id, permission_id) VALUES ('system:admin', 'project.admin')`, nil},
		{`INSERT INTO project_members(project_id, user_id, role_id, enabled, created_at, updated_at) VALUES (?, ?, 'system:admin', 1, ?, ?)`, []any{string(projectID), string(userID), stamp, stamp}},
		{`INSERT INTO audit_log(id, timestamp, user_id, project_id, action, result) VALUES (?, ?, ?, ?, 'identity.bootstrap', 'success')`, []any{string(auditID), stamp, string(userID), string(projectID)}},
	}
	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			return Principal{}, fmt.Errorf("bootstrap identity: %w", err)
		}
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return Principal{}, err
	}
	committed = true
	return Principal{UserID: userID, Username: username, ProjectID: projectID, ProjectKey: projectKey, Permissions: map[string]bool{PermissionProjectAdmin: true}}, nil
}
