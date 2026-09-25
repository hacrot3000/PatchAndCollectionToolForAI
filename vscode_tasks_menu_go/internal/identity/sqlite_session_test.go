package identity

import (
	"context"
	"database/sql/driver"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openFakeIdentitySessionStore(t *testing.T, state *fakeSQLiteState) *sqliteDatabase {
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

func TestSQLiteSessionStoreCreateAndRead(t *testing.T) {
	created := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	expires := created.Add(24 * time.Hour)
	lastSeen := created.Add(5 * time.Minute)

	state := &fakeSQLiteState{
		currentVersion: int64(schemaVersion),
		queryResponses: []fakeSQLiteQueryResponse{
			{
				contains: "FROM AUTH_SESSIONS WHERE TOKEN_HASH = ?",
				columns: []string{
					"id", "user_id", "token_hash", "created_at", "expires_at",
					"last_seen_at", "revoked_at", "client_metadata",
				},
				values: [][]driver.Value{{
					"session-1",
					"user-1",
					"sha256-token-hash",
					created.Format(time.RFC3339Nano),
					expires.Format(time.RFC3339Nano),
					lastSeen.Format(time.RFC3339Nano),
					nil,
					"browser",
				}},
			},
		},
	}
	db := openFakeIdentitySessionStore(t, state)

	err := db.CreateAuthSession(context.Background(), AuthSession{
		ID:             "session-1",
		UserID:         "user-1",
		TokenHash:      "sha256-token-hash",
		CreatedAt:      created,
		ExpiresAt:      expires,
		LastSeenAt:     lastSeen,
		ClientMetadata: "browser",
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := db.AuthSessionByTokenHash(context.Background(), "sha256-token-hash")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "session-1" || got.UserID != "user-1" || got.TokenHash != "sha256-token-hash" {
		t.Fatalf("unexpected session: %#v", got)
	}
	if !got.CreatedAt.Equal(created) || !got.ExpiresAt.Equal(expires) || !got.LastSeenAt.Equal(lastSeen) {
		t.Fatalf("unexpected session timestamps: %#v", got)
	}
	if got.RevokedAt != nil {
		t.Fatalf("revoked_at=%v want nil", got.RevokedAt)
	}

	_, execs, queries := state.snapshot()
	assertSQLLogContains(t, execs, "INSERT INTO auth_sessions")
	assertSQLLogContains(t, queries, "FROM auth_sessions")
}

func TestSQLiteSessionStoreTouchAndRevoke(t *testing.T) {
	state := &fakeSQLiteState{currentVersion: int64(schemaVersion)}
	db := openFakeIdentitySessionStore(t, state)

	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	if err := db.TouchAuthSession(context.Background(), "session-1", now); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeAuthSession(context.Background(), "session-1", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeUserSessions(context.Background(), "user-1", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	_, execs, _ := state.snapshot()
	assertSQLLogContains(t, execs, "SET last_seen_at = ?")
	assertSQLLogContains(t, execs, "WHERE id = ? AND revoked_at IS NULL")
	assertSQLLogContains(t, execs, "WHERE user_id = ? AND revoked_at IS NULL")
}

func TestSQLiteSessionStoreRejectsInvalidCreate(t *testing.T) {
	state := &fakeSQLiteState{currentVersion: int64(schemaVersion)}
	db := openFakeIdentitySessionStore(t, state)

	now := time.Now().UTC()
	err := db.CreateAuthSession(context.Background(), AuthSession{
		ID:        "session-1",
		UserID:    "user-1",
		TokenHash: "",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
		LastSeenAt: now,
	})
	if err == nil || !strings.Contains(err.Error(), "token_hash") {
		t.Fatalf("err=%v want token_hash validation", err)
	}

	err = db.CreateAuthSession(context.Background(), AuthSession{
		ID:         "session-1",
		UserID:     "user-1",
		TokenHash:  "hash",
		CreatedAt:  now,
		ExpiresAt:  now,
		LastSeenAt: now,
	})
	if err == nil || !strings.Contains(err.Error(), "expires_at must be after") {
		t.Fatalf("err=%v want expiry validation", err)
	}
}

func TestSQLiteSessionStoreEmptyHashIsNotFound(t *testing.T) {
	state := &fakeSQLiteState{currentVersion: int64(schemaVersion)}
	db := openFakeIdentitySessionStore(t, state)

	_, err := db.AuthSessionByTokenHash(context.Background(), "")
	if err != ErrNotFound {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}
