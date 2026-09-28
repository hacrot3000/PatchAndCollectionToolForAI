package dbmysql

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const (
	maxClientStdoutBytes = 8 << 20
	maxClientStderrBytes = 64 << 10
)

var errClientOutputTooLarge = errors.New("MySQL client output exceeded limit")

type commandOutput struct {
	stdout []byte
	stderr []byte
}

func runClient(ctx context.Context, client Client, config Config, statement string) (commandOutput, error) {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return commandOutput{}, errors.New("MySQL statement is required")
	}
	return runClientReader(ctx, client, config, strings.NewReader(statement+"\n"))
}

func runClientFile(ctx context.Context, client Client, config Config, path string) (commandOutput, int64, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return commandOutput{}, 0, errors.New("MySQL import path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return commandOutput{}, 0, fmt.Errorf("open MySQL import file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return commandOutput{}, 0, fmt.Errorf("stat MySQL import file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return commandOutput{}, 0, errors.New("MySQL import path is not a regular file")
	}
	output, err := runClientReader(ctx, client, config, file)
	return output, info.Size(), err
}

func runClientReader(ctx context.Context, client Client, config Config, input io.Reader) (commandOutput, error) {
	if ctx == nil {
		return commandOutput{}, errors.New("MySQL command context is required")
	}
	if strings.TrimSpace(client.Path) == "" {
		return commandOutput{}, errors.New("MySQL client path is required")
	}
	if input == nil {
		return commandOutput{}, errors.New("MySQL input is required")
	}

	credentials, err := createCredentialFile(config.Secret)
	if err != nil {
		return commandOutput{}, err
	}
	if credentials != nil {
		defer credentials.Close()
	}

	cmd := exec.CommandContext(ctx, client.Path, clientArgs(config, credentials)...)
	cmd.Stdin = input

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return commandOutput{}, fmt.Errorf("open MySQL stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return commandOutput{}, fmt.Errorf("open MySQL stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return commandOutput{}, fmt.Errorf("start MySQL client: %w", err)
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
		stdoutData, stdoutErr = readBounded(stdout, maxClientStdoutBytes)
		if errors.Is(stdoutErr, errClientOutputTooLarge) && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()
	go func() {
		defer wg.Done()
		stderrData, stderrErr = readBounded(stderr, maxClientStderrBytes)
		if errors.Is(stderrErr, errClientOutputTooLarge) && cmd.Process != nil {
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
		return commandOutput{}, ctx.Err()
	}
	if stdoutErr != nil {
		if errors.Is(stdoutErr, errClientOutputTooLarge) {
			return commandOutput{}, fmt.Errorf("MySQL result exceeds %d bytes", maxClientStdoutBytes)
		}
		return commandOutput{}, fmt.Errorf("read MySQL output: %w", stdoutErr)
	}
	if stderrErr != nil {
		if errors.Is(stderrErr, errClientOutputTooLarge) {
			return commandOutput{}, fmt.Errorf("MySQL diagnostics exceed %d bytes", maxClientStderrBytes)
		}
		return commandOutput{}, fmt.Errorf("read MySQL diagnostics: %w", stderrErr)
	}
	output := commandOutput{stdout: stdoutData, stderr: stderrData}
	if waitErr != nil {
		message := sanitizeClientDiagnostic(stderrData, config.Secret)
		if message == "" {
			message = waitErr.Error()
		}
		return output, fmt.Errorf("MySQL client failed: %s", message)
	}
	return output, nil
}

func readBounded(reader io.Reader, limit int) ([]byte, error) {
	if limit < 1 {
		return nil, errors.New("output limit must be positive")
	}
	var buffer bytes.Buffer
	n, err := io.CopyN(&buffer, reader, int64(limit)+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if n > int64(limit) {
		return nil, errClientOutputTooLarge
	}
	return buffer.Bytes(), nil
}

func sanitizeClientDiagnostic(data []byte, secret string) string {
	text := strings.TrimSpace(string(data))
	if secret != "" {
		text = strings.ReplaceAll(text, secret, "[redacted]")
	}
	if len(text) > maxClientStderrBytes {
		text = text[:maxClientStderrBytes]
	}
	return text
}
