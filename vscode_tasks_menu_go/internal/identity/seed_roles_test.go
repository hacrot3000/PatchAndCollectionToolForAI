package identity

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSystemRoleSeedUpgradesBootstrapWithoutResettingDecisions(t *testing.T) {
	db := openRealSQLiteDatabase(t, filepath.Join(t.TempDir(), identityDBName))
	ctx := context.Background()
	p, err := db.BootstrapFirstAdmin(ctx, "test", "alice", testScryptHash, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SeedSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	permissions, err := db.EffectivePermissions(ctx, p.ProjectID, p.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(permissions) != len(PermissionRegistry()) {
		t.Fatalf("admin grants=%d", len(permissions))
	}
	if err := db.SetRolePermissions(ctx, "system:admin", []string{PermissionProjectAdmin, PermissionFilesRead}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetMemberPermission(ctx, MemberPermission{ProjectID: p.ProjectID, UserID: p.UserID, PermissionID: ID(PermissionFilesRead), Effect: PermissionDeny}); err != nil {
		t.Fatal(err)
	}
	if err := db.SeedSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	permissions, err = db.EffectivePermissions(ctx, p.ProjectID, p.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(permissions) != 1 || !permissions[PermissionProjectAdmin] {
		t.Fatalf("seed reset authorization: %v", permissions)
	}
}


func TestPermissionSeedV2AddsGranularPermissionsWithoutWideningNonAdminRoles(t *testing.T) {
	db := openRealSQLiteDatabase(t, filepath.Join(t.TempDir(), identityDBName))
	ctx := context.Background()
	p, err := db.BootstrapFirstAdmin(ctx, "test", "alice", testScryptHash, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SeedSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}

	// Simulate an administrator deliberately narrowing the admin role after v1.
	if err := db.SetRolePermissions(ctx, "system:admin", []string{PermissionProjectAdmin, PermissionFilesRead}); err != nil {
		t.Fatal(err)
	}
	// Re-run seed after the database already knows both seed versions; no old
	// permission may be restored.
	if err := db.SeedSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	permissions, err := db.EffectivePermissions(ctx, p.ProjectID, p.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(permissions) != 2 || !permissions[PermissionProjectAdmin] || !permissions[PermissionFilesRead] {
		t.Fatalf("repeat seed widened narrowed admin role: %v", permissions)
	}

	// Verify the v2 vocabulary exists and the conservative default only grants
	// it to admin on a fresh database. Non-admin system roles stay unchanged.
	for _, key := range PermissionUpgradeV2Keys() {
		if !KnownPermission(key) {
			t.Fatalf("v2 permission %q is not registered", key)
		}
	}
	for _, role := range SystemRoles() {
		if role.ID == "system:admin" {
			continue
		}
		grants := map[string]bool{}
		for _, key := range role.Permissions {
			grants[key] = true
		}
		for _, key := range PermissionUpgradeV2Keys() {
			if grants[key] {
				t.Fatalf("role %s unexpectedly receives new sensitive permission %q", role.ID, key)
			}
		}
	}
}
