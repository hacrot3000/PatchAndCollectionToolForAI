package identity

import (
    "context"
    "path/filepath"
    "testing"
    "time"
)

// A previously deployed restricted-role seed can leave stale member overrides.
// The V7 cleanup must use the composite (project,user) identity, not user_id
// alone: the same account may have a different role in another project.
// This also exercises the EXISTS-based SQL used instead of SQLite 3.15+ row
// values, which were incompatible with older sqlite3 libraries.
func TestRestrictedRoleV7CleanupPreservesOtherProjectOverrides(t *testing.T) {
    ctx := context.Background()
    db := openRealSQLiteDatabase(t, filepath.Join(t.TempDir(), identityDBName))
    now := time.Now().UTC()

    admin, err := db.BootstrapFirstAdmin(ctx, "restricted-project", "admin", testScryptHash, now)
    if err != nil {
        t.Fatal(err)
    }
    if err := db.SeedSystemRoles(ctx); err != nil {
        t.Fatal(err)
    }
    second, err := db.EnsureProject(ctx, Project{
        ID: "another-project-id", Key: "another-project",
        Enabled: true, CreatedAt: now, UpdatedAt: now,
    })
    if err != nil {
        t.Fatal(err)
    }
    user := User{
        ID: "shared-user", Username: "shared-bob", PasswordHash: testScryptHash,
        Enabled: true, CreatedAt: now, UpdatedAt: now,
    }
    if err := db.CreateUser(ctx, user); err != nil {
        t.Fatal(err)
    }
    for _, member := range []ProjectMember{
        {ProjectID: admin.ProjectID, UserID: user.ID, RoleID: RestrictedDeveloperRoleID,
            Enabled: true, CreatedAt: now, UpdatedAt: now},
        {ProjectID: second.ID, UserID: user.ID, RoleID: "system:developer",
            Enabled: true, CreatedAt: now, UpdatedAt: now},
    } {
        if err := db.UpsertProjectMember(ctx, member); err != nil {
            t.Fatal(err)
        }
    }

    // Inject pre-migration overrides, bypassing the modern API's protection.
    for _, projectID := range []ID{admin.ProjectID, second.ID} {
        if _, err := db.db.ExecContext(ctx, `INSERT INTO member_permissions
            (project_id,user_id,permission_id,effect) VALUES(?,?,?,'ALLOW')`,
            string(projectID), string(user.ID), PermissionUsersManage); err != nil {
            t.Fatal(err)
        }
    }
    if _, err := db.db.ExecContext(ctx, "DELETE FROM identity_role_seeds WHERE version=7"); err != nil {
        t.Fatal(err)
    }

    // Retry exactly as a daemon restart would. The transaction must complete.
    for attempt := 0; attempt < 2; attempt++ {
        if err := db.SeedSystemRoles(ctx); err != nil {
            t.Fatalf("role seed attempt %d: %v", attempt+1, err)
        }
        for _, check := range []struct {
            project ID
            want    int
        }{
            {admin.ProjectID, 0},
            {second.ID, 1},
        } {
            var got int
            err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM member_permissions
                WHERE project_id=? AND user_id=? AND permission_id=?`,
                string(check.project), string(user.ID), PermissionUsersManage).Scan(&got)
            if err != nil {
                t.Fatal(err)
            }
            if got != check.want {
                t.Fatalf("attempt %d project %s: overrides=%d, want %d",
                    attempt+1, check.project, got, check.want)
            }
        }
    }
}
