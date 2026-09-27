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
