package dbmysql

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

type mysqlTransactionWorker struct {
	mu sync.Mutex

	client      Client
	config      Config
	credentials *credentialFile
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      *bufio.Reader

	diagMu     sync.Mutex
	diag       bytes.Buffer
	diagNotify chan struct{}

	connectionID int64
	closed       bool
	broken       bool
}

func startMySQLTransaction(ctx context.Context, client Client, config Config) (*mysqlTransactionWorker, error) {
	if ctx == nil {
		return nil, errors.New("MySQL transaction context is required")
	}
	if strings.TrimSpace(client.Path) == "" {
		return nil, errors.New("MySQL client path is required")
	}
	credentials, err := createCredentialFile(config.Secret)
	if err != nil {
		return nil, err
	}
	args := clientArgs(config, credentials)
	args = append(args, "--force", "--unbuffered")
	cmd := exec.Command(client.Path, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		if credentials != nil { credentials.Close() }
		return nil, fmt.Errorf("open MySQL transaction stdin: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		if credentials != nil { credentials.Close() }
		return nil, fmt.Errorf("open MySQL transaction stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close(); _ = stdoutPipe.Close()
		if credentials != nil { credentials.Close() }
		return nil, fmt.Errorf("open MySQL transaction stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close(); _ = stdoutPipe.Close(); _ = stderr.Close()
		if credentials != nil { credentials.Close() }
		return nil, fmt.Errorf("start MySQL transaction client: %w", err)
	}
	worker := &mysqlTransactionWorker{
		client: client, config: config, credentials: credentials, cmd: cmd, stdin: stdin,
		stdout: bufio.NewReaderSize(stdoutPipe, 64<<10),
		diagNotify: make(chan struct{}, 1),
	}
	go worker.captureDiagnostics(stderr)

	if _, err := worker.executeRaw(ctx, "START TRANSACTION", dbadapter.MaxRows); err != nil {
		worker.forceClose()
		return nil, fmt.Errorf("start MySQL transaction: %w", err)
	}
	connectionResult, err := worker.executeRaw(ctx, "SELECT CONNECTION_ID() AS taskdeck_connection_id", 1)
	if err != nil {
		worker.forceClose()
		return nil, fmt.Errorf("read MySQL transaction connection id: %w", err)
	}
	if len(connectionResult.Rows) != 1 || len(connectionResult.Rows[0]) != 1 {
		worker.forceClose()
		return nil, errors.New("MySQL transaction connection id query returned no row")
	}
	connectionID, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(connectionResult.Rows[0][0])), 10, 64)
	if err != nil || connectionID < 1 {
		worker.forceClose()
		return nil, errors.New("MySQL transaction connection id is invalid")
	}
	worker.connectionID = connectionID
	return worker, nil
}

func (w *mysqlTransactionWorker) captureDiagnostics(reader io.ReadCloser) {
	defer reader.Close()
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			w.diagMu.Lock()
			remaining := maxClientStderrBytes - w.diag.Len()
			if remaining > 0 {
				if n > remaining { n = remaining }
				_, _ = w.diag.Write(buf[:n])
			}
			w.diagMu.Unlock()
			select {
			case w.diagNotify <- struct{}{}:
			default:
			}
		}
		if err != nil { return }
	}
}

func (w *mysqlTransactionWorker) diagnosticOffset() int {
	w.diagMu.Lock(); defer w.diagMu.Unlock()
	return w.diag.Len()
}

func (w *mysqlTransactionWorker) diagnosticSince(offset int) string {
	w.diagMu.Lock(); defer w.diagMu.Unlock()
	data := w.diag.Bytes()
	if offset < 0 { offset = 0 }
	if offset > len(data) { offset = len(data) }
	return sanitizeClientDiagnostic(data[offset:], w.config.Secret)
}

func (w *mysqlTransactionWorker) waitForDiagnostic(offset int, timeout time.Duration) {
	if w == nil || timeout <= 0 || w.diagnosticOffset() > offset {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-w.diagNotify:
	case <-timer.C:
	}
}

func mysqlTransactionToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil { return "", err }
	return "taskdeck_tx_" + hex.EncodeToString(raw[:]), nil
}

func normalizeTransactionStatement(statement string) (string, error) {
	statement = strings.TrimSpace(statement)
	if statement == "" { return "", errors.New("MySQL transaction statement is required") }
	if strings.ContainsRune(statement, '\x00') { return "", errors.New("MySQL transaction statement contains NUL") }
	return strings.TrimSuffix(statement, ";"), nil
}

func (w *mysqlTransactionWorker) writeStatement(statement, token string) error {
	statement, err := normalizeTransactionStatement(statement)
	if err != nil { return err }
	boundary := "SELECT '" + token + "' AS __taskdeck_boundary;"
	if _, err := io.WriteString(w.stdin, statement+";\n"+boundary+"\n"); err != nil {
		return fmt.Errorf("write MySQL transaction statement: %w", err)
	}
	return nil
}

func trimMySQLBoundaryResult(data []byte) []byte {
	resultset := bytes.LastIndex(data, []byte("<resultset"))
	if resultset < 0 { return bytes.TrimSpace(data) }
	cut := resultset
	if declaration := bytes.LastIndex(data[:resultset], []byte("<?xml")); declaration >= 0 { cut = declaration }
	return bytes.TrimSpace(data[:cut])
}

func (w *mysqlTransactionWorker) readUntilBoundary(token string) ([]byte, error) {
	var output bytes.Buffer
	marker := []byte("<field name=\"__taskdeck_boundary\">" + token + "</field>")
	found := false
	for {
		line, err := w.stdout.ReadBytes('\n')
		if len(line) > 0 {
			if output.Len()+len(line) > maxClientStdoutBytes { return nil, fmt.Errorf("MySQL transaction result exceeds %d bytes", maxClientStdoutBytes) }
			_, _ = output.Write(line)
			if bytes.Contains(line, marker) || bytes.Contains(output.Bytes(), marker) { found = true }
			if found && bytes.Contains(line, []byte("</resultset>")) { return trimMySQLBoundaryResult(output.Bytes()), nil }
		}
		if err != nil {
			diagnostic := w.diagnosticSince(0)
			if diagnostic != "" { return nil, fmt.Errorf("MySQL transaction client exited: %s", diagnostic) }
			return nil, fmt.Errorf("read MySQL transaction output: %w", err)
		}
	}
}

func (w *mysqlTransactionWorker) killQuery() error {
	if w == nil || w.connectionID < 1 { return errors.New("MySQL transaction connection id is unavailable") }
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	statement := "KILL QUERY " + strconv.FormatInt(w.connectionID, 10)
	_, err := runClient(ctx, w.client, w.config, statement)
	return err
}

func (w *mysqlTransactionWorker) executeRaw(ctx context.Context, statement string, maxRows int) (dbadapter.ExecuteResult, error) {
	if w == nil { return dbadapter.ExecuteResult{}, errors.New("MySQL transaction is not active") }
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.broken { return dbadapter.ExecuteResult{}, errors.New("MySQL transaction is not active") }
	token, err := mysqlTransactionToken()
	if err != nil { return dbadapter.ExecuteResult{}, err }
	diagOffset := w.diagnosticOffset()
	if err := w.writeStatement(statement, token); err != nil { w.broken = true; return dbadapter.ExecuteResult{}, err }

	type readResult struct { data []byte; err error }
	resultCh := make(chan readResult, 1)
	go func() { data, err := w.readUntilBoundary(token); resultCh <- readResult{data: data, err: err} }()

	canceled := false
	select {
	case result := <-resultCh:
		if result.err != nil { w.broken = true; return dbadapter.ExecuteResult{}, result.err }
		w.waitForDiagnostic(diagOffset, 3*time.Millisecond)
		diagnostic := w.diagnosticSince(diagOffset)
		if diagnostic != "" { return dbadapter.ExecuteResult{}, errors.New(diagnostic) }
		return parseXMLResult(result.data, maxRows)
	case <-ctx.Done():
		canceled = true
	}

	if canceled {
		if err := w.killQuery(); err != nil {
			w.broken = true
			w.forceCloseLocked()
			return dbadapter.ExecuteResult{}, ctx.Err()
		}
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case result := <-resultCh:
			if result.err != nil { w.broken = true }
			w.waitForDiagnostic(diagOffset, 50*time.Millisecond)
			return dbadapter.ExecuteResult{}, ctx.Err()
		case <-timer.C:
			w.broken = true
			w.forceCloseLocked()
			return dbadapter.ExecuteResult{}, ctx.Err()
		}
	}
	return dbadapter.ExecuteResult{}, ctx.Err()
}

func (w *mysqlTransactionWorker) Execute(ctx context.Context, config Config, statement string, maxRows int) (dbadapter.ExecuteResult, error) {
	if strings.TrimSpace(config.Database) != "" && config.Database != w.config.Database {
		identifier, err := mysqlIdentifier(config.Database)
		if err != nil { return dbadapter.ExecuteResult{}, err }
		if _, err := w.executeRaw(ctx, "USE "+identifier, dbadapter.MaxRows); err != nil { return dbadapter.ExecuteResult{}, err }
		w.config.Database = config.Database
	}
	return w.executeRaw(ctx, statement, maxRows)
}

func (w *mysqlTransactionWorker) finish(ctx context.Context, statement, message string) (dbadapter.TransactionResult, error) {
	if w == nil { return dbadapter.TransactionResult{}, errors.New("MySQL transaction is not active") }
	_, err := w.executeRaw(ctx, statement, dbadapter.MaxRows)
	if err != nil { return dbadapter.TransactionResult{}, err }
	w.mu.Lock()
	w.closed = true
	if w.stdin != nil { _ = w.stdin.Close() }
	cmd := w.cmd
	w.mu.Unlock()
	if cmd != nil {
		if waitErr := cmd.Wait(); waitErr != nil { return dbadapter.TransactionResult{}, fmt.Errorf("MySQL transaction client exit: %w", waitErr) }
	}
	if w.credentials != nil { w.credentials.Close(); w.credentials = nil }
	return dbadapter.TransactionResult{Active: false, Message: message}, nil
}

func (w *mysqlTransactionWorker) Commit(ctx context.Context) (dbadapter.TransactionResult, error) {
	return w.finish(ctx, "COMMIT", "MySQL transaction committed")
}

func (w *mysqlTransactionWorker) Rollback(ctx context.Context) (dbadapter.TransactionResult, error) {
	return w.finish(ctx, "ROLLBACK", "MySQL transaction rolled back")
}

func (w *mysqlTransactionWorker) forceCloseLocked() {
	if w.closed { return }
	w.closed = true
	if w.stdin != nil { _ = w.stdin.Close() }
	if w.cmd != nil && w.cmd.Process != nil { _ = w.cmd.Process.Kill() }
}

func (w *mysqlTransactionWorker) forceClose() {
	if w == nil { return }
	w.mu.Lock()
	w.forceCloseLocked()
	cmd := w.cmd
	w.mu.Unlock()
	if cmd != nil { _ = cmd.Wait() }
	if w.credentials != nil { w.credentials.Close(); w.credentials = nil }
}

func (w *mysqlTransactionWorker) rollbackBestEffort() {
	if w == nil { return }
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := w.Rollback(ctx); err != nil { w.forceClose() }
}
