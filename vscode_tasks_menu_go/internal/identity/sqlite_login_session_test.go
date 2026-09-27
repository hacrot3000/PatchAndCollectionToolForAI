package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLoginSessionRejectsChangesAfterPasswordVerification(t *testing.T) {
	for _, mutation := range []string{
		"UPDATE users SET password_hash='changed'",
		"UPDATE users SET enabled=0",
		"UPDATE projects SET enabled=0",
		"UPDATE project_members SET enabled=0",
		"DELETE FROM project_members",
	} {
		t.Run(mutation, func(t *testing.T) {
			db, now := authenticationFixture(t)
			ctx := context.Background()
			user, err := db.UserByID(ctx, "alice")
			if err != nil {
				t.Fatal(err)
			}
			principal, err := ResolveUserPrincipal(ctx, db, "test", user)
			if err != nil {
				t.Fatal(err)
			}
			// Simulate an administrator changing access after password verification.
			if _, err := db.db.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			if token, _, err := CreateLoginBrowserSession(ctx, db, principal, user.PasswordHash, now.Add(time.Second)); !errors.Is(err, ErrUnauthenticated) || token != "" {
				t.Fatalf("stale login minted a token: %v", err)
			}
			var count int
			if err := db.db.QueryRow("SELECT COUNT(*) FROM auth_sessions").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("failed login left an active session")
			}
		})
	}
}
