package identity

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func authenticationFixture(t *testing.T) (*sqliteDatabase, time.Time) {
	t.Helper()
	db := openRealSQLiteDatabase(t, filepath.Join(t.TempDir(), identityDBName))
	ctx := context.Background()
	now := time.Now().UTC()
	for _, err := range []error{
		db.CreateUser(ctx, User{ID: "alice", Username: "alice", PasswordHash: "hash", Enabled: true, CreatedAt: now, UpdatedAt: now}),
		db.CreateRole(ctx, Role{ID: "member", Name: "member"}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.EnsureProject(ctx, Project{ID: "project", Key: "test", Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertProjectMember(ctx, ProjectMember{ProjectID: "project", UserID: "alice", RoleID: "member", Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return db, now
}

func TestBrowserSessionRoundTripAndLogout(t *testing.T) {
	db, now := authenticationFixture(t)
	ctx := context.Background()
	token, session, err := CreateBrowserSession(ctx, db, "alice", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 || len(session.TokenHash) != 64 || string(session.ID) == token || session.TokenHash == token {
		t.Fatal("token/hash separation failed")
	}
	if _, err := db.AuthSessionByTokenHash(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("raw token stored: %v", err)
	}
	p, _, err := AuthenticateBrowserSession(ctx, db, "test", token, now.Add(2*time.Minute))
	if err != nil || p.UserID != "alice" || p.ProjectKey != "test" {
		t.Fatalf("principal=%+v err=%v", p, err)
	}
	stored, err := db.AuthSessionByTokenHash(ctx, session.TokenHash)
	if err != nil || !stored.LastSeenAt.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("touch failed: %v", err)
	}
	if err := RevokeBrowserSession(ctx, db, token, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := AuthenticateBrowserSession(ctx, db, "test", token, now.Add(4*time.Minute)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked token accepted: %v", err)
	}
}

func TestBrowserSessionRechecksAccessAndExpiration(t *testing.T) {
	for _, test := range []struct {
		name, sql, key string
		elapsed        time.Duration
	}{
		{name: "disabled user", sql: "UPDATE users SET enabled=0"},
		{name: "disabled project", sql: "UPDATE projects SET enabled=0"},
		{name: "disabled membership", sql: "UPDATE project_members SET enabled=0"},
		{name: "removed membership", sql: "DELETE FROM project_members"},
		{name: "other project", key: "other"},
		{name: "idle expiry", elapsed: SessionIdleTimeout},
		{name: "absolute expiry", elapsed: SessionLifetime},
		{name: "future session", elapsed: -time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, now := authenticationFixture(t)
			ctx := context.Background()
			token, _, err := CreateBrowserSession(ctx, db, "alice", now)
			if err != nil {
				t.Fatal(err)
			}
			if test.sql != "" {
				if _, err := db.db.Exec(test.sql); err != nil {
					t.Fatal(err)
				}
			}
			key := test.key
			if key == "" {
				key = "test"
			}
			if _, _, err := AuthenticateBrowserSession(ctx, db, key, token, now.Add(test.elapsed)); !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("access allowed: %v", err)
			}
		})
	}
}

func TestPasswordChangeInvalidatesBrowserSession(t *testing.T) {
	db, now := authenticationFixture(t)
	ctx := context.Background()
	token, _, err := CreateBrowserSession(ctx, db, "alice", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserPasswordHash(ctx, "alice", "new-hash", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := AuthenticateBrowserSession(ctx, db, "test", token, now.Add(2*time.Second)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("old password session accepted: %v", err)
	}
}

func TestMalformedBrowserTokenFailsBeforeStoreAccess(t *testing.T) {
	for _, token := range []string{"", "short", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA!"} {
		if _, _, err := AuthenticateBrowserSession(context.Background(), nil, "test", token, time.Now()); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("malformed token: %v", err)
		}
	}
}
