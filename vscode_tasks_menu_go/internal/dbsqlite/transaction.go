package dbsqlite

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

//go:embed sqlite_transaction_worker.py
var sqliteTransactionWorkerScript string

type sqliteTransactionResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type sqliteTransactionWorker struct {
	mu sync.Mutex

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	diagMu sync.Mutex
	diag   bytes.Buffer
	closed bool
	broken atomic.Bool
}

func startSQLiteTransaction(ctx context.Context, python Python, config Config) (*sqliteTransactionWorker, error) {
	if ctx == nil {
		return nil, errors.New("SQLite transaction context is required")
	}
	if strings.TrimSpace(python.Path) == "" {
		return nil, errors.New("Python 3 path is required")
	}
	cmd := exec.Command(python.Path, "-I", "-c", sqliteTransactionWorkerScript)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open SQLite transaction stdin: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("open SQLite transaction stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdoutPipe.Close()
		return nil, fmt.Errorf("open SQLite transaction stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdoutPipe.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("start SQLite transaction worker: %w", err)
	}
	worker := &sqliteTransactionWorker{
		cmd: cmd, stdin: stdin, stdout: bufio.NewReaderSize(stdoutPipe, 64<<10),
	}
	go worker.captureDiagnostics(stderr)

	open := map[string]interface{}{
		"file":            config.File,
		"read_only":       config.ReadOnly,
		"busy_timeout_ms": config.BusyTimeoutMS,
	}
	if err := worker.writeJSON(open); err != nil {
		worker.forceClose()
		return nil, err
	}
	response, err := worker.readResponse(ctx)
	if err != nil {
		worker.forceClose()
		return nil, fmt.Errorf("start SQLite transaction: %w", err)
	}
	var result dbadapter.TransactionResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		worker.forceClose()
		return nil, fmt.Errorf("decode SQLite transaction start result: %w", err)
	}
	if !result.Active {
		worker.forceClose()
		return nil, errors.New("SQLite transaction worker did not enter an active transaction")
	}
	return worker, nil
}

func (w *sqliteTransactionWorker) captureDiagnostics(reader io.ReadCloser) {
	defer reader.Close()
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			w.diagMu.Lock()
			remaining := maxHelperStderrBytes - w.diag.Len()
			if remaining > 0 {
				if n > remaining {
					n = remaining
				}
				_, _ = w.diag.Write(buffer[:n])
			}
			w.diagMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (w *sqliteTransactionWorker) diagnostic() string {
	if w == nil {
		return ""
	}
	w.diagMu.Lock()
	defer w.diagMu.Unlock()
	return strings.TrimSpace(w.diag.String())
}

func (w *sqliteTransactionWorker) writeJSON(value interface{}) error {
	if w == nil || w.stdin == nil {
		return errors.New("SQLite transaction worker is unavailable")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode SQLite transaction request: %w", err)
	}
	data = append(data, '\n')
	if _, err := w.stdin.Write(data); err != nil {
		return fmt.Errorf("write SQLite transaction request: %w", err)
	}
	return nil
}

func (w *sqliteTransactionWorker) readResponse(ctx context.Context) (sqliteTransactionResponse, error) {
	if w == nil || w.stdout == nil {
		return sqliteTransactionResponse{}, errors.New("SQLite transaction worker is unavailable")
	}
	type readResult struct {
		line []byte
		err  error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		line, err := w.stdout.ReadBytes('\n')
		resultCh <- readResult{line: line, err: err}
	}()

	select {
	case result := <-resultCh:
		return w.parseResponse(result.line, result.err)
	case <-ctx.Done():
		if w.cmd != nil && w.cmd.Process != nil {
			_ = w.cmd.Process.Signal(os.Interrupt)
		}
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case result := <-resultCh:
			_, _ = w.parseResponse(result.line, result.err)
			return sqliteTransactionResponse{}, ctx.Err()
		case <-timer.C:
			w.broken.Store(true)
			if w.cmd != nil && w.cmd.Process != nil {
				_ = w.cmd.Process.Kill()
			}
			return sqliteTransactionResponse{}, ctx.Err()
		}
	}
}

func (w *sqliteTransactionWorker) parseResponse(line []byte, readErr error) (sqliteTransactionResponse, error) {
	if readErr != nil && len(line) == 0 {
		diag := w.diagnostic()
		if diag != "" {
			return sqliteTransactionResponse{}, fmt.Errorf("SQLite transaction worker exited: %s", diag)
		}
		return sqliteTransactionResponse{}, fmt.Errorf("read SQLite transaction response: %w", readErr)
	}
	if len(line) > maxHelperStdoutBytes {
		return sqliteTransactionResponse{}, fmt.Errorf("SQLite transaction response exceeds %d bytes", maxHelperStdoutBytes)
	}
	var response sqliteTransactionResponse
	if err := json.Unmarshal(bytes.TrimSpace(line), &response); err != nil {
		return sqliteTransactionResponse{}, fmt.Errorf("decode SQLite transaction response: %w", err)
	}
	if !response.OK {
		message := strings.TrimSpace(response.Error)
		if message == "" {
			message = "SQLite transaction operation failed"
		}
		return sqliteTransactionResponse{}, errors.New(message)
	}
	return response, nil
}

func (w *sqliteTransactionWorker) request(ctx context.Context, operation string, payload interface{}, target interface{}) error {
	if w == nil {
		return errors.New("SQLite transaction is not active")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("SQLite transaction is not active")
	}
	request := map[string]interface{}{"operation": operation}
	if payload != nil {
		request["payload"] = payload
	}
	if err := w.writeJSON(request); err != nil {
		return err
	}
	response, err := w.readResponse(ctx)
	if err != nil {
		return err
	}
	if target == nil {
		return nil
	}
	if len(response.Result) == 0 {
		return errors.New("SQLite transaction result payload is missing")
	}
	if err := json.Unmarshal(response.Result, target); err != nil {
		return fmt.Errorf("decode SQLite transaction result: %w", err)
	}
	return nil
}

func (w *sqliteTransactionWorker) Execute(ctx context.Context, payload dbadapter.ExecutePayload) (dbadapter.ExecuteResult, error) {
	var result dbadapter.ExecuteResult
	err := w.request(ctx, "execute", payload, &result)
	if err == nil {
		err = dbadapter.ValidateExecuteResult(result)
	}
	return result, err
}

func (w *sqliteTransactionWorker) finish(ctx context.Context, operation string) (dbadapter.TransactionResult, error) {
	var result dbadapter.TransactionResult
	err := w.request(ctx, operation, nil, &result)
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		_ = w.stdin.Close()
	}
	w.mu.Unlock()
	if w.cmd != nil {
		waitErr := w.cmd.Wait()
		if err == nil && waitErr != nil {
			err = fmt.Errorf("SQLite transaction worker exit: %w", waitErr)
		}
	}
	return result, err
}

func (w *sqliteTransactionWorker) Commit(ctx context.Context) (dbadapter.TransactionResult, error) {
	return w.finish(ctx, "commit")
}

func (w *sqliteTransactionWorker) Rollback(ctx context.Context) (dbadapter.TransactionResult, error) {
	return w.finish(ctx, "rollback")
}

func (w *sqliteTransactionWorker) forceClose() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	if w.stdin != nil {
		_ = w.stdin.Close()
	}
	cmd := w.cmd
	w.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

func (w *sqliteTransactionWorker) rollbackBestEffort() {
	if w == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := w.Rollback(ctx)
	if err != nil {
		w.forceClose()
	}
}
