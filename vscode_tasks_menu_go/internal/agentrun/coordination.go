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
	GroupID      string    `json:"group_id,omitempty"`
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
	GroupID          string           `json:"group_id,omitempty"`
	ConversationID   string           `json:"conversation_id,omitempty"`
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
	OwnerID      string                `json:"owner_id,omitempty"`
	ProjectID    string                `json:"project_id,omitempty"`
	Peers        map[string]Peer       `json:"peers"`
	Messages     []PeerMessage         `json:"messages"`
	Contracts    map[string]Contract   `json:"contracts"`
	Artifacts    map[string]Artifact   `json:"artifacts"`
	Dependencies map[string]Dependency `json:"dependencies"`
	Events       []CoordinationEvent   `json:"events"`
	EventSeq     uint64                `json:"event_seq"`
}

type CoordinationHub struct {
	mu        *sync.Mutex
	path      string
	ownerID   string
	projectID string
}

// LockCoordinationJournal takes the cross-process exclusive lock that guards
// one coordination journal. Callers that migrate or repair the journal must
// hold it for the whole read-modify-write cycle so no concurrent request can
// observe or overwrite a half-migrated journal.
func LockCoordinationJournal(path string) (func(), error) {
	return lockCoordinationJournal(path)
}

func NewCoordinationHub(path string, scope ...string) (*CoordinationHub, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("absolute coordination hub path required")
	}
	if len(scope) != 0 && len(scope) != 2 {
		return nil, errors.New("coordination hub scope must include owner and project")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	cleanPath := filepath.Clean(path)
	hub := &CoordinationHub{mu: coordinationJournalMutex(cleanPath), path: cleanPath}
	if len(scope) == 2 {
		hub.ownerID, hub.projectID = scope[0], scope[1]
	}
	return hub, nil
}

// lockJournal takes the cross-process exclusive lock of this journal. It is
// always acquired after the in-process mutex, so the lock order is consistent
// and cannot deadlock: in-process mutex first, then the advisory file lock.
func (h *CoordinationHub) lockJournal() (func(), error) {
	return lockCoordinationJournal(h.path)
}

func (h *CoordinationHub) load() (coordinationState, error) {
	state := coordinationState{Version: 2, OwnerID: h.ownerID, ProjectID: h.projectID, Peers: map[string]Peer{}, Contracts: map[string]Contract{}, Artifacts: map[string]Artifact{}, Dependencies: map[string]Dependency{}}
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
	switch state.Version {
	case 1:
		state.migrateVersionOne()
	case 2:
	default:
		return state, errors.New("unsupported coordination hub version")
	}
	if h.ownerID != "" {
		if state.OwnerID != "" && state.OwnerID != h.ownerID {
			return state, errors.New("coordination hub owner mismatch")
		}
		state.OwnerID = h.ownerID
	}
	if h.projectID != "" {
		if state.ProjectID != "" && state.ProjectID != h.projectID {
			return state, errors.New("coordination hub project mismatch")
		}
		state.ProjectID = h.projectID
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

func (state *coordinationState) migrateVersionOne() {
	legacyPeers := state.Peers
	state.Peers = make(map[string]Peer, len(legacyPeers))
	for legacyKey, peer := range legacyPeers {
		if peer.ConversationID == "" {
			peer.ConversationID = legacyKey
		}
		state.Peers[coordinationPeerStorageKey(peer.GroupID, peer.ConversationID)] = peer
	}
	groupByContract := make(map[string]string, len(state.Contracts))
	contracts := make(map[string]Contract, len(state.Contracts))
	for legacyKey, contract := range state.Contracts {
		if contract.Key == "" {
			contract.Key = legacyKey
		}
		contracts[coordinationContractStorageKey(contract.OwnerGroup, contract.Key)] = contract
		groupByContract[contract.Key] = contract.OwnerGroup
	}
	state.Contracts = contracts
	for key, dependency := range state.Dependencies {
		if dependency.ID == "" {
			dependency.ID = key
		}
		if dependency.GroupID == "" {
			dependency.GroupID = groupByContract[dependency.ContractKey]
		}
		state.Dependencies[dependency.ID] = dependency
		if key != dependency.ID {
			delete(state.Dependencies, key)
		}
	}
	state.Version = 2
}

func coordinationContractStorageKey(groupID, key string) string {
	return groupID + "\x00" + key
}

func coordinationPeerStorageKey(groupID, conversationID string) string {
	return groupID + "\x00" + conversationID
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
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return Peer{}, err
	}
	defer unlockJournal()
	state, err := h.load()
	if err != nil {
		return Peer{}, err
	}
	storageKey := coordinationPeerStorageKey(peer.GroupID, peer.ConversationID)
	if existing, ok := state.Peers[storageKey]; ok && existing.GroupID != "" && peer.GroupID == "" {
		peer.GroupID = existing.GroupID
		storageKey = coordinationPeerStorageKey(peer.GroupID, peer.ConversationID)
	}
	peer.UpdatedAt = time.Now().UTC()
	state.Peers[storageKey] = peer
	state.appendEvent(CoordinationEvent{GroupID: peer.GroupID, Kind: "peer_progress", Actor: peer.ConversationID, Summary: string(peer.Status), OccurredAt: peer.UpdatedAt})
	if err := h.save(state); err != nil {
		return Peer{}, err
	}
	return peer, nil
}

func (h *CoordinationHub) Peers(groupID string) ([]Peer, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return nil, err
	}
	defer unlockJournal()
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
	sort.Slice(out, func(i, j int) bool {
		if out[i].GroupID == out[j].GroupID {
			return out[i].ConversationID < out[j].ConversationID
		}
		return out[i].GroupID < out[j].GroupID
	})
	return out, nil
}

func (h *CoordinationHub) PeersForScope(groupID, conversationID string) ([]Peer, error) {
	peers, err := h.Peers(groupID)
	if err != nil || groupID != "" {
		return peers, err
	}
	out := make([]Peer, 0, len(peers))
	for _, peer := range peers {
		if peer.ConversationID == conversationID {
			out = append(out, peer)
		}
	}
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
		message.CorrelationID = "corr-" + shortDigest(message.GroupID+"|"+message.FromConversation+"|"+message.ToConversation+"|"+message.Kind+"|"+message.Body)
	}
	if !message.ExpiresAt.IsZero() && message.ExpiresAt.Before(time.Now().UTC()) {
		return PeerMessage{}, errors.New("peer message already expired")
	}
	if message.ExpiresAt.IsZero() {
		message.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return PeerMessage{}, err
	}
	defer unlockJournal()
	state, err := h.load()
	if err != nil {
		return PeerMessage{}, err
	}
	for _, existing := range state.Messages {
		if existing.GroupID == message.GroupID && existing.CorrelationID == message.CorrelationID && existing.FromConversation == message.FromConversation && existing.Kind == message.Kind {
			return existing, nil
		}
	}
	if message.Kind == "answer" {
		for i := range state.Messages {
			if state.Messages[i].GroupID == message.GroupID && state.Messages[i].CorrelationID == message.CorrelationID &&
				state.Messages[i].ToConversation == message.FromConversation && state.Messages[i].FromConversation == message.ToConversation && state.Messages[i].Status == "pending" {
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
		return PeerMessage{}, errors.New("peer answer does not match a pending question in this group")
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
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return nil, err
	}
	defer unlockJournal()
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
	if contract.Key == "" || len(contract.Key) > 200 || strings.ContainsRune(contract.Key, '\x00') || contract.Signature == "" || len(contract.Signature) > 2000 || strings.ContainsRune(contract.OwnerGroup, '\x00') {
		return Contract{}, errors.New("invalid contract")
	}
	switch contract.Status {
	case ContractPlanned, ContractImplemented, ContractNeedsReview:
	default:
		return Contract{}, errors.New("contract status may only be planned, implemented or needs_review on publish")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return Contract{}, err
	}
	defer unlockJournal()
	state, err := h.load()
	if err != nil {
		return Contract{}, err
	}
	storageKey := coordinationContractStorageKey(contract.OwnerGroup, contract.Key)
	existing, ok := state.Contracts[storageKey]
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
	state.Contracts[storageKey] = contract
	if len(state.Contracts) > maxContracts {
		return Contract{}, errors.New("contract registry capacity exceeded")
	}
	state.appendEvent(CoordinationEvent{GroupID: contract.OwnerGroup, Kind: "contract_published", Actor: contract.OwnerTask, Target: contract.Key, Summary: string(contract.Status), OccurredAt: contract.UpdatedAt})
	if err := h.save(state); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

// AdvanceContract preserves the legacy unscoped API only when the key is unique.
func (h *CoordinationHub) AdvanceContract(key string, revision uint64, status ContractStatus, integratedRef, testEvidence string) (Contract, error) {
	return h.advanceContract("", key, revision, status, integratedRef, testEvidence)
}

func (h *CoordinationHub) AdvanceContractInGroup(groupID, key string, revision uint64, status ContractStatus, integratedRef, testEvidence string) (Contract, error) {
	return h.advanceContract(groupID, key, revision, status, integratedRef, testEvidence)
}

func (h *CoordinationHub) advanceContract(groupID, key string, revision uint64, status ContractStatus, integratedRef, testEvidence string) (Contract, error) {
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
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return Contract{}, err
	}
	defer unlockJournal()
	state, err := h.load()
	if err != nil {
		return Contract{}, err
	}
	storageKey := coordinationContractStorageKey(groupID, key)
	var contract Contract
	var ok bool
	if groupID != "" {
		contract, ok = state.Contracts[storageKey]
	} else {
		for candidateKey, candidate := range state.Contracts {
			if candidate.Key != key {
				continue
			}
			if ok {
				return Contract{}, errors.New("contract key is ambiguous without a group")
			}
			contract, storageKey, ok = candidate, candidateKey, true
		}
	}
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
	state.Contracts[storageKey] = contract
	state.appendEvent(CoordinationEvent{GroupID: contract.OwnerGroup, Kind: "contract_updated", Actor: contract.OwnerTask, Target: key, Summary: string(status), OccurredAt: contract.UpdatedAt})
	if err := h.save(state); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

func (h *CoordinationHub) Contracts(groupID string) ([]Contract, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return nil, err
	}
	defer unlockJournal()
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
	sort.Slice(out, func(i, j int) bool {
		if out[i].OwnerGroup == out[j].OwnerGroup {
			return out[i].Key < out[j].Key
		}
		return out[i].OwnerGroup < out[j].OwnerGroup
	})
	return out, nil
}

func (h *CoordinationHub) ContractsForScope(groupID string) ([]Contract, error) {
	contracts, err := h.Contracts(groupID)
	if err != nil || groupID != "" {
		return contracts, err
	}
	out := make([]Contract, 0, len(contracts))
	for _, contract := range contracts {
		if contract.OwnerGroup == "" {
			out = append(out, contract)
		}
	}
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
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return Artifact{}, err
	}
	defer unlockJournal()
	state, err := h.load()
	if err != nil {
		return Artifact{}, err
	}
	if artifact.GroupID != "" && strings.ContainsRune(artifact.GroupID, '\x00') {
		return Artifact{}, errors.New("invalid artifact group")
	}
	if artifact.ID == "" {
		id, idErr := newID()
		if idErr != nil {
			return Artifact{}, idErr
		}
		artifact.ID = id
	}
	if existing, ok := state.Artifacts[artifact.ID]; ok {
		if existing.GroupID != artifact.GroupID || existing.Conversation != artifact.Conversation || existing.Ref != artifact.Ref || existing.SHA256 != artifact.SHA256 || existing.Kind != artifact.Kind {
			return Artifact{}, errors.New("artifact ID is already owned by another scope or payload")
		}
		return existing, nil
	}
	artifact.CreatedAt = time.Now().UTC()
	state.Artifacts[artifact.ID] = artifact
	if len(state.Artifacts) > maxArtifacts {
		return Artifact{}, errors.New("artifact registry capacity exceeded")
	}
	state.appendEvent(CoordinationEvent{GroupID: artifact.GroupID, Kind: "artifact_published", Actor: artifact.ProducerRun, Target: artifact.ID, Summary: artifact.Kind, OccurredAt: artifact.CreatedAt})
	if err := h.save(state); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func (h *CoordinationHub) Artifacts() ([]Artifact, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return nil, err
	}
	defer unlockJournal()
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

func (h *CoordinationHub) ArtifactsForScope(groupID, conversationID string) ([]Artifact, error) {
	artifacts, err := h.Artifacts()
	if err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if groupID != "" {
			if artifact.GroupID == groupID || artifact.GroupID == "" && conversationID != "" && artifact.Conversation == conversationID {
				out = append(out, artifact)
			}
			continue
		}
		if artifact.GroupID == "" && conversationID != "" && artifact.Conversation == conversationID {
			out = append(out, artifact)
		}
	}
	return out, nil
}

func (h *CoordinationHub) RegisterDependency(dependency Dependency) (Dependency, error) {
	if strings.ContainsAny(dependency.GroupID+dependency.ConversationID, "\x00\r\n") {
		return Dependency{}, errors.New("invalid dependency scope")
	}
	if dependency.ConsumerTask == "" || dependency.ProducerTask == "" || dependency.ContractKey == "" {
		return Dependency{}, errors.New("invalid dependency")
	}
	if dependency.ConsumerTask == dependency.ProducerTask {
		return Dependency{}, errors.New("a task cannot depend on itself")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return Dependency{}, err
	}
	defer unlockJournal()
	state, err := h.load()
	if err != nil {
		return Dependency{}, err
	}
	if dependency.ID == "" {
		dependency.ID = "dep-" + shortDigest(dependency.GroupID+"|"+dependency.ConsumerTask+"|"+dependency.ProducerTask+"|"+dependency.ContractKey)
	}
	if existing, ok := state.Dependencies[dependency.ID]; ok && (existing.GroupID != dependency.GroupID || existing.ContractKey != dependency.ContractKey) {
		return Dependency{}, errors.New("dependency ID is already owned by another group or contract")
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
	var contract Contract
	var ok bool
	if dependency.GroupID != "" {
		contract, ok = state.Contracts[coordinationContractStorageKey(dependency.GroupID, dependency.ContractKey)]
	} else {
		for _, candidate := range state.Contracts {
			if candidate.Key != dependency.ContractKey {
				continue
			}
			if ok {
				return Dependency{}, errors.New("dependency contract key is ambiguous without a group")
			}
			contract, dependency.GroupID, ok = candidate, candidate.OwnerGroup, true
		}
	}
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
	state.appendEvent(CoordinationEvent{GroupID: dependency.GroupID, Kind: "dependency_registered", Actor: dependency.ConversationID, Target: dependency.ProducerTask, Summary: dependency.ContractKey, OccurredAt: dependency.UpdatedAt})
	if err := h.save(state); err != nil {
		return Dependency{}, err
	}
	return dependency, nil
}

func (h *CoordinationHub) Dependencies() ([]Dependency, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return nil, err
	}
	defer unlockJournal()
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

func (h *CoordinationHub) DependenciesForScope(groupID, conversationID string) ([]Dependency, error) {
	dependencies, err := h.Dependencies()
	if err != nil {
		return nil, err
	}
	out := make([]Dependency, 0, len(dependencies))
	for _, dependency := range dependencies {
		if groupID != "" {
			if dependency.GroupID == groupID {
				out = append(out, dependency)
			}
			continue
		}
		if dependency.GroupID == "" && conversationID != "" && dependency.ConversationID == conversationID {
			out = append(out, dependency)
		}
	}
	return out, nil
}

func (h *CoordinationHub) Events(after uint64, limit int) ([]CoordinationEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	unlockJournal, err := h.lockJournal()
	if err != nil {
		return nil, err
	}
	defer unlockJournal()
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

func (h *CoordinationHub) EventsForScope(groupID, conversationID string, after uint64, limit int) ([]CoordinationEvent, error) {
	events, err := h.Events(after, maxCoordinationEvents)
	if err != nil {
		return nil, err
	}
	out := make([]CoordinationEvent, 0, len(events))
	for _, event := range events {
		if groupID != "" {
			if event.GroupID == groupID {
				out = append(out, event)
			}
			continue
		}
		if event.GroupID == "" && conversationID != "" && (event.Actor == conversationID || event.Target == conversationID) {
			out = append(out, event)
		}
	}
	if limit <= 0 || limit > maxCoordinationEvents {
		limit = maxCoordinationEvents
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
