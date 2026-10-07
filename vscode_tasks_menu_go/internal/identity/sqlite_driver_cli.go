package identity

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cliSQLiteDriverName = "taskdeck-cli-sqlite"
	cliSQLiteMaxOutput  = 32 << 20
)

func init() {
	sql.Register(cliSQLiteDriverName, cliSQLiteDriver{})
}

type cliSQLiteDriver struct{}

func (cliSQLiteDriver) Open(name string) (driver.Conn, error) {
	return openCLISQLiteConn(name)
}

type cliSQLiteConn struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	stderr    *boundedTextBuffer
	nullToken string
	closed    bool
	waited    bool
}

type cliSQLiteStmt struct {
	conn  *cliSQLiteConn
	query string
}

type cliSQLiteTx struct {
	conn *cliSQLiteConn
}

type cliSQLiteResult struct {
	lastInsertID int64
	rowsAffected int64
}

type cliSQLiteRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

var (
	_ driver.Conn               = (*cliSQLiteConn)(nil)
	_ driver.ConnBeginTx        = (*cliSQLiteConn)(nil)
	_ driver.ExecerContext      = (*cliSQLiteConn)(nil)
	_ driver.QueryerContext     = (*cliSQLiteConn)(nil)
	_ driver.Pinger             = (*cliSQLiteConn)(nil)
	_ driver.Validator          = (*cliSQLiteConn)(nil)
	_ driver.Stmt               = (*cliSQLiteStmt)(nil)
	_ driver.StmtExecContext    = (*cliSQLiteStmt)(nil)
	_ driver.StmtQueryContext   = (*cliSQLiteStmt)(nil)
	_ driver.Result             = cliSQLiteResult{}
	_ driver.Rows               = (*cliSQLiteRows)(nil)
)

func resolveSQLiteCLICommand() (string, error) {
	configured := strings.TrimSpace(os.Getenv("TASKDECK_SQLITE3"))
	candidates := []string{"sqlite3"}
	if configured != "" {
		candidates = []string{configured}
	}
	var failures []string
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err != nil {
			failures = append(failures, candidate+": "+err.Error())
			continue
		}
		if err := probeSQLiteCLI(path); err != nil {
			failures = append(failures, path+": "+err.Error())
			continue
		}
		return path, nil
	}
	if configured != "" {
		return "", fmt.Errorf("TASKDECK_SQLITE3 is not a compatible sqlite3 CLI: %s", strings.Join(failures, "; "))
	}
	if len(failures) == 0 {
		return "", fmt.Errorf("sqlite3 CLI was not found in PATH")
	}
	return "", fmt.Errorf("compatible sqlite3 CLI unavailable: %s", strings.Join(failures, "; "))
}

func probeSQLiteCLI(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	const probe = "CREATE TABLE t(k TEXT UNIQUE); INSERT INTO t(k) VALUES('x') ON CONFLICT(k) DO NOTHING; INSERT INTO t(k) VALUES('x') ON CONFLICT(k) DO NOTHING; SELECT count(*) FROM t;"
	cmd := exec.CommandContext(ctx, path, "-batch", ":memory:", probe)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("sqlite3 capability probe timed out")
	}
	if err != nil {
		return fmt.Errorf("sqlite3 lacks required UPSERT support (SQLite 3.24+ required): %s", strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "1" {
		return fmt.Errorf("unexpected sqlite3 capability probe output %q", strings.TrimSpace(string(output)))
	}
	return nil
}

func openCLISQLiteConn(dbPath string) (*cliSQLiteConn, error) {
	executable, err := resolveSQLiteCLICommand()
	if err != nil {
		return nil, err
	}
	nullToken, err := newSQLiteCLIToken("NULL")
	if err != nil {
		return nil, err
	}
	ready, err := newSQLiteCLIToken("READY")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(executable, "-batch", dbPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create sqlite3 CLI stdin: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("create sqlite3 CLI stdout: %w", err)
	}
	stderr := &boundedTextBuffer{limit: 32 * 1024}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start sqlite3 CLI: %w", err)
	}

	conn := &cliSQLiteConn{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    bufio.NewReader(stdoutPipe),
		stderr:    stderr,
		nullToken: nullToken,
	}
	setup := strings.Join([]string{
		".bail on",
		".headers on",
		".mode csv",
		".nullvalue " + nullToken,
		".print " + ready,
		"",
	}, "\n")
	if _, err := io.WriteString(stdin, setup); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("initialize sqlite3 CLI: %w", err)
	}
	if _, err := conn.readUntilMarkerLocked(context.Background(), ready); err != nil {
		_ = conn.closeLocked()
		return nil, fmt.Errorf("initialize sqlite3 CLI: %w", err)
	}
	return conn, nil
}

func newSQLiteCLIToken(kind string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate sqlite3 CLI framing token: %w", err)
	}
	if kind == "NULL" {
		// sqlite3 shell truncates .nullvalue to a small fixed buffer on older
		// releases. Keep this token below 20 bytes so exact NULL framing works
		// on the legacy distributions where this fallback is needed most.
		return "__TDN_" + hex.EncodeToString(raw[:6]), nil
	}
	return "__TASKDECK_" + kind + "_" + hex.EncodeToString(raw[:]) + "__", nil
}

func (c *cliSQLiteConn) Prepare(query string) (driver.Stmt, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("SQLite statement is empty")
	}
	return &cliSQLiteStmt{conn: c, query: query}, nil
}

func (c *cliSQLiteConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *cliSQLiteConn) closeLocked() error {
	if c.closed {
		return nil
	}
	c.closed = true
	_, _ = io.WriteString(c.stdin, ".quit\n")
	_ = c.stdin.Close()
	if c.waited {
		return nil
	}
	c.waited = true
	err := c.cmd.Wait()
	if err != nil {
		detail := strings.TrimSpace(c.stderr.String())
		if detail != "" {
			return fmt.Errorf("stop sqlite3 CLI: %s", detail)
		}
	}
	return nil
}

func (c *cliSQLiteConn) IsValid() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed
}

func (c *cliSQLiteConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *cliSQLiteConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
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
	return &cliSQLiteTx{conn: c}, nil
}

func (c *cliSQLiteConn) Ping(ctx context.Context) error {
	rows, err := c.QueryContext(ctx, "SELECT 1", nil)
	if err != nil {
		return err
	}
	return rows.Close()
}

func (c *cliSQLiteConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	bound, err := bindSQLiteCLIArgs(query, args)
	if err != nil {
		return nil, err
	}
	marker, err := newSQLiteCLIToken("END")
	if err != nil {
		return nil, err
	}
	script := ".headers off\n.mode csv\n" + terminateSQLiteStatement(bound) +
		"\nSELECT changes(), last_insert_rowid();\n.print " + marker + "\n"
	payload, err := c.exchange(ctx, script, marker)
	if err != nil {
		return nil, err
	}
	records, err := readSQLiteCSV(payload)
	if err != nil {
		return nil, fmt.Errorf("parse sqlite3 CLI exec result: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("sqlite3 CLI returned no exec metadata")
	}
	last := records[len(records)-1]
	if len(last) != 2 {
		return nil, fmt.Errorf("sqlite3 CLI returned invalid exec metadata")
	}
	affected, err := strconv.ParseInt(strings.TrimSpace(last[0]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse sqlite3 rows affected: %w", err)
	}
	lastID, err := strconv.ParseInt(strings.TrimSpace(last[1]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse sqlite3 last insert id: %w", err)
	}
	return cliSQLiteResult{lastInsertID: lastID, rowsAffected: affected}, nil
}

func (c *cliSQLiteConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	bound, err := bindSQLiteCLIArgs(query, args)
	if err != nil {
		return nil, err
	}
	marker, err := newSQLiteCLIToken("END")
	if err != nil {
		return nil, err
	}
	script := ".headers on\n.mode csv\n" + terminateSQLiteStatement(bound) +
		"\n.print " + marker + "\n"
	payload, err := c.exchange(ctx, script, marker)
	if err != nil {
		return nil, err
	}
	records, err := readSQLiteCSV(payload)
	if err != nil {
		return nil, fmt.Errorf("parse sqlite3 CLI query result: %w", err)
	}
	if len(records) == 0 {
		return &cliSQLiteRows{}, nil
	}
	columns := append([]string(nil), records[0]...)
	rows := make([][]driver.Value, 0, len(records)-1)
	for _, record := range records[1:] {
		if len(record) != len(columns) {
			return nil, fmt.Errorf("sqlite3 CLI query returned %d values for %d columns", len(record), len(columns))
		}
		row := make([]driver.Value, len(record))
		for i, value := range record {
			if value == c.nullToken {
				row[i] = nil
			} else {
				row[i] = value
			}
		}
		rows = append(rows, row)
	}
	return &cliSQLiteRows{columns: columns, rows: rows}, nil
}

func (c *cliSQLiteConn) exchange(ctx context.Context, script, marker string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, driver.ErrBadConn
	}
	if _, err := io.WriteString(c.stdin, script); err != nil {
		return nil, c.processFailureLocked("write sqlite3 CLI request", err)
	}
	payload, err := c.readUntilMarkerLocked(ctx, marker)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, c.processFailureLocked("read sqlite3 CLI response", err)
	}
	return payload, nil
}

type cliReadResult struct {
	payload []byte
	err     error
}

func (c *cliSQLiteConn) readUntilMarkerLocked(ctx context.Context, marker string) ([]byte, error) {
	result := make(chan cliReadResult, 1)
	go func() {
		var out bytes.Buffer
		for {
			line, err := c.stdout.ReadString('\n')
			if line != "" {
				if strings.TrimRight(line, "\r\n") == marker {
					result <- cliReadResult{payload: out.Bytes()}
					return
				}
				if out.Len()+len(line) > cliSQLiteMaxOutput {
					result <- cliReadResult{err: fmt.Errorf("sqlite3 CLI response exceeds %d bytes", cliSQLiteMaxOutput)}
					return
				}
				_, _ = out.WriteString(line)
			}
			if err != nil {
				result <- cliReadResult{err: err}
				return
			}
		}
	}()

	select {
	case res := <-result:
		return res.payload, res.err
	case <-ctx.Done():
		c.closed = true
		_ = c.stdin.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		if !c.waited {
			c.waited = true
			_ = c.cmd.Wait()
		}
		<-result
		return nil, ctx.Err()
	}
}

func (c *cliSQLiteConn) processFailureLocked(action string, cause error) error {
	c.closed = true
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	if !c.waited {
		c.waited = true
		_ = c.cmd.Wait()
	}
	detail := strings.TrimSpace(c.stderr.String())
	if detail == "" {
		detail = cause.Error()
	}
	return fmt.Errorf("%s: %s", action, detail)
}

func terminateSQLiteStatement(query string) string {
	query = strings.TrimSpace(query)
	if strings.HasSuffix(query, ";") {
		return query
	}
	return query + ";"
}

func readSQLiteCSV(payload []byte) ([][]string, error) {
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, nil
	}
	reader := csv.NewReader(bytes.NewReader(payload))
	reader.FieldsPerRecord = -1
	return reader.ReadAll()
}

func bindSQLiteCLIArgs(query string, args []driver.NamedValue) (string, error) {
	if len(args) == 0 {
		return query, nil
	}
	var out strings.Builder
	out.Grow(len(query) + len(args)*24)
	argIndex := 0
	const (
		stateNormal = iota
		stateSingle
		stateDouble
		stateBacktick
		stateBracket
		stateLineComment
		stateBlockComment
	)
	state := stateNormal
	for i := 0; i < len(query); i++ {
		ch := query[i]
		switch state {
		case stateSingle:
			out.WriteByte(ch)
			if ch == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					i++
					out.WriteByte(query[i])
				} else {
					state = stateNormal
				}
			}
			continue
		case stateDouble:
			out.WriteByte(ch)
			if ch == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					i++
					out.WriteByte(query[i])
				} else {
					state = stateNormal
				}
			}
			continue
		case stateBacktick:
			out.WriteByte(ch)
			if ch == '`' {
				state = stateNormal
			}
			continue
		case stateBracket:
			out.WriteByte(ch)
			if ch == ']' {
				state = stateNormal
			}
			continue
		case stateLineComment:
			out.WriteByte(ch)
			if ch == '\n' {
				state = stateNormal
			}
			continue
		case stateBlockComment:
			out.WriteByte(ch)
			if ch == '*' && i+1 < len(query) && query[i+1] == '/' {
				i++
				out.WriteByte('/')
				state = stateNormal
			}
			continue
		}

		if ch == '\'' {
			state = stateSingle
			out.WriteByte(ch)
			continue
		}
		if ch == '"' {
			state = stateDouble
			out.WriteByte(ch)
			continue
		}
		if ch == '`' {
			state = stateBacktick
			out.WriteByte(ch)
			continue
		}
		if ch == '[' {
			state = stateBracket
			out.WriteByte(ch)
			continue
		}
		if ch == '-' && i+1 < len(query) && query[i+1] == '-' {
			state = stateLineComment
			out.WriteString("--")
			i++
			continue
		}
		if ch == '/' && i+1 < len(query) && query[i+1] == '*' {
			state = stateBlockComment
			out.WriteString("/*")
			i++
			continue
		}
		if ch != '?' {
			out.WriteByte(ch)
			continue
		}
		if i+1 < len(query) && query[i+1] >= '0' && query[i+1] <= '9' {
			return "", fmt.Errorf("sqlite3 CLI fallback does not support numbered placeholders")
		}
		if argIndex >= len(args) {
			return "", fmt.Errorf("SQLite statement has more placeholders than arguments")
		}
		if args[argIndex].Name != "" {
			return "", fmt.Errorf("sqlite3 CLI fallback does not support named arguments")
		}
		literal, err := sqliteCLILiteral(args[argIndex].Value)
		if err != nil {
			return "", fmt.Errorf("encode SQLite argument %d: %w", args[argIndex].Ordinal, err)
		}
		out.WriteString(literal)
		argIndex++
	}
	if argIndex != len(args) {
		return "", fmt.Errorf("SQLite statement has %d placeholders for %d arguments", argIndex, len(args))
	}
	return out.String(), nil
}

func sqliteCLILiteral(value any) (string, error) {
	switch value := value.(type) {
	case nil:
		return "NULL", nil
	case string:
		return "CAST(X'" + strings.ToUpper(hex.EncodeToString([]byte(value))) + "' AS TEXT)", nil
	case []byte:
		return "X'" + strings.ToUpper(hex.EncodeToString(value)) + "'", nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "", fmt.Errorf("non-finite float is not supported")
		}
		return strconv.FormatFloat(value, 'g', -1, 64), nil
	case bool:
		if value {
			return "1", nil
		}
		return "0", nil
	case time.Time:
		text := value.UTC().Format(time.RFC3339Nano)
		return "CAST(X'" + strings.ToUpper(hex.EncodeToString([]byte(text))) + "' AS TEXT)", nil
	default:
		return "", fmt.Errorf("unsupported value type %T", value)
	}
}

func (s *cliSQLiteStmt) Close() error { return nil }
func (s *cliSQLiteStmt) NumInput() int { return -1 }

func (s *cliSQLiteStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), namedDriverValues(args))
}

func (s *cliSQLiteStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), namedDriverValues(args))
}

func (s *cliSQLiteStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}

func (s *cliSQLiteStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

func (tx *cliSQLiteTx) Commit() error {
	_, err := tx.conn.ExecContext(context.Background(), "COMMIT", nil)
	return err
}

func (tx *cliSQLiteTx) Rollback() error {
	_, err := tx.conn.ExecContext(context.Background(), "ROLLBACK", nil)
	return err
}

func (r cliSQLiteResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }
func (r cliSQLiteResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

func (r *cliSQLiteRows) Columns() []string { return r.columns }
func (r *cliSQLiteRows) Close() error      { return nil }

func (r *cliSQLiteRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}

