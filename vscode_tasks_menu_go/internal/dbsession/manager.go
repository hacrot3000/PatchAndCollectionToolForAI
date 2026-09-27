package dbsession

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

var ErrSessionNotFound = errors.New("database session not found")

const (
	defaultMaxSessions   = 32
	adapterHandshakeTime = 10 * time.Second
)

type Metadata struct {
	ID          string `json:"id"`
	ProfileID   string `json:"profile_id"`
	AdapterID   string `json:"adapter_id"`
	AdapterKind string `json:"adapter_kind"`
	StartedAt   string `json:"started_at"`
}

type session struct {
	meta    Metadata
	process *dbadapter.Process
}

type Manager struct {
	registry *dbadapter.Registry
	max      int

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.RWMutex
	sessions map[string]*session
	opening  int
}

func NewManager(registry *dbadapter.Registry, maxSessions int) (*Manager, error) {
	if registry == nil {
		return nil, errors.New("database adapter registry is required")
	}
	if maxSessions <= 0 {
		maxSessions = defaultMaxSessions
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		registry: registry,
		max:      maxSessions,
		ctx:      ctx,
		cancel:   cancel,
		sessions: make(map[string]*session),
	}, nil
}

func (m *Manager) Open(
	ctx context.Context,
	profileID string,
	adapterID string,
	options dbadapter.ProcessOptions,
	connect dbadapter.ConnectPayload,
) (Metadata, error) {
	if m == nil {
		return Metadata{}, errors.New("database session manager is nil")
	}
	if ctx == nil {
		return Metadata{}, errors.New("database session context is required")
	}
	profileID = strings.TrimSpace(profileID)
	adapterID = strings.TrimSpace(adapterID)
	if profileID == "" {
		return Metadata{}, errors.New("database profile id is required")
	}
	manifest, err := m.registry.Get(adapterID)
	if err != nil {
		return Metadata{}, err
	}

	m.mu.Lock()
	if len(m.sessions)+m.opening >= m.max {
		m.mu.Unlock()
		return Metadata{}, fmt.Errorf("database session limit %d reached", m.max)
	}
	m.opening++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.opening--
		m.mu.Unlock()
	}()

	sessionID, err := newSessionID()
	if err != nil {
		return Metadata{}, err
	}
	process, err := dbadapter.StartProcess(m.ctx, manifest, options)
	if err != nil {
		return Metadata{}, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = process.Close()
		}
	}()

	handshakeCtx, cancel := boundedHandshakeContext(ctx)
	defer cancel()
	if err := verifyAdapter(handshakeCtx, process, manifest, sessionID); err != nil {
		return Metadata{}, err
	}
	if _, err := process.Request(handshakeCtx, dbadapter.OpConnect, connect, sessionID); err != nil {
		return Metadata{}, fmt.Errorf("database adapter connect failed: %w", err)
	}

	meta := Metadata{
		ID:          sessionID,
		ProfileID:   profileID,
		AdapterID:   manifest.ID,
		AdapterKind: manifest.Kind,
		StartedAt:   time.Now().Format(time.RFC3339),
	}
	m.mu.Lock()
	m.sessions[sessionID] = &session{meta: meta, process: process}
	m.mu.Unlock()
	closeOnError = false
	go m.watch(sessionID, process)
	return meta, nil
}

func (m *Manager) List() []Metadata {
	if m == nil {
		return []Metadata{}
	}
	m.mu.RLock()
	out := make([]Metadata, 0, len(m.sessions))
	for _, item := range m.sessions {
		out = append(out, item.meta)
	}
	m.mu.RUnlock()
	return out
}

func (m *Manager) Get(id string) (Metadata, error) {
	item, err := m.getSession(id)
	if err != nil {
		return Metadata{}, err
	}
	return item.meta, nil
}

func (m *Manager) Request(ctx context.Context, id string, operation dbadapter.Operation, payload interface{}) (dbadapter.Envelope, error) {
	if ctx == nil {
		return dbadapter.Envelope{}, errors.New("database request context is required")
	}
	switch operation {
	case dbadapter.OpHello, dbadapter.OpCapabilities, dbadapter.OpConnect, dbadapter.OpDisconnect:
		return dbadapter.Envelope{}, fmt.Errorf("database operation %q is managed internally", operation)
	}
	item, err := m.getSession(id)
	if err != nil {
		return dbadapter.Envelope{}, err
	}
	return item.process.Request(ctx, operation, payload, item.meta.ID)
}

func (m *Manager) Diagnostics(id string) (string, error) {
	item, err := m.getSession(id)
	if err != nil {
		return "", err
	}
	return item.process.Diagnostics(), nil
}

func (m *Manager) CloseSession(id string) error {
	item, err := m.getSession(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, _ = item.process.Request(ctx, dbadapter.OpDisconnect, nil, item.meta.ID)
	cancel()
	_ = item.process.Close()

	m.mu.Lock()
	delete(m.sessions, item.meta.ID)
	m.mu.Unlock()
	return nil
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.cancel()
	m.mu.Lock()
	sessions := m.sessions
	m.sessions = make(map[string]*session)
	m.mu.Unlock()
	for _, item := range sessions {
		_ = item.process.Close()
	}
}

func (m *Manager) getSession(id string) (*session, error) {
	if m == nil {
		return nil, errors.New("database session manager is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrSessionNotFound
	}
	m.mu.RLock()
	item, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrSessionNotFound
	}
	return item, nil
}

func (m *Manager) watch(id string, process *dbadapter.Process) {
	<-process.Done()
	m.mu.Lock()
	if current, ok := m.sessions[id]; ok && current.process == process {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
}

func boundedHandshakeContext(parent context.Context) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) <= adapterHandshakeTime {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, adapterHandshakeTime)
}

func verifyAdapter(ctx context.Context, process *dbadapter.Process, manifest dbadapter.Manifest, sessionID string) error {
	hello, err := process.Request(ctx, dbadapter.OpHello, nil, sessionID)
	if err != nil {
		return fmt.Errorf("database adapter hello failed: %w", err)
	}
	var identity struct {
		AdapterID       string `json:"adapter_id"`
		AdapterName     string `json:"adapter_name"`
		AdapterKind     string `json:"adapter_kind"`
		ProtocolVersion int    `json:"protocol_version"`
	}
	if err := json.Unmarshal(hello.Payload, &identity); err != nil {
		return fmt.Errorf("decode database adapter hello: %w", err)
	}
	if identity.AdapterID != manifest.ID ||
		identity.AdapterKind != manifest.Kind ||
		identity.ProtocolVersion != manifest.ProtocolVersion {
		return fmt.Errorf(
			"database adapter identity mismatch: got id=%q kind=%q protocol=%d",
			identity.AdapterID,
			identity.AdapterKind,
			identity.ProtocolVersion,
		)
	}

	capabilitiesResponse, err := process.Request(ctx, dbadapter.OpCapabilities, nil, sessionID)
	if err != nil {
		return fmt.Errorf("database adapter capabilities failed: %w", err)
	}
	var capabilities dbadapter.CapabilitySet
	if err := json.Unmarshal(capabilitiesResponse.Payload, &capabilities); err != nil {
		return fmt.Errorf("decode database adapter capabilities: %w", err)
	}
	if capabilities != manifest.Capabilities {
		return fmt.Errorf("database adapter capabilities do not match registered manifest")
	}
	return nil
}

func newSessionID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate database session id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
