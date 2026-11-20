package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// brokerLockBusy checks the broker's existing advisory lock without starting
// another process or changing the running broker's ownership of PTY sessions.
func brokerLockBusy(workspace string) (bool, error) {
	file, err := os.OpenFile(LockPath(workspace), os.O_RDWR, 0)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open session broker lock: %w", err)
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return true, nil
		}
		return false, fmt.Errorf("inspect session broker lock: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return false, nil
}

// recoverMissingBrokerInfo verifies the still-running broker directly through
// its private Unix socket. macOS can remove an old runtime metadata JSON while
// the broker process and PTYs remain alive. Never kill/restart that broker:
// query its authoritative /v1/info and safely restore only the lost JSON.
func recoverMissingBrokerInfo(ctx context.Context, workspace string) (Info, error) {
	busy, err := brokerLockBusy(workspace)
	if err != nil {
		return Info{}, err
	}
	if !busy {
		return Info{}, fmt.Errorf("session broker metadata is missing and no broker holds its lock")
	}
	socket := SocketPath(workspace)
	// Do not follow symlinks or trust sockets in publicly writable directories.
	dirInfo, err := os.Lstat(filepath.Dir(socket))
	if err != nil {
		return Info{}, fmt.Errorf("session broker holds its lock but socket directory is unavailable: %w", err)
	}
	if !dirInfo.IsDir() || dirInfo.Mode().Perm()&0o077 != 0 {
		return Info{}, fmt.Errorf("session broker socket directory is not private")
	}
	socketInfo, err := os.Lstat(socket)
	if err != nil {
		return Info{}, fmt.Errorf("session broker holds its lock but Unix socket is unavailable: %w", err)
	}
	if socketInfo.Mode()&os.ModeSocket == 0 || socketInfo.Mode().Perm()&0o077 != 0 {
		return Info{}, fmt.Errorf("session broker socket is not a private Unix socket")
	}
	transport := unixTransport(socket)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 1500 * time.Millisecond}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://session-broker/v1/info", nil)
	if err != nil {
		return Info{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Info{}, fmt.Errorf("session broker lock held but live Unix socket probe failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Info{}, fmt.Errorf("session broker returned %s during metadata recovery", resp.Status)
	}
	var live Info
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 64<<10)).Decode(&live); err != nil {
		return Info{}, fmt.Errorf("decode live session broker identity: %w", err)
	}
	if live.ProtocolVersion != ProtocolVersion || live.Workspace != workspace ||
		live.SocketPath != socket || live.PID <= 0 {
		return Info{}, fmt.Errorf("live session broker identity mismatch; refusing metadata recovery")
	}
	// If an in-progress broker startup supplied its own metadata meanwhile,
	// only accept the identity already written by that process.
	if existing, err := LoadInfo(workspace); err == nil {
		if existing.PID != live.PID || existing.SocketPath != live.SocketPath ||
			existing.Workspace != live.Workspace || existing.ProtocolVersion != live.ProtocolVersion {
			return Info{}, fmt.Errorf("session broker identity changed during metadata recovery")
		}
		return existing, nil
	} else if !os.IsNotExist(err) {
		return Info{}, fmt.Errorf("read session broker metadata during recovery: %w", err)
	}
	if err := SaveInfo(live); err != nil {
		return Info{}, fmt.Errorf("restore verified session broker metadata: %w", err)
	}
	return live, nil
}
