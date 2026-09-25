package identity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const fakeSQLiteDriverName = "taskdeck-identity-fake-sqlite"

var (
	registerFakeSQLiteDriver sync.Once
	activeFakeSQLiteState    *fakeSQLiteState
)

type fakeSQLiteState struct {
	mu             sync.Mutex
	currentVersion int64
	journalMode    string
	execs          []string
	queries        []string
}

type fakeSQLiteDriver struct{}

func (fakeSQLiteDriver) Open(string) (driver.Conn, error) {
	if activeFakeSQLiteState == nil {
		return nil, errors.New("fake SQLite state is not configured")
	}
	return &fakeSQLiteConn{state: activeFakeSQLiteState}, nil
}

type fakeSQLiteConn struct {
	state *fakeSQLiteState
}

func (c *fakeSQLiteConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported by fake SQLite driver")
}

func (c *fakeSQLiteConn) Close() error { return nil }

func (c *fakeSQLiteConn) Begin() (driver.Tx, error) {
	return nil, errors.New("database/sql Begin is not used by identity migrations")
}

func (c *fakeSQLiteConn) Ping(context.Context) error { return nil }

func (c *fakeSQLiteConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.execs = append(c.state.execs, query)

	normalized := strings.ToUpper(strings.Join(strings.Fields(query), " "))
	if strings.HasPrefix(normalized, "INSERT INTO SCHEMA_MIGRATIONS") && len(args) > 0 {
		version, ok := args[0].Value.(int64)
		if !ok {
			return nil, errors.New("fake migration version is not int64")
		}
		c.state.currentVersion = version
	}
	return driver.RowsAffected(1), nil
}

func (c *fakeSQLiteConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.queries = append(c.state.queries, query)

	normalized := strings.ToUpper(strings.Join(strings.Fields(query), " "))
	switch {
	case normalized == "PRAGMA JOURNAL_MODE = WAL":
		mode := c.state.journalMode
		if mode == "" {
			mode = "wal"
		}
		return &fakeSQLiteRows{
			columns: []string{"journal_mode"},
			values:  [][]driver.Value{{mode}},
		}, nil
	case strings.HasPrefix(normalized, "SELECT COALESCE(MAX(VERSION), 0) FROM SCHEMA_MIGRATIONS"):
		return &fakeSQLiteRows{
			columns: []string{"version"},
			values:  [][]driver.Value{{c.state.currentVersion}},
		}, nil
	default:
		return nil, errors.New("unexpected fake SQLite query: " + query)
	}
}

type fakeSQLiteRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *fakeSQLiteRows) Columns() []string { return r.columns }
func (r *fakeSQLiteRows) Close() error      { return nil }

func (r *fakeSQLiteRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func useFakeSQLiteDriver(t *testing.T, state *fakeSQLiteState) {
	t.Helper()
	registerFakeSQLiteDriver.Do(func() {
		sql.Register(fakeSQLiteDriverName, fakeSQLiteDriver{})
	})
	if activeFakeSQLiteState != nil {
		t.Fatal("fake SQLite driver already active; identity SQLite tests must not run in parallel")
	}
	activeFakeSQLiteState = state
	t.Cleanup(func() {
		activeFakeSQLiteState = nil
	})
}

func (s *fakeSQLiteState) snapshot() (int64, []string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentVersion, append([]string(nil), s.execs...), append([]string(nil), s.queries...)
}

func TestOpenSQLiteDatabaseConfiguresAndMigratesV1(t *testing.T) {
	state := &fakeSQLiteState{journalMode: "wal"}
	useFakeSQLiteDriver(t, state)

	dbPath := filepath.Join(t.TempDir(), "taskdeck", identityDBName)
	db, err := openSQLiteDatabase(context.Background(), fakeSQLiteDriverName, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if db.path != dbPath {
		t.Fatalf("db.path=%q want=%q", db.path, dbPath)
	}
	info, err := os.Lstat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("identity DB is not a regular non-symlink file: mode=%v", info.Mode())
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("identity DB mode=%#o grants group/other access", info.Mode().Perm())
	}

	version, execs, queries := state.snapshot()
	if version != int64(schemaVersion) {
		t.Fatalf("schema version=%d want=%d", version, schemaVersion)
	}
	assertSQLLogContains(t, queries, "PRAGMA journal_mode = WAL")
	assertSQLLogContains(t, execs, "PRAGMA foreign_keys = ON")
	assertSQLLogContains(t, execs, "PRAGMA busy_timeout = 5000")
	assertSQLLogContains(t, execs, "BEGIN IMMEDIATE")
	assertSQLLogContains(t, execs, "CREATE TABLE IF NOT EXISTS users")
	assertSQLLogContains(t, execs, "INSERT INTO schema_migrations")
	assertSQLLogContains(t, execs, "COMMIT")
}

func TestOpenSQLiteDatabaseDoesNotReapplyCurrentSchema(t *testing.T) {
	state := &fakeSQLiteState{journalMode: "wal", currentVersion: int64(schemaVersion)}
	useFakeSQLiteDriver(t, state)

	dbPath := filepath.Join(t.TempDir(), identityDBName)
	db, err := openSQLiteDatabase(context.Background(), fakeSQLiteDriverName, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, execs, _ := state.snapshot()
	for _, query := range execs {
		if strings.Contains(query, "CREATE TABLE IF NOT EXISTS users") {
			t.Fatalf("current schema was reapplied: %q", query)
		}
		if strings.Contains(query, "INSERT INTO schema_migrations") {
			t.Fatalf("current schema version was inserted again: %q", query)
		}
	}
	assertSQLLogContains(t, execs, "BEGIN IMMEDIATE")
	assertSQLLogContains(t, execs, "COMMIT")
}

func TestOpenSQLiteDatabaseRejectsFutureSchemaVersion(t *testing.T) {
	state := &fakeSQLiteState{journalMode: "wal", currentVersion: int64(schemaVersion + 1)}
	useFakeSQLiteDriver(t, state)

	dbPath := filepath.Join(t.TempDir(), identityDBName)
	db, err := openSQLiteDatabase(context.Background(), fakeSQLiteDriverName, dbPath)
	if db != nil {
		_ = db.Close()
		t.Fatal("future schema unexpectedly returned a DB")
	}
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("err=%v want future-schema rejection", err)
	}

	_, execs, _ := state.snapshot()
	assertSQLLogContains(t, execs, "ROLLBACK")
}

func TestOpenSQLiteDatabaseRejectsUnavailableWAL(t *testing.T) {
	state := &fakeSQLiteState{journalMode: "delete"}
	useFakeSQLiteDriver(t, state)

	dbPath := filepath.Join(t.TempDir(), identityDBName)
	db, err := openSQLiteDatabase(context.Background(), fakeSQLiteDriverName, dbPath)
	if db != nil {
		_ = db.Close()
		t.Fatal("non-WAL database unexpectedly returned a DB")
	}
	if err == nil || !strings.Contains(err.Error(), "WAL unavailable") {
		t.Fatalf("err=%v want WAL rejection", err)
	}
}

func TestEnsureDBFileRejectsBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	dbPath := filepath.Join(t.TempDir(), identityDBName)
	if err := os.WriteFile(dbPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureDBFile(dbPath); err == nil || !strings.Contains(err.Error(), "permissions are too broad") {
		t.Fatalf("err=%v want broad-permission rejection", err)
	}
}

func TestEnsureDBFileRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target.db")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, identityDBName)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ensureDBFile(link); err == nil {
		t.Fatal("symlink identity DB accepted")
	}
}

func assertSQLLogContains(t *testing.T, log []string, want string) {
	t.Helper()
	for _, entry := range log {
		if strings.Contains(entry, want) {
			return
		}
	}
	t.Fatalf("SQL log missing %q: %#v", want, log)
}
