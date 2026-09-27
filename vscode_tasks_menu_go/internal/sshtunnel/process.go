package sshtunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/sshclient"
)

const (
	maxDiagnosticBytes = 64 << 10
	readinessPollDelay = 40 * time.Millisecond
	readinessDialTime  = 80 * time.Millisecond
)

type Tunnel struct {
	meta Metadata
	cmd  *exec.Cmd

	mu      sync.RWMutex
	waitErr error
	diag    boundedDiagnostic

	done     chan struct{}
	doneOnce sync.Once
	closeOnce sync.Once
}

func startTunnelProcess(
	lifetimeCtx context.Context,
	startupCtx context.Context,
	command sshclient.Command,
	env []string,
	meta Metadata,
	reservation *portReservation,
) (*Tunnel, error) {
	if lifetimeCtx == nil || startupCtx == nil {
		return nil, errors.New("SSH tunnel contexts are required")
	}
	if strings.TrimSpace(command.Executable) == "" {
		return nil, errors.New("SSH tunnel executable is required")
	}
	if reservation == nil || reservation.Port() != meta.LocalPort || reservation.Endpoint() == "" {
		return nil, errors.New("SSH tunnel loopback reservation is invalid")
	}

	cmd := exec.CommandContext(lifetimeCtx, command.Executable, command.Args...)
	if env != nil {
		cmd.Env = append([]string(nil), env...)
	} else {
		cmd.Env = os.Environ()
	}

	tunnel := &Tunnel{
		meta: meta,
		cmd:  cmd,
		done: make(chan struct{}),
		diag: boundedDiagnostic{remaining: maxDiagnosticBytes},
	}
	cmd.Stdout = &tunnel.diag
	cmd.Stderr = &tunnel.diag

	// Hold the loopback port until the last possible moment. OpenSSH must bind
	// it itself, so the reservation has to be released immediately before Start.
	if err := reservation.Close(); err != nil {
		return nil, fmt.Errorf("release SSH tunnel loopback reservation: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start SSH tunnel: %w", err)
	}
	go tunnel.wait()

	if err := tunnel.waitReady(startupCtx); err != nil {
		_ = tunnel.Close()
		return nil, err
	}
	return tunnel, nil
}

func (t *Tunnel) Metadata() Metadata {
	if t == nil {
		return Metadata{}
	}
	return t.meta
}

func (t *Tunnel) LocalEndpoint() string {
	if t == nil || t.meta.LocalPort == 0 {
		return ""
	}
	return net.JoinHostPort(t.meta.LocalHost, fmt.Sprint(t.meta.LocalPort))
}

func (t *Tunnel) Done() <-chan struct{} {
	if t == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return t.done
}

func (t *Tunnel) WaitError() error {
	if t == nil {
		return errors.New("SSH tunnel is nil")
	}
	<-t.done
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.waitErr
}

func (t *Tunnel) Diagnostics() string {
	if t == nil {
		return ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.diag.String()
}

func (t *Tunnel) Close() error {
	if t == nil {
		return nil
	}
	var killErr error
	t.closeOnce.Do(func() {
		if t.cmd != nil && t.cmd.Process != nil {
			select {
			case <-t.done:
				return
			default:
			}
			if err := t.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				killErr = fmt.Errorf("kill SSH tunnel: %w", err)
			}
		}
	})
	<-t.done
	return killErr
}

func (t *Tunnel) waitReady(ctx context.Context) error {
	endpoint := t.LocalEndpoint()
	if endpoint == "" {
		return errors.New("SSH tunnel local endpoint is unavailable")
	}
	ticker := time.NewTicker(readinessPollDelay)
	defer ticker.Stop()

	for {
		conn, err := net.DialTimeout("tcp4", endpoint, readinessDialTime)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-t.done:
			message := t.Diagnostics()
			t.mu.RLock()
			waitErr := t.waitErr
			t.mu.RUnlock()
			if message != "" {
				return fmt.Errorf("SSH tunnel exited before ready: %s", message)
			}
			if waitErr != nil {
				return fmt.Errorf("SSH tunnel exited before ready: %w", waitErr)
			}
			return errors.New("SSH tunnel exited before ready")
		case <-ctx.Done():
			return fmt.Errorf("SSH tunnel startup failed: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (t *Tunnel) wait() {
	err := t.cmd.Wait()
	t.doneOnce.Do(func() {
		t.mu.Lock()
		t.waitErr = err
		t.mu.Unlock()
		close(t.done)
	})
}

type boundedDiagnostic struct {
	mu        sync.Mutex
	builder   strings.Builder
	remaining int
	truncated bool
}

func (w *boundedDiagnostic) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	original := len(p)
	if w.remaining <= 0 {
		w.truncated = w.truncated || original > 0
		return original, nil
	}
	if len(p) > w.remaining {
		p = p[:w.remaining]
		w.truncated = true
	}
	_, _ = w.builder.Write(p)
	w.remaining -= len(p)
	return original, nil
}

func (w *boundedDiagnostic) String() string {
	if w == nil {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	text := strings.TrimSpace(w.builder.String())
	if w.truncated {
		if text != "" {
			text += "\n"
		}
		text += "[diagnostic output truncated]"
	}
	return text
}
