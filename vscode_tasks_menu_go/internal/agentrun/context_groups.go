package agentrun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxContextGroups       = 128
	maxContextInstructions = 32 << 10
	maxContextGroupTitle   = 120
)

type ContextGroup struct {
	ID                 string    `json:"id"`
	OwnerWorkspaceID   string    `json:"owner_workspace_id"`
	ProjectID          string    `json:"project_id"`
	Title              string    `json:"title"`
	Instructions       string    `json:"instructions"`
	Revision           uint64    `json:"revision"`
	InstructionsSHA256 string    `json:"instructions_sha256"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ContextGroupStore struct{ dir string }

var contextGroupLocks sync.Map

func NewContextGroupStore(dir string) (*ContextGroupStore, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("absolute context-group directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("unsafe context-group directory")
	}
	return &ContextGroupStore{dir: filepath.Clean(dir)}, nil
}

func (s *ContextGroupStore) Create(ownerWorkspaceID, projectID, title, instructions string) (ContextGroup, error) {
	if ownerWorkspaceID == "" || projectID == "" || !validContextGroupText(title, instructions) {
		return ContextGroup{}, errors.New("invalid context-group data")
	}
	groups, err := s.List(ownerWorkspaceID, projectID)
	if err != nil {
		return ContextGroup{}, err
	}
	if len(groups) >= maxContextGroups {
		return ContextGroup{}, errors.New("context-group capacity reached")
	}
	id, err := newID()
	if err != nil {
		return ContextGroup{}, err
	}
	now := time.Now().UTC()
	group := ContextGroup{ID: id, OwnerWorkspaceID: ownerWorkspaceID, ProjectID: projectID, Title: strings.TrimSpace(title), Instructions: instructions, Revision: 1, InstructionsSHA256: contextInstructionsDigest(instructions), CreatedAt: now, UpdatedAt: now}
	if err := s.write(group, 0); err != nil {
		return ContextGroup{}, err
	}
	return group, nil
}

func (s *ContextGroupStore) Get(id, ownerWorkspaceID, projectID string) (ContextGroup, error) {
	if !validWorkflowID(id) {
		return ContextGroup{}, os.ErrNotExist
	}
	path := s.file(id)
	info, err := os.Lstat(path)
	if err != nil {
		return ContextGroup{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() > maxContextInstructions+4096 {
		return ContextGroup{}, errors.New("unsafe context-group record")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ContextGroup{}, err
	}
	var group ContextGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return ContextGroup{}, errors.New("invalid context-group record")
	}
	if group.ID != id || group.OwnerWorkspaceID != ownerWorkspaceID || group.ProjectID != projectID || group.Revision == 0 || !validContextGroupText(group.Title, group.Instructions) || group.InstructionsSHA256 != contextInstructionsDigest(group.Instructions) {
		return ContextGroup{}, errors.New("context-group binding or integrity mismatch")
	}
	return group, nil
}

func (s *ContextGroupStore) List(ownerWorkspaceID, projectID string) ([]ContextGroup, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := make([]ContextGroup, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		group, err := s.Get(id, ownerWorkspaceID, projectID)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, group)
	}
	if len(out) > maxContextGroups {
		return nil, errors.New("context-group storage capacity exceeded")
	}
	return out, nil
}

func (s *ContextGroupStore) Update(id, ownerWorkspaceID, projectID string, expectedRevision uint64, title, instructions string) (ContextGroup, error) {
	group, err := s.Get(id, ownerWorkspaceID, projectID)
	if err != nil {
		return ContextGroup{}, err
	}
	if group.Revision != expectedRevision {
		return ContextGroup{}, errors.New("context-group revision mismatch")
	}
	if !validContextGroupText(title, instructions) {
		return ContextGroup{}, errors.New("invalid context-group data")
	}
	group.Title, group.Instructions = strings.TrimSpace(title), instructions
	group.InstructionsSHA256 = contextInstructionsDigest(instructions)
	group.Revision++
	group.UpdatedAt = time.Now().UTC()
	if err := s.write(group, expectedRevision); err != nil {
		return ContextGroup{}, err
	}
	return group, nil
}

func (s *ContextGroupStore) write(group ContextGroup, expectedRevision uint64) error {
	path := s.file(group.ID)
	lockValue, _ := contextGroupLocks.LoadOrStore(path, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if expectedRevision > 0 {
		current, err := s.Get(group.ID, group.OwnerWorkspaceID, group.ProjectID)
		if err != nil || current.Revision != expectedRevision {
			return errors.New("context-group revision mismatch")
		}
	} else if _, err := os.Lstat(path); err == nil {
		return errors.New("context-group ID already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	body, err := json.Marshal(group)
	if err != nil || len(body) > maxContextInstructions+4096 {
		return errors.New("context-group record exceeds limit")
	}
	tmp, err := os.CreateTemp(s.dir, ".context-group-*.tmp")
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
	return os.Rename(name, path)
}

func (s *ContextGroupStore) file(id string) string { return filepath.Join(s.dir, id+".json") }
func validContextGroupText(title, instructions string) bool {
	return strings.TrimSpace(title) != "" && len(title) <= maxContextGroupTitle && len(instructions) <= maxContextInstructions && utf8.ValidString(title) && utf8.ValidString(instructions) && !strings.ContainsRune(title+instructions, '\x00')
}
func contextInstructionsDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
func ContextInstructionsDigest(text string) string { return contextInstructionsDigest(text) }
