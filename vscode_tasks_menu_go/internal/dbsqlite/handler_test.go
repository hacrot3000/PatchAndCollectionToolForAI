package dbsqlite

import (
	"context"
	"fmt"
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


func TestSQLiteDescribeIncludesForeignKeysAndReferences(t *testing.T) {
	python, err := FindPython()
	if err != nil {
		t.Skipf("Python 3 unavailable: %v", err)
	}
	path := createSQLiteFixture(t, python)
	script := "import sqlite3,sys\n" +
		"c=sqlite3.connect(sys.argv[1])\n" +
		"c.execute('CREATE TABLE orders(id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY(user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL)')\n" +
		"c.commit()\n"
	cmd := exec.Command(python.Path, "-I", "-c", script, path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create SQLite FK fixture: %v output=%s", err, output)
	}

	handler := connectSQLiteHandler(t, path, true)
	defer handler.disconnect()

	ordersPayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "describe-orders", dbadapter.OpDescribeObject, dbadapter.DescribeObjectPayload{Catalog: "main", Name: "orders"}))
	if protocolErr != nil { t.Fatalf("describe orders error=%+v", protocolErr) }
	orders := ordersPayload.(map[string]interface{})
	foreignKeys, ok := orders["foreign_keys"].([]interface{})
	if !ok || len(foreignKeys) != 1 {
		t.Fatalf("orders foreign_keys=%#v", orders["foreign_keys"])
	}
	fk, ok := foreignKeys[0].(map[string]interface{})
	if !ok || fk["column"] != "user_id" || fk["referenced_table"] != "users" || fk["referenced_column"] != "id" || fk["delete_rule"] != "SET NULL" {
		t.Fatalf("orders foreign key=%#v", foreignKeys[0])
	}

	usersPayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "describe-users-ref", dbadapter.OpDescribeObject, dbadapter.DescribeObjectPayload{Catalog: "main", Name: "users"}))
	if protocolErr != nil { t.Fatalf("describe users error=%+v", protocolErr) }
	users := usersPayload.(map[string]interface{})
	referencedBy, ok := users["referenced_by"].([]interface{})
	if !ok || len(referencedBy) != 1 {
		t.Fatalf("users referenced_by=%#v", users["referenced_by"])
	}
	ref, ok := referencedBy[0].(map[string]interface{})
	if !ok || ref["table"] != "orders" || ref["column"] != "user_id" || ref["referenced_column"] != "id" {
		t.Fatalf("users reference=%#v", referencedBy[0])
	}
}

func TestSQLiteHandlerImportsSQLFileWithTransactionAndTrigger(t *testing.T) {
	python, err := FindPython()
	if err != nil {
		t.Skipf("Python 3 unavailable: %v", err)
	}
	path := createSQLiteFixture(t, python)
	handler := connectSQLiteHandler(t, path, false)
	defer handler.disconnect()

	scriptPath := filepath.Join(t.TempDir(), "import.sql")
	script := "BEGIN;\n" +
		"CREATE TABLE import_log(name TEXT);\n" +
		"CREATE TRIGGER users_import_log AFTER INSERT ON users BEGIN\n" +
		"  INSERT INTO import_log(name) VALUES (NEW.name);\n" +
		"END;\n" +
		"INSERT INTO users(name,active) VALUES ('Imported',1);\n" +
		"COMMIT;\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}

	payload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "import-1", dbadapter.OpImportSQL, dbadapter.ImportSQLPayload{
		Path:    scriptPath,
		Catalog: "main",
	}))
	if protocolErr != nil {
		t.Fatalf("import error=%+v", protocolErr)
	}
	result, ok := payload.(dbadapter.ImportSQLResult)
	if !ok || result.ImportedBytes != int64(len(script)) {
		t.Fatalf("import result=%#v", payload)
	}

	verifyPayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "import-verify", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "SELECT CAST((SELECT COUNT(*) FROM users WHERE name='Imported') AS TEXT) AS users_count, CAST((SELECT COUNT(*) FROM import_log WHERE name='Imported') AS TEXT) AS log_count",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("verify error=%+v", protocolErr)
	}
	verify := verifyPayload.(dbadapter.ExecuteResult)
	if len(verify.Rows) != 1 || verify.Rows[0][0] != "1" || verify.Rows[0][1] != "1" {
		t.Fatalf("verify=%+v", verify)
	}

	readOnly := connectSQLiteHandler(t, path, true)
	defer readOnly.disconnect()
	_, protocolErr = readOnly.Handle(context.Background(), sqliteAdapterRequest(t, "import-ro", dbadapter.OpImportSQL, dbadapter.ImportSQLPayload{Path: scriptPath}))
	if protocolErr == nil || protocolErr.Code != "READ_ONLY" {
		t.Fatalf("read-only import error=%+v", protocolErr)
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


func TestSQLiteHandlerWorkbenchBrowseAndReadOnlyGate(t *testing.T) {
	python, err := FindPython()
	if err != nil {
		t.Skipf("Python 3 unavailable: %v", err)
	}
	path := createSQLiteFixture(t, python)

	readOnly := connectSQLiteHandler(t, path, true)
	defer readOnly.disconnect()

	payload, protocolErr := readOnly.Handle(context.Background(), sqliteAdapterRequest(t, "browse-ro", dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{
		Catalog: "main",
		Kind:    "table",
		Name:    "users",
		Limit:   1,
	}))
	if protocolErr != nil {
		t.Fatalf("browse error=%+v", protocolErr)
	}
	result, ok := payload.(dbadapter.BrowseRowsResult)
	if !ok {
		t.Fatalf("browse payload type=%T", payload)
	}
	if len(result.Rows) != 1 || !result.HasMore || result.Rows[0].Values[1] != "Alice" {
		t.Fatalf("browse result=%+v", result)
	}
	if result.Editable || !strings.Contains(strings.ToLower(result.EditabilityReason), "read-only") {
		t.Fatalf("read-only editability=%+v", result)
	}
	if len(result.Columns) < 1 || result.Columns[0].Name != "id" || !result.Columns[0].Identity {
		t.Fatalf("browse columns=%+v", result.Columns)
	}

	_, protocolErr = readOnly.Handle(context.Background(), sqliteAdapterRequest(t, "mutate-ro", dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{
		Catalog: "main",
		Name:    "users",
		Mutations: []dbadapter.RowMutation{
			{Action: "delete", Identity: map[string]interface{}{"id": 1}},
		},
	}))
	if protocolErr == nil || protocolErr.Code != "READ_ONLY" {
		t.Fatalf("read-only mutation error=%+v", protocolErr)
	}

	countPayload, protocolErr := readOnly.Handle(context.Background(), sqliteAdapterRequest(t, "count-ro", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "main", Kind: "table", Name: "users", Action: "count_rows",
	}))
	if protocolErr != nil {
		t.Fatalf("count error=%+v", protocolErr)
	}
	count := countPayload.(dbadapter.ObjectActionResult)
	if count.Count == nil || *count.Count != 2 {
		t.Fatalf("count result=%+v", count)
	}

	_, protocolErr = readOnly.Handle(context.Background(), sqliteAdapterRequest(t, "truncate-ro", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "main", Kind: "table", Name: "users", Action: "truncate",
	}))
	if protocolErr == nil || protocolErr.Code != "READ_ONLY" {
		t.Fatalf("read-only truncate error=%+v", protocolErr)
	}
}

func TestSQLiteHandlerWorkbenchMutationsAndObjectActions(t *testing.T) {
	python, err := FindPython()
	if err != nil {
		t.Skipf("Python 3 unavailable: %v", err)
	}
	path := createSQLiteFixture(t, python)
	handler := connectSQLiteHandler(t, path, false)
	defer handler.disconnect()

	browsePayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "browse-rw", dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{
		Catalog: "main", Kind: "table", Name: "users", Limit: 100,
	}))
	if protocolErr != nil {
		t.Fatalf("browse error=%+v", protocolErr)
	}
	browse := browsePayload.(dbadapter.BrowseRowsResult)
	if !browse.Editable || len(browse.Rows) != 2 {
		t.Fatalf("browse result=%+v", browse)
	}

	mutatePayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "mutate-rw", dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{
		Catalog: "main",
		Kind:    "table",
		Name:    "users",
		Mutations: []dbadapter.RowMutation{
			{Action: "update", Identity: map[string]interface{}{"id": 1}, Values: map[string]interface{}{"name": "Alicia"}},
			{Action: "insert", Values: map[string]interface{}{"name": "Carol", "active": 1}},
			{Action: "delete", Identity: map[string]interface{}{"id": 2}},
		},
	}))
	if protocolErr != nil {
		t.Fatalf("mutate error=%+v", protocolErr)
	}
	mutations := mutatePayload.(dbadapter.MutateRowsResult)
	if len(mutations.Results) != 3 {
		t.Fatalf("mutation results=%+v", mutations)
	}
	for _, item := range mutations.Results {
		if item.Error != nil || item.AffectedRows != 1 {
			t.Fatalf("mutation item=%+v", item)
		}
	}

	verifyPayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "verify-browse", dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{
		Catalog: "main", Kind: "table", Name: "users", Limit: 100,
		Sort: []dbadapter.RowSort{{Column: "id", Direction: "asc"}},
	}))
	if protocolErr != nil {
		t.Fatalf("verify browse error=%+v", protocolErr)
	}
	verify := verifyPayload.(dbadapter.BrowseRowsResult)
	if len(verify.Rows) != 2 || verify.Rows[0].Values[1] != "Alicia" || verify.Rows[1].Values[1] != "Carol" {
		t.Fatalf("verify rows=%+v", verify.Rows)
	}

	truncatePayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "truncate-rw", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "main", Kind: "table", Name: "users", Action: "truncate",
	}))
	if protocolErr != nil {
		t.Fatalf("truncate error=%+v", protocolErr)
	}
	truncate := truncatePayload.(dbadapter.ObjectActionResult)
	if !strings.Contains(strings.ToLower(truncate.Message), "deleted") {
		t.Fatalf("truncate result=%+v", truncate)
	}

	countPayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "count-empty", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "main", Kind: "table", Name: "users", Action: "count_rows",
	}))
	if protocolErr != nil {
		t.Fatalf("count empty error=%+v", protocolErr)
	}
	count := countPayload.(dbadapter.ObjectActionResult)
	if count.Count == nil || *count.Count != 0 {
		t.Fatalf("empty count=%+v", count)
	}

	dropPayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "drop-view", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "main", Kind: "view", Name: "active_users", Action: "drop",
	}))
	if protocolErr != nil {
		t.Fatalf("drop view error=%+v", protocolErr)
	}
	drop := dropPayload.(dbadapter.ObjectActionResult)
	if !strings.Contains(strings.ToLower(drop.Message), "dropped") {
		t.Fatalf("drop result=%+v", drop)
	}
}

func TestSQLiteManifestAdvertisesWorkbenchCapabilities(t *testing.T) {
	manifest, err := BuiltinManifest("/opt/taskdeck")
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Capabilities.BrowseRows || !manifest.Capabilities.MutateRows || !manifest.Capabilities.ObjectActions {
		t.Fatalf("workbench capabilities=%+v", manifest.Capabilities)
	}
}

func TestSQLiteTransactionsCommitAndRollback(t *testing.T) {
	python, err := FindPython()
	if err != nil {
		t.Skipf("Python 3 unavailable: %v", err)
	}
	path := createSQLiteFixture(t, python)
	handler := connectSQLiteHandler(t, path, false)
	defer handler.disconnect()

	beginPayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "begin-1", dbadapter.OpBegin, nil))
	if protocolErr != nil {
		t.Fatalf("begin error=%+v", protocolErr)
	}
	if result := beginPayload.(dbadapter.TransactionResult); !result.Active {
		t.Fatalf("begin result=%+v", result)
	}
	if handler.transaction == nil {
		t.Fatal("transaction worker was not retained")
	}

	_, protocolErr = handler.Handle(context.Background(), sqliteAdapterRequest(t, "insert-1", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "INSERT INTO users(name,active) VALUES ('TxRollback',1)",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("transaction insert error=%+v", protocolErr)
	}
	insidePayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "inside-1", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "SELECT COUNT(*) AS n FROM users WHERE name='TxRollback'",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("transaction select error=%+v", protocolErr)
	}
	inside := insidePayload.(dbadapter.ExecuteResult)
	if len(inside.Rows) != 1 || fmt.Sprint(inside.Rows[0][0]) != "1" {
		t.Fatalf("inside transaction=%+v", inside)
	}

	_, protocolErr = handler.Handle(context.Background(), sqliteAdapterRequest(t, "browse-blocked", dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{Name: "users", Limit: 10}))
	if protocolErr == nil || protocolErr.Code != "TRANSACTION_ACTIVE" {
		t.Fatalf("grid operation should be blocked during transaction: %+v", protocolErr)
	}

	rollbackPayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "rollback-1", dbadapter.OpRollback, nil))
	if protocolErr != nil {
		t.Fatalf("rollback error=%+v", protocolErr)
	}
	if result := rollbackPayload.(dbadapter.TransactionResult); result.Active {
		t.Fatalf("rollback result=%+v", result)
	}
	if handler.transaction != nil {
		t.Fatal("transaction worker remains after rollback")
	}

	verifyPayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "verify-rollback", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "SELECT COUNT(*) AS n FROM users WHERE name='TxRollback'",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("verify rollback error=%+v", protocolErr)
	}
	verify := verifyPayload.(dbadapter.ExecuteResult)
	if len(verify.Rows) != 1 || fmt.Sprint(verify.Rows[0][0]) != "0" {
		t.Fatalf("rollback persisted row: %+v", verify)
	}

	_, protocolErr = handler.Handle(context.Background(), sqliteAdapterRequest(t, "begin-2", dbadapter.OpBegin, nil))
	if protocolErr != nil {
		t.Fatalf("second begin error=%+v", protocolErr)
	}
	_, protocolErr = handler.Handle(context.Background(), sqliteAdapterRequest(t, "insert-2", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "INSERT INTO users(name,active) VALUES ('TxCommit',1)",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("commit insert error=%+v", protocolErr)
	}
	commitPayload, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "commit-1", dbadapter.OpCommit, nil))
	if protocolErr != nil {
		t.Fatalf("commit error=%+v", protocolErr)
	}
	if result := commitPayload.(dbadapter.TransactionResult); result.Active {
		t.Fatalf("commit result=%+v", result)
	}

	verifyPayload, protocolErr = handler.Handle(context.Background(), sqliteAdapterRequest(t, "verify-commit", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "SELECT COUNT(*) AS n FROM users WHERE name='TxCommit'",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("verify commit error=%+v", protocolErr)
	}
	verify = verifyPayload.(dbadapter.ExecuteResult)
	if len(verify.Rows) != 1 || fmt.Sprint(verify.Rows[0][0]) != "1" {
		t.Fatalf("commit did not persist row: %+v", verify)
	}
}

func TestSQLiteTransactionDisconnectRollsBack(t *testing.T) {
	python, err := FindPython()
	if err != nil {
		t.Skipf("Python 3 unavailable: %v", err)
	}
	path := createSQLiteFixture(t, python)
	handler := connectSQLiteHandler(t, path, false)
	if _, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "begin-disconnect", dbadapter.OpBegin, nil)); protocolErr != nil {
		t.Fatal(protocolErr)
	}
	if _, protocolErr := handler.Handle(context.Background(), sqliteAdapterRequest(t, "insert-disconnect", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "INSERT INTO users(name,active) VALUES ('DisconnectRollback',1)", MaxRows: 10,
	})); protocolErr != nil {
		t.Fatal(protocolErr)
	}
	handler.disconnect()

	verifyHandler := connectSQLiteHandler(t, path, false)
	defer verifyHandler.disconnect()
	payload, protocolErr := verifyHandler.Handle(context.Background(), sqliteAdapterRequest(t, "verify-disconnect", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "SELECT COUNT(*) AS n FROM users WHERE name='DisconnectRollback'", MaxRows: 10,
	}))
	if protocolErr != nil {
		t.Fatal(protocolErr)
	}
	result := payload.(dbadapter.ExecuteResult)
	if fmt.Sprint(result.Rows[0][0]) != "0" {
		t.Fatalf("disconnect did not rollback: %+v", result)
	}
}

func TestSQLiteManifestAdvertisesTransactionsAndCancel(t *testing.T) {
	manifest, err := BuiltinManifest("/opt/taskdeck")
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Capabilities.Transactions || !manifest.Capabilities.Cancel {
		t.Fatalf("capabilities=%+v", manifest.Capabilities)
	}
}
