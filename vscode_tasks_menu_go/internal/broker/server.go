package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"

	runtimestate "bletonfc/vscode_tasks_menu/internal/state"
)

func acquireLock(workspace string) (*os.File, error) {
	if err := runtimestate.EnsureDir(workspace); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(LockPath(workspace), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, fmt.Errorf("session broker for workspace is already running")
		}
		return nil, fmt.Errorf("lock session broker: %w", err)
	}
	return f, nil
}

// Run serves the minimal broker control plane. At protocol v1 foundation stage
// it intentionally owns no PTY sessions yet; later stages add session methods
// without changing discovery/state identity.
func Run(ctx context.Context, workspace string, logger *log.Logger) error {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	lock, err := acquireLock(workspace)
	if err != nil {
		return err
	}
	defer lock.Close()

	if err := runtimestate.EnsureDir(workspace); err != nil {
		return err
	}
	socketPath := SocketPath(workspace)
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen session broker: %w", err)
	}
	defer ln.Close()
	defer os.Remove(socketPath)
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return fmt.Errorf("protect session broker socket: %w", err)
	}

	info := NewInfo(workspace)
	if err := SaveInfo(info); err != nil {
		return err
	}
	defer RemoveInfoIfPID(workspace, info.PID)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "protocol_version": ProtocolVersion})
	})
	mux.HandleFunc("/v1/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
	})

	httpServer := &http.Server{Handler: mux}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			_ = httpServer.Shutdown(shutdownCtx)
			cancel()
			_ = ln.Close()
		case <-done:
		}
	}()

	logger.Printf("session broker workspace=%s socket=%s protocol=%d", workspace, socketPath, ProtocolVersion)
	err = httpServer.Serve(ln)
	close(done)
	if err == nil || err == http.ErrServerClosed || ctx.Err() != nil {
		return nil
	}
	return err
}

func Probe(ctx context.Context, workspace string) (Info, error) {
	info, err := LoadInfo(workspace)
	if err != nil {
		return Info{}, err
	}
	dialer := &net.Dialer{Timeout: time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", info.SocketPath)
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 1500 * time.Millisecond}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://session-broker/v1/info", nil)
	if err != nil {
		return Info{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Info{}, fmt.Errorf("probe session broker: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Info{}, fmt.Errorf("probe session broker returned %s", resp.Status)
	}
	var live Info
	if err := json.NewDecoder(resp.Body).Decode(&live); err != nil {
		return Info{}, fmt.Errorf("decode session broker info: %w", err)
	}
	if live.ProtocolVersion != ProtocolVersion {
		return Info{}, fmt.Errorf("session broker protocol mismatch: got %d want %d", live.ProtocolVersion, ProtocolVersion)
	}
	if live.Workspace != workspace {
		return Info{}, fmt.Errorf("session broker workspace mismatch")
	}
	if live.PID != info.PID || live.SocketPath != info.SocketPath {
		return Info{}, fmt.Errorf("session broker state does not match live process")
	}
	return live, nil
}
