package sshtunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshaskpass"
	"bletonfc/vscode_tasks_menu/internal/sshclient"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

const (
	defaultMaxTunnels = 32
	maxTunnelLogBytes = 64 << 10
)

var ErrTunnelNotFound = errors.New("ssh tunnel not found")

type Metadata struct {
	ID           string `json:"id"`
	SSHProfileID string `json:"ssh_profile_id"`
	LocalHost    string `json:"local_host"`
	LocalPort    int    `json:"local_port"`
	RemoteHost   string `json:"remote_host"`
	RemotePort   int    `json:"remote_port"`
	StartedAt    string `json:"started_at"`
}

type Tunnel struct {
	meta Metadata

	cmd    *exec.Cmd
	ticket *sshaskpass.Ticket
	log    *boundedLog

	done     chan struct{}
	doneOnce sync.Once

	mu      sync.Mutex
	waitErr error
}

type Options struct {
	RuntimeDir    string
	AskpassHelper string
	SSHExecutable string
	MaxTunnels    int
}

type Manager struct {
	secrets secretstore.Store
	options Options

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.RWMutex
	tunnels  map[string]*Tunnel
	opening  int
}

func NewManager(secrets secretstore.Store, options Options) (*Manager, error) {
	options.RuntimeDir = strings.TrimSpace(options.RuntimeDir)
	options.AskpassHelper = strings.TrimSpace(options.AskpassHelper)
	options.SSHExecutable = strings.TrimSpace(options.SSHExecutable)
	if options.RuntimeDir == "" || !filepath.IsAbs(options.RuntimeDir) {
		return nil, errors.New("SSH tunnel runtime directory must be absolute")
	}
	if options.MaxTunnels <= 0 {
		options.MaxTunnels = defaultMaxTunnels
	}
	if options.MaxTunnels > 256 {
		return nil, errors.New("SSH tunnel limit must not exceed 256")
	}
	if err := os.MkdirAll(options.RuntimeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create SSH tunnel runtime directory: %w", err)
	}
	if err := os.Chmod(options.RuntimeDir, 0o700); err != nil {
		return nil, fmt.Errorf("protect SSH tunnel runtime directory: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		secrets: secrets,
		options: options,
		ctx: ctx,
		cancel: cancel,
		tunnels: make(map[string]*Tunnel),
	}, nil
}

func (m *Manager) Open(
	ctx context.Context,
	profile sshprofile.Profile,
	remoteHost string,
	remotePort int,
) (Metadata, error) {
	if m == nil {
		return Metadata{}, errors.New("SSH tunnel manager is nil")
	}
	if ctx == nil {
		return Metadata{}, errors.New("SSH tunnel context is required")
	}
	profile, err := sshprofile.Normalize(profile)
	if err != nil {
		return Metadata{}, err
	}
	if profile.SecretRef != "" && m.secrets == nil {
		return Metadata{}, errors.New("SSH tunnel secret store is unavailable")
	}

	m.mu.Lock()
	if len(m.tunnels)+m.opening >= m.options.MaxTunnels {
		m.mu.Unlock()
		return Metadata{}, fmt.Errorf("SSH tunnel limit %d reached", m.options.MaxTunnels)
	}
	m.opening++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.opening--
		m.mu.Unlock()
	}()

	localPort, err := allocateLoopbackPort()
	if err != nil {
		return Metadata{}, err
	}
	executable := m.options.SSHExecutable
	if executable == "" {
		executable, err = sshclient.FindOpenSSH()
		if err != nil {
			return Metadata{}, err
		}
	}
	command, err := sshclient.BuildTunnelCommand(executable, profile, localPort, remoteHost, remotePort)
	if err != nil {
		return Metadata{}, err
	}

	var ticket *sshaskpass.Ticket
	env := os.Environ()
	if profile.SecretRef != "" {
		if m.options.AskpassHelper == "" {
			return Metadata{}, errors.New("SSH tunnel askpass helper is unavailable")
		}
		ticket, err = sshaskpass.Prepare(m.secrets, profile.SecretRef, m.options.RuntimeDir)
		if err != nil {
			return Metadata{}, err
		}
		env = setEnvironment(env, map[string]string{
			"DISPLAY":                     "taskdeck-ssh-askpass",
			"SSH_ASKPASS":                 m.options.AskpassHelper,
			"SSH_ASKPASS_REQUIRE":         "force",
			"TASKDECK_SSH_ASKPASS":        "1",
			"TASKDECK_SSH_ASKPASS_SOCKET": ticket.SocketPath,
			"TASKDECK_SSH_ASKPASS_TOKEN":  ticket.Token,
		})
	}

	tunnelID, err := sshprofile.NewID()
	if err != nil {
		if ticket != nil {
			ticket.Close()
		}
		return Metadata{}, err
	}
	logBuffer := &boundedLog{remaining: maxTunnelLogBytes}
	cmd := exec.CommandContext(m.ctx, command.Executable, command.Args...)
	cmd.Env = env
	cmd.Stdout = logBuffer
	cmd.Stderr = logBuffer
	tunnel := &Tunnel{
		meta: Metadata{
			ID: tunnelID,
			SSHProfileID: profile.ID,
			LocalHost: "127.0.0.1",
			LocalPort: localPort,
			RemoteHost: strings.TrimSpace(remoteHost),
			RemotePort: remotePort,
			StartedAt: time.Now().Format(time.RFC3339),
		},
		cmd: cmd,
		ticket: ticket,
		log: logBuffer,
		done: make(chan struct{}),
	}
	if err := cmd.Start(); err != nil {
		if ticket != nil {
			ticket.Close()
		}
		return Metadata{}, fmt.Errorf("start SSH tunnel: %w", err)
	}

	m.mu.Lock()
	m.tunnels[tunnelID] = tunnel
	m.mu.Unlock()
	go m.wait(tunnel)

	startupTimeout := time.Duration(profile.ConnectTimeoutSeconds+3) * time.Second
	timer := time.NewTimer(startupTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		if listenerReady(tunnel.meta.LocalHost, tunnel.meta.LocalPort) {
			if ticket != nil {
				ticket.Close()
				tunnel.ticket = nil
			}
			return tunnel.meta, nil
		}
		select {
		case <-ctx.Done():
			_ = m.CloseTunnel(tunnelID)
			return Metadata{}, ctx.Err()
		case <-timer.C:
			diagnostic := tunnel.Diagnostics()
			_ = m.CloseTunnel(tunnelID)
			if diagnostic != "" {
				return Metadata{}, fmt.Errorf("SSH tunnel startup timed out: %s", diagnostic)
			}
			return Metadata{}, errors.New("SSH tunnel startup timed out")
		case <-tunnel.done:
			diagnostic := tunnel.Diagnostics()
			err := tunnel.WaitError()
			if diagnostic != "" {
				return Metadata{}, fmt.Errorf("SSH tunnel exited before ready: %s", diagnostic)
			}
			if err != nil {
				return Metadata{}, err
			}
			return Metadata{}, errors.New("SSH tunnel exited before ready")
		case <-ticker.C:
		}
	}
}

func (m *Manager) Get(id string) (Metadata, error) {
	tunnel, err := m.get(id)
	if err != nil {
		return Metadata{}, err
	}
	return tunnel.meta, nil
}

func (m *Manager) List() []Metadata {
	if m == nil {
		return []Metadata{}
	}
	m.mu.RLock()
	out := make([]Metadata, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		out = append(out, tunnel.meta)
	}
	m.mu.RUnlock()
	return out
}

func (m *Manager) Diagnostics(id string) (string, error) {
	tunnel, err := m.get(id)
	if err != nil {
		return "", err
	}
	return tunnel.Diagnostics(), nil
}

func (m *Manager) CloseTunnel(id string) error {
	tunnel, err := m.get(id)
	if err != nil {
		return err
	}
	if tunnel.ticket != nil {
		tunnel.ticket.Close()
		tunnel.ticket = nil
	}
	if tunnel.cmd != nil && tunnel.cmd.Process != nil {
		if err := tunnel.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill SSH tunnel: %w", err)
		}
	}
	select {
	case <-tunnel.done:
	case <-time.After(2 * time.Second):
		return errors.New("SSH tunnel did not stop after kill")
	}
	m.mu.Lock()
	delete(m.tunnels, id)
	m.mu.Unlock()
	return nil
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.cancel()
	m.mu.RLock()
	ids := make([]string, 0, len(m.tunnels))
	for id := range m.tunnels {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		_ = m.CloseTunnel(id)
	}
}

func (m *Manager) wait(tunnel *Tunnel) {
	err := tunnel.cmd.Wait()
	tunnel.mu.Lock()
	if err != nil {
		tunnel.waitErr = fmt.Errorf("SSH tunnel process exited: %w", err)
	}
	if tunnel.ticket != nil {
		tunnel.ticket.Close()
		tunnel.ticket = nil
	}
	tunnel.mu.Unlock()
	tunnel.doneOnce.Do(func() { close(tunnel.done) })

	m.mu.Lock()
	if current, ok := m.tunnels[tunnel.meta.ID]; ok && current == tunnel {
		delete(m.tunnels, tunnel.meta.ID)
	}
	m.mu.Unlock()
}

func (m *Manager) get(id string) (*Tunnel, error) {
	if m == nil {
		return nil, errors.New("SSH tunnel manager is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrTunnelNotFound
	}
	m.mu.RLock()
	tunnel, ok := m.tunnels[id]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrTunnelNotFound
	}
	return tunnel, nil
}

func (t *Tunnel) Diagnostics() string {
	if t == nil || t.log == nil {
		return ""
	}
	return t.log.String()
}

func (t *Tunnel) WaitError() error {
	if t == nil {
		return errors.New("SSH tunnel is nil")
	}
	<-t.done
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.waitErr
}

func allocateLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate SSH tunnel loopback port: %w", err)
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || addr.Port < 1 {
		return 0, errors.New("allocated SSH tunnel port is invalid")
	}
	return addr.Port, nil
}

func listenerReady(host string, port int) bool {
	conn, err := net.DialTimeout("tcp4", net.JoinHostPort(host, strconv.Itoa(port)), 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func setEnvironment(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, item := range base {
		if key, value, ok := strings.Cut(item, "="); ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

type boundedLog struct {
	mu        sync.Mutex
	builder   strings.Builder
	remaining int
	truncated bool
}

func (w *boundedLog) Write(data []byte) (int, error) {
	if w == nil {
		return len(data), nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	original := len(data)
	if w.remaining <= 0 {
		w.truncated = w.truncated || original > 0
		return original, nil
	}
	if len(data) > w.remaining {
		data = data[:w.remaining]
		w.truncated = true
	}
	_, _ = w.builder.Write(data)
	w.remaining -= len(data)
	return original, nil
}

func (w *boundedLog) String() string {
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
		text += "[SSH tunnel diagnostics truncated]"
	}
	return text
}
