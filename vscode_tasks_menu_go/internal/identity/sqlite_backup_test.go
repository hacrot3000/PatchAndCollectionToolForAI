package identity

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSQLiteBackupAndLiveRestore(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "identity.db")
	store, err := OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 26, 15, 0, 0, 0, time.UTC)
	hash, err := HashPassword(ctx, "backup-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateUser(ctx, User{
		ID: "backup-user", Username: "backup-user", DisplayName: "Before backup",
		PasswordHash: hash, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(root, "backups", "identity-snapshot.db")
	if err := BackupSQLiteFile(ctx, dbPath, backupPath); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode=%#o want 0600", info.Mode().Perm())
	}
	if err := BackupSQLiteFile(ctx, dbPath, backupPath); err == nil {
		t.Fatal("backup unexpectedly overwrote existing destination")
	}

	if err := store.SetUserDisplayName(ctx, "backup-user", "After backup", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	changed, err := store.UserByID(ctx, "backup-user")
	if err != nil || changed.DisplayName != "After backup" {
		t.Fatalf("pre-restore user=%+v err=%v", changed, err)
	}

	safetyPath, err := RestoreSQLiteFile(ctx, dbPath, backupPath, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if safetyPath == "" {
		t.Fatal("restore did not return safety backup path")
	}
	safetyInfo, err := os.Stat(safetyPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && safetyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("safety backup mode=%#o want 0600", safetyInfo.Mode().Perm())
	}

	restored, err := store.UserByID(ctx, "backup-user")
	if err != nil {
		t.Fatal(err)
	}
	if restored.DisplayName != "Before backup" {
		t.Fatalf("live connection did not observe restored data: %+v", restored)
	}

	safetyStore, err := OpenSQLiteStore(ctx, safetyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer safetyStore.Close()
	safetyUser, err := safetyStore.UserByID(ctx, "backup-user")
	if err != nil {
		t.Fatal(err)
	}
	if safetyUser.DisplayName != "After backup" {
		t.Fatalf("pre-restore safety snapshot lost current state: %+v", safetyUser)
	}
}

func TestSQLiteBackupRestoreRejectsUnsafePaths(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	file := filepath.Join(root, "file.db")
	if err := os.WriteFile(file, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := BackupSQLiteFile(ctx, "relative.db", filepath.Join(root, "out.db")); err == nil {
		t.Fatal("relative source path accepted")
	}
	if _, err := RestoreSQLiteFile(ctx, file, file, time.Now()); err == nil {
		t.Fatal("restore accepted identical source and target")
	}
}
