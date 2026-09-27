package dbmysql

import (
	"strings"
	"testing"
)

func TestNormalizeSingleStatementAllowsQuotedSemicolonAndTrailingTerminator(t *testing.T) {
	got, err := normalizeSingleStatement(" SELECT 'a;b', `semi;colon` FROM t;   ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "SELECT 'a;b', `semi;colon` FROM t" {
		t.Fatalf("statement=%q", got)
	}
}

func TestNormalizeSingleStatementRejectsMultipleStatements(t *testing.T) {
	for _, statement := range []string{
		"SELECT 1; SELECT 2",
		"SELECT 1;;",
		"SELECT 1; -- trailing comment",
		"SELECT 'unterminated",
		"SELECT 1 /* unterminated",
	} {
		if _, err := normalizeSingleStatement(statement); err == nil {
			t.Fatalf("statement %q unexpectedly accepted", statement)
		}
	}
}

func TestReadOnlyStatementAllowsConservativeReadOperations(t *testing.T) {
	for _, statement := range []string{
		"SELECT 1",
		"/* leading */ SELECT 'UPDATE x' AS text",
		"SHOW DATABASES",
		"DESCRIBE users",
		"DESC users",
		"EXPLAIN UPDATE users SET name='x'",
	} {
		if err := readOnlyStatement(statement); err != nil {
			t.Fatalf("statement %q rejected: %v", statement, err)
		}
	}
}

func TestReadOnlyStatementRejectsWritesAndDangerousSelectForms(t *testing.T) {
	for _, statement := range []string{
		"UPDATE users SET name='x'",
		"DELETE FROM users",
		"INSERT INTO users VALUES (1)",
		"CREATE TABLE x (id INT)",
		"WITH x AS (SELECT 1) SELECT * FROM x",
		"SELECT * FROM users FOR UPDATE",
		"SELECT * INTO OUTFILE '/tmp/x' FROM users",
		"SELECT * INTO DUMPFILE '/tmp/x' FROM users",
		"SELECT * FROM users LOCK IN SHARE MODE",
	} {
		if err := readOnlyStatement(statement); err == nil {
			t.Fatalf("statement %q unexpectedly accepted", statement)
		}
	}
}

func TestReadOnlyStatementRejectsExecutableComments(t *testing.T) {
	for _, statement := range []string{
		"SELECT 1 /*! INTO OUTFILE '/tmp/leak' */",
		"SELECT /*!50000 SQL_NO_CACHE */ 1",
		"SELECT 1 /*M! INTO OUTFILE '/tmp/leak' */",
	} {
		if err := readOnlyStatement(statement); err == nil || !strings.Contains(err.Error(), "executable comments") {
			t.Fatalf("statement %q error=%v", statement, err)
		}
	}
	if err := readOnlyStatement("SELECT '/*! not executable */' AS value"); err != nil {
		t.Fatalf("quoted marker rejected: %v", err)
	}
}

func TestReadOnlyKeywordCheckIgnoresQuotedAndCommentText(t *testing.T) {
	statement := "SELECT 'FOR UPDATE', 'INTO OUTFILE' /* LOCK IN SHARE MODE */"
	if err := readOnlyStatement(statement); err != nil {
		t.Fatal(err)
	}
}

func TestWrapReadOnlyStatementUsesReadOnlyTransaction(t *testing.T) {
	got := wrapReadOnlyStatement("SELECT 1")
	for _, want := range []string{
		"START TRANSACTION READ ONLY;",
		"SELECT 1;",
		"ROLLBACK",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("wrapped statement missing %q: %q", want, got)
		}
	}
}
