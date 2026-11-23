package agentrun

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/taskmanifest"
)

const maxWorkflowPlans = 256

type WorkflowStatus string

const (
	WorkflowQueued      WorkflowStatus = "queued"
	WorkflowRunning     WorkflowStatus = "running"
	WorkflowReview      WorkflowStatus = "waiting_review"
	WorkflowCompleted   WorkflowStatus = "completed"
	WorkflowFailed      WorkflowStatus = "failed"
	WorkflowStopped     WorkflowStatus = "stopped"
	WorkflowInterrupted WorkflowStatus = "interrupted"
)

type WorkflowRecord struct {
	ID                    string                `json:"id"`
	OwnerWorkspaceID      string                `json:"owner_workspace_id"`
	ProjectID             string                `json:"project_id"`
	Manifest              taskmanifest.Manifest `json:"manifest"`
	ContextGroupID        string                `json:"context_group_id,omitempty"`
	ContextGroupRevision  uint64                `json:"context_group_revision,omitempty"`
	ContextGroupSHA256    string                `json:"context_group_sha256,omitempty"`
	SharedInstructions    string                `json:"shared_instructions,omitempty"`
	DAG                   DAGSnapshot           `json:"dag"`
	ProviderID            string                `json:"provider_id"`
	ModelID               string                `json:"model_id"`
	ProviderConfigVersion string                `json:"provider_config_version"`
	CredentialFingerprint string               `json:"credential_fingerprint"`
	Concurrency           int                   `json:"concurrency"`
	Status                WorkflowStatus        `json:"status"`
	Revision              uint64                `json:"revision"`
	CreatedAt             time.Time             `json:"created_at"`
	UpdatedAt             time.Time             `json:"updated_at"`
}

type planStoreFile struct {
	Version int            `json:"version"`
	Plan    WorkflowRecord `json:"plan"`
}
type PlanStore struct{ dir string }

var planStoreLocks sync.Map

func NewPlanStore(dir string) (*PlanStore, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("absolute workflow storage directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("unsafe workflow storage directory")
	}
	return &PlanStore{dir: filepath.Clean(dir)}, nil
}

func (s *PlanStore) Create(record WorkflowRecord) (WorkflowRecord, error) {
	if record.ID == "" {
		id, err := newID()
		if err != nil {
			return WorkflowRecord{}, err
		}
		record.ID = id
	}
	if !record.Manifest.Valid || record.Manifest.ProjectID != record.ProjectID || record.Manifest.PlanningOnly != true || record.Concurrency < 1 || record.Concurrency > MaxConcurrentRuns || record.ProviderID == "" || record.ModelID == "" || record.ProviderConfigVersion == "" || len(record.CredentialFingerprint) != 64 || strings.Trim(record.CredentialFingerprint, "0123456789abcdef") != "" || record.OwnerWorkspaceID == "" || !validWorkflowContextGroup(record) {
		return WorkflowRecord{}, errors.New("invalid workflow binding")
	}
	if _, err := RestoreDAGScheduler(record.Manifest, record.DAG); err != nil || record.DAG.Concurrency != record.Concurrency {
		return WorkflowRecord{}, errors.New("workflow DAG snapshot is invalid")
	}
	if record.Status != WorkflowQueued {
		return WorkflowRecord{}, errors.New("new workflow must be queued")
	}
	now := time.Now().UTC()
	record.CreatedAt, record.UpdatedAt, record.Revision = now, now, 1
	if err := s.write(record, 0); err != nil {
		return WorkflowRecord{}, err
	}
	return record, nil
}

func (s *PlanStore) Get(id string) (WorkflowRecord, error) {
	if !validWorkflowID(id) {
		return WorkflowRecord{}, os.ErrNotExist
	}
	path := s.file(id)
	info, err := os.Lstat(path)
	if err != nil {
		return WorkflowRecord{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() > 2<<20 {
		return WorkflowRecord{}, errors.New("unsafe workflow record")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return WorkflowRecord{}, err
	}
	var stored planStoreFile
	if err := json.Unmarshal(body, &stored); err != nil || stored.Version != 1 || stored.Plan.ID != id {
		return WorkflowRecord{}, errors.New("invalid workflow record")
	}
	return stored.Plan, nil
}

func (s *PlanStore) List() ([]WorkflowRecord, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := make([]WorkflowRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-len(".json")]
		plan, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		out = append(out, plan)
		if len(out) > maxWorkflowPlans {
			return nil, errors.New("workflow store capacity exceeded")
		}
	}
	return out, nil
}

func (s *PlanStore) Update(record WorkflowRecord, expectedRevision uint64) (WorkflowRecord, error) {
	current, err := s.Get(record.ID)
	if err != nil {
		return WorkflowRecord{}, err
	}
	if current.Revision != expectedRevision {
		return WorkflowRecord{}, errors.New("workflow revision mismatch")
	}
	if record.Manifest.ManifestID != current.Manifest.ManifestID || record.ProjectID != current.ProjectID || record.OwnerWorkspaceID != current.OwnerWorkspaceID || record.ContextGroupID != current.ContextGroupID || record.ContextGroupRevision != current.ContextGroupRevision || record.ContextGroupSHA256 != current.ContextGroupSHA256 || record.SharedInstructions != current.SharedInstructions || record.ProviderID != current.ProviderID || record.ModelID != current.ModelID || record.ProviderConfigVersion != current.ProviderConfigVersion || record.CredentialFingerprint != current.CredentialFingerprint || record.Concurrency != current.Concurrency {
		return WorkflowRecord{}, errors.New("workflow immutable binding changed")
	}
	record.CreatedAt = current.CreatedAt
	record.Revision = current.Revision + 1
	record.UpdatedAt = time.Now().UTC()
	if err := s.write(record, expectedRevision); err != nil {
		return WorkflowRecord{}, err
	}
	return record, nil
}

func (s *PlanStore) write(record WorkflowRecord, expectedRevision uint64) error {
	path := s.file(record.ID)
	lockValue, _ := planStoreLocks.LoadOrStore(path, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if expectedRevision == 0 {
		if _, err := os.Lstat(path); err == nil {
			return errors.New("workflow ID already exists")
		} else if !os.IsNotExist(err) {
			return err
		}
	} else {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("workflow record unavailable")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var current planStoreFile
		if err := json.Unmarshal(body, &current); err != nil || current.Plan.Revision != expectedRevision {
			return errors.New("workflow revision mismatch")
		}
	}
	body, err := json.Marshal(planStoreFile{Version: 1, Plan: record})
	if err != nil || len(body) > 2<<20 {
		return errors.New("workflow record exceeds limit")
	}
	tmp, err := os.CreateTemp(s.dir, ".workflow-*.tmp")
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

func (s *PlanStore) file(id string) string { return filepath.Join(s.dir, id+".json") }
func validWorkflowID(id string) bool {
	return len(id) == 32 && len(strings.Trim(id, "0123456789abcdef")) == 0
}

func validWorkflowContextGroup(record WorkflowRecord) bool {
	if record.ContextGroupID == "" {
		return record.ContextGroupRevision == 0 && record.ContextGroupSHA256 == "" && record.SharedInstructions == ""
	}
	return validWorkflowID(record.ContextGroupID) && record.ContextGroupRevision > 0 && len(record.SharedInstructions) <= 32<<10 && ContextInstructionsDigest(record.SharedInstructions) == record.ContextGroupSHA256
}
