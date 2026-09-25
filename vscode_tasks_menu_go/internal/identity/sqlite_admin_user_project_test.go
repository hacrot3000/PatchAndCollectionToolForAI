package identity

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"
)

func TestSQLiteAdminStoreUserLifecycle(t *testing.T) {
	state := &fakeSQLiteState{currentVersion: int64(schemaVersion)}
	db := openFakeIdentitySessionStore(t, state)

	now := time.Date(2026, 9, 25, 14, 0, 0, 0, time.UTC)
	err := db.CreateUser(context.Background(), User{
		ID:           "user-1",
		Username:     "alice",
		DisplayName:  "Alice",
		PasswordHash: "argon2id-hash",
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserEnabled(context.Background(), "user-1", false, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserPasswordHash(context.Background(), "user-1", "new-argon2id-hash", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	_, execs, _ := state.snapshot()
	assertSQLLogContains(t, execs, "INSERT INTO users")
	assertSQLLogContains(t, execs, "SET enabled = ?, updated_at = ?")
	assertSQLLogContains(t, execs, "SET password_hash = ?, password_changed_at = ?, updated_at = ?")
}

func TestSQLiteAdminStoreCreateUserConflict(t *testing.T) {
	state := &fakeSQLiteState{
		currentVersion: int64(schemaVersion),
		execResponses: []fakeSQLiteExecResponse{
			{contains: "INSERT INTO USERS", affected: 0},
		},
	}
	db := openFakeIdentitySessionStore(t, state)

	now := time.Now().UTC()
	err := db.CreateUser(context.Background(), User{
		ID:           "user-2",
		Username:     "alice",
		PasswordHash: "hash",
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v want ErrConflict", err)
	}
}

func TestSQLiteAdminStoreUpdateMissingUser(t *testing.T) {
	state := &fakeSQLiteState{
		currentVersion: int64(schemaVersion),
		execResponses: []fakeSQLiteExecResponse{
			{contains: "UPDATE USERS SET ENABLED", affected: 0},
		},
	}
	db := openFakeIdentitySessionStore(t, state)

	err := db.SetUserEnabled(context.Background(), "missing", true, time.Now().UTC())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

func TestSQLiteAdminStoreEnsureProjectReturnsStoredProject(t *testing.T) {
	created := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	updated := created.Add(time.Hour)
	state := &fakeSQLiteState{
		currentVersion: int64(schemaVersion),
		queryResponses: []fakeSQLiteQueryResponse{
			{
				contains: "FROM PROJECTS WHERE PROJECT_KEY = ?",
				columns:  []string{"id", "project_key", "display_name", "enabled", "created_at", "updated_at"},
				values: [][]driver.Value{{
					"existing-project-id",
					"m3-client",
					"Existing M3",
					int64(1),
					created.Format(time.RFC3339Nano),
					updated.Format(time.RFC3339Nano),
				}},
			},
		},
	}
	db := openFakeIdentitySessionStore(t, state)

	got, err := db.EnsureProject(context.Background(), Project{
		ID:          "candidate-id",
		Key:         "m3-client",
		DisplayName: "Candidate",
		Enabled:     true,
		CreatedAt:   created,
		UpdatedAt:   created,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "existing-project-id" || got.Key != "m3-client" || got.DisplayName != "Existing M3" {
		t.Fatalf("unexpected ensured project: %#v", got)
	}

	if err := db.SetProjectEnabled(context.Background(), got.ID, false, updated.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	_, execs, _ := state.snapshot()
	assertSQLLogContains(t, execs, "INSERT INTO projects")
	assertSQLLogContains(t, execs, "UPDATE projects")
}
