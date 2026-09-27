package dbsqlite

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func createSQLiteFixture(t *testing.T, python Python) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.sqlite")
	script := "import sqlite3,sys\n" +
		"c=sqlite3.connect(sys.argv[1])\n" +
		"c.executescript('CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT NOT NULL, active INTEGER DEFAULT 1); CREATE INDEX idx_users_name ON users(name); INSERT INTO users(name,active) VALUES (\"Alice\",1),(\"Bob\",0); CREATE VIEW active_users AS SELECT id,name FROM users WHERE active=1;')\n" +
		"c.commit()\n"
	cmd := exec.Command(python.Path, "-I", "-c", script, path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create SQLite fixture: %v output=%s", err, output)
	}
	return path
}

func sqliteAdapterRequest(t *testing.T, id string, operation dbadapter.Operation, payload interface{}) dbadapter.Envelope {
	t.Helper()
	request, err := dbadapter.NewRequest(id, operation, payload)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func connectSQLiteHandler(t *testing.T, path string, readOnly bool) *Handler {
	t.Helper()
	handler, err := NewHandler(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	_, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "connect-1", dbadapter.OpConnect, dbadapter.ConnectPayload{
		File:     path,
		ReadOnly: readOnly,
	}))
	if protocolErr != nil {
		t.Fatalf("connect error=%+v", protocolErr)
	}
	return handler
}

func TestSQLiteHandlerBrowseDescribeReadOnlyAndWriteMode(t *testing.T) {
	python, err := FindPython()
	if err != nil {
		t.Skipf("Python 3 unavailable: %v", err)
	}
	path := createSQLiteFixture(t, python)

	readOnly := connectSQLiteHandler(t, path, true)
	defer readOnly.disconnect()

	catalogPayload, protocolErr := readOnly.Handle(context.Background(), sqliteAdapterRequest(t, "catalog-1", dbadapter.OpListCatalogs, nil))
	if protocolErr != nil {
		t.Fatalf("catalog error=%+v", protocolErr)
	}
	catalogs := catalogPayload.([]dbadapter.Object)
	if len(catalogs) != 1 || catalogs[0].Name != "main" {
		t.Fatalf("catalogs=%+v", catalogs)
	}

	objectsPayload, protocolErr := readOnly.Handle(context.Background(), sqliteAdapterRequest(t, "objects-1", dbadapter.OpListObjects, dbadapter.ListObjectsPayload{Catalog: "main"}))
	if protocolErr != nil {
		t.Fatalf("objects error=%+v", protocolErr)
	}
	objectsMap := objectsPayload.(map[string]interface{})
	objects := objectsMap["objects"].([]dbadapter.Object)
	if len(objects) != 2 {
		t.Fatalf("objects=%+v", objects)
	}

	detailPayload, protocolErr := readOnly.Handle(context.Background(), sqliteAdapterRequest(t, "describe-1", dbadapter.OpDescribeObject, dbadapter.DescribeObjectPayload{Catalog: "main", Name: "users"}))
	if protocolErr != nil {
		t.Fatalf("describe error=%+v", protocolErr)
	}
	detail := detailPayload.(map[string]interface{})
	if detail["name"] != "users" {
		t.Fatalf("detail=%+v", detail)
	}

	queryPayload, protocolErr := readOnly.Handle(context.Background(), sqliteAdapterRequest(t, "select-1", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "SELECT id,name,active FROM users ORDER BY id",
		MaxRows:   1,
	}))
	if protocolErr != nil {
		t.Fatalf("select error=%+v", protocolErr)
	}
	result := queryPayload.(dbadapter.ExecuteResult)
	if len(result.Rows) != 1 || !result.Truncated || result.Rows[0][1] != "Alice" {
		t.Fatalf("result=%+v", result)
	}

	_, protocolErr = readOnly.Handle(context.Background(), sqliteAdapterRequest(t, "write-ro", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "UPDATE users SET name='Mallory' WHERE id=1",
		MaxRows:   10,
	}))
	if protocolErr == nil || protocolErr.Code != "QUERY_FAILED" {
		t.Fatalf("read-only write protocol error=%+v", protocolErr)
	}
	readOnly.disconnect()

	readWrite := connectSQLiteHandler(t, path, false)
	defer readWrite.disconnect()
	writePayload, protocolErr := readWrite.Handle(context.Background(), sqliteAdapterRequest(t, "write-rw", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "UPDATE users SET name='Carol' WHERE id=1",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("write error=%+v", protocolErr)
	}
	writeResult := writePayload.(dbadapter.ExecuteResult)
	if writeResult.AffectedRows != 1 {
		t.Fatalf("write result=%+v", writeResult)
	}

	verifyPayload, protocolErr := readWrite.Handle(context.Background(), sqliteAdapterRequest(t, "verify-1", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "SELECT name FROM users WHERE id=1",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("verify error=%+v", protocolErr)
	}
	verify := verifyPayload.(dbadapter.ExecuteResult)
	if len(verify.Rows) != 1 || verify.Rows[0][0] != "Carol" {
		t.Fatalf("verify=%+v", verify)
	}
}

func TestSQLiteHelperRejectsMultipleStatements(t *testing.T) {
	python, err := FindPython()
	if err != nil {
		t.Skipf("Python 3 unavailable: %v", err)
	}
	path := createSQLiteFixture(t, python)
	handler := connectSQLiteHandler(t, path, false)
	defer handler.disconnect()

	_, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "multi-1", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "SELECT 1; DELETE FROM users",
		MaxRows:   10,
	}))
	if protocolErr == nil || !strings.Contains(protocolErr.Message, "one statement") {
		t.Fatalf("multi-statement error=%+v", protocolErr)
	}
}
