package state

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type State struct {
	PID       int    `json:"pid"`
	Workspace string `json:"workspace"`
	URL       string `json:"url"`
	HealthURL string `json:"health_url,omitempty"`
	Address   string `json:"address"`
	StartedAt string `json:"started_at"`
}

func Dir(workspace string) string {
	sum := sha256.Sum256([]byte(workspace))
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "vscode_tasks_menu", hex.EncodeToString(sum[:8]))
}

func Path(workspace string) string    { return filepath.Join(Dir(workspace), "server.json") }
func LogPath(workspace string) string { return filepath.Join(Dir(workspace), "server.log") }

func Save(s State) error {
	dir := Dir(s.Workspace)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "server.json.tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, Path(s.Workspace))
}

func Load(workspace string) (State, error) {
	data, err := os.ReadFile(Path(workspace))
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, err
	}
	return s, nil
}

func Remove(workspace string) { _ = os.Remove(Path(workspace)) }

func Healthy(s State) bool {
	healthURL := s.HealthURL
	if healthURL == "" {
		healthURL = s.URL
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.HasPrefix(strings.ToLower(healthURL), "https://") {
		// Chỉ dùng cho health-check loopback của daemon. Public clients vẫn
		// xác minh TLS bình thường. Hỗ trợ cả certificate tự ký cục bộ.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402
	}
	client := &http.Client{Timeout: 500 * time.Millisecond, Transport: transport}
	resp, err := client.Get(healthURL + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func New(workspace, url, healthURL, address string) State {
	return State{PID: os.Getpid(), Workspace: workspace, URL: url, HealthURL: healthURL, Address: address, StartedAt: time.Now().Format(time.RFC3339)}
}

func EnsureDir(workspace string) error {
	if err := os.MkdirAll(Dir(workspace), 0o700); err != nil {
		return fmt.Errorf("tạo runtime dir: %w", err)
	}
	return nil
}

func StartLockPath(workspace string) string  { return filepath.Join(Dir(workspace), "start.lock") }
func DaemonLockPath(workspace string) string { return filepath.Join(Dir(workspace), "daemon.lock") }

func AcquireStartLock(workspace string) (*os.File, error) {
	if err := EnsureDir(workspace); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(StartLockPath(workspace), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock launcher: %w", err)
	}
	return f, nil
}

func AcquireDaemonLock(workspace string) (*os.File, error) {
	if err := EnsureDir(workspace); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(DaemonLockPath(workspace), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, fmt.Errorf("daemon cho workspace này đã chạy")
		}
		return nil, fmt.Errorf("lock daemon: %w", err)
	}
	return f, nil
}
