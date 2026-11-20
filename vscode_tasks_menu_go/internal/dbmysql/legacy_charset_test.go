package dbmysql

import (
    "context"
    "os"
    "path/filepath"
    "runtime"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/dbadapter"
)

// Emulate a MySQL server old enough to accept --default-character-set=utf8
// and ordinary SELECT, but reject all SQL statements mentioning utf8mb4.
func TestLegacyMySQLUTF8ConnectionCanListTablesAndInspectMetadata(t *testing.T) {
    if runtime.GOOS == "windows" {
        t.Skip("shell fixture is POSIX-only")
    }
    dir := t.TempDir()
    clientPath := filepath.Join(dir, "mysql")
    logPath := filepath.Join(dir, "sql.txt")
    argsPath := filepath.Join(dir, "argv.txt")
    t.Setenv("TASKDECK_MYSQL_LEGACY_LOG", logPath)
    t.Setenv("TASKDECK_MYSQL_LEGACY_ARGS", argsPath)
    script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo 'mysql  Ver 14.14 Distrib 5.5.0'; exit 0; fi
printf '%s\n' "$@" >> "$TASKDECK_MYSQL_LEGACY_ARGS"
for arg in "$@"; do
  case "$arg" in
    --default-character-set=utf8) ;;
    --default-character-set=*)
      echo "ERROR 1115 (42000): Unknown character set" >&2
      exit 1 ;;
  esac
done
sql=$(cat)
printf '%s\n' "$sql" >> "$TASKDECK_MYSQL_LEGACY_LOG"
case "$sql" in
  *utf8mb4*)
    echo "ERROR 1115 (42000) at line 1: Unknown character set: 'utf8mb4'" >&2
    exit 1 ;;
  *taskdeck_connect*)
    echo '<resultset><row><field name="taskdeck_connect">1</field></row></resultset>' ;;
  *taskdeck_database_inspect*)
    echo '<resultset><row><field name="charset">utf8</field><field name="collation">utf8_general_ci</field><field name="table_count">1</field><field name="view_count">0</field><field name="size_bytes">2048</field></row></resultset>' ;;
  *information_schema.TABLES*)
    echo '<resultset><row><field name="catalog">main</field><field name="name">users</field><field name="kind">table</field></row></resultset>' ;;
  *taskdeck_describe_create*)
    echo '<resultset><row><field name="Table">users</field><field name="Create Table">CREATE TABLE users (id int PRIMARY KEY)</field></row></resultset>' ;;
  *taskdeck_describe_indexes*)
    echo '<resultset><row><field name="name">PRIMARY</field><field name="non_unique">0</field><field name="column_name">id</field><field name="seq">1</field><field name="index_type">BTREE</field></row></resultset>' ;;
  *taskdeck_describe_foreign_keys*|*taskdeck_describe_referenced_by*)
    echo '<resultset></resultset>' ;;
  *information_schema.COLUMNS*)
    case "$sql" in
      *CHARACTER_SET_NAME*)
        echo '<resultset><row><field name="name">id</field><field name="type">int</field><field name="nullable">NO</field><field name="character_set"></field><field name="collation"></field></row><row><field name="name">name</field><field name="type">varchar(255)</field><field name="nullable">YES</field><field name="character_set">utf8</field><field name="collation">utf8_general_ci</field></row></resultset>' ;;
      *)
        echo '<resultset><row><field name="name">id</field><field name="type">int</field><field name="nullable">NO</field><field name="default_value"></field><field name="extra"></field></row></resultset>' ;;
    esac ;;
  *information_schema.STATISTICS*)
    echo '<resultset><row><field name="index_name">PRIMARY</field><field name="column_name">id</field><field name="seq">1</field><field name="nullable">NO</field></row></resultset>' ;;
  *taskdeck_browse*)
    echo '<resultset><row><field name="id">1</field><field name="name">Alice</field></row></resultset>' ;;
  *'SELECT 42 AS answer'*)
    echo '<resultset><row><field name="answer">42</field></row></resultset>' ;;
  *)
    echo '<resultset></resultset>' ;;
esac
`
    if err := os.WriteFile(clientPath, []byte(script), 0o700); err != nil {
        t.Fatal(err)
    }
    t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
    h, err := NewHandler(os.Args[0])
    if err != nil {
        t.Fatal(err)
    }
    ctx := context.Background()
    connect := dbadapter.ConnectPayload{
        Host: "127.0.0.1", Port: 3306, Username: "app",
        Database: "main", Options: map[string]string{"charset": "utf8"},
    }
    if _, pe := h.Handle(ctx, adapterRequest(t, "legacy-connect", dbadapter.OpConnect, connect)); pe != nil {
        t.Fatalf("legacy MySQL connect: %+v", pe)
    }
    defer h.disconnect()
    if h.config.Charset != "utf8" {
        t.Fatalf("connection lost explicit charset: %s", h.config.Charset)
    }
    if result, pe := h.Handle(ctx, adapterRequest(t, "legacy-select", dbadapter.OpExecute,
        dbadapter.ExecutePayload{Statement: "SELECT 42 AS answer", MaxRows: 5})); pe != nil {
        t.Fatalf("direct SELECT must work: %+v", pe)
    } else if data, ok := result.(dbadapter.ExecuteResult); !ok || len(data.Rows) != 1 || data.Rows[0][0] != "42" {
        t.Fatalf("direct SELECT result=%#v", result)
    }

    result, pe := h.Handle(ctx, adapterRequest(t, "legacy-tables", dbadapter.OpListObjects,
        dbadapter.ListObjectsPayload{Catalog: "main"}))
    if pe != nil {
        t.Fatalf("list tables should not require utf8mb4: %+v", pe)
    }
    values, ok := result.(map[string]interface{})
    if !ok {
        t.Fatalf("list tables response=%#v", result)
    }
    objects, ok := values["objects"].([]dbadapter.Object)
    if !ok || len(objects) != 1 || objects[0].Name != "users" {
        t.Fatalf("list tables response=%#v", result)
    }
    if _, pe := h.Handle(ctx, adapterRequest(t, "legacy-describe", dbadapter.OpDescribeObject,
        dbadapter.DescribeObjectPayload{Catalog: "main", Name: "users"})); pe != nil {
        t.Fatalf("describe legacy table: %+v", pe)
    }
    if _, pe := h.Handle(ctx, adapterRequest(t, "legacy-inspect", dbadapter.OpObjectAction,
        dbadapter.ObjectActionPayload{Kind: "database", Action: "inspect", Name: "main"})); pe != nil {
        t.Fatalf("inspect legacy database: %+v", pe)
    }
    if _, pe := h.Handle(ctx, adapterRequest(t, "legacy-browse", dbadapter.OpBrowseRows,
        dbadapter.BrowseRowsPayload{Catalog: "main", Name: "users", Kind: "table", Limit: 10})); pe != nil {
        t.Fatalf("browse legacy table: %+v", pe)
    }
    sql, err := os.ReadFile(logPath)
    if err != nil {
        t.Fatal(err)
    }
    if strings.Contains(strings.ToLower(string(sql)), "utf8mb4") {
        t.Fatalf("legacy connection SQL still contains unsupported utf8mb4: %s", sql)
    }
    if !strings.Contains(string(sql), "USING utf8") {
        t.Fatalf("legacy metadata SQL did not use compatible utf8: %s", sql)
    }
    args, err := os.ReadFile(argsPath)
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(string(args), "--default-character-set=utf8") {
        t.Fatalf("MySQL CLI charset did not match connection profile: %s", args)
    }
}

func TestLegacyCharsetAppliedToTextAndBinaryProjectionExpressions(t *testing.T) {
    if got := mysqlTextExpression("thử nghiệm", "utf8"); !strings.Contains(got, " USING utf8)") || strings.Contains(got, "utf8mb4") {
        t.Fatalf("legacy text expression=%q", got)
    }
    if got := mysqlTextExpression("table", "latin1"); !strings.Contains(got, " USING utf8)") {
        t.Fatalf("UTF-8 hex literals must not be decoded as latin1: %q", got)
    }
    if got := mysqlTextExpression("table"); !strings.Contains(got, " USING utf8mb4)") {
        t.Fatalf("modern default charset changed: %q", got)
    }
    value, err := mysqlValueExpression("Alice", "utf8")
    if err != nil || strings.Contains(value, "utf8mb4") {
        t.Fatalf("value expression=%q err=%v", value, err)
    }
    filter, err := mysqlFilterExpression("name", dbadapter.RowFilter{Column: "name", Operator: "eq", Value: "Alice"}, "utf8")
    if err != nil || strings.Contains(filter, "utf8mb4") || !strings.Contains(filter, " USING utf8)") {
        t.Fatalf("filter expression=%q err=%v", filter, err)
    }
    projection, err := mysqlBrowseProjection([]mysqlBrowseColumn{{Name: "payload", Type: "blob"}}, "utf8")
    if err != nil || strings.Contains(projection, "utf8mb4") || !strings.Contains(projection, " USING utf8)") {
        t.Fatalf("binary hex projection=%q err=%v", projection, err)
    }
}
