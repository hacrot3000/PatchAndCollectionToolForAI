package identity

import (
	"context"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openFakeIdentityReader(t *testing.T, state *fakeSQLiteState) *sqliteDatabase {
	t.Helper()
	if state.currentVersion == 0 {
		state.currentVersion = int64(schemaVersion)
	}
	if state.journalMode == "" {
		state.journalMode = "wal"
	}
	useFakeSQLiteDriver(t, state)

	db, err := openSQLiteDatabase(
		context.Background(),
		fakeSQLiteDriverName,
		filepath.Join(t.TempDir(), identityDBName),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestSQLiteReaderLoadsIdentityRecords(t *testing.T) {
	created := "2026-09-25T10:00:00Z"
	updated := "2026-09-25T11:00:00Z"
	lastLogin := "2026-09-25T12:00:00Z"

	state := &fakeSQLiteState{
		currentVersion: int64(schemaVersion),
		queryResponses: []fakeSQLiteQueryResponse{
			{
				contains: "FROM USERS WHERE USERNAME = ?",
				columns: []string{
					"id", "username", "display_name", "password_hash", "enabled",
					"created_at", "updated_at", "last_login_at", "password_changed_at",
				},
				values: [][]driver.Value{{
					"user-1", "alice", "Alice", "argon2id-hash", int64(1),
					created, updated, lastLogin, nil,
				}},
			},
			{
				contains: "FROM PROJECTS WHERE PROJECT_KEY = ?",
				columns:  []string{"id", "project_key", "display_name", "enabled", "created_at", "updated_at"},
				values: [][]driver.Value{{
					"project-1", "m3-client", "M3 Client", int64(1), created, updated,
				}},
			},
			{
				contains: "FROM PROJECT_MEMBERS WHERE PROJECT_ID = ? AND USER_ID = ?",
				columns:  []string{"project_id", "user_id", "role_id", "enabled", "created_at", "updated_at"},
				values: [][]driver.Value{{
					"project-1", "user-1", "role-1", int64(1), created, updated,
				}},
			},
		},
	}
	db := openFakeIdentityReader(t, state)

	user, err := db.UserByUsername(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "user-1" || user.Username != "alice" || !user.Enabled {
		t.Fatalf("unexpected user: %#v", user)
	}
	if user.LastLoginAt == nil || user.LastLoginAt.Format(time.RFC3339) != lastLogin {
		t.Fatalf("unexpected last login: %#v", user.LastLoginAt)
	}
	if user.PasswordChangedAt != nil {
		t.Fatalf("password_changed_at=%v want nil", user.PasswordChangedAt)
	}

	project, err := db.ProjectByKey(context.Background(), "m3-client")
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != "project-1" || project.Key != "m3-client" || !project.Enabled {
		t.Fatalf("unexpected project: %#v", project)
	}

	member, err := db.ProjectMember(context.Background(), "project-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if member.ProjectID != "project-1" || member.UserID != "user-1" || member.RoleID != "role-1" || !member.Enabled {
		t.Fatalf("unexpected member: %#v", member)
	}
}

func TestSQLiteReaderEffectivePermissionsAppliesOverrides(t *testing.T) {
	state := &fakeSQLiteState{
		currentVersion: int64(schemaVersion),
		queryResponses: []fakeSQLiteQueryResponse{
			{
				contains: "FROM PROJECT_MEMBERS JOIN USERS",
				columns:  []string{"user_enabled", "project_enabled", "member_enabled", "role_id"},
				values:   [][]driver.Value{{int64(1), int64(1), int64(1), "role-1"}},
			},
			{
				contains: "FROM ROLE_PERMISSIONS",
				columns:  []string{"permission_key"},
				values: [][]driver.Value{
					{"files.read"},
					{"tasks.view"},
					{"terminal.create"},
				},
			},
			{
				contains: "FROM MEMBER_PERMISSIONS",
				columns:  []string{"permission_key", "effect"},
				values: [][]driver.Value{
					{"files.read", "DENY"},
					{"patch.view", "ALLOW"},
				},
			},
		},
	}
	db := openFakeIdentityReader(t, state)

	got, err := db.EffectivePermissions(context.Background(), "project-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tasks.view", "terminal.create", "patch.view"} {
		if !got[key] {
			t.Fatalf("permission %q missing from %#v", key, got)
		}
	}
	if got["files.read"] {
		t.Fatalf("explicit DENY did not override role ALLOW: %#v", got)
	}
}

func TestSQLiteReaderEffectivePermissionsDenyDisabledMembership(t *testing.T) {
	state := &fakeSQLiteState{
		currentVersion: int64(schemaVersion),
		queryResponses: []fakeSQLiteQueryResponse{
			{
				contains: "FROM PROJECT_MEMBERS JOIN USERS",
				columns:  []string{"user_enabled", "project_enabled", "member_enabled", "role_id"},
				values:   [][]driver.Value{{int64(1), int64(1), int64(0), "role-1"}},
			},
		},
	}
	db := openFakeIdentityReader(t, state)

	got, err := db.EffectivePermissions(context.Background(), "project-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("disabled membership permissions=%#v want empty", got)
	}
}

func TestSQLiteReaderReturnsNotFound(t *testing.T) {
	state := &fakeSQLiteState{
		currentVersion: int64(schemaVersion),
		queryResponses: []fakeSQLiteQueryResponse{
			{
				contains: "FROM USERS WHERE ID = ?",
				columns: []string{
					"id", "username", "display_name", "password_hash", "enabled",
					"created_at", "updated_at", "last_login_at", "password_changed_at",
				},
			},
		},
	}
	db := openFakeIdentityReader(t, state)

	_, err := db.UserByID(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

func TestSQLiteReaderRejectsInvalidPermissionEffect(t *testing.T) {
	state := &fakeSQLiteState{
		currentVersion: int64(schemaVersion),
		queryResponses: []fakeSQLiteQueryResponse{
			{
				contains: "FROM PROJECT_MEMBERS JOIN USERS",
				columns:  []string{"user_enabled", "project_enabled", "member_enabled", "role_id"},
				values:   [][]driver.Value{{int64(1), int64(1), int64(1), "role-1"}},
			},
			{
				contains: "FROM ROLE_PERMISSIONS",
				columns:  []string{"permission_key"},
			},
			{
				contains: "FROM MEMBER_PERMISSIONS",
				columns:  []string{"permission_key", "effect"},
				values:   [][]driver.Value{{"tasks.run", "MAYBE"}},
			},
		},
	}
	db := openFakeIdentityReader(t, state)

	_, err := db.EffectivePermissions(context.Background(), "project-1", "user-1")
	if err == nil || !strings.Contains(err.Error(), "invalid identity permission effect") {
		t.Fatalf("err=%v want invalid effect rejection", err)
	}
}
