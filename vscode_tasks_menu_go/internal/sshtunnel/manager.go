package sshtunnel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshaskpass"
	"bletonfc/vscode_tasks_menu/internal/sshclient"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

const defaultMaxTunnels = 32

var ErrTunnelNotFound = errors.New("ssh tunnel not found")

type Options struct {
	RuntimeDir     string
	AskpassHelper  string
	SSHExecutable  string
	StartupTimeout time.Duration
	MaxTunnels     int
}

type Manager struct {
	secrets secretstore.Store
	options Options

	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.RWMutex
	tunnels map[string]*Tunnel
	opening int
}

func NewManager(secrets secretstore.Store, options Options) (*Manager, error) {
	options.RuntimeDir = strings.TrimSpace(options.RuntimeDir)
	options.AskpassHelper = strings.TrimSpace(options.AskpassHelper)
	options.SSHExecutable = strings.TrimSpace(options.SSHExecutable)
	options.StartupTimeout = normalizeStartupTimeout(options.StartupTimeout)
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
		ctx:     ctx,
		cancel:  cancel,
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
	spec, err := normalizeSpec(Spec{
		SSHProfile: profile,
		RemoteHost: remoteHost,
		RemotePort: remotePort,
	})
	if err != nil {
		return Metadata{}, err
	}
	if spec.SSHProfile.SecretRef != "" && m.secrets == nil {
		return Metadata{}, errors.New("SSH tunnel secret store is unavailable")
	}

	m.mu.Lock()
	if len(m.tunnels)+m.opening >= m.options.MaxTunnels {
		m.mu.Unlock()
		return Metadata{}, fmt.Errorf("SSH tunnel limit %d reached", m.options.MaxTunnels)
	}
	select {
	case <-m.ctx.Done():
		m.mu.Unlock()
		return Metadata{}, errors.New("SSH tunnel manager is closed")
	default:
	}
	m.opening++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.opening--
		m.mu.Unlock()
	}()

	reservation, err := reserveLoopbackPort()
	if err != nil {
		return Metadata{}, err
	}
	defer reservation.Close()

	executable := m.options.SSHExecutable
	if executable == "" {
		executable, err = sshclient.FindOpenSSH()
		if err != nil {
			return Metadata{}, err
		}
	}
	command, err := sshclient.BuildTunnelCommand(
		executable,
		spec.SSHProfile,
		reservation.Port(),
		spec.RemoteHost,
		spec.RemotePort,
	)
	if err != nil {
		return Metadata{}, err
	}

	env := sanitizedTunnelEnvironment(os.Environ())
	var ticket *sshaskpass.Ticket
	if spec.SSHProfile.SecretRef != "" {
		ticket, env, err = m.prepareAskpass(spec.SSHProfile.SecretRef, env)
		if err != nil {
			return Metadata{}, err
		}
		defer ticket.Close()
	}

	tunnelID, err := sshprofile.NewID()
	if err != nil {
		return Metadata{}, err
	}
	meta := Metadata{
		ID:           tunnelID,
		SSHProfileID: spec.SSHProfile.ID,
		LocalHost:    "127.0.0.1",
		LocalPort:    reservation.Port(),
		RemoteHost:   spec.RemoteHost,
		RemotePort:   spec.RemotePort,
		StartedAt:    time.Now().Format(time.RFC3339),
	}

	startupTimeout := m.options.StartupTimeout
	profileTimeout := time.Duration(spec.SSHProfile.ConnectTimeoutSeconds+3) * time.Second
	if profileTimeout > 0 && profileTimeout < startupTimeout {
		startupTimeout = profileTimeout
	}
	startupCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	tunnel, err := startTunnelProcess(m.ctx, startupCtx, command, env, meta, reservation)
	if err != nil {
		return Metadata{}, err
	}
	m.mu.Lock()
	select {
	case <-m.ctx.Done():
		m.mu.Unlock()
		_ = tunnel.Close()
		return Metadata{}, errors.New("SSH tunnel manager closed during startup")
	default:
	}
	m.tunnels[tunnelID] = tunnel
	m.mu.Unlock()
	go m.watch(tunnelID, tunnel)
	return tunnel.Metadata(), nil
}

func (m *Manager) Get(id string) (Metadata, error) {
	tunnel, err := m.get(id)
	if err != nil {
		return Metadata{}, err
	}
	return tunnel.Metadata(), nil
}

func (m *Manager) List() []Metadata {
	if m == nil {
		return []Metadata{}
	}
	m.mu.RLock()
	out := make([]Metadata, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		out = append(out, tunnel.Metadata())
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt != out[j].StartedAt {
			return out[i].StartedAt < out[j].StartedAt
		}
		return out[i].ID < out[j].ID
	})
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
	closeErr := tunnel.Close()
	m.mu.Lock()
	delete(m.tunnels, tunnel.Metadata().ID)
	m.mu.Unlock()
	return closeErr
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.cancel()
	m.mu.Lock()
	tunnels := make([]*Tunnel, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		tunnels = append(tunnels, tunnel)
	}
	m.tunnels = make(map[string]*Tunnel)
	m.mu.Unlock()
	for _, tunnel := range tunnels {
		_ = tunnel.Close()
	}
}

func (m *Manager) watch(id string, tunnel *Tunnel) {
	<-tunnel.Done()
	m.mu.Lock()
	if current, ok := m.tunnels[id]; ok && current == tunnel {
		delete(m.tunnels, id)
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

func (m *Manager) prepareAskpass(secretRef string, env []string) (*sshaskpass.Ticket, []string, error) {
	if m.secrets == nil {
		return nil, nil, errors.New("SSH tunnel secret store is unavailable")
	}
	helper := m.options.AskpassHelper
	if helper == "" {
		var err error
		helper, err = os.Executable()
		if err != nil {
			return nil, nil, fmt.Errorf("resolve TaskDeck askpass helper: %w", err)
		}
	}
	ticket, err := sshaskpass.Prepare(m.secrets, secretRef, m.options.RuntimeDir)
	if err != nil {
		return nil, nil, err
	}
	env = setEnvironment(env, map[string]string{
		"DISPLAY":                     "taskdeck-ssh-askpass",
		"SSH_ASKPASS":                 helper,
		"SSH_ASKPASS_REQUIRE":         "force",
		"TASKDECK_SSH_ASKPASS":        "1",
		"TASKDECK_SSH_ASKPASS_SOCKET": ticket.SocketPath,
		"TASKDECK_SSH_ASKPASS_TOKEN":  ticket.Token,
	})
	return ticket, env, nil
}

func sanitizedTunnelEnvironment(base []string) []string {
	blocked := map[string]bool{
		"SSH_ASKPASS":                 true,
		"SSH_ASKPASS_REQUIRE":         true,
		"TASKDECK_SSH_ASKPASS":        true,
		"TASKDECK_SSH_ASKPASS_SOCKET": true,
		"TASKDECK_SSH_ASKPASS_TOKEN":  true,
	}
	out := make([]string, 0, len(base))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok && blocked[key] {
			continue
		}
		out = append(out, item)
	}
	return out
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
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(values))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}
