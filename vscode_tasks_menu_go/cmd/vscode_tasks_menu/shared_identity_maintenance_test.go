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

func TestSharedIdentityMaintenanceKeepsSensitiveFilesOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".vscode"), 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "identity", "identity.db")
	cfg := config.Default()
	cfg.SharedServerEnabled = true
	cfg.SharedIdentityDB = dbPath

	store, err := identity.OpenSQLiteStore(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 26, 16, 0, 0, 0, time.UTC)
	hash, err := identity.HashPassword(context.Background(), "maintenance-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateUser(context.Background(), identity.User{
		ID: "maintenance-user", Username: "maintenance-user", DisplayName: "Before",
		PasswordHash: hash, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := runSharedIdentityBackup(context.Background(), workspace, cfg, filepath.Join(workspace, "identity-backup.db")); err == nil {
		t.Fatal("identity backup inside workspace was accepted")
	}

	backupPath := filepath.Join(root, "backup", "identity.db")
	resolved, err := runSharedIdentityBackup(context.Background(), workspace, cfg, backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != backupPath {
		t.Fatalf("backup path=%q want %q", resolved, backupPath)
	}

	if err := store.SetUserDisplayName(context.Background(), "maintenance-user", "After", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	source, safety, err := runSharedIdentityRestore(context.Background(), workspace, cfg, backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if source != backupPath || safety == "" {
		t.Fatalf("restore source=%q safety=%q", source, safety)
	}
	user, err := store.UserByID(context.Background(), "maintenance-user")
	if err != nil {
		t.Fatal(err)
	}
	if user.DisplayName != "Before" {
		t.Fatalf("restore did not replace modified identity state: %+v", user)
	}
}

func TestSharedIdentityMaintenanceRequiresSharedMode(t *testing.T) {
	cfg := config.Default()
	if _, err := runSharedIdentityBackup(context.Background(), t.TempDir(), cfg, filepath.Join(t.TempDir(), "backup.db")); err == nil {
		t.Fatal("backup accepted with shared-server mode disabled")
	}
}
