package identity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const pythonSQLiteDriverName = "taskdeck-python-sqlite"

func init() {
	sql.Register(pythonSQLiteDriverName, pythonSQLiteDriver{})
}

// OpenSQLiteStore opens the shared identity store using Python's standard-library
// sqlite3 module. This keeps TaskDeck's Go build free of a third-party SQLite
// dependency while still using the native SQLite locking/WAL implementation.
//
// Shared-server mode fails closed if no suitable Python 3 runtime with sqlite3
// support is available. Legacy mode never calls this function.
func OpenSQLiteStore(ctx context.Context, dbPath string) (Store, error) {
	return openSQLiteDatabase(ctx, pythonSQLiteDriverName, dbPath)
}

type pythonSQLiteDriver struct{}

func (pythonSQLiteDriver) Open(name string) (driver.Conn, error) {
	return openPythonSQLiteConn(name)
}

type pythonSQLiteConn struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	enc    *json.Encoder
	dec    *json.Decoder
	stderr *boundedTextBuffer
	closed bool
}

var (
	_ driver.Conn           = (*pythonSQLiteConn)(nil)
	_ driver.Pinger         = (*pythonSQLiteConn)(nil)
	_ driver.ExecerContext  = (*pythonSQLiteConn)(nil)
	_ driver.QueryerContext = (*pythonSQLiteConn)(nil)
	_ driver.ConnBeginTx    = (*pythonSQLiteConn)(nil)
)

type pythonSQLiteStmt struct {
	conn  *pythonSQLiteConn
	query string
}

var (
	_ driver.Stmt             = (*pythonSQLiteStmt)(nil)
	_ driver.StmtExecContext  = (*pythonSQLiteStmt)(nil)
	_ driver.StmtQueryContext = (*pythonSQLiteStmt)(nil)
)

type pythonSQLiteTx struct {
	conn *pythonSQLiteConn
}

type pythonSQLiteRequest struct {
	Op   string              `json:"op"`
	SQL  string              `json:"sql,omitempty"`
	Args []pythonSQLiteValue `json:"args,omitempty"`
}

type pythonSQLiteResponse struct {
	OK           bool                  `json:"ok"`
	Error        string                `json:"error,omitempty"`
	SQLite       string                `json:"sqlite_version,omitempty"`
	Columns      []string              `json:"columns,omitempty"`
	Rows         [][]pythonSQLiteValue `json:"rows,omitempty"`
	RowsAffected int64                 `json:"rows_affected,omitempty"`
	LastInsertID int64                 `json:"last_insert_id,omitempty"`
}

type pythonSQLiteValue struct {
	Type  string  `json:"t"`
	Text  string  `json:"s,omitempty"`
	Int   int64   `json:"i,omitempty"`
	Float float64 `json:"f,omitempty"`
	Bool  bool    `json:"b,omitempty"`
}

func openPythonSQLiteConn(dbPath string) (*pythonSQLiteConn, error) {
	executable, prefix, err := resolvePythonSQLiteCommand()
	if err != nil {
		return nil, err
	}

	args := append(append([]string(nil), prefix...), "-u", "-c", pythonSQLiteHelper, dbPath)
	cmd := exec.Command(executable, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create Python SQLite stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("create Python SQLite stdout: %w", err)
	}
	stderr := &boundedTextBuffer{limit: 16 * 1024}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start Python SQLite helper: %w", err)
	}

	conn := &pythonSQLiteConn{
		cmd:    cmd,
		stdin:  stdin,
		enc:    json.NewEncoder(stdin),
		dec:    json.NewDecoder(stdout),
		stderr: stderr,
	}
	var hello pythonSQLiteResponse
	if err := conn.dec.Decode(&hello); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("initialize Python SQLite helper: %s", detail)
	}
	if !hello.OK {
		_ = stdin.Close()
		_ = cmd.Wait()
		return nil, fmt.Errorf("initialize Python SQLite helper: %s", hello.Error)
	}
	return conn, nil
}

func resolvePythonSQLiteCommand() (string, []string, error) {
	if configured := strings.TrimSpace(os.Getenv("TASKDECK_PYTHON")); configured != "" {
		path, err := exec.LookPath(configured)
		if err != nil {
			return "", nil, fmt.Errorf("TASKDECK_PYTHON is not executable: %w", err)
		}
		return path, nil, nil
	}

	type candidate struct {
		name   string
		prefix []string
	}
	candidates := []candidate{{name: "python3"}, {name: "python"}}
	if runtime.GOOS == "windows" {
		candidates = append([]candidate{{name: "py", prefix: []string{"-3"}}}, candidates...)
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate.name); err == nil {
			return path, candidate.prefix, nil
		}
	}
	return "", nil, fmt.Errorf("shared-server SQLite requires Python 3.10+ with the standard sqlite3 module")
}

func (c *pythonSQLiteConn) Prepare(query string) (driver.Stmt, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("SQLite statement is empty")
	}
	return &pythonSQLiteStmt{conn: c, query: query}, nil
}

func (c *pythonSQLiteConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true

	_ = c.enc.Encode(pythonSQLiteRequest{Op: "close"})
	var response pythonSQLiteResponse
	_ = c.dec.Decode(&response)
	_ = c.stdin.Close()
	err := c.cmd.Wait()
	if err != nil {
		detail := strings.TrimSpace(c.stderr.String())
		if detail != "" {
			return fmt.Errorf("stop Python SQLite helper: %s", detail)
		}
	}
	return nil
}

func (c *pythonSQLiteConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *pythonSQLiteConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if opts.ReadOnly {
		return nil, fmt.Errorf("read-only SQLite transaction is not supported by this driver")
	}
	switch opts.Isolation {
	case driver.IsolationLevel(sql.LevelDefault), driver.IsolationLevel(sql.LevelSerializable):
	default:
		return nil, fmt.Errorf("unsupported SQLite isolation level %d", opts.Isolation)
	}
	if _, err := c.ExecContext(ctx, "BEGIN", nil); err != nil {
		return nil, err
	}
	return &pythonSQLiteTx{conn: c}, nil
}

func (c *pythonSQLiteConn) Ping(ctx context.Context) error {
	_, err := c.exchange(ctx, pythonSQLiteRequest{Op: "ping"})
	return err
}

func (c *pythonSQLiteConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	wireArgs, err := encodePythonSQLiteArgs(args)
	if err != nil {
		return nil, err
	}
	response, err := c.exchange(ctx, pythonSQLiteRequest{Op: "exec", SQL: query, Args: wireArgs})
	if err != nil {
		return nil, err
	}
	return pythonSQLiteResult{
		lastInsertID: response.LastInsertID,
		rowsAffected: response.RowsAffected,
	}, nil
}

func (c *pythonSQLiteConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	wireArgs, err := encodePythonSQLiteArgs(args)
	if err != nil {
		return nil, err
	}
	response, err := c.exchange(ctx, pythonSQLiteRequest{Op: "query", SQL: query, Args: wireArgs})
	if err != nil {
		return nil, err
	}
	rows := make([][]driver.Value, 0, len(response.Rows))
	for _, wireRow := range response.Rows {
		row := make([]driver.Value, len(wireRow))
		for i, value := range wireRow {
			decoded, err := decodePythonSQLiteValue(value)
			if err != nil {
				return nil, err
			}
			row[i] = decoded
		}
		rows = append(rows, row)
	}
	return &pythonSQLiteRows{columns: response.Columns, rows: rows}, nil
}

func (c *pythonSQLiteConn) exchange(ctx context.Context, request pythonSQLiteRequest) (pythonSQLiteResponse, error) {
	if err := ctx.Err(); err != nil {
		return pythonSQLiteResponse{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return pythonSQLiteResponse{}, driver.ErrBadConn
	}
	if err := c.enc.Encode(request); err != nil {
		return pythonSQLiteResponse{}, c.badConnError("write Python SQLite request", err)
	}

	var response pythonSQLiteResponse
	if err := c.dec.Decode(&response); err != nil {
		return pythonSQLiteResponse{}, c.badConnError("read Python SQLite response", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "unknown SQLite helper error"
		}
		return pythonSQLiteResponse{}, errors.New(response.Error)
	}
	if err := ctx.Err(); err != nil {
		return pythonSQLiteResponse{}, err
	}
	return response, nil
}

func (c *pythonSQLiteConn) badConnError(action string, cause error) error {
	detail := strings.TrimSpace(c.stderr.String())
	if detail == "" {
		detail = cause.Error()
	}
	return fmt.Errorf("%s: %s: %w", action, detail, driver.ErrBadConn)
}

func (s *pythonSQLiteStmt) Close() error { return nil }
func (s *pythonSQLiteStmt) NumInput() int { return -1 }

func (s *pythonSQLiteStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), namedDriverValues(args))
}

func (s *pythonSQLiteStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), namedDriverValues(args))
}

func (s *pythonSQLiteStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}

func (s *pythonSQLiteStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

func (tx *pythonSQLiteTx) Commit() error {
	_, err := tx.conn.ExecContext(context.Background(), "COMMIT", nil)
	return err
}

func (tx *pythonSQLiteTx) Rollback() error {
	_, err := tx.conn.ExecContext(context.Background(), "ROLLBACK", nil)
	return err
}

type pythonSQLiteResult struct {
	lastInsertID int64
	rowsAffected int64
}

func (r pythonSQLiteResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }
func (r pythonSQLiteResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

type pythonSQLiteRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *pythonSQLiteRows) Columns() []string { return r.columns }
func (r *pythonSQLiteRows) Close() error      { return nil }

func (r *pythonSQLiteRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}

func namedDriverValues(values []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(values))
	for i, value := range values {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: value}
	}
	return out
}

func encodePythonSQLiteArgs(args []driver.NamedValue) ([]pythonSQLiteValue, error) {
	out := make([]pythonSQLiteValue, len(args))
	for i, arg := range args {
		value, err := encodePythonSQLiteValue(arg.Value)
		if err != nil {
			return nil, fmt.Errorf("encode SQLite argument %d: %w", arg.Ordinal, err)
		}
		out[i] = value
	}
	return out, nil
}

func encodePythonSQLiteValue(value any) (pythonSQLiteValue, error) {
	switch value := value.(type) {
	case nil:
		return pythonSQLiteValue{Type: "n"}, nil
	case string:
		return pythonSQLiteValue{Type: "s", Text: value}, nil
	case []byte:
		return pythonSQLiteValue{Type: "x", Text: base64.StdEncoding.EncodeToString(value)}, nil
	case int64:
		return pythonSQLiteValue{Type: "i", Int: value}, nil
	case float64:
		return pythonSQLiteValue{Type: "f", Float: value}, nil
	case bool:
		return pythonSQLiteValue{Type: "b", Bool: value}, nil
	case time.Time:
		return pythonSQLiteValue{Type: "s", Text: value.UTC().Format(time.RFC3339Nano)}, nil
	default:
		return pythonSQLiteValue{}, fmt.Errorf("unsupported value type %T", value)
	}
}

func decodePythonSQLiteValue(value pythonSQLiteValue) (driver.Value, error) {
	switch value.Type {
	case "n":
		return nil, nil
	case "s":
		return value.Text, nil
	case "x":
		decoded, err := base64.StdEncoding.DecodeString(value.Text)
		if err != nil {
			return nil, fmt.Errorf("decode SQLite blob: %w", err)
		}
		return decoded, nil
	case "i":
		return value.Int, nil
	case "f":
		return value.Float, nil
	case "b":
		return value.Bool, nil
	default:
		return nil, fmt.Errorf("unsupported SQLite response type %q", value.Type)
	}
}

type boundedTextBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (b *boundedTextBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		b.data = append(b.data, p[:remaining]...)
	}
	return len(p), nil
}

func (b *boundedTextBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte(nil), b.data...))
}

const pythonSQLiteHelper = `
import base64
import json
import sqlite3
import sys

if sys.version_info < (3, 10):
    raise RuntimeError("TaskDeck shared-server SQLite requires Python 3.10+")

path = sys.argv[1]
db = sqlite3.connect(path, timeout=5.0, isolation_level=None)

def value_in(v):
    t = v.get("t")
    if t == "n":
        return None
    if t == "s":
        return v.get("s", "")
    if t == "x":
        return base64.b64decode(v.get("s", ""))
    if t == "i":
        return int(v.get("i", 0))
    if t == "f":
        return float(v.get("f", 0.0))
    if t == "b":
        return bool(v.get("b", False))
    raise ValueError("unsupported argument type: " + str(t))

def value_out(v):
    if v is None:
        return {"t": "n"}
    if isinstance(v, bool):
        return {"t": "b", "b": v}
    if isinstance(v, int):
        return {"t": "i", "i": v}
    if isinstance(v, float):
        return {"t": "f", "f": v}
    if isinstance(v, bytes):
        return {"t": "x", "s": base64.b64encode(v).decode("ascii")}
    return {"t": "s", "s": str(v)}

def exec_sql(sql, args):
    if args:
        cur = db.execute(sql, args)
        return cur.rowcount, cur.lastrowid or 0

    # sqlite3.Connection.execute accepts one statement only. The identity schema
    # is a fixed trusted script, so split it with sqlite3.complete_statement and
    # execute each statement on the existing transaction without executescript(),
    # which could alter transaction boundaries.
    pending = ""
    total = 0
    last_id = 0
    for line in sql.splitlines(True):
        pending += line
        if sqlite3.complete_statement(pending):
            statement = pending.strip()
            pending = ""
            if statement:
                cur = db.execute(statement)
                if cur.rowcount > 0:
                    total += cur.rowcount
                if cur.lastrowid:
                    last_id = cur.lastrowid
    if pending.strip():
        cur = db.execute(pending)
        if cur.rowcount > 0:
            total += cur.rowcount
        if cur.lastrowid:
            last_id = cur.lastrowid
    return total, last_id

print(json.dumps({"ok": True, "sqlite_version": sqlite3.sqlite_version}), flush=True)

for line in sys.stdin:
    try:
        req = json.loads(line)
        op = req.get("op")
        if op == "close":
            db.close()
            print(json.dumps({"ok": True}), flush=True)
            break
        if op == "ping":
            db.execute("SELECT 1").fetchone()
            print(json.dumps({"ok": True}), flush=True)
            continue

        sql = req.get("sql", "")
        args = [value_in(v) for v in req.get("args", [])]
        if op == "exec":
            affected, last_id = exec_sql(sql, args)
            print(json.dumps({
                "ok": True,
                "rows_affected": affected,
                "last_insert_id": last_id,
            }), flush=True)
            continue
        if op == "query":
            cur = db.execute(sql, args)
            rows = [[value_out(v) for v in row] for row in cur.fetchall()]
            columns = [d[0] for d in cur.description] if cur.description else []
            print(json.dumps({
                "ok": True,
                "columns": columns,
                "rows": rows,
            }), flush=True)
            continue
        raise ValueError("unsupported operation: " + str(op))
    except Exception as exc:
        print(json.dumps({
            "ok": False,
            "error": type(exc).__name__ + ": " + str(exc),
        }), flush=True)
`
