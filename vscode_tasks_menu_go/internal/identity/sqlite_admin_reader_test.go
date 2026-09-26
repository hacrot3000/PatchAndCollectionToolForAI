package identity

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteAdminReaderListsSafeProjectViews(t *testing.T) {
	ctx := context.Background()
	db := openRealSQLiteDatabase(t, filepath.Join(t.TempDir(), identityDBName))
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	if _, err := db.BootstrapFirstAdmin(ctx, "project-one", "alice", "$scrypt$v=1,ln=17,r=8,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", now); err != nil {
		t.Fatal(err)
	}
	if err := db.SeedSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := db.ProjectByKey(ctx, "project-one")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateUser(ctx, User{ID: "bob", Username: "bob", DisplayName: "Bob", PasswordHash: "not-returned", Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertProjectMember(ctx, ProjectMember{ProjectID: project.ID, UserID: "bob", RoleID: "system:viewer", Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetMemberPermission(ctx, MemberPermission{ProjectID: project.ID, UserID: "bob", PermissionID: PermissionTasksRun, Effect: PermissionDeny}); err != nil {
		t.Fatal(err)
	}
	members, err := db.ListProjectMembers(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || members[1].Username != "bob" || members[1].RoleName != "viewer" || members[1].PermissionOverrides[PermissionTasksRun] != PermissionDeny {
		t.Fatalf("unexpected project members: %#v", members)
	}
	roles, err := db.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) < 4 || len(roles[0].Permissions) == 0 {
		t.Fatalf("unexpected roles: %#v", roles)
	}
	if err := db.CreateAuthSession(ctx, AuthSession{ID: "session-bob", UserID: "bob", TokenHash: "internal-token-hash", CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now}); err != nil {
		t.Fatal(err)
	}
	sessions, err := db.ListAuthSessions(ctx, AuthSessionQuery{ProjectID: project.ID, UserID: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != "session-bob" || sessions[0].TokenHash == "" {
		t.Fatalf("unexpected internal session view: %#v", sessions)
	}

	session, err := db.AuthSessionForProject(ctx, project.ID, "session-bob")
	if err != nil {
		t.Fatal(err)
	}
	if session.ID != "session-bob" || session.UserID != "bob" || session.TokenHash != "internal-token-hash" {
		t.Fatalf("unexpected scoped session: %#v", session)
	}
	if _, err := db.AuthSessionForProject(ctx, "other-project", "session-bob"); err != ErrNotFound {
		t.Fatalf("cross-project session lookup err=%v want=%v", err, ErrNotFound)
	}
}
