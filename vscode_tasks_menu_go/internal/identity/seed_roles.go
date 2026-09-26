package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SeedSystemRoles is a versioned, transactional upgrade of the Phase 3
// bootstrap role. Reopening a daemon must never restore permissions removed by
// an administrator or clear member DENY overrides.
func (d *sqliteDatabase) SeedSystemRoles(ctx context.Context) error {
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS identity_role_seeds(version INTEGER PRIMARY KEY);
CREATE TABLE IF NOT EXISTS project_roles(
 role_id TEXT PRIMARY KEY REFERENCES roles(id) ON DELETE CASCADE,
 project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE
)`); err != nil {
		return err
	}
	var version int
	err = conn.QueryRowContext(ctx, "SELECT version FROM identity_role_seeds WHERE version=1").Scan(&version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		for _, permission := range PermissionRegistry() {
			if _, err := conn.ExecContext(ctx, `INSERT INTO permissions(id,permission_key,description) VALUES(?,?,?) ON CONFLICT(permission_key) DO NOTHING`, permission.Key, permission.Key, permission.Module); err != nil {
				return err
			}
		}
		for _, role := range SystemRoles() {
			if _, err := conn.ExecContext(ctx, `INSERT INTO roles(id,name,description,system_role) VALUES(?,?,?,1) ON CONFLICT(id) DO NOTHING`, string(role.ID), role.Name, "System "+role.Name); err != nil {
				return err
			}
			var name string
			var system int
			if err := conn.QueryRowContext(ctx, "SELECT name,system_role FROM roles WHERE id=?", string(role.ID)).Scan(&name, &system); err != nil {
				return err
			}
			if name != role.Name || system != 1 {
				return fmt.Errorf("%w: incompatible system role %s", ErrConflict, role.Name)
			}
			for _, key := range role.Permissions {
				if _, err := conn.ExecContext(ctx, `INSERT INTO role_permissions(role_id,permission_id) SELECT ?,id FROM permissions WHERE permission_key=? ON CONFLICT DO NOTHING`, string(role.ID), key); err != nil {
					return err
				}
			}
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO identity_role_seeds(version) VALUES(1)"); err != nil {
			return err
		}
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}
