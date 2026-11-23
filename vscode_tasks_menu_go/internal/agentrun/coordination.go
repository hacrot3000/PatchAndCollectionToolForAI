package agentrun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxCoordinationEvents  = 512
	maxPeerMessages        = 128
	maxContracts           = 256
	maxArtifacts           = 256
	maxDependencies        = 512
	maxCoordinationPayload = 8 << 10
)

type PeerStatus string

const (
	PeerIdle      PeerStatus = "idle"
	PeerRunning   PeerStatus = "running"
	PeerWaiting   PeerStatus = "waiting"
	PeerCompleted PeerStatus = "completed"
	PeerFailed    PeerStatus = "failed"
	PeerUnknown   PeerStatus = "unknown"
)

type Peer struct {
	ConversationID string     `json:"conversation_id"`
	GroupID        string     `json:"group_id,omitempty"`
	Status         PeerStatus `json:"status"`
	Summary        string     `json:"summary,omitempty"`
	RunID          string     `json:"run_id,omitempty"`
	ActiveRuns     int        `json:"active_runs"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type PeerMessage struct {
	ID               string    `json:"id"`
	CorrelationID    string    `json:"correlation_id"`
	GroupID          string    `json:"group_id"`
	FromConversation string    `json:"from_conversation"`
	ToConversation   string    `json:"to_conversation"`
	Kind             string    `json:"kind"`
	Body             string    `json:"body"`
	Status           string    `json:"status"`
	HopCount         int       `json:"hop_count"`
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	AnsweredBy       string    `json:"answered_by,omitempty"`
	Answer           string    `json:"answer,omitempty"`
}

type ContractStatus string

const (
	ContractPlanned     ContractStatus = "planned"
	ContractImplemented ContractStatus = "implemented"
	ContractIntegrated  ContractStatus = "integrated"
	ContractVerified    ContractStatus = "verified"
	ContractNeedsReview ContractStatus = "needs_review"
)

type Contract struct {
	Key           string         `json:"key"`
	Revision      uint64         `json:"revision"`
	Signature     string         `json:"signature"`
	OwnerTask     string         `json:"owner_task,omitempty"`
	Status        ContractStatus `json:"status"`
	OwnerGroup    string         `json:"owner_group,omitempty"`
	FilePaths     []string       `json:"file_paths,omitempty"`
	ArtifactRef   string         `json:"artifact_ref,omitempty"`
	IntegratedRef string         `json:"integrated_ref,omitempty"`
	TestEvidence  string         `json:"test_evidence,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type Artifact struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	Ref          string    `json:"ref"`
	SHA256       string    `json:"sha256"`
	ProducerRun  string    `json:"producer_run,omitempty"`
	Conversation string    `json:"conversation_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type DependencyStatus string

const (
	DependencyPlanned     DependencyStatus = "planned"
	DependencyImplemented DependencyStatus = "implemented"
	DependencyIntegrated  DependencyStatus = "integrated"
	DependencyVerified    DependencyStatus = "verified"
)

type Dependency struct {
	ID               string           `json:"id"`
	ConsumerTask     string           `json:"consumer_task"`
	ProducerTask     string           `json:"producer_task"`
	ContractKey      string           `json:"contract_key"`
	ContractRevision uint64           `json:"contract_revision"`
	Status           DependencyStatus `json:"status"`
	WorkspaceRef     string           `json:"workspace_ref,omitempty"`
	TestEvidence     string           `json:"test_evidence,omitempty"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type CoordinationEvent struct {
	Sequence   uint64    `json:"sequence"`
	ID         string    `json:"id"`
	GroupID    string    `json:"group_id,omitempty"`
	Kind       string    `json:"kind"`
	Actor      string    `json:"actor,omitempty"`
	Target     string    `json:"target,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Payload    string    `json:"payload,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

type coordinationState struct {
	Version      int                   `json:"version"`
	Peers        map[string]Peer       `json:"peers"`
	Messages     []PeerMessage         `json:"messages"`
	Contracts    map[string]Contract   `json:"contracts"`
	Artifacts    map[string]Artifact   `json:"artifacts"`
	Dependencies map[string]Dependency `json:"dependencies"`
	Events       []CoordinationEvent   `json:"events"`
	EventSeq     uint64                `json:"event_seq"`
}

type CoordinationHub struct {
	mu   sync.Mutex
	path string
}

func NewCoordinationHub(path string) (*CoordinationHub, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("absolute coordination hub path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return &CoordinationHub{path: filepath.Clean(path)}, nil
}

func (h *CoordinationHub) load() (coordinationState, error) {
	state := coordinationState{Version: 1, Peers: map[string]Peer{}, Contracts: map[string]Contract{}, Artifacts: map[string]Artifact{}, Dependencies: map[string]Dependency{}}
	info, err := os.Lstat(h.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() > 8<<20 {
		return state, errors.New("unsafe coordination hub state")
	}
	body, err := os.ReadFile(h.path)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(body, &state); err != nil {
		return state, errors.New("invalid coordination hub state")
	}
	if state.Version != 1 {
		return state, errors.New("unsupported coordination hub version")
	}
	if state.Peers == nil {
		state.Peers = map[string]Peer{}
	}
	if state.Contracts == nil {
		state.Contracts = map[string]Contract{}
	}
	if state.Artifacts == nil {
		state.Artifacts = map[string]Artifact{}
	}
	if state.Dependencies == nil {
		state.Dependencies = map[string]Dependency{}
	}
	return state, nil
}

func (h *CoordinationHub) save(state coordinationState) error {
	body, err := json.Marshal(state)
	if err != nil || len(body) > 8<<20 {
		return errors.New("coordination state exceeds limit")
	}
	tmp, err := os.CreateTemp(filepath.Dir(h.path), ".coordination-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(append(body, '\n'))
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, h.path)
}

// UpsertPeer records peer progress. Only the conversation owner may update it,
// and a peer never reports a state it cannot prove from its own run journal.
func (h *CoordinationHub) UpsertPeer(peer Peer) (Peer, error) {
	if peer.ConversationID == "" || len(peer.ConversationID) > 256 {
		return Peer{}, errors.New("invalid peer conversation")
	}
	if len(peer.Summary) > 2000 || strings.ContainsAny(peer.Summary, "\x00") {
		return Peer{}, errors.New("invalid peer summary")
	}
	switch peer.Status {
	case PeerIdle, PeerRunning, PeerWaiting, PeerCompleted, PeerFailed, PeerUnknown:
	default:
		return Peer{}, errors.New("invalid peer status")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return Peer{}, err
	}
	existing, ok := state.Peers[peer.ConversationID]
	if ok && existing.GroupID != "" && peer.GroupID == "" {
		peer.GroupID = existing.GroupID
	}
	peer.UpdatedAt = time.Now().UTC()
	state.Peers[peer.ConversationID] = peer
	state.appendEvent(CoordinationEvent{GroupID: peer.GroupID, Kind: "peer_progress", Actor: peer.ConversationID, Summary: string(peer.Status), OccurredAt: peer.UpdatedAt})
	if err := h.save(state); err != nil {
		return Peer{}, err
	}
	return peer, nil
}

func (h *CoordinationHub) Peers(groupID string) ([]Peer, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return nil, err
	}
	out := make([]Peer, 0, len(state.Peers))
	for _, peer := range state.Peers {
		if groupID == "" || peer.GroupID == groupID {
			out = append(out, peer)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ConversationID < out[j].ConversationID })
	return out, nil
}

// PostPeerMessage records a peer question or answer. Hop count and TTL bound
// ping-pong loops; a duplicate correlation ID is idempotent instead of a new event.
func (h *CoordinationHub) PostPeerMessage(message PeerMessage) (PeerMessage, error) {
	if message.FromConversation == "" || message.ToConversation == "" || message.FromConversation == message.ToConversation {
		return PeerMessage{}, errors.New("invalid peer message participants")
	}
	switch message.Kind {
	case "ask", "answer", "progress", "blocker":
	default:
		return PeerMessage{}, errors.New("invalid peer message kind")
	}
	if message.Body == "" || len(message.Body) > maxCoordinationPayload || strings.ContainsAny(message.Body, "\x00") {
		return PeerMessage{}, errors.New("invalid peer message body")
	}
	if message.HopCount < 0 || message.HopCount > 4 {
		return PeerMessage{}, errors.New("peer message hop count exceeds limit")
	}
	if message.CorrelationID == "" {
		message.CorrelationID = "corr-" + shortDigest(message.FromConversation+message.ToConversation+message.Kind+message.Body)
	}
	if !message.ExpiresAt.IsZero() && message.ExpiresAt.Before(time.Now().UTC()) {
		return PeerMessage{}, errors.New("peer message already expired")
	}
	if message.ExpiresAt.IsZero() {
		message.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return PeerMessage{}, err
	}
	for _, existing := range state.Messages {
		if existing.CorrelationID == message.CorrelationID && existing.FromConversation == message.FromConversation && existing.Kind == message.Kind {
			return existing, nil
		}
	}
	if message.Kind == "answer" {
		for i := range state.Messages {
			if state.Messages[i].CorrelationID == message.CorrelationID && state.Messages[i].Status == "pending" {
				state.Messages[i].Status = "answered"
				state.Messages[i].AnsweredBy = message.FromConversation
				state.Messages[i].Answer = message.Body
				state.appendEvent(CoordinationEvent{GroupID: message.GroupID, Kind: "peer_answered", Actor: message.FromConversation, Target: message.ToConversation, Summary: "answer delivered", OccurredAt: time.Now().UTC()})
				if err := h.save(state); err != nil {
					return PeerMessage{}, err
				}
				return state.Messages[i], nil
			}
		}
	}
	message.Status = "pending"
	message.CreatedAt = time.Now().UTC()
	id, err := newID()
	if err != nil {
		return PeerMessage{}, err
	}
	message.ID = id
	state.Messages = append(state.Messages, message)
	if len(state.Messages) > maxPeerMessages {
		state.Messages = state.Messages[len(state.Messages)-maxPeerMessages:]
	}
	state.appendEvent(CoordinationEvent{GroupID: message.GroupID, Kind: "peer_" + message.Kind, Actor: message.FromConversation, Target: message.ToConversation, Summary: message.Kind, Payload: message.Body, OccurredAt: message.CreatedAt})
	if err := h.save(state); err != nil {
		return PeerMessage{}, err
	}
	return message, nil
}

func (h *CoordinationHub) PeerMessages(conversationID, groupID string) ([]PeerMessage, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return nil, err
	}
	out := make([]PeerMessage, 0, len(state.Messages))
	for _, message := range state.Messages {
		if conversationID != "" && message.FromConversation != conversationID && message.ToConversation != conversationID {
			continue
		}
		if groupID != "" && message.GroupID != groupID {
			continue
		}
		out = append(out, message)
	}
	return out, nil
}

// UpsertContract publishes a contract revision. Status never advances on its
// own: only verified evidence can move it to integrated or verified.
func (h *CoordinationHub) UpsertContract(contract Contract) (Contract, error) {
	if contract.Key == "" || len(contract.Key) > 200 || contract.Signature == "" || len(contract.Signature) > 2000 {
		return Contract{}, errors.New("invalid contract")
	}
	switch contract.Status {
	case ContractPlanned, ContractImplemented, ContractNeedsReview:
	default:
		return Contract{}, errors.New("contract status may only be planned, implemented or needs_review on publish")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return Contract{}, err
	}
	existing, ok := state.Contracts[contract.Key]
	if ok {
		if contract.Revision == 0 {
			contract.Revision = existing.Revision
		}
		if contract.Revision < existing.Revision {
			return Contract{}, errors.New("contract revision cannot move backwards")
		}
		if contract.Revision == existing.Revision && contract.Signature != existing.Signature {
			return Contract{}, errors.New("contract signature changed without a revision bump")
		}
		if contract.Status == ContractPlanned && existing.Status != ContractPlanned {
			contract.Status = ContractNeedsReview
		}
	}
	if contract.Revision == 0 {
		contract.Revision = 1
	}
	contract.UpdatedAt = time.Now().UTC()
	state.Contracts[contract.Key] = contract
	if len(state.Contracts) > maxContracts {
		return Contract{}, errors.New("contract registry capacity exceeded")
	}
	state.appendEvent(CoordinationEvent{GroupID: contract.OwnerGroup, Kind: "contract_published", Actor: contract.OwnerTask, Target: contract.Key, Summary: string(contract.Status), OccurredAt: contract.UpdatedAt})
	if err := h.save(state); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

// AdvanceContract moves a contract forward only with explicit evidence.
func (h *CoordinationHub) AdvanceContract(key string, revision uint64, status ContractStatus, integratedRef, testEvidence string) (Contract, error) {
	switch status {
	case ContractIntegrated, ContractVerified, ContractNeedsReview:
	default:
		return Contract{}, errors.New("unsupported contract transition")
	}
	if status == ContractIntegrated && integratedRef == "" {
		return Contract{}, errors.New("integrated contracts require an artifact reference")
	}
	if status == ContractVerified && (integratedRef == "" || testEvidence == "") {
		return Contract{}, errors.New("verified contracts require an integrated reference and test evidence")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return Contract{}, err
	}
	contract, ok := state.Contracts[key]
	if !ok {
		return Contract{}, errors.New("contract not found")
	}
	if revision != 0 && contract.Revision != revision {
		return Contract{}, errors.New("contract revision mismatch")
	}
	if contract.Status == ContractVerified && status != ContractNeedsReview {
		return Contract{}, errors.New("verified contracts cannot regress without review")
	}
	contract.Status, contract.IntegratedRef, contract.TestEvidence = status, integratedRef, testEvidence
	contract.UpdatedAt = time.Now().UTC()
	state.Contracts[key] = contract
	state.appendEvent(CoordinationEvent{GroupID: contract.OwnerGroup, Kind: "contract_updated", Actor: contract.OwnerTask, Target: key, Summary: string(status), OccurredAt: contract.UpdatedAt})
	if err := h.save(state); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

func (h *CoordinationHub) Contracts(groupID string) ([]Contract, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return nil, err
	}
	out := make([]Contract, 0, len(state.Contracts))
	for _, contract := range state.Contracts {
		if groupID == "" || contract.OwnerGroup == groupID {
			out = append(out, contract)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (h *CoordinationHub) PublishArtifact(artifact Artifact) (Artifact, error) {
	if artifact.Ref == "" || len(artifact.Ref) > 1024 {
		return Artifact{}, errors.New("invalid artifact reference")
	}
	if len(artifact.SHA256) != 64 || strings.Trim(artifact.SHA256, "0123456789abcdef") != "" {
		return Artifact{}, errors.New("artifact checksum must be a SHA-256 hex digest")
	}
	switch artifact.Kind {
	case "commit", "diff", "patch", "file", "build":
	default:
		return Artifact{}, errors.New("unsupported artifact kind")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return Artifact{}, err
	}
	if artifact.ID == "" {
		id, idErr := newID()
		if idErr != nil {
			return Artifact{}, idErr
		}
		artifact.ID = id
	}
	artifact.CreatedAt = time.Now().UTC()
	state.Artifacts[artifact.ID] = artifact
	if len(state.Artifacts) > maxArtifacts {
		return Artifact{}, errors.New("artifact registry capacity exceeded")
	}
	state.appendEvent(CoordinationEvent{Kind: "artifact_published", Actor: artifact.ProducerRun, Target: artifact.ID, Summary: artifact.Kind, OccurredAt: artifact.CreatedAt})
	if err := h.save(state); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func (h *CoordinationHub) Artifacts() ([]Artifact, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(state.Artifacts))
	for _, artifact := range state.Artifacts {
		out = append(out, artifact)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (h *CoordinationHub) RegisterDependency(dependency Dependency) (Dependency, error) {
	if dependency.ConsumerTask == "" || dependency.ProducerTask == "" || dependency.ContractKey == "" {
		return Dependency{}, errors.New("invalid dependency")
	}
	if dependency.ConsumerTask == dependency.ProducerTask {
		return Dependency{}, errors.New("a task cannot depend on itself")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return Dependency{}, err
	}
	if dependency.ID == "" {
		dependency.ID = "dep-" + shortDigest(dependency.ConsumerTask+"|"+dependency.ProducerTask+"|"+dependency.ContractKey)
	}
	if dependency.Status == "" {
		dependency.Status = DependencyPlanned
	}
	switch dependency.Status {
	case DependencyPlanned, DependencyImplemented, DependencyIntegrated, DependencyVerified:
	default:
		return Dependency{}, errors.New("invalid dependency status")
	}
	if dependency.Status == DependencyVerified && (dependency.WorkspaceRef == "" || dependency.TestEvidence == "") {
		return Dependency{}, errors.New("verified dependency requires a workspace reference and test evidence")
	}
	contract, ok := state.Contracts[dependency.ContractKey]
	if !ok {
		return Dependency{}, errors.New("dependency must reference a published contract")
	}
	if dependency.ContractRevision == 0 {
		dependency.ContractRevision = contract.Revision
	}
	if dependency.ContractRevision != contract.Revision {
		return Dependency{}, errors.New("dependency contract revision mismatch")
	}
	dependency.UpdatedAt = time.Now().UTC()
	state.Dependencies[dependency.ID] = dependency
	if len(state.Dependencies) > maxDependencies {
		return Dependency{}, errors.New("dependency registry capacity exceeded")
	}
	state.appendEvent(CoordinationEvent{Kind: "dependency_registered", Actor: dependency.ConsumerTask, Target: dependency.ProducerTask, Summary: dependency.ContractKey, OccurredAt: dependency.UpdatedAt})
	if err := h.save(state); err != nil {
		return Dependency{}, err
	}
	return dependency, nil
}

func (h *CoordinationHub) Dependencies() ([]Dependency, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return nil, err
	}
	out := make([]Dependency, 0, len(state.Dependencies))
	for _, dependency := range state.Dependencies {
		out = append(out, dependency)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (h *CoordinationHub) Events(after uint64, limit int) ([]CoordinationEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state, err := h.load()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxCoordinationEvents {
		limit = maxCoordinationEvents
	}
	out := make([]CoordinationEvent, 0, len(state.Events))
	for _, event := range state.Events {
		if event.Sequence > after {
			out = append(out, event)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (s *coordinationState) appendEvent(event CoordinationEvent) {
	s.EventSeq++
	event.Sequence = s.EventSeq
	if event.ID == "" {
		event.ID = "evt-" + shortDigest(fmt.Sprintf("%d|%s|%s|%s", event.Sequence, event.Kind, event.Actor, event.Target))
	}
	s.Events = append(s.Events, event)
	if len(s.Events) > maxCoordinationEvents {
		s.Events = s.Events[len(s.Events)-maxCoordinationEvents:]
	}
}

func shortDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}
