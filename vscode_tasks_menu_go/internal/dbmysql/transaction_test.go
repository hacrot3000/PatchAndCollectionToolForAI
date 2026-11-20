package dbmysql

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func writeTransactionFixture(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" { t.Skip("shell fixture is POSIX-only") }
	dir := t.TempDir()
	path := filepath.Join(dir, "mysql")
	state := filepath.Join(dir, "committed.state")
	logPath := filepath.Join(dir, "sql.log")
	cancelPath := filepath.Join(dir, "cancel.state")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "mysql fixture 8.0"; exit 0; fi
pending=0
while IFS= read -r sql; do
  if [ -n "$TASKDECK_MYSQL_TX_LOG" ]; then printf "%s\n" "$sql" >> "$TASKDECK_MYSQL_TX_LOG"; fi
  case "$sql" in
    "START TRANSACTION;") pending=0 ;;
    "SELECT CONNECTION_ID() AS taskdeck_connection_id;")
      printf "%s\n" '<resultset><row><field name="taskdeck_connection_id">4242</field></row></resultset>' ;;
    "INSERT INTO tx_test(value) VALUES (1);") pending=1 ;;
    "SELECT COUNT(*) AS n FROM tx_test;")
      count=0
      if [ "$pending" = "1" ] || [ -f "$TASKDECK_MYSQL_TX_STATE" ]; then count=1; fi
      printf '<resultset><row><field name="n">%s</field></row></resultset>\n' "$count" ;;
    "COMMIT;")
      if [ "$pending" = "1" ]; then : > "$TASKDECK_MYSQL_TX_STATE"; fi
      pending=0 ;;
    "ROLLBACK;") pending=0 ;;
    *taskdeck_slow*)
      while [ ! -f "$TASKDECK_MYSQL_TX_CANCEL" ]; do sleep 0.01; done
      printf "%s\n" "ERROR 1317 (70100): Query execution was interrupted" >&2 ;;
    USE*) : ;;
    KILL\ QUERY*)
      : > "$TASKDECK_MYSQL_TX_CANCEL" ;;
    SELECT\ 1\ AS\ taskdeck_connect\;*)
      printf "%s\n" '<resultset><row><field name="taskdeck_connect">1</field></row></resultset>' ;;
    *__taskdeck_boundary*)
      token=$(printf "%s" "$sql" | sed -n "s/^SELECT '\([^']*\)' AS __taskdeck_boundary;$/\1/p")
      printf '<resultset><row><field name="__taskdeck_boundary">%s</field></row></resultset>\n' "$token" ;;
    *) printf "%s\n" '<resultset></resultset>' ;;
  esac
done
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { t.Fatal(err) }
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TASKDECK_MYSQL_TX_STATE", state)
	t.Setenv("TASKDECK_MYSQL_TX_LOG", logPath)
	t.Setenv("TASKDECK_MYSQL_TX_CANCEL", cancelPath)
	return state, logPath
}

func transactionFixtureClient(t *testing.T) Client {
	t.Helper()
	client, err := FindClient()
	if err != nil { t.Fatal(err) }
	return client
}

func TestMySQLTransactionWorkerCommitRollbackAndCatalog(t *testing.T) {
	state, logPath := writeTransactionFixture(t)
	client := transactionFixtureClient(t)
	config := Config{Host: "127.0.0.1", Port: 3306, Username: "app", Database: "main", Charset: defaultCharset, ConnectTimeoutSeconds: defaultConnectTimeout}

	worker, err := startMySQLTransaction(context.Background(), client, config)
	if err != nil { t.Fatal(err) }
	if worker.connectionID != 4242 { t.Fatalf("connection id=%d", worker.connectionID) }
	if _, err := worker.Execute(context.Background(), config, "INSERT INTO tx_test(value) VALUES (1)", 10); err != nil { t.Fatal(err) }
	inside, err := worker.Execute(context.Background(), config, "SELECT COUNT(*) AS n FROM tx_test", 10)
	if err != nil { t.Fatal(err) }
	if len(inside.Rows) != 1 || inside.Rows[0][0] != "1" { t.Fatalf("inside transaction=%+v", inside) }
	if _, err := worker.Rollback(context.Background()); err != nil { t.Fatal(err) }
	if _, err := os.Stat(state); !os.IsNotExist(err) { t.Fatalf("rollback persisted state: err=%v", err) }

	worker, err = startMySQLTransaction(context.Background(), client, config)
	if err != nil { t.Fatal(err) }
	selected := config; selected.Database = "selected_db"
	if _, err := worker.Execute(context.Background(), selected, "INSERT INTO tx_test(value) VALUES (1)", 10); err != nil { t.Fatal(err) }
	if _, err := worker.Commit(context.Background()); err != nil { t.Fatal(err) }
	if _, err := os.Stat(state); err != nil { t.Fatalf("commit did not persist state: %v", err) }
	raw, err := os.ReadFile(logPath)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(raw), "USE `selected_db`;") { t.Fatalf("catalog switch missing from log:\n%s", raw) }
}

func TestMySQLTransactionCancelKeepsSessionAlive(t *testing.T) {
	_, logPath := writeTransactionFixture(t)
	_ = os.Remove(os.Getenv("TASKDECK_MYSQL_TX_CANCEL"))
	client := transactionFixtureClient(t)
	config := Config{Host: "127.0.0.1", Port: 3306, Username: "app", Database: "main", Charset: defaultCharset, ConnectTimeoutSeconds: defaultConnectTimeout}
	worker, err := startMySQLTransaction(context.Background(), client, config)
	if err != nil { t.Fatal(err) }
	defer worker.rollbackBestEffort()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err = worker.Execute(ctx, config, "SELECT taskdeck_slow", 10)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		raw, _ := os.ReadFile(logPath)
		t.Fatalf("cancel error=%v\nSQL log:\n%s", err, raw)
	}
	if worker.broken || worker.closed {
		t.Fatalf("worker unusable after query cancel: broken=%v closed=%v", worker.broken, worker.closed)
	}
	result, err := worker.Execute(context.Background(), config, "SELECT COUNT(*) AS n FROM tx_test", 10)
	if err != nil { t.Fatalf("query after cancel failed: %v", err) }
	if len(result.Rows) != 1 || result.Rows[0][0] != "0" {
		t.Fatalf("query after cancel=%+v", result)
	}
}

func TestMySQLHandlerTransactionLifecycleAndGridGate(t *testing.T) {
	writeTransactionFixture(t)
	handler, err := NewHandler(os.Args[0])
	if err != nil { t.Fatal(err) }
	client := transactionFixtureClient(t)
	handler.client = client
	handler.config = Config{Host: "127.0.0.1", Port: 3306, Username: "app", Database: "main", Charset: defaultCharset, ConnectTimeoutSeconds: defaultConnectTimeout}
	handler.connected = true
	defer handler.disconnect()

	begin, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "tx-begin", dbadapter.OpBegin, nil))
	if protocolErr != nil { t.Fatalf("begin error=%+v", protocolErr) }
	if result := begin.(dbadapter.TransactionResult); !result.Active { t.Fatalf("begin=%+v", result) }
	if handler.transaction == nil { t.Fatal("transaction worker not retained") }

	_, protocolErr = handler.Handle(context.Background(), adapterRequest(t, "tx-grid", dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{Name: "tx_test", Limit: 10}))
	if protocolErr == nil || protocolErr.Code != "TRANSACTION_ACTIVE" { t.Fatalf("grid gate=%+v", protocolErr) }

	_, protocolErr = handler.Handle(context.Background(), adapterRequest(t, "tx-insert", dbadapter.OpExecute, dbadapter.ExecutePayload{Statement: "INSERT INTO tx_test(value) VALUES (1)", MaxRows: 10}))
	if protocolErr != nil { t.Fatalf("transaction execute=%+v", protocolErr) }

	commit, protocolErr := handler.Handle(context.Background(), adapterRequest(t, "tx-commit", dbadapter.OpCommit, nil))
	if protocolErr != nil { t.Fatalf("commit error=%+v", protocolErr) }
	if result := commit.(dbadapter.TransactionResult); result.Active { t.Fatalf("commit=%+v", result) }
	if handler.transaction != nil { t.Fatal("transaction worker retained after commit") }
}

func TestMySQLManifestAdvertisesTransactionsAndCancel(t *testing.T) {
	manifest, err := BuiltinManifest("/opt/taskdeck")
	if err != nil { t.Fatal(err) }
	if !manifest.Capabilities.Transactions || !manifest.Capabilities.Cancel { t.Fatalf("capabilities=%+v", manifest.Capabilities) }
}
