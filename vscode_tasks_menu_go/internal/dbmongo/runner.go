package dbmongo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
)

const (
	maxShellStdoutBytes = 8 << 20
	maxShellStderrBytes = 64 << 10
	resultMarker        = "__TASKDECK_MONGO_JSON__"
)

var errShellOutputTooLarge = errors.New("mongosh output exceeded limit")

type shellOutput struct {
	stdout []byte
	stderr []byte
}

type scriptFile struct {
	dir  string
	path string
}

func createScriptFile(script string) (*scriptFile, error) {
	dir, err := os.MkdirTemp("", "taskdeck-mongo-*")
	if err != nil {
		return nil, fmt.Errorf("create mongosh script directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := os.Chmod(dir, 0o700); err != nil {
		cleanup()
		return nil, fmt.Errorf("protect mongosh script directory: %w", err)
	}
	path := filepath.Join(dir, "operation.js")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("create mongosh script: %w", err)
	}
	if _, err := file.WriteString(script); err != nil {
		_ = file.Close()
		cleanup()
		return nil, fmt.Errorf("write mongosh script: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		cleanup()
		return nil, fmt.Errorf("sync mongosh script: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return nil, fmt.Errorf("close mongosh script: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		cleanup()
		return nil, fmt.Errorf("protect mongosh script: %w", err)
	}
	return &scriptFile{dir: dir, path: path}, nil
}

func (f *scriptFile) Close() {
	if f == nil || f.dir == "" {
		return
	}
	_ = os.RemoveAll(f.dir)
	f.dir = ""
	f.path = ""
}

func runScript(ctx context.Context, client Client, script, secret string) (shellOutput, error) {
	if ctx == nil {
		return shellOutput{}, errors.New("mongosh context is required")
	}
	if strings.TrimSpace(client.Path) == "" {
		return shellOutput{}, errors.New("mongosh path is required")
	}
	scriptFile, err := createScriptFile(script)
	if err != nil {
		return shellOutput{}, err
	}
	defer scriptFile.Close()

	args := []string{"--nodb", "--norc", "--quiet", "--file", scriptFile.path}
	cmd := exec.CommandContext(ctx, client.Path, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return shellOutput{}, fmt.Errorf("open mongosh stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return shellOutput{}, fmt.Errorf("open mongosh stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return shellOutput{}, fmt.Errorf("start mongosh: %w", err)
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
		stdoutData, stdoutErr = readBounded(stdout, maxShellStdoutBytes)
		if errors.Is(stdoutErr, errShellOutputTooLarge) && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()
	go func() {
		defer wg.Done()
		stderrData, stderrErr = readBounded(stderr, maxShellStderrBytes)
		if errors.Is(stderrErr, errShellOutputTooLarge) && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()
	waitErr := cmd.Wait()
	wg.Wait()

	if stdoutErr != nil {
		if errors.Is(stdoutErr, errShellOutputTooLarge) {
			return shellOutput{}, fmt.Errorf("mongosh result exceeds %d bytes", maxShellStdoutBytes)
		}
		return shellOutput{}, fmt.Errorf("read mongosh output: %w", stdoutErr)
	}
	if stderrErr != nil {
		if errors.Is(stderrErr, errShellOutputTooLarge) {
			return shellOutput{}, fmt.Errorf("mongosh diagnostics exceed %d bytes", maxShellStderrBytes)
		}
		return shellOutput{}, fmt.Errorf("read mongosh diagnostics: %w", stderrErr)
	}
	if ctx.Err() != nil {
		return shellOutput{}, ctx.Err()
	}

	output := shellOutput{stdout: stdoutData, stderr: stderrData}
	if waitErr != nil {
		message := sanitizeShellDiagnostic(stderrData, secret)
		if message == "" {
			message = waitErr.Error()
		}
		return output, fmt.Errorf("mongosh failed: %s", message)
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
		return nil, errShellOutputTooLarge
	}
	return buffer.Bytes(), nil
}

func redactMongoSecret(text, secret string) string {
	if secret == "" {
		return text
	}
	candidates := []string{
		secret,
		url.PathEscape(secret),
		url.QueryEscape(secret),
	}
	encodedUserInfo := url.UserPassword("taskdeck", secret).String()
	if prefix := "taskdeck:"; strings.HasPrefix(encodedUserInfo, prefix) {
		candidates = append(candidates, strings.TrimPrefix(encodedUserInfo, prefix))
	}
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || candidate == "[redacted]" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		text = strings.ReplaceAll(text, candidate, "[redacted]")
	}
	return text
}

func sanitizeShellDiagnostic(data []byte, secret string) string {
	text := strings.TrimSpace(string(data))
	text = redactMongoSecret(text, secret)
	if len(text) > maxShellStderrBytes {
		text = text[:maxShellStderrBytes]
	}
	return text
}
