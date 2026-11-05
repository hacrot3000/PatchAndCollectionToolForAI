package broker

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	runtimestate "bletonfc/vscode_tasks_menu/internal/state"
)

func LogPath(workspace string) string {
	return filepath.Join(runtimestate.Dir(workspace), "session-broker.log")
}

func StartDetached(workspace string) error {
	if err := runtimestate.EnsureDir(workspace); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(LogPath(workspace), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--workspace", workspace, "--session-broker")
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Stdin = nil
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start session broker: %w", err)
	}
	_ = cmd.Process.Release()
	return logFile.Close()
}

func EnsureClient(workspace string, logger *log.Logger) (*Client, error) {
	client, initialErr := NewClient(workspace)
	if initialErr == nil {
		if logger != nil {
			logger.Printf("reusing session broker pid=%d socket=%s", client.info.PID, client.info.SocketPath)
		}
		return client, nil
	}

	// A live broker owns terminal PTYs even when its state JSON was deleted.
	// If its advisory lock is held, do not start competing broker processes
	// that cannot claim the lock and might tempt unsafe cleanup of PTYs.
	if busy, err := brokerLockBusy(workspace); err != nil {
		return nil, fmt.Errorf("inspect existing session broker: %w; see %s", err, LogPath(workspace))
	} else if busy {
		return nil, fmt.Errorf("session broker is still running, but its metadata/socket could not be recovered: %w; broker log: %s; socket: %s (do not kill broker or remove its socket while terminal sessions may be active)", initialErr, LogPath(workspace), SocketPath(workspace))
	}

	if err := StartDetached(workspace); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		client, err := NewClient(workspace)
		if err == nil {
			if logger != nil {
				logger.Printf("started session broker pid=%d socket=%s", client.info.PID, client.info.SocketPath)
			}
			return client, nil
		}
		lastErr = err
		time.Sleep(40 * time.Millisecond)
	}
	return nil, fmt.Errorf("session broker did not become ready: %w; see %s", lastErr, LogPath(workspace))
}
