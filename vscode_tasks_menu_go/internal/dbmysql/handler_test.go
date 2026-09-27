package dbmysql

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func writeHandlerFixture(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "mysql")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo 'mysql fixture 8.0'; exit 0; fi\n" +
		"sql=$(cat)\n" +
		"case \"$sql\" in\n" +
		"  *taskdeck_connect*) printf '%s\\n' '<resultset><row><field name=\"taskdeck_connect\">1</field></row></resultset>' ;;\n" +
		"  *taskdeck_ping*) printf '%s\\n' '<resultset><row><field name=\"taskdeck_ping\">1</field></row></resultset>' ;;\n" +
		"  *information_schema.SCHEMATA*) printf '%s\\n' '<resultset><row><field name=\"name\">information_schema</field></row><row><field name=\"name\">main</field></row></resultset>' ;;\n" +
		"  *information_schema.TABLES*) printf '%s\\n' '<resultset><row><field name=\"catalog\">main</field><field name=\"name\">users</field><field name=\"kind\">table</field></row></resultset>' ;;\n" +
		"  *information_schema.STATISTICS*) printf '%s\\n' '<resultset><row><field name=\"index_name\">PRIMARY</field><field name=\"column_name\">id</field><field name=\"seq\">1</field><field name=\"nullable\">NO</field></row></resultset>' ;;\n" +
		"  *default_value*) printf '%s\\n' '<resultset><row><field name=\"name\">id</field><field name=\"type\">bigint</field><field name=\"nullable\">NO</field><field name=\"default_value\" xsi:nil=\"true\" xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\"/><field name=\"extra\">auto_increment</field></row></resultset>' ;;\n" +
		"  *information_schema.COLUMNS*) printf '%s\\n' '<resultset><row><field name=\"name\">id</field><field name=\"type\">bigint</field><field name=\"nullable\">NO</field></row><row><field name=\"name\">name</field><field name=\"type\">varchar(255)</field><field name=\"nullable\">YES</field></row></resultset>' ;;\n" +
		"  *taskdeck_browse*) printf '%s\\n' '<resultset><row><field name=\"id\">1</field><field name=\"name\">Alice</field></row><row><field name=\"id\">2</field><field name=\"name\">Bob</field></row></resultset>' ;;\n" +
		"  *taskdeck_mutation*) printf '%s\\n' '<resultset><row><field name=\"affected_rows\">1</field></row></resultset>' ;;\n" +
		"  *taskdeck_count*) printf '%s\\n' '<resultset><row><field name=\"row_count\">2</field></row></resultset>' ;;\n" +
		"  *taskdeck_object_action*) : ;;\n" +
		"  *'SELECT 42 AS answer'*) printf '%s\\n' '<resultset><row><field name=\"answer\">42</field></row></resultset>' ;;\n" +
		"  *) printf '%s\\n' '<resultset></resultset>' ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	currentPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+currentPath)
	return path
}

func adapterRequest(t *testing.T, id string, operation dbadapter.Operation, payload interface{}) dbadapter.Envelope {
	t.Helper()
	request, err := dbadapter.NewRequest(id, operation, payload)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func connectFixtureHandler(t *testing.T, readOnly bool) *Handler {
	t.Helper()
	writeHandlerFixture(t)
	handler, err := NewHandler(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	_, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "connect-1", dbadapter.OpConnect, dbadapter.ConnectPayload{
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "app",
		Database: "main",
		Secret:   "top-secret",
		ReadOnly: readOnly,
	}))
	if protocolErr != nil {
		t.Fatalf("connect error=%+v", protocolErr)
	}
	return handler
}

func TestHandlerConnectPingBrowseAndExecute(t *testing.T) {
	handler := connectFixtureHandler(t, false)
	if !handler.connected || handler.client.Path == "" {
		t.Fatalf("handler not connected: %+v", handler)
	}

	if _, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "ping-1", dbadapter.OpPing, nil)); protocolErr != nil {
		t.Fatalf("ping error=%+v", protocolErr)
	}

	catalogPayload, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "catalog-1", dbadapter.OpListCatalogs, nil))
	if protocolErr != nil {
		t.Fatalf("catalog error=%+v", protocolErr)
	}
	catalogs, ok := catalogPayload.([]dbadapter.Object)
	if !ok || len(catalogs) != 2 || catalogs[1].Name != "main" {
		t.Fatalf("catalogs=%#v", catalogPayload)
	}

	objectPayload, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "objects-1", dbadapter.OpListObjects, dbadapter.ListObjectsPayload{Catalog: "main"}))
	if protocolErr != nil {
		t.Fatalf("objects error=%+v", protocolErr)
	}
	objectsMap, ok := objectPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("objects payload type=%T", objectPayload)
	}
	objects, ok := objectsMap["objects"].([]dbadapter.Object)
	if !ok || len(objects) != 1 || objects[0].Name != "users" || objects[0].Kind != "table" {
		t.Fatalf("objects=%#v", objectPayload)
	}

	describePayload, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "describe-1", dbadapter.OpDescribeObject, dbadapter.DescribeObjectPayload{Catalog: "main", Name: "users"}))
	if protocolErr != nil {
		t.Fatalf("describe error=%+v", protocolErr)
	}
	describe, ok := describePayload.(map[string]interface{})
	if !ok {
		t.Fatalf("describe type=%T", describePayload)
	}
	columns, ok := describe["columns"].([]map[string]interface{})
	if !ok || len(columns) != 1 || columns[0]["name"] != "id" || columns[0]["nullable"] != false {
		t.Fatalf("describe=%#v", describePayload)
	}

	executePayload, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "execute-1", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "SELECT 42 AS answer",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("execute error=%+v", protocolErr)
	}
	result, ok := executePayload.(dbadapter.ExecuteResult)
	if !ok || len(result.Rows) != 1 || result.Rows[0][0] != "42" {
		t.Fatalf("result=%#v", executePayload)
	}

	disconnectPayload, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "disconnect-1", dbadapter.OpDisconnect, nil))
	if protocolErr != nil {
		t.Fatalf("disconnect error=%+v", protocolErr)
	}
	data, err := json.Marshal(disconnectPayload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "disconnected") {
		t.Fatalf("disconnect payload=%s", data)
	}
	if handler.connected || handler.config.Secret != "" {
		t.Fatalf("handler retained connection secret/state: %+v", handler)
	}
}

func TestHandlerReadOnlyRejectsWritesBeforeClientExecution(t *testing.T) {
	handler := connectFixtureHandler(t, true)
	_, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "execute-write", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "UPDATE users SET name='x'",
	}))
	if protocolErr == nil || protocolErr.Code != "READ_ONLY" {
		t.Fatalf("protocol error=%+v", protocolErr)
	}
}

func TestMySQLTextExpressionDoesNotEmbedRawValue(t *testing.T) {
	value := "db' OR 1=1 --"
	expression := mysqlTextExpression(value)
	if strings.Contains(expression, value) || !strings.HasPrefix(expression, "CONVERT(0x") {
		t.Fatalf("expression=%q", expression)
	}
}


func TestHandlerBrowseRowsUsesStablePrimaryKeyIdentity(t *testing.T) {
	handler := connectFixtureHandler(t, false)
	payload, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "browse-1", dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{
		Catalog: "main",
		Kind:    "table",
		Name:    "users",
		Limit:   100,
	}))
	if protocolErr != nil {
		t.Fatalf("browse error=%+v", protocolErr)
	}
	result, ok := payload.(dbadapter.BrowseRowsResult)
	if !ok {
		t.Fatalf("browse payload type=%T", payload)
	}
	if !result.Editable || result.EditabilityReason != "" {
		t.Fatalf("unexpected editability: %+v", result)
	}
	if len(result.Columns) != 2 || !result.Columns[0].Identity || result.Columns[0].Name != "id" {
		t.Fatalf("columns=%+v", result.Columns)
	}
	if len(result.Rows) != 2 || result.Rows[0].Identity["id"] != "1" || result.Rows[1].Values[1] != "Bob" {
		t.Fatalf("rows=%+v", result.Rows)
	}
}

func TestHandlerBrowseRowsHonorsReadOnlyProfile(t *testing.T) {
	handler := connectFixtureHandler(t, true)
	payload, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "browse-ro", dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{
		Catalog: "main",
		Kind:    "table",
		Name:    "users",
		Limit:   50,
	}))
	if protocolErr != nil {
		t.Fatalf("browse error=%+v", protocolErr)
	}
	result := payload.(dbadapter.BrowseRowsResult)
	if result.Editable || !strings.Contains(strings.ToLower(result.EditabilityReason), "read-only") {
		t.Fatalf("unexpected read-only browse result: %+v", result)
	}
}

func TestMySQLIdentifierQuotesNamesWithoutRawSQLSyntax(t *testing.T) {
	value := "odd`name"
	quoted, err := mysqlIdentifier(value)
	if err != nil {
		t.Fatal(err)
	}
	if quoted != "`odd``name`" {
		t.Fatalf("quoted=%q", quoted)
	}
}


func TestHandlerMutateRowsUsesStableIdentity(t *testing.T) {
	handler := connectFixtureHandler(t, false)
	payload, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "mutate-1", dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{
		Catalog: "main",
		Kind:    "table",
		Name:    "users",
		Mutations: []dbadapter.RowMutation{
			{Action: "insert", Values: map[string]interface{}{"name": "Carol"}},
			{Action: "update", Identity: map[string]interface{}{"id": "1"}, Values: map[string]interface{}{"name": "Alicia"}},
			{Action: "delete", Identity: map[string]interface{}{"id": "2"}},
		},
	}))
	if protocolErr != nil {
		t.Fatalf("mutate error=%+v", protocolErr)
	}
	result, ok := payload.(dbadapter.MutateRowsResult)
	if !ok || len(result.Results) != 3 {
		t.Fatalf("mutation result=%#v", payload)
	}
	for _, item := range result.Results {
		if item.Error != nil || item.AffectedRows != 1 {
			t.Fatalf("mutation item=%+v", item)
		}
	}
}

func TestHandlerMutateRowsRejectsReadOnlyBeforeClientMutation(t *testing.T) {
	handler := connectFixtureHandler(t, true)
	_, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "mutate-ro", dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{
		Catalog: "main",
		Name:    "users",
		Mutations: []dbadapter.RowMutation{{Action: "delete", Identity: map[string]interface{}{"id": "1"}}},
	}))
	if protocolErr == nil || protocolErr.Code != "READ_ONLY" {
		t.Fatalf("protocol error=%+v", protocolErr)
	}
}

func TestBuildMySQLMutationRejectsIncompleteIdentity(t *testing.T) {
	columns := map[string]mysqlBrowseColumn{
		"id":   {Name: "id"},
		"name": {Name: "name"},
	}
	_, err := buildMySQLMutation("`main`.`users`", columns, []string{"id"}, dbadapter.RowMutation{
		Action: "update",
		Values: map[string]interface{}{"name": "x"},
	})
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected identity error, got %v", err)
	}
}


func TestHandlerObjectActionsCountAndReadOnlyGate(t *testing.T) {
	readOnly := connectFixtureHandler(t, true)
	payload, protocolErr := readOnly.Handle(context.Background(), adapterRequest(t, "count-1", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "main", Kind: "table", Name: "users", Action: "count_rows",
	}))
	if protocolErr != nil {
		t.Fatalf("count error=%+v", protocolErr)
	}
	result, ok := payload.(dbadapter.ObjectActionResult)
	if !ok || result.Count == nil || *result.Count != 2 {
		t.Fatalf("count result=%#v", payload)
	}
	_, protocolErr = readOnly.Handle(context.Background(), adapterRequest(t, "truncate-ro", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "main", Kind: "table", Name: "users", Action: "truncate",
	}))
	if protocolErr == nil || protocolErr.Code != "READ_ONLY" {
		t.Fatalf("truncate protocol error=%+v", protocolErr)
	}
}

func TestHandlerObjectActionsAllowWriteProfileDrop(t *testing.T) {
	handler := connectFixtureHandler(t, false)
	payload, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "drop-1", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "main", Kind: "table", Name: "users", Action: "drop",
	}))
	if protocolErr != nil {
		t.Fatalf("drop error=%+v", protocolErr)
	}
	result, ok := payload.(dbadapter.ObjectActionResult)
	if !ok || !strings.Contains(strings.ToLower(result.Message), "dropped") {
		t.Fatalf("drop result=%#v", payload)
	}
}
