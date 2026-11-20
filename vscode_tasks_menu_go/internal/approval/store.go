package approval

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	ModeOff     = "off"
	ModeConfirm = "confirm"
	ModeType    = "type"
	ModeAdmin   = "admin"

	defaultGrantTTL   = 5 * time.Minute
	defaultRequestTTL = 15 * time.Minute
	maxRecords        = 200
)

type Rule struct {
	Action string `json:"action"`
	Mode   string `json:"mode"`
}

type Policy struct {
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}

type Request struct {
	ID          string `json:"id"`
	Action      string `json:"action"`
	Resource    string `json:"resource"`
	Mode        string `json:"mode"`
	Phrase      string `json:"phrase,omitempty"`
	RequesterID string `json:"requester_id"`
	Requester   string `json:"requester,omitempty"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	ExpiresAt   string `json:"expires_at"`
	ResolvedAt  string `json:"resolved_at,omitempty"`
	ResolverID  string `json:"resolver_id,omitempty"`
}

type fileState struct {
	Version  int       `json:"version"`
	Policy   Policy    `json:"policy"`
	Requests []Request `json:"requests,omitempty"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func DefaultStorePath(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", errors.New("approval workspace is required")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve approval workspace: %w", err)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	if strings.TrimSpace(base) == "" {
		return "", errors.New("user config dir is empty")
	}
	sum := sha256.Sum256([]byte(filepath.Clean(abs)))
	return filepath.Join(base, "vscode_tasks_menu", "approvals", hex.EncodeToString(sum[:8]), "approvals.json"), nil
}

func NewStore(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("approval store path must be absolute")
	}
	return &Store{path: path}, nil
}

func DefaultPolicy() Policy {
	return Policy{Version: 1, Rules: []Rule{}}
}

func NormalizeMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", ModeOff:
		return ModeOff, nil
	case ModeConfirm:
		return ModeConfirm, nil
	case ModeType:
		return ModeType, nil
	case ModeAdmin:
		return ModeAdmin, nil
	default:
		return "", fmt.Errorf("unsupported approval mode %q", value)
	}
}

func NormalizeAction(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("approval action is required")
	}
	if len(value) > 120 {
		return "", errors.New("approval action is too long")
	}
	for _, r := range value {
		if !(r == '.' || r == '_' || r == '-' || r == ':' || r == '/' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return "", errors.New("approval action contains unsupported characters")
		}
	}
	return value, nil
}

func normalizePolicy(policy Policy) (Policy, error) {
	seen := map[string]bool{}
	out := Policy{Version: 1, Rules: make([]Rule, 0, len(policy.Rules))}
	for _, rule := range policy.Rules {
		action, err := NormalizeAction(rule.Action)
		if err != nil {
			return Policy{}, err
		}
		mode, err := NormalizeMode(rule.Mode)
		if err != nil {
			return Policy{}, err
		}
		if seen[action] {
			return Policy{}, fmt.Errorf("duplicate approval action %q", action)
		}
		seen[action] = true
		out.Rules = append(out.Rules, Rule{Action: action, Mode: mode})
	}
	return out, nil
}

func (s *Store) Policy() (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return Policy{}, err
	}
	return state.Policy, nil
}

func (s *Store) SetPolicy(policy Policy) (Policy, error) {
	normalized, err := normalizePolicy(policy)
	if err != nil {
		return Policy{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return Policy{}, err
	}
	state.Policy = normalized
	if err := s.saveLocked(state); err != nil {
		return Policy{}, err
	}
	return normalized, nil
}

func (s *Store) ModeFor(action string) (string, error) {
	action, err := NormalizeAction(action)
	if err != nil {
		return "", err
	}
	policy, err := s.Policy()
	if err != nil {
		return "", err
	}
	for _, rule := range policy.Rules {
		if rule.Action == action {
			return NormalizeMode(rule.Mode)
		}
	}
	return ModeOff, nil
}

func (s *Store) Create(action, resource, requesterID, requester, phrase string) (Request, error) {
	action, err := NormalizeAction(action)
	if err != nil {
		return Request{}, err
	}
	resource = strings.TrimSpace(resource)
	if len(resource) > 500 {
		return Request{}, errors.New("approval resource is too long")
	}
	requesterID = strings.TrimSpace(requesterID)
	if requesterID == "" {
		return Request{}, errors.New("approval requester is required")
	}
	mode, err := s.ModeFor(action)
	if err != nil {
		return Request{}, err
	}
	if mode == ModeOff {
		return Request{}, errors.New("approval is not required for this action")
	}
	id, err := newID()
	if err != nil {
		return Request{}, err
	}
	now := time.Now().UTC()
	status := "pending"
	expires := now.Add(defaultRequestTTL)
	if mode == ModeConfirm || mode == ModeType {
		status = "approved"
		expires = now.Add(defaultGrantTTL)
	}
	req := Request{
		ID: id, Action: action, Resource: resource, Mode: mode, Phrase: strings.TrimSpace(phrase),
		RequesterID: requesterID, Requester: strings.TrimSpace(requester), Status: status,
		CreatedAt: now.Format(time.RFC3339), ExpiresAt: expires.Format(time.RFC3339),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return Request{}, err
	}
	state.Requests = append(state.Requests, req)
	state.Requests = compactRequests(state.Requests, now)
	if err := s.saveLocked(state); err != nil {
		return Request{}, err
	}
	return req, nil
}

func (s *Store) Get(id string) (Request, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Request{}, errors.New("approval request id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return Request{}, err
	}
	now := time.Now().UTC()
	for i := range state.Requests {
		if state.Requests[i].ID != id {
			continue
		}
		if expired(state.Requests[i], now) && (state.Requests[i].Status == "pending" || state.Requests[i].Status == "approved") {
			state.Requests[i].Status = "expired"
			if err := s.saveLocked(state); err != nil {
				return Request{}, err
			}
		}
		return state.Requests[i], nil
	}
	return Request{}, errors.New("approval request not found")
}

func (s *Store) List() ([]Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	state.Requests = compactRequests(state.Requests, now)
	if err := s.saveLocked(state); err != nil {
		return nil, err
	}
	out := append([]Request(nil), state.Requests...)
	return out, nil
}

func (s *Store) Resolve(id, resolverID, status string) (Request, error) {
	id = strings.TrimSpace(id)
	resolverID = strings.TrimSpace(resolverID)
	status = strings.ToLower(strings.TrimSpace(status))
	if id == "" || resolverID == "" {
		return Request{}, errors.New("approval id and resolver are required")
	}
	if status != "approved" && status != "rejected" {
		return Request{}, errors.New("approval resolution must be approved or rejected")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return Request{}, err
	}
	now := time.Now().UTC()
	for i := range state.Requests {
		if state.Requests[i].ID != id {
			continue
		}
		req := &state.Requests[i]
		if req.Mode != ModeAdmin || req.Status != "pending" {
			return Request{}, errors.New("approval request is not pending admin approval")
		}
		if expired(*req, now) {
			req.Status = "expired"
			_ = s.saveLocked(state)
			return Request{}, errors.New("approval request expired")
		}
		req.Status = status
		req.ResolverID = resolverID
		req.ResolvedAt = now.Format(time.RFC3339)
		if status == "approved" {
			req.ExpiresAt = now.Add(defaultGrantTTL).Format(time.RFC3339)
		}
		if err := s.saveLocked(state); err != nil {
			return Request{}, err
		}
		return *req, nil
	}
	return Request{}, errors.New("approval request not found")
}

func (s *Store) Consume(id, action, resource, requesterID string) (Request, error) {
	id = strings.TrimSpace(id)
	action, err := NormalizeAction(action)
	if err != nil {
		return Request{}, err
	}
	resource = strings.TrimSpace(resource)
	requesterID = strings.TrimSpace(requesterID)
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return Request{}, err
	}
	now := time.Now().UTC()
	for i := range state.Requests {
		req := &state.Requests[i]
		if req.ID != id {
			continue
		}
		if req.Status != "approved" {
			return Request{}, errors.New("approval request is not approved")
		}
		if expired(*req, now) {
			req.Status = "expired"
			_ = s.saveLocked(state)
			return Request{}, errors.New("approval request expired")
		}
		if req.Action != action || req.Resource != resource || req.RequesterID != requesterID {
			return Request{}, errors.New("approval grant does not match this action")
		}
		req.Status = "consumed"
		req.ResolvedAt = now.Format(time.RFC3339)
		if err := s.saveLocked(state); err != nil {
			return Request{}, err
		}
		return *req, nil
	}
	return Request{}, errors.New("approval request not found")
}

func (s *Store) loadLocked() (fileState, error) {
	state := fileState{Version: 1, Policy: DefaultPolicy(), Requests: []Request{}}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return fileState{}, err
	}
	if len(data) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return fileState{}, fmt.Errorf("decode approval store: %w", err)
	}
	policy, err := normalizePolicy(state.Policy)
	if err != nil {
		return fileState{}, err
	}
	state.Version = 1
	state.Policy = policy
	if state.Requests == nil {
		state.Requests = []Request{}
	}
	return state, nil
}

func (s *Store) saveLocked(state fileState) error {
	state.Version = 1
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Chmod(s.path, 0o600)
}

func compactRequests(items []Request, now time.Time) []Request {
	out := make([]Request, 0, len(items))
	for _, req := range items {
		if expired(req, now) && (req.Status == "pending" || req.Status == "approved") {
			req.Status = "expired"
		}
		out = append(out, req)
	}
	if len(out) > maxRecords {
		out = out[len(out)-maxRecords:]
	}
	return out
}

func expired(req Request, now time.Time) bool {
	value, err := time.Parse(time.RFC3339, req.ExpiresAt)
	return err != nil || !now.Before(value)
}

func newID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
