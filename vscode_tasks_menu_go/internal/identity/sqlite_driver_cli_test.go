package identity

import (
	"context"
	"database/sql/driver"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requireSQLiteCLI(t *testing.T) {
	t.Helper()
	if _, err := resolveSQLiteCLICommand(); err != nil {
		t.Skipf("sqlite3 CLI runtime unavailable: %v", err)
	}
}

func openCLIIdentityDatabase(t *testing.T, dbPath string) *sqliteDatabase {
	t.Helper()
	requireSQLiteCLI(t)
	db, err := openSQLiteDatabase(context.Background(), cliSQLiteDriverName, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestSQLiteCLIBindArgsIgnoresQuotedAndCommentQuestionMarks(t *testing.T) {
	query := "SELECT '?', \"?\", col FROM t -- ?\nWHERE a=? AND b='it''s ?' /* ? */ AND c=?"
	got, err := bindSQLiteCLIArgs(query, []driver.NamedValue{
		{Ordinal: 1, Value: "A,'\nB"},
		{Ordinal: 2, Value: int64(7)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "a=?") || strings.Contains(got, "c=?") {
		t.Fatalf("placeholders were not bound: %s", got)
	}
	if !strings.Contains(got, "CAST(X'") || !strings.Contains(got, "c=7") {
		t.Fatalf("unexpected bound SQL: %s", got)
	}
	if !strings.Contains(got, "SELECT '?', \"?\"") || !strings.Contains(got, "b='it''s ?'") {
		t.Fatalf("quoted question marks changed: %s", got)
	}
}

func TestSQLiteCLIDriverIdentityRoundTripAndTransaction(t *testing.T) {
	db := openCLIIdentityDatabase(t, filepath.Join(t.TempDir(), "identity", identityDBName))
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

	user := User{
		ID:           "cli-user",
		Username:     "cli-user",
		DisplayName:  "Comma, quote ' and newline\nname",
		PasswordHash: "hash",
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	got, err := db.UserByUsername(ctx, user.Username)
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != user.DisplayName || got.Username != user.Username {
		t.Fatalf("round trip mismatch: got=%#v want=%#v", got, user)
	}

	var nullable any
	if err := db.db.QueryRowContext(ctx, "SELECT NULL").Scan(&nullable); err != nil {
		t.Fatal(err)
	}
	if nullable != nil {
		t.Fatalf("NULL decoded as %#v", nullable)
	}

	if _, err := db.db.ExecContext(ctx, "CREATE TABLE cli_tx_probe(id INTEGER PRIMARY KEY, value TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	conn, err := db.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO cli_tx_probe(value) VALUES (?)", "rolled back"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cli_tx_probe").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rollback left %d rows", count)
	}
}

func TestSQLiteCLIUniqueConstraintReturnsOperationError(t *testing.T) {
	db := openCLIIdentityDatabase(t, filepath.Join(t.TempDir(), identityDBName))
	ctx := context.Background()
	if _, err := db.db.ExecContext(ctx, "CREATE TABLE cli_unique_probe(value TEXT UNIQUE)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, "INSERT INTO cli_unique_probe(value) VALUES (?)", "same"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, "INSERT INTO cli_unique_probe(value) VALUES (?)", "same"); err == nil {
		t.Fatal("duplicate value unexpectedly succeeded")
	}
	// A failed sqlite3 shell is discarded by database/sql through Validator.
	// The next operation must transparently acquire a fresh CLI connection.
	var count int
	if err := db.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cli_unique_probe").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count=%d want 1", count)
	}
}
