package identity

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requirePythonSQLite(t *testing.T) {
	t.Helper()
	if _, _, err := resolvePythonSQLiteCommand(); err != nil {
		t.Skipf("Python SQLite runtime unavailable: %v", err)
	}
}

func openRealSQLiteDatabase(t *testing.T, dbPath string) *sqliteDatabase {
	t.Helper()
	requirePythonSQLite(t)
	db, err := openSQLiteDatabase(context.Background(), pythonSQLiteDriverName, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestPythonSQLiteDriverInitializesRealIdentityDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "identity", identityDBName)
	db := openRealSQLiteDatabase(t, dbPath)

	var journalMode string
	if err := db.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("journal_mode=%q want wal", journalMode)
	}

	var foreignKeys int
	if err := db.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys=%d want=1", foreignKeys)
	}

	var busyTimeout int
	if err := db.db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != int(defaultSQLiteBusyTimeout/time.Millisecond) {
		t.Fatalf("busy_timeout=%d want=%d", busyTimeout, defaultSQLiteBusyTimeout/time.Millisecond)
	}

	var version int
	if err := db.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("schema version=%d want=%d", version, schemaVersion)
	}

	if _, err := os.Stat(dbPath + "-wal"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestPythonSQLiteDriverStoreRoundTrip(t *testing.T) {
	db := openRealSQLiteDatabase(t, filepath.Join(t.TempDir(), identityDBName))
	ctx := context.Background()
	now := time.Date(2026, 9, 25, 16, 0, 0, 0, time.UTC)

	if err := db.CreateUser(ctx, User{
		ID:           "user-1",
		Username:     "alice",
		DisplayName:  "Alice",
		PasswordHash: "argon2id-placeholder",
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatal(err)
	}
	user, err := db.UserByUsername(ctx, "ALICE")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "user-1" || user.Username != "alice" || !user.Enabled {
		t.Fatalf("unexpected user: %#v", user)
	}

	project, err := db.EnsureProject(ctx, Project{
		ID:          "project-1",
		Key:         "m3-client",
		DisplayName: "M3 Client",
		Enabled:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != "project-1" || project.Key != "m3-client" {
		t.Fatalf("unexpected project: %#v", project)
	}
}

func TestPythonSQLiteDriverMultiProcessWALBusyTimeout(t *testing.T) {
	requirePythonSQLite(t)
	dbPath := filepath.Join(t.TempDir(), identityDBName)
	first := openRealSQLiteDatabase(t, dbPath)
	second := openRealSQLiteDatabase(t, dbPath)
	ctx := context.Background()

	if _, err := first.db.ExecContext(ctx, "CREATE TABLE concurrency_probe(id INTEGER PRIMARY KEY, value TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}

	conn, err := first.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if _, err := conn.ExecContext(ctx, "INSERT INTO concurrency_probe(value) VALUES (?)", "first"); err != nil {
		t.Fatal(err)
	}

	writeDone := make(chan error, 1)
	go func() {
		_, err := second.db.ExecContext(context.Background(), "INSERT INTO concurrency_probe(value) VALUES (?)", "second")
		writeDone <- err
	}()

	select {
	case err := <-writeDone:
		t.Fatalf("second writer returned before first transaction released the lock: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		t.Fatal(err)
	}
	committed = true

	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("second writer failed after lock release: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second writer did not resume after lock release")
	}

	var count int
	if err := first.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM concurrency_probe").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("row count=%d want=2", count)
	}
}

func TestOpenSQLiteStoreFailsClosedWithInvalidConfiguredPython(t *testing.T) {
	t.Setenv("TASKDECK_PYTHON", filepath.Join(t.TempDir(), "missing-python"))

	store, err := OpenSQLiteStore(context.Background(), filepath.Join(t.TempDir(), identityDBName))
	if store != nil {
		_ = store.Close()
		t.Fatal("invalid Python runtime unexpectedly opened identity store")
	}
	if err == nil || !strings.Contains(err.Error(), "TASKDECK_PYTHON is not executable") {
		t.Fatalf("err=%v want configured Python failure", err)
	}
}

func TestPythonSQLiteDriverHonorsUniqueConstraints(t *testing.T) {
	db := openRealSQLiteDatabase(t, filepath.Join(t.TempDir(), identityDBName))
	ctx := context.Background()
	now := time.Now().UTC()

	user := User{
		ID:           "user-1",
		Username:     "alice",
		PasswordHash: "hash",
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	user.ID = "user-2"
	if err := db.CreateUser(ctx, user); !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v want ErrConflict", err)
	}
}

var _ Store = (*sqliteDatabase)(nil)
var _ sql.Result = pythonSQLiteResult{}
