package identity

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultSQLiteBusyTimeout = 5 * time.Second

const schemaMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
)
`

// sqliteDatabase contains the shared identity database plumbing without choosing
// a concrete SQLite driver. Go's standard library provides database/sql but not
// an SQLite driver, so driver selection is intentionally kept outside this
// checkpoint.
type sqliteDatabase struct {
	db   *sql.DB
	path string
}

func openSQLiteDatabase(ctx context.Context, driverName, dbPath string) (*sqliteDatabase, error) {
	driverName = strings.TrimSpace(driverName)
	if driverName == "" {
		return nil, fmt.Errorf("SQLite driver name is required")
	}
	dbPath = filepath.Clean(strings.TrimSpace(dbPath))
	if dbPath == "." || dbPath == "" || !filepath.IsAbs(dbPath) {
		return nil, fmt.Errorf("identity DB path must be absolute")
	}
	if err := EnsureDBParent(dbPath); err != nil {
		return nil, err
	}

	// sql.Open validates that the requested driver is registered before TaskDeck
	// creates any durable file. This keeps a missing optional driver fail-closed.
	db, err := sql.Open(driverName, dbPath)
	if err != nil {
		return nil, fmt.Errorf("open identity SQLite driver: %w", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = db.Close()
		}
	}()

	if err := ensureDBFile(dbPath); err != nil {
		return nil, err
	}

	// SQLite PRAGMAs such as foreign_keys and busy_timeout are connection-local.
	// Keeping one connection per daemon makes the stdlib-only setup deterministic;
	// WAL still permits several TaskDeck processes to share the same DB.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping identity SQLite DB: %w", err)
	}
	if err := configureSQLite(ctx, db, defaultSQLiteBusyTimeout); err != nil {
		return nil, err
	}
	if err := migrateSQLite(ctx, db, time.Now().UTC()); err != nil {
		return nil, err
	}

	closeOnError = false
	return &sqliteDatabase{db: db, path: dbPath}, nil
}

func (d *sqliteDatabase) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

func ensureDBFile(dbPath string) error {
	info, err := os.Lstat(dbPath)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("identity DB path is not a regular file: %s", dbPath)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("identity DB permissions are too broad: %s mode=%#o", dbPath, info.Mode().Perm())
		}
		return nil
	case !os.IsNotExist(err):
		return fmt.Errorf("inspect identity DB path: %w", err)
	}

	file, err := os.OpenFile(dbPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if os.IsExist(err) {
		// Another TaskDeck process may have won the first-create race.
		return ensureDBFile(dbPath)
	}
	if err != nil {
		return fmt.Errorf("create identity DB file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close new identity DB file: %w", err)
	}
	return nil
}

func configureSQLite(ctx context.Context, db *sql.DB, busyTimeout time.Duration) error {
	if busyTimeout <= 0 {
		busyTimeout = defaultSQLiteBusyTimeout
	}

	timeoutMS := busyTimeout / time.Millisecond
	if timeoutMS < 1 {
		timeoutMS = 1
	}
	// Configure the busy handler before any pragma that may need to acquire a
	// database lock. In particular, two project daemons can race while the
	// identity DB is first being switched to WAL mode.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", timeoutMS)); err != nil {
		return fmt.Errorf("configure SQLite busy timeout: %w", err)
	}

	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		return fmt.Errorf("enable SQLite WAL: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(journalMode), "wal") {
		return fmt.Errorf("SQLite WAL unavailable: journal_mode=%q", journalMode)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable SQLite foreign keys: %w", err)
	}
	return nil
}

func migrateSQLite(ctx context.Context, db *sql.DB, appliedAt time.Time) error {
	if schemaVersion != 1 {
		return fmt.Errorf("identity migration registry is incomplete for schema version %d", schemaVersion)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve identity migration connection: %w", err)
	}
	defer conn.Close()

	// BEGIN IMMEDIATE serializes schema migration across project daemons before
	// reading schema_migrations, avoiding two processes both deciding to apply
	// the same migration from a stale read.
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin identity migration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	if _, err := conn.ExecContext(ctx, schemaMigrationsTableSQL); err != nil {
		return fmt.Errorf("create identity migration table: %w", err)
	}

	var current int
	if err := conn.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current); err != nil {
		return fmt.Errorf("read identity schema version: %w", err)
	}
	if current > schemaVersion {
		return fmt.Errorf("identity DB schema version %d is newer than supported version %d", current, schemaVersion)
	}

	if current < 1 {
		if _, err := conn.ExecContext(ctx, SchemaV1); err != nil {
			return fmt.Errorf("apply identity schema v1: %w", err)
		}
		if _, err := conn.ExecContext(
			ctx,
			"INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)",
			1,
			appliedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record identity schema v1: %w", err)
		}
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit identity migration: %w", err)
	}
	committed = true
	return nil
}
