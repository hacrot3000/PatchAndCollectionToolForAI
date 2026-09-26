package identity

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSQLiteAdminStoreRoleAndMembership(t *testing.T) {
	state := &fakeSQLiteState{
		currentVersion: int64(schemaVersion),
		queryResponses: []fakeSQLiteQueryResponse{
			{
				contains: "SELECT 1 FROM ROLES WHERE ID = ?",
				columns:  []string{"exists"},
				values:   [][]driver.Value{{int64(1)}},
			},
		},
	}
	db := openFakeIdentitySessionStore(t, state)

	if err := db.CreateRole(context.Background(), Role{
		ID:          "role-1",
		Name:        "developer",
		Description: "Developer",
		SystemRole:  true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetRolePermissions(context.Background(), "role-1", []string{
		"tasks.view",
		"files.read",
		"tasks.view",
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
	if err := db.UpsertProjectMember(context.Background(), ProjectMember{
		ProjectID: "project-1",
		UserID:    "user-1",
		RoleID:    "role-1",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetMemberPermission(context.Background(), MemberPermission{
		ProjectID:    "project-1",
		UserID:       "user-1",
		PermissionID: "permission-1",
		Effect:       PermissionDeny,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteMemberPermission(context.Background(), "project-1", "user-1", "permission-1"); err != nil {
		t.Fatal(err)
	}

	_, execs, _ := state.snapshot()
	assertSQLLogContains(t, execs, "INSERT INTO roles")
	assertSQLLogContains(t, execs, "DELETE FROM role_permissions")
	assertSQLLogContains(t, execs, "INSERT INTO project_members")
	assertSQLLogContains(t, execs, "INSERT INTO member_permissions")
	assertSQLLogContains(t, execs, "DELETE FROM member_permissions")

	rolePermissionInserts := 0
	for _, query := range execs {
		if strings.Contains(query, "INSERT INTO role_permissions") {
			rolePermissionInserts++
		}
	}
	if rolePermissionInserts != 2 {
		t.Fatalf("role permission inserts=%d want=2 (duplicates should be removed)", rolePermissionInserts)
	}
}

func TestSQLiteAdminStoreSetRolePermissionsMissingRole(t *testing.T) {
	state := &fakeSQLiteState{
		currentVersion: int64(schemaVersion),
		queryResponses: []fakeSQLiteQueryResponse{
			{
				contains: "SELECT 1 FROM ROLES WHERE ID = ?",
				columns:  []string{"exists"},
			},
		},
	}
	db := openFakeIdentitySessionStore(t, state)

	err := db.SetRolePermissions(context.Background(), "missing-role", []string{"tasks.view"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}

	_, execs, _ := state.snapshot()
	assertSQLLogContains(t, execs, "ROLLBACK")
}

func TestSQLiteAdminStoreRejectsInvalidMemberPermission(t *testing.T) {
	state := &fakeSQLiteState{currentVersion: int64(schemaVersion)}
	db := openFakeIdentitySessionStore(t, state)

	err := db.SetMemberPermission(context.Background(), MemberPermission{
		ProjectID:    "project-1",
		UserID:       "user-1",
		PermissionID: "permission-1",
		Effect:       PermissionEffect("UNKNOWN"),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid member permission effect") {
		t.Fatalf("err=%v want invalid effect rejection", err)
	}
}

func TestNormalizePermissionKeysRejectsEmpty(t *testing.T) {
	_, err := normalizePermissionKeys([]string{"tasks.view", " "})
	if err == nil {
		t.Fatal("empty permission key accepted")
	}
}

func TestSQLiteProjectRolesAreScopedAndSystemRolesAreReadOnly(t *testing.T) {
	ctx := context.Background()
	db := openRealSQLiteDatabase(t, t.TempDir()+"/identity.db")
	now := time.Date(2026, 9, 26, 18, 0, 0, 0, time.UTC)
	if _, err := db.BootstrapFirstAdmin(ctx, "project-one", "alice", "$scrypt$v=1,ln=17,r=8,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", now); err != nil {
		t.Fatal(err)
	}
	if err := db.SeedSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	projectOne, err := db.ProjectByKey(ctx, "project-one")
	if err != nil {
		t.Fatal(err)
	}
	projectTwo, err := db.EnsureProject(ctx, Project{ID: "project-two-id", Key: "project-two", Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateProjectRole(ctx, projectOne.ID, Role{ID: "custom:one", Name: "project-one-developer", Description: "Project one custom role"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetProjectRolePermissions(ctx, projectOne.ID, "custom:one", []string{PermissionTasksView, PermissionFilesRead}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetProjectRolePermissions(ctx, projectTwo.ID, "custom:one", []string{PermissionTasksView}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project role update err=%v want ErrNotFound", err)
	}
	if err := db.SetProjectRolePermissions(ctx, projectOne.ID, "system:admin", []string{PermissionTasksView}); !errors.Is(err, ErrConflict) {
		t.Fatalf("system role update err=%v want ErrConflict", err)
	}
	if err := db.CreateProjectRole(ctx, projectTwo.ID, Role{ID: "custom:duplicate", Name: "project-one-developer"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate role name err=%v want ErrConflict", err)
	}
	rolesOne, err := db.ListProjectRoles(ctx, projectOne.ID)
	if err != nil {
		t.Fatal(err)
	}
	rolesTwo, err := db.ListProjectRoles(ctx, projectTwo.ID)
	if err != nil {
		t.Fatal(err)
	}
	var oneFound, twoFound bool
	for _, role := range rolesOne {
		if role.ID == "custom:one" {
			oneFound = len(role.Permissions) == 2
		}
	}
	for _, role := range rolesTwo {
		if role.ID == "custom:one" || role.ID == "custom:duplicate" {
			twoFound = true
		}
	}
	if !oneFound || twoFound {
		t.Fatalf("project role scope one=%#v two=%#v", rolesOne, rolesTwo)
	}
}
