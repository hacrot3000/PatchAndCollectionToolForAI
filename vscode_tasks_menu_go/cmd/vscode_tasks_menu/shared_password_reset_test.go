package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedPasswordResetChangesCredentialAndRevokesSessions(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "identity", "identity.db")
	cfg := config.Default()
	cfg.SharedServerEnabled = true
	cfg.SharedIdentityDB = dbPath

	store, err := identity.OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 26, 17, 0, 0, 0, time.UTC)
	oldPassword := "operator-reset-old-password"
	oldHash, err := identity.HashPassword(ctx, oldPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateUser(ctx, identity.User{
		ID: "reset-user", Username: "reset-user", PasswordHash: oldHash,
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAuthSession(ctx, identity.AuthSession{
		ID: "reset-session", UserID: "reset-user", TokenHash: "reset-session-hash",
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	newPassword := "operator-reset-new-password"
	if err := runSharedPasswordReset(ctx, workspace, cfg, "reset-user", newPassword); err != nil {
		t.Fatal(err)
	}
	user, err := store.UserByUsername(ctx, "reset-user")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := identity.VerifyPassword(ctx, newPassword, user.PasswordHash)
	if err != nil || !ok {
		t.Fatalf("new password verification ok=%v err=%v", ok, err)
	}
	ok, err = identity.VerifyPassword(ctx, oldPassword, user.PasswordHash)
	if err != nil || ok {
		t.Fatalf("old password still verifies ok=%v err=%v", ok, err)
	}
	session, err := store.AuthSessionByTokenHash(ctx, "reset-session-hash")
	if err != nil {
		t.Fatal(err)
	}
	if session.RevokedAt == nil {
		t.Fatal("operator password reset did not revoke existing sessions")
	}
}

func TestSharedPasswordResetRejectsWeakReplacement(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.SharedServerEnabled = true
	cfg.SharedIdentityDB = filepath.Join(root, "identity", "identity.db")
	if err := runSharedPasswordReset(ctx, workspace, cfg, "user", "short"); err == nil {
		t.Fatal("weak reset password accepted")
	}
}
