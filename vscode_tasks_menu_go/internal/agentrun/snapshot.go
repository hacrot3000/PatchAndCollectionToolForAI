package agentrun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
)

const maxSnapshotBytes = 4 << 20
const maxSnapshotHistory = 256

type AutomationSnapshot struct {
	AutoContinueText bool `json:"auto_continue_text,omitempty"`
	AutoContinuePlan bool `json:"auto_continue_plan,omitempty"`
	// ContinuationMode is set only by the server when an automatic turn is
	// queued. The immutable snapshot revalidates current owner consent at egress.
	ContinuationMode string `json:"continuation_mode,omitempty"`
	AutoPatch              bool       `json:"auto_patch,omitempty"`
	AutoCollection         *bool      `json:"auto_collection,omitempty"`
	AutoFeedback           bool       `json:"auto_feedback,omitempty"`
	AutoFeedbackPersistent bool       `json:"auto_feedback_persistent,omitempty"`
	AutoFeedbackUntil      *time.Time `json:"auto_feedback_until,omitempty"`
}

type RunBindingSnapshot struct {
	RunID                 string              `json:"run_id"`
	ConversationID        string              `json:"conversation_id"`
	OwnerWorkspaceID      string              `json:"owner_workspace_id"`
	ExecutionWorkspaceID  string              `json:"execution_workspace_id"`
	ParentTurnID          string              `json:"parent_turn_id,omitempty"`
	ProviderID            string              `json:"provider_id"`
	ModelID               string              `json:"model_id"`
	ProviderConfigVersion string              `json:"provider_config_version"`
	CredentialFingerprint string              `json:"credential_fingerprint"`
	ExcludedPendingRunIDs string              `json:"excluded_pending_run_ids,omitempty"`
	RetryMode             agentchat.RetryMode `json:"retry_mode"`
}

type ContextSnapshotPolicy struct {
	SystemInstructions string              `json:"system_instructions,omitempty"`
	Automation         AutomationSnapshot  `json:"automation,omitempty"`
	RetryMode          agentchat.RetryMode `json:"retry_mode,omitempty"`
	ReasoningEffort    string              `json:"reasoning_effort,omitempty"`
	RunBinding         *RunBindingSnapshot `json:"run_binding,omitempty"`
	// Non-authorizing progress metadata from the existing drain worker.
	CollectionProgress json.RawMessage `json:"collection_progress,omitempty"`
 // Optional for pre-policy snapshots; absence preserves the original digest.
 CollectionGuardPolicy *CollectionGuardPolicySnapshot `json:"collection_guard_policy,omitempty"`
}

// CollectionGuardPolicySnapshot pins advisory thresholds, not permissions.
type CollectionGuardPolicySnapshot struct {
 Version int `json:"version"`
 Thresholds cp.Policy `json:"thresholds"`
}
func NewCollectionGuardPolicySnapshot() *CollectionGuardPolicySnapshot {
 return &CollectionGuardPolicySnapshot{Version:cp.Version,Thresholds:cp.Defaults()}
}
// CollectionPolicy resolves pre-policy runs using frozen v1 defaults. Unknown
// or invalid explicit metadata disables the advisory guard, not native reads.
func (p *ContextSnapshotPolicy) CollectionPolicy()(cp.Policy,bool){
 if p==nil{return cp.Policy{},false}
 if p.CollectionGuardPolicy==nil{
  return cp.Policy{FirstReadAdvisory:5,SecondReadAdvisory:10,DuplicateStreak:3,DuplicateCooldown:5},true
 }
 v:=p.CollectionGuardPolicy
 return v.Thresholds,v.Version==cp.Version&&v.Thresholds.Valid()
}

type ContextSnapshot struct {
	ID               string                  `json:"id"`
	OwnerWorkspaceID string                  `json:"owner_workspace_id"`
	ConversationID   string                  `json:"conversation_id"`
	ProjectID        string                  `json:"project_id"`
	History          []agentchat.HistoryItem `json:"history"`
	SourceRefs       []string                `json:"source_refs,omitempty"`
	Policy           *ContextSnapshotPolicy  `json:"policy,omitempty"`
	PayloadSHA256    string                  `json:"payload_sha256"`
	CreatedAt        time.Time               `json:"created_at"`
}

type SnapshotStore struct{ dir string }

func NewSnapshotStore(dir string) (*SnapshotStore, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("absolute context snapshot directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("unsafe context snapshot directory")
	}
	return &SnapshotStore{dir: filepath.Clean(dir)}, nil
}

func (s *SnapshotStore) Create(ownerWorkspaceID, conversationID, projectID string, history []agentchat.HistoryItem) (ContextSnapshot, error) {
	return s.CreateWithPolicy(ownerWorkspaceID, conversationID, projectID, history, nil, nil)
}

func (s *SnapshotStore) CreateWithSources(ownerWorkspaceID, conversationID, projectID string, history []agentchat.HistoryItem, sourceRefs []string) (ContextSnapshot, error) {
	return s.CreateWithPolicy(ownerWorkspaceID, conversationID, projectID, history, sourceRefs, nil)
}

func (s *SnapshotStore) CreateWithPolicy(ownerWorkspaceID, conversationID, projectID string, history []agentchat.HistoryItem, sourceRefs []string, policy *ContextSnapshotPolicy) (ContextSnapshot, error) {
	if ownerWorkspaceID == "" || conversationID == "" || projectID == "" || len(history) == 0 || len(history) > maxSnapshotHistory || len(sourceRefs) > 32 {
		return ContextSnapshot{}, errors.New("context snapshot binding/history is invalid")
	}
	for _, item := range history {
		if !validSnapshotHistoryItem(item) {
			return ContextSnapshot{}, errors.New("invalid context snapshot history item")
		}
	}
	for _, source := range sourceRefs {
		if strings.TrimSpace(source) == "" || len(source) > 256 || strings.ContainsAny(source, "\x00\r\n") {
			return ContextSnapshot{}, errors.New("invalid context snapshot source reference")
		}
	}
	if err := validateSnapshotPolicy(policy, ownerWorkspaceID, conversationID); err != nil {
		return ContextSnapshot{}, err
	}
	payload, err := marshalSnapshotPayload(history, sourceRefs, policy)
	if err != nil || len(payload) > maxSnapshotBytes {
		return ContextSnapshot{}, errors.New("context snapshot exceeds size limit")
	}
	id, err := newID()
	if err != nil {
		return ContextSnapshot{}, err
	}
	digest := sha256.Sum256(payload)
	snapshot := ContextSnapshot{ID: id, OwnerWorkspaceID: ownerWorkspaceID, ConversationID: conversationID,
		ProjectID: projectID, History: append([]agentchat.HistoryItem(nil), history...), SourceRefs: append([]string(nil), sourceRefs...),
		Policy: cloneSnapshotPolicy(policy), PayloadSHA256: hex.EncodeToString(digest[:]), CreatedAt: time.Now().UTC()}
	body, err := json.Marshal(snapshot)
	if err != nil || len(body) > maxSnapshotBytes+4096 {
		return ContextSnapshot{}, errors.New("context snapshot record exceeds size limit")
	}
	path := s.file(id)
	tmp, err := os.CreateTemp(s.dir, ".snapshot-*.tmp")
	if err != nil {
		return ContextSnapshot{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
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
		return ContextSnapshot{}, err
	}
	if _, err := os.Lstat(path); err == nil {
		return ContextSnapshot{}, errors.New("context snapshot ID collision")
	} else if !os.IsNotExist(err) {
		return ContextSnapshot{}, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return ContextSnapshot{}, err
	}
	return snapshot, nil
}

func (s *SnapshotStore) Get(id, ownerWorkspaceID, conversationID, projectID string) (ContextSnapshot, error) {
	if len(id) != 32 || strings.Trim(id, "0123456789abcdef") != "" {
		return ContextSnapshot{}, os.ErrNotExist
	}
	path := s.file(id)
	info, err := os.Lstat(path)
	if err != nil {
		return ContextSnapshot{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() > maxSnapshotBytes+4096 {
		return ContextSnapshot{}, errors.New("unsafe context snapshot record")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ContextSnapshot{}, err
	}
	var snapshot ContextSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return ContextSnapshot{}, errors.New("invalid context snapshot record")
	}
	if snapshot.ID != id || snapshot.OwnerWorkspaceID != ownerWorkspaceID || snapshot.ConversationID != conversationID || snapshot.ProjectID != projectID {
		return ContextSnapshot{}, errors.New("context snapshot binding mismatch")
	}
	payload, err := marshalSnapshotPayload(snapshot.History, snapshot.SourceRefs, snapshot.Policy)
	if err != nil {
		return ContextSnapshot{}, err
	}
	digest := sha256.Sum256(payload)
	if hex.EncodeToString(digest[:]) != snapshot.PayloadSHA256 {
		return ContextSnapshot{}, errors.New("context snapshot integrity mismatch")
	}
	return snapshot, nil
}

type snapshotPayload struct {
	History []agentchat.HistoryItem `json:"history"`
	Sources []string                `json:"sources,omitempty"`
	Policy  *ContextSnapshotPolicy  `json:"policy,omitempty"`
}

func marshalSnapshotPayload(history []agentchat.HistoryItem, sources []string, policy *ContextSnapshotPolicy) ([]byte, error) {
	return json.Marshal(snapshotPayload{History: history, Sources: sources, Policy: policy})
}

func validateSnapshotPolicy(policy *ContextSnapshotPolicy, ownerWorkspaceID, conversationID string) error {
	if policy == nil {
		return nil
	}
	if len(policy.CollectionProgress)>96<<10 || (len(policy.CollectionProgress)>0&&!json.Valid(policy.CollectionProgress)){return errors.New("invalid bounded collection progress metadata")}
	if len(policy.SystemInstructions) > 512<<10 || strings.ContainsRune(policy.SystemInstructions, 0) {
		return errors.New("context snapshot instructions exceed limit or contain NUL")
	}
	switch policy.Automation.ContinuationMode {
	case "", "conversation", "plan":
	default:
		return errors.New("invalid run continuation mode")
	}
	if policy.Automation.ContinuationMode == "conversation" && !policy.Automation.AutoContinueText ||
		policy.Automation.ContinuationMode == "plan" && !policy.Automation.AutoContinuePlan {
		return errors.New("run continuation is missing pinned owner consent")
	}
	switch policy.RetryMode {
	case "", agentchat.RetryOff, agentchat.RetryLimited, agentchat.RetryUnlimited:
	default:
		return errors.New("invalid context snapshot retry policy")
	}
	switch policy.ReasoningEffort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return errors.New("invalid context snapshot reasoning effort")
	}
	if binding := policy.RunBinding; binding != nil {
		if binding.RunID == "" || len(binding.RunID) > 64 || binding.ConversationID != conversationID || binding.OwnerWorkspaceID != ownerWorkspaceID ||
			binding.ExecutionWorkspaceID == "" || binding.ProviderID == "" || binding.ModelID == "" || len(binding.ParentTurnID) > 256 ||
			!snapshotFingerprint(binding.ProviderConfigVersion) || !snapshotFingerprint(binding.CredentialFingerprint) ||
			binding.RetryMode != policy.RetryMode || len(binding.ExcludedPendingRunIDs) > 1300 ||
			strings.ContainsAny(binding.RunID+binding.ExecutionWorkspaceID+binding.ProviderID+binding.ModelID+binding.ParentTurnID+binding.ExcludedPendingRunIDs, "\x00\r\n") {
			return errors.New("invalid immutable run binding in context snapshot")
		}
	}
	return nil
}

func snapshotFingerprint(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}

func cloneSnapshotPolicy(policy *ContextSnapshotPolicy) *ContextSnapshotPolicy {
	if policy == nil {
		return nil
	}
	copy := *policy
	copy.CollectionProgress=append(json.RawMessage(nil),policy.CollectionProgress...)
 if policy.CollectionGuardPolicy!=nil{v:=*policy.CollectionGuardPolicy;copy.CollectionGuardPolicy=&v}
	if policy.RunBinding!=nil{binding:=*policy.RunBinding;copy.RunBinding=&binding}
	if policy.Automation.AutoCollection != nil {
		autoCollection := *policy.Automation.AutoCollection
		copy.Automation.AutoCollection = &autoCollection
	}
	if policy.Automation.AutoFeedbackUntil != nil {
		autoFeedbackUntil := *policy.Automation.AutoFeedbackUntil
		copy.Automation.AutoFeedbackUntil = &autoFeedbackUntil
	}
	return &copy
}

func (s *SnapshotStore) file(id string) string { return filepath.Join(s.dir, id+".json") }

func validSnapshotHistoryItem(item agentchat.HistoryItem) bool {
	if len(item.Content) > 512<<10 || len(item.Arguments) > 512<<10 || len(item.CallID) > 256 || len(item.Name) > 128 {
		return false
	}
	switch item.Type {
	case "user", "assistant":
		return item.Content != "" && item.CallID == "" && item.Name == "" && item.Arguments == ""
	case "function_call":
		return item.CallID != "" && item.Name != "" && item.Content == "" && json.Valid([]byte(item.Arguments))
	case "function_call_output":
		return item.CallID != "" && item.Content != "" && item.Name == "" && item.Arguments == ""
	default:
		return false
	}
}
