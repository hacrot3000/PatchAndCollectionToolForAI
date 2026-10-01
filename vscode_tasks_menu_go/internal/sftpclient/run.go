package sftpclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const DefaultMaxOutputBytes = 4 << 20

type Result struct {
	Stdout string
	Stderr string
}

type boundedBuffer struct {
	buf       bytes.Buffer
	remaining int
	truncated bool
}

func newBoundedBuffer(limit int) *boundedBuffer {
	if limit <= 0 {
		limit = DefaultMaxOutputBytes
	}
	return &boundedBuffer{remaining: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	if b.remaining <= 0 {
		b.truncated = b.truncated || original > 0
		return original, nil
	}
	if len(p) > b.remaining {
		p = p[:b.remaining]
		b.truncated = true
	}
	_, _ = b.buf.Write(p)
	b.remaining -= len(p)
	return original, nil
}

func (b *boundedBuffer) String() string {
	value := b.buf.String()
	if b.truncated {
		value += "\n[output truncated]"
	}
	return value
}

func Run(ctx context.Context, command Command, commands string, env []string, cwd string, maxOutputBytes int) (Result, error) {
	if strings.TrimSpace(command.Executable) == "" {
		return Result{}, errors.New("OpenSSH sftp client path is required")
	}
	if strings.TrimSpace(commands) == "" {
		return Result{}, errors.New("sftp command input is empty")
	}
	if !strings.HasSuffix(commands, "\n") {
		commands += "\n"
	}
	stdout := newBoundedBuffer(maxOutputBytes)
	stderr := newBoundedBuffer(maxOutputBytes)
	cmd := exec.CommandContext(ctx, command.Executable, command.Args...)
	cmd.Stdin = strings.NewReader(commands)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if env != nil {
		cmd.Env = append([]string(nil), env...)
	}
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}
	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if ctx.Err() != nil {
		return result, fmt.Errorf("sftp operation canceled or timed out: %w", ctx.Err())
	}
	if stdout.truncated || stderr.truncated {
		return result, fmt.Errorf("sftp output exceeds %d bytes", maxOutputBytes)
	}
	if err != nil {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = strings.TrimSpace(result.Stdout)
		}
		if message == "" {
			message = err.Error()
		}
		return result, fmt.Errorf("sftp command failed: %s", message)
	}
	if message := sftpErrorMessage(result.Stderr); message != "" {
		return result, errors.New(message)
	}
	return result, nil
}

func sftpErrorMessage(stderr string) string {
	for _, raw := range strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "permission denied") ||
			strings.Contains(lower, "no such file") ||
			strings.Contains(lower, "couldn't") ||
			strings.Contains(lower, "not found") ||
			strings.Contains(lower, "failure") ||
			strings.Contains(lower, "invalid command") {
			return "sftp command failed: " + line
		}
	}
	return ""
}

func CopyFile(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	return err
}
