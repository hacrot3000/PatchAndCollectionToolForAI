package agentrun

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

const (
	maxConversationGroups = 128
	maxGroupMembers       = 256
)

type ConversationGroup struct {
	ID                 string    `json:"id"`
	OwnerWorkspaceID   string    `json:"owner_workspace_id"`
	ProjectID          string    `json:"project_id"`
	Title              string    `json:"title"`
	SharedRevision     uint64    `json:"shared_revision"`
	SharedInstructions string    `json:"shared_instructions"`
	InstructionsSHA256 string    `json:"instructions_sha256"`
	Members            []string  `json:"members"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ConversationFork struct {
	SourceConversationID    string    `json:"source_conversation_id"`
	ForkTurnID              string    `json:"fork_turn_id,omitempty"`
	SharedContextSnapshotID string    `json:"shared_context_snapshot_id,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
}

type conversationGroupState struct {
	Version int                          `json:"version"`
	Groups  map[string]ConversationGroup `json:"groups"`
	Forks   map[string]ConversationFork  `json:"forks"`
}

type ConversationRegistry struct {
	mu   sync.Mutex
	path string
}

func NewConversationRegistry(path string) (*ConversationRegistry, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("absolute conversation registry path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return &ConversationRegistry{path: filepath.Clean(path)}, nil
}

func (r *ConversationRegistry) load() (conversationGroupState, error) {
	state := conversationGroupState{Version: 1, Groups: map[string]ConversationGroup{}, Forks: map[string]ConversationFork{}}
	info, err := os.Lstat(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() > 4<<20 {
		return state, errors.New("unsafe conversation registry")
	}
	body, err := os.ReadFile(r.path)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(body, &state); err != nil {
		return state, errors.New("invalid conversation registry")
	}
	if state.Version != 1 {
		return state, errors.New("unsupported conversation registry version")
	}
	if state.Groups == nil {
		state.Groups = map[string]ConversationGroup{}
	}
	if state.Forks == nil {
		state.Forks = map[string]ConversationFork{}
	}
	return state, nil
}

func (r *ConversationRegistry) save(state conversationGroupState) error {
	body, err := json.Marshal(state)
	if err != nil || len(body) > 4<<20 {
		return errors.New("conversation registry exceeds limit")
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.path), ".conversations-*.tmp")
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
	return os.Rename(name, r.path)
}

func (r *ConversationRegistry) CreateGroup(ownerWorkspaceID, projectID, title string) (ConversationGroup, error) {
	if ownerWorkspaceID == "" || projectID == "" || strings.TrimSpace(title) == "" || len(title) > 120 {
		return ConversationGroup{}, errors.New("invalid conversation group")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return ConversationGroup{}, err
	}
	if len(state.Groups) >= maxConversationGroups {
		return ConversationGroup{}, errors.New("conversation group capacity reached")
	}
	id, err := newID()
	if err != nil {
		return ConversationGroup{}, err
	}
	now := time.Now().UTC()
	group := ConversationGroup{ID: id, OwnerWorkspaceID: ownerWorkspaceID, ProjectID: projectID, Title: strings.TrimSpace(title), SharedRevision: 1, Members: []string{}, CreatedAt: now, UpdatedAt: now}
	state.Groups[id] = group
	if err := r.save(state); err != nil {
		return ConversationGroup{}, err
	}
	return group, nil
}

func (r *ConversationRegistry) UpdateGroupInstructions(id, ownerWorkspaceID, projectID string, expectedRevision uint64, instructions string) (ConversationGroup, error) {
	if len(instructions) > 32<<10 || strings.ContainsRune(instructions, '\x00') {
		return ConversationGroup{}, errors.New("invalid shared instructions")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return ConversationGroup{}, err
	}
	group, ok := state.Groups[id]
	if !ok || group.OwnerWorkspaceID != ownerWorkspaceID || group.ProjectID != projectID {
		return ConversationGroup{}, errors.New("conversation group not found")
	}
	if group.SharedRevision != expectedRevision {
		return ConversationGroup{}, errors.New("conversation group revision mismatch")
	}
	group.SharedInstructions = instructions
	group.InstructionsSHA256 = ContextInstructionsDigest(instructions)
	group.SharedRevision++
	group.UpdatedAt = time.Now().UTC()
	state.Groups[id] = group
	if err := r.save(state); err != nil {
		return ConversationGroup{}, err
	}
	return group, nil
}

func (r *ConversationRegistry) Groups(ownerWorkspaceID, projectID string) ([]ConversationGroup, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return nil, err
	}
	out := make([]ConversationGroup, 0, len(state.Groups))
	for _, group := range state.Groups {
		if group.OwnerWorkspaceID == ownerWorkspaceID && group.ProjectID == projectID {
			out = append(out, group)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (r *ConversationRegistry) AddMember(id, ownerWorkspaceID, projectID, conversationID string) (ConversationGroup, error) {
	if conversationID == "" {
		return ConversationGroup{}, errors.New("invalid group member")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return ConversationGroup{}, err
	}
	group, ok := state.Groups[id]
	if !ok || group.OwnerWorkspaceID != ownerWorkspaceID || group.ProjectID != projectID {
		return ConversationGroup{}, errors.New("conversation group not found")
	}
	for _, member := range group.Members {
		if member == conversationID {
			return group, nil
		}
	}
	if len(group.Members) >= maxGroupMembers {
		return ConversationGroup{}, errors.New("conversation group member capacity reached")
	}
	group.Members = append(group.Members, conversationID)
	group.UpdatedAt = time.Now().UTC()
	state.Groups[id] = group
	if err := r.save(state); err != nil {
		return ConversationGroup{}, err
	}
	return group, nil
}

func (r *ConversationRegistry) RemoveMember(id, ownerWorkspaceID, projectID, conversationID string) (ConversationGroup, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return ConversationGroup{}, err
	}
	group, ok := state.Groups[id]
	if !ok || group.OwnerWorkspaceID != ownerWorkspaceID || group.ProjectID != projectID {
		return ConversationGroup{}, errors.New("conversation group not found")
	}
	members := make([]string, 0, len(group.Members))
	for _, member := range group.Members {
		if member != conversationID {
			members = append(members, member)
		}
	}
	group.Members = members
	group.UpdatedAt = time.Now().UTC()
	state.Groups[id] = group
	if err := r.save(state); err != nil {
		return ConversationGroup{}, err
	}
	return group, nil
}

// RecordFork captures the lineage of a forked conversation. The fork inherits
// the ancestor prefix of the source but never merges later local turns.
func (r *ConversationRegistry) RecordFork(forkedConversationID, sourceConversationID, forkTurnID, sharedSnapshotID string) (ConversationFork, error) {
	if forkedConversationID == "" || sourceConversationID == "" || forkedConversationID == sourceConversationID {
		return ConversationFork{}, errors.New("invalid fork lineage")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return ConversationFork{}, err
	}
	if existing, ok := state.Forks[forkedConversationID]; ok {
		return existing, nil
	}
	fork := ConversationFork{SourceConversationID: sourceConversationID, ForkTurnID: forkTurnID, SharedContextSnapshotID: sharedSnapshotID, CreatedAt: time.Now().UTC()}
	state.Forks[forkedConversationID] = fork
	if err := r.save(state); err != nil {
		return ConversationFork{}, err
	}
	return fork, nil
}

func (r *ConversationRegistry) Fork(forkedConversationID string) (ConversationFork, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return ConversationFork{}, false
	}
	fork, ok := state.Forks[forkedConversationID]
	return fork, ok
}

// SharedSnapshotFor returns the ancestor prefix history for a fork. The prefix
// stops at the recorded fork turn so later local turns never leak into it.
func (r *ConversationRegistry) SharedSnapshotFor(forkedConversationID string, source []agentchat.HistoryItem) ([]agentchat.HistoryItem, bool) {
	fork, ok := r.Fork(forkedConversationID)
	if !ok {
		return nil, false
	}
	if fork.ForkTurnID == "" {
		return append([]agentchat.HistoryItem(nil), source...), true
	}
	prefix, err := ForkHistoryThroughTurn(source, fork.ForkTurnID)
	if err != nil {
		return nil, false
	}
	return prefix, true
}

// ForkHistoryThroughTurn returns the ancestor prefix through the selected turn.
func ForkHistoryThroughTurn(source []agentchat.HistoryItem, forkTurnID string) ([]agentchat.HistoryItem, error) {
	if forkTurnID == "" {
		return append([]agentchat.HistoryItem(nil), source...), nil
	}
	targetEnd := -1
	for i, item := range source {
		if item.OriginRunID == forkTurnID {
			targetEnd = i
		}
	}
	if targetEnd < 0 {
		for i, item := range source {
			if item.ParentTurnID == forkTurnID {
				return append([]agentchat.HistoryItem(nil), source[:i]...), nil
			}
		}
		return nil, errors.New("fork turn is not present in source history")
	}
	end := len(source)
	for i := targetEnd + 1; i < len(source); i++ {
		if source[i].ParentTurnID == forkTurnID || source[i].OriginRunID != "" && source[i].OriginRunID != forkTurnID {
			end = i
			break
		}
	}
	return append([]agentchat.HistoryItem(nil), source[:end]...), nil
}
