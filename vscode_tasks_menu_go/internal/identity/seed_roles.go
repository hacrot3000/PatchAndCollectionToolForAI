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
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO permissions(id,permission_key,description) VALUES(?,?,?)`, permission.Key, permission.Key, permission.Module); err != nil {
				return err
			}
		}
		for _, role := range SystemRoles() {
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO roles(id,name,description,system_role) VALUES(?,?,?,1)`, string(role.ID), role.Name, "System "+role.Name); err != nil {
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
				if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO role_permissions(role_id,permission_id) SELECT ?,id FROM permissions WHERE permission_key=?`, string(role.ID), key); err != nil {
					return err
				}
			}
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO identity_role_seeds(version) VALUES(1)"); err != nil {
			return err
		}
	}
	var version2 int
	err = conn.QueryRowContext(ctx, "SELECT version FROM identity_role_seeds WHERE version=2").Scan(&version2)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		for _, permission := range PermissionRegistry() {
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO permissions(id,permission_key,description) VALUES(?,?,?)`, permission.Key, permission.Key, permission.Module); err != nil {
				return err
			}
		}
		for _, key := range PermissionUpgradeV2Keys() {
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO role_permissions(role_id,permission_id) SELECT 'system:admin',id FROM permissions WHERE permission_key=?`, key); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO identity_role_seeds(version) VALUES(2)"); err != nil {
			return err
		}
	}

	var version3 int
	err = conn.QueryRowContext(ctx, "SELECT version FROM identity_role_seeds WHERE version=3").Scan(&version3)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		for _, permission := range PermissionRegistry() {
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO permissions(id,permission_key,description) VALUES(?,?,?)`, permission.Key, permission.Key, permission.Module); err != nil {
				return err
			}
		}
		for _, key := range PermissionUpgradeV3Keys() {
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO role_permissions(role_id,permission_id) SELECT 'system:admin',id FROM permissions WHERE permission_key=?`, key); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO identity_role_seeds(version) VALUES(3)"); err != nil {
			return err
		}
	}

	var version4 int
	err = conn.QueryRowContext(ctx, "SELECT version FROM identity_role_seeds WHERE version=4").Scan(&version4)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		for _, permission := range PermissionRegistry() {
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO permissions(id,permission_key,description) VALUES(?,?,?)`, permission.Key, permission.Key, permission.Module); err != nil {
				return err
			}
		}
		for _, key := range PermissionUpgradeV4Keys() {
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO role_permissions(role_id,permission_id) SELECT 'system:admin',id FROM permissions WHERE permission_key=?`, key); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO identity_role_seeds(version) VALUES(4)"); err != nil {
			return err
		}
	}


 // V6: register Schedule capabilities without restoring deliberately
 // revoked admin grants from a previously deployed main V5 database.
 var scheduleSeedV6 int
 err=conn.QueryRowContext(ctx,"SELECT version FROM identity_role_seeds WHERE version=6").Scan(&scheduleSeedV6)
 if err!=nil&&!errors.Is(err,sql.ErrNoRows){return err}
 if errors.Is(err,sql.ErrNoRows){
  for _,key:=range PermissionUpgradeV5Keys(){
   var alreadyRegistered int
   if err=conn.QueryRowContext(ctx,"SELECT COUNT(*) FROM permissions WHERE permission_key=?",key).Scan(&alreadyRegistered);err!=nil{return err}
   if _,err=conn.ExecContext(ctx,"INSERT OR IGNORE INTO permissions(id,permission_key,description) VALUES(?,?,?)",key,key,"schedules");err!=nil{return err}
   if alreadyRegistered==0{
    if _,err=conn.ExecContext(ctx,"INSERT OR IGNORE INTO role_permissions(role_id,permission_id) SELECT 'system:admin',id FROM permissions WHERE permission_key=?",key);err!=nil{return err}
   }
  }
  if _,err=conn.ExecContext(ctx,"INSERT INTO identity_role_seeds(version) VALUES(6)");err!=nil{return err}
 }

 // Version 7 introduces an immutable per-project restricted-developer role.
 // An existing v5 database from either branch must receive it without
 // altering administrator-edited permissions of existing system roles.
 var restrictedSeed int
 err = conn.QueryRowContext(ctx, "SELECT version FROM identity_role_seeds WHERE version=7").Scan(&restrictedSeed)
 if err != nil && !errors.Is(err, sql.ErrNoRows) { return err }
 if errors.Is(err, sql.ErrNoRows) {
  roleID := string(RestrictedDeveloperRoleID)
  if _,err=conn.ExecContext(ctx,
   "INSERT OR IGNORE INTO roles(id,name,description,system_role) VALUES(?,?,?,1)",
   roleID,"restricted-developer","Protected source windows with persistent quotas; member permission overrides prohibited");err!=nil{return err}
  var name string
  var system int
  if err=conn.QueryRowContext(ctx,"SELECT name,system_role FROM roles WHERE id=?",roleID).Scan(&name,&system);err!=nil{return err}
  if name!="restricted-developer"||system!=1{return fmt.Errorf("%w: incompatible restricted-developer role",ErrConflict)}
  // Replace accidental/legacy role grants with the immutable minimal set.
  if _,err=conn.ExecContext(ctx,"DELETE FROM role_permissions WHERE role_id=?",roleID);err!=nil{return err}
  for _,key:=range RestrictedDeveloperPermissions() {
   if _,err=conn.ExecContext(ctx,"INSERT INTO role_permissions(role_id,permission_id) SELECT ?,id FROM permissions WHERE permission_key=?",roleID,key);err!=nil{return err}
  }
  // A user reassigned to this role must never inherit older per-user ALLOW.
  // SQLite versions earlier than 3.15 do not support row-value tuples
  // like (project_id,user_id) IN (...). Use a correlated EXISTS so older
  // Python sqlite3 runtimes and the supported sqlite3 CLI fallback can run
  // this idempotent V7 migration without preventing the daemon from starting.
  // Match BOTH keys: a global identity may belong to multiple projects.
  if _,err=conn.ExecContext(ctx,`DELETE FROM member_permissions
    WHERE EXISTS (
      SELECT 1 FROM project_members
      WHERE project_members.project_id = member_permissions.project_id
        AND project_members.user_id = member_permissions.user_id
        AND project_members.role_id = ?
    )`,roleID);err!=nil{return fmt.Errorf("clean legacy restricted-member permissions: %w",err)}
  if _,err=conn.ExecContext(ctx,"INSERT INTO identity_role_seeds(version) VALUES(7)");err!=nil{return err}
 }

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}
