package dbsqlite

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

const (
	maxHelperStdoutBytes = 8 << 20
	maxHelperStderrBytes = 64 << 10
)

var errHelperOutputTooLarge = errors.New("SQLite helper output exceeded limit")

//go:embed sqlite_helper.py
var sqliteHelperScript string

type helperRequest struct {
	Operation     string      `json:"operation"`
	File          string      `json:"file"`
	ReadOnly      bool        `json:"read_only"`
	BusyTimeoutMS int         `json:"busy_timeout_ms"`
	Payload       interface{} `json:"payload,omitempty"`
}

type helperResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func runHelper(
	ctx context.Context,
	python Python,
	config Config,
	operation string,
	payload interface{},
	target interface{},
) error {
	if ctx == nil {
		return errors.New("SQLite helper context is required")
	}
	if strings.TrimSpace(python.Path) == "" {
		return errors.New("Python 3 path is required")
	}
	request := helperRequest{
		Operation:     strings.TrimSpace(operation),
		File:          config.File,
		ReadOnly:      config.ReadOnly,
		BusyTimeoutMS: config.BusyTimeoutMS,
		Payload:       payload,
	}
	requestData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode SQLite helper request: %w", err)
	}

	cmd := exec.CommandContext(ctx, python.Path, "-I", "-c", sqliteHelperScript)
	cmd.Stdin = bytes.NewReader(requestData)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open SQLite helper stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return fmt.Errorf("open SQLite helper stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return fmt.Errorf("start SQLite helper: %w", err)
	}

	var (
		stdoutData []byte
		stderrData []byte
		stdoutErr  error
		stderrErr  error
		wg         sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		stdoutData, stdoutErr = readHelperBounded(stdout, maxHelperStdoutBytes)
		if errors.Is(stdoutErr, errHelperOutputTooLarge) && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()
	go func() {
		defer wg.Done()
		stderrData, stderrErr = readHelperBounded(stderr, maxHelperStderrBytes)
		if errors.Is(stderrErr, errHelperOutputTooLarge) && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()
	readersDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(readersDone)
	}()

	var waitErr error
	select {
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		waitErr = cmd.Wait()
		<-readersDone
	case <-readersDone:
		waitErr = cmd.Wait()
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	if stdoutErr != nil {
		if errors.Is(stdoutErr, errHelperOutputTooLarge) {
			return fmt.Errorf("SQLite helper result exceeds %d bytes", maxHelperStdoutBytes)
		}
		return fmt.Errorf("read SQLite helper output: %w", stdoutErr)
	}
	if stderrErr != nil {
		if errors.Is(stderrErr, errHelperOutputTooLarge) {
			return fmt.Errorf("SQLite helper diagnostics exceed %d bytes", maxHelperStderrBytes)
		}
		return fmt.Errorf("read SQLite helper diagnostics: %w", stderrErr)
	}
	if waitErr != nil {
		message := strings.TrimSpace(string(stderrData))
		if message == "" {
			message = waitErr.Error()
		}
		if len(message) > 4096 {
			message = message[:4096]
		}
		return fmt.Errorf("SQLite helper failed: %s", message)
	}
	return parseHelperResponse(stdoutData, target)
}

func parseHelperResponse(data []byte, target interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var response helperResponse
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("decode SQLite helper response: %w", err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("SQLite helper emitted multiple JSON values")
		}
		return fmt.Errorf("SQLite helper emitted trailing output: %w", err)
	}
	if !response.OK {
		message := strings.TrimSpace(response.Error)
		if message == "" {
			message = "SQLite operation failed"
		}
		return errors.New(message)
	}
	if target == nil {
		return nil
	}
	if len(response.Result) == 0 {
		return errors.New("SQLite helper result payload is missing")
	}
	decoder = json.NewDecoder(bytes.NewReader(response.Result))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode SQLite helper result: %w", err)
	}
	return nil
}

func readHelperBounded(reader io.Reader, limit int) ([]byte, error) {
	if limit < 1 {
		return nil, errors.New("output limit must be positive")
	}
	var buffer bytes.Buffer
	n, err := io.CopyN(&buffer, reader, int64(limit)+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if n > int64(limit) {
		return nil, errHelperOutputTooLarge
	}
	return buffer.Bytes(), nil
}
