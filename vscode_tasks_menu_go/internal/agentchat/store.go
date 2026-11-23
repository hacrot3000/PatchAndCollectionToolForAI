package agentchat

import (
	"crypto/rand"
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

const maxSessionBytes = 4 << 20

const (
	maxSessionEvents = 128
	maxEventDetail   = 120
)

// SessionEvent is one durable transition marker. Detail is a short machine
// code (never prompt text, tool arguments or provider output).
type SessionEvent struct {
	Seq    uint64    `json:"seq"`
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Status string    `json:"status,omitempty"`
	CallID string    `json:"call_id,omitempty"`
	Detail string    `json:"detail,omitempty"`
	HistoryIndex int `json:"history_index,omitempty"` // insertion point for durable activity in conversation transcript
}

// AppendEvent records a bounded transition. The journal is trimmed
// oldest-first so it cannot grow without limit.
func (x *Session) AppendEvent(kind, status, callID, detail string) {
	if kind == "" || len(kind) > 32 {
		return
	}
	if len(detail) > maxEventDetail {
		detail = detail[:maxEventDetail]
	}
	x.EventSeq++
	x.Events = append(x.Events, SessionEvent{
		Seq: x.EventSeq, At: time.Now().UTC(), Kind: kind,
		Status: status, CallID: callID, Detail: detail,
		HistoryIndex: len(x.History),
	})
	if len(x.Events) > maxSessionEvents {
		x.Events = append([]SessionEvent(nil), x.Events[len(x.Events)-maxSessionEvents:]...)
	}
}

// SessionSchemaVersion identifies the persisted session layout. Records
// written before this field existed decode as version 0 and are accepted as
// local sessions; mutating them upgrades the version explicitly.
const SessionSchemaVersion = 1

var (
	ErrNotFound       = errors.New("agent session not found")
	ErrStaleRevision  = errors.New("agent session revision mismatch")
	ErrInvalidSession = errors.New("invalid agent session")
 // A canonical journal name was atomically renamed but directory fsync
 // failed. It may already be committed. NEVER interpret this as proof the
 // transaction did not occur, or as permission to retry external egress.
 ErrJournalDurabilityUnknown = errors.New("agent journal commit durability is unconfirmed")
	// All Store instances in this daemon serialize journal mutation.
	// Multi-process writers must use a shared database/lock before deployment.
	agentJournalMu sync.Mutex
)

// ToolCallRecord is a durable, owner-bound execution intent. An absent or
// ambiguous run is never retried automatically. A call is identified by its
// provider call_id and the canonical transcript contains its exact arguments.
//
// The approval snapshot (arguments digest, Git HEAD/branch, worktree status
// digest and expiry) is authoritative server-side state. A model must never
// be able to supply or refresh it. PromptID/SelectedIndex/RunID bind the
// final native result to the exact queue item this call approved.
type ToolCallRecord struct {
	CallID                string     `json:"call_id"`
	ModelTurnID           string     `json:"model_turn_id,omitempty"`
	Name                  string     `json:"name"`
	TextFallback          bool       `json:"text_fallback,omitempty"` // locally synthesized review item, not a provider tool call
	Status                string     `json:"status"`
	ArgumentsSHA256       string     `json:"arguments_sha256,omitempty"`
	QueuePath             string     `json:"queue_path,omitempty"`
	PackageSHA256         string     `json:"package_sha256,omitempty"`
	PackageSize           int64      `json:"package_size,omitempty"`
	PatchSessionID        string     `json:"patch_session_id,omitempty"`
	NativeStartedAt       *time.Time `json:"native_started_at,omitempty"`
	AutoRunOnce           bool       `json:"auto_run_once,omitempty"`
	SendAfterRetry        bool       `json:"send_after_retry,omitempty"`
	PromptID              string     `json:"prompt_id,omitempty"`
	SelectedIndex         int        `json:"selected_index,omitempty"`
	RunID                 string     `json:"run_id,omitempty"`
	ExpectedHead          string     `json:"expected_head,omitempty"`
	ExpectedBranch        string     `json:"expected_branch,omitempty"`
	WorkspaceStatusSHA256 string     `json:"workspace_status_sha256,omitempty"`
	GitIsRepo             bool       `json:"git_is_repo,omitempty"`
	ApprovalExpiresAt     *time.Time `json:"approval_expires_at,omitempty"`
	Result                string     `json:"result,omitempty"`
	// FeedbackSent records that an outgoing tool-result attempt was persisted
	// before egress. It is at-most-once intent, NOT proof the provider stored the
	// result (see FeedbackStatus).
	FeedbackSent bool `json:"feedback_sent,omitempty"`
	// FeedbackStatus distinguishes delivery evidence:
	// "" | "prepared" | "sending" | "acknowledged" | "failed" | "unknown".
	FeedbackStatus string `json:"feedback_status,omitempty"`
	// Previous manual executions are preserved when a user explicitly chooses
	// a fresh Patch retry. Never discard the old native run/receipt evidence.
	Attempts []ToolAttemptRecord `json:"attempts,omitempty"`
	ManualRetryOnly bool `json:"manual_retry_only,omitempty"`
}

// ToolAttemptRecord retains earlier execution attempts for audit. This is
// separate from the active, exact SHA-256 native Patch receipt.
type ToolAttemptRecord struct {
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
	QueuePath string `json:"queue_path,omitempty"`
	PackageSHA256 string `json:"package_sha256,omitempty"`
	PackageSize int64 `json:"package_size,omitempty"`
	PatchSessionID string `json:"patch_session_id,omitempty"`
	NativeStartedAt *time.Time `json:"native_started_at,omitempty"`
	PromptID string `json:"prompt_id,omitempty"`
	SelectedIndex int `json:"selected_index,omitempty"`
	RunID string `json:"run_id,omitempty"`
}

// RetryMode is an explicit user preference: off, limited (five retries),
// or unlimited (until Stop/cancellation). No retry is implicit.
type RetryMode string

const (
	RetryOff RetryMode = "off"
	RetryLimited RetryMode = "limited"
	RetryUnlimited RetryMode = "unlimited"
)

// Session is persisted locally even when the provider offers server-side
// context. It never contains API keys. All mutations require owner/identity
// matching and revision compare-and-swap.
type Session struct {
	ID                 string      `json:"id"`
	Owner              string      `json:"owner"`
	Workspace          string      `json:"workspace"`
	ProjectID          string      `json:"project_id"`
	ProviderID         string      `json:"provider_id"`
	ModelID            string      `json:"model_id"`
	Mode               ContextMode `json:"mode"`
	AIPackFingerprint  string      `json:"ai_pack_fingerprint,omitempty"`
	PreviousResponseID string      `json:"previous_response_id,omitempty"`
	ConversationID     string      `json:"conversation_id,omitempty"`
	// ReasoningEffort is the per-session selection. Empty means "use the model
	// default". It must be one of the model's advertised efforts.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	RetryMode RetryMode `json:"retry_mode,omitempty"`
	Title           string `json:"title,omitempty"`
	Archived        bool   `json:"archived,omitempty"`
	Target          string `json:"target,omitempty"` // "" or "local"; remote targets are not enabled
	SchemaVersion   int    `json:"schema_version,omitempty"`
	// Durable idempotency for the last model turn: a retried browser request
	// with the same key replays the recorded outcome instead of contacting the
	// provider (or executing a tool) twice.
	LastRequestKey      string `json:"last_request_key,omitempty"`
	LastRequestSHA      string `json:"last_request_sha,omitempty"`
	LastRequestCode     int    `json:"last_request_code,omitempty"`
	LastRequestResponse string `json:"last_request_response,omitempty"`
	// AutoFeedback is an explicit per-session opt-in to deliver each verified
	// native result to the provider without a click, valid until
	// AutoFeedbackUntil. It never implies tool execution or patch approval.
	// Events is a bounded, append-only journal used to resync a reconnected
	// browser. It records state transitions, not prompt or result content.
	Events            []SessionEvent   `json:"events,omitempty"`
	EventSeq          uint64           `json:"event_seq,omitempty"`
	AutoFeedback      bool             `json:"auto_feedback,omitempty"`
	// Long-lived per-conversation provider-sharing opt-in via Automation.
	// Legacy 30-minute grants still use AutoFeedbackUntil.
	AutoFeedbackPersistent bool        `json:"auto_feedback_persistent,omitempty"`
	AutoPatch         bool             `json:"auto_patch,omitempty"`
	// Nil preserves automatic read-only Collection for existing sessions.
	AutoCollection    *bool            `json:"auto_collection,omitempty"`
	AutoFeedbackUntil *time.Time       `json:"auto_feedback_until,omitempty"`
	Status            string           `json:"status"`
	ProviderRun       *ProviderRun     `json:"provider_run,omitempty"`
	TaskPlan          *TaskPlan        `json:"task_plan,omitempty"`
 TaskPlanSegments []TaskPlanSegmentRef `json:"task_plan_segments,omitempty"`
	History           []HistoryItem    `json:"history"`
	HistoryArchiveParts uint64 `json:"history_archive_parts,omitempty"`
	ArchivedHistoryItems uint64 `json:"archived_history_items,omitempty"`
	ToolCalls         []ToolCallRecord `json:"tool_calls,omitempty"`
 PatchArchiveParts uint64 `json:"patch_archive_parts,omitempty"`
 ArchivedPatchCalls uint64 `json:"archived_patch_calls,omitempty"`
 PatchArchiveSummaries []string `json:"patch_archive_summaries,omitempty"`
	Revision          uint64           `json:"revision"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

func NewSession(owner, workspace, projectID, providerID, modelID string, mode ContextMode) (Session, error) {
	if owner == "" || len(owner) > 256 || workspace == "" || len(workspace) > 4096 || projectID == "" || len(projectID) > 128 || providerID == "" || len(providerID) > 128 || modelID == "" || len(modelID) > 128 {
		return Session{}, ErrInvalidSession
	}
	if mode != Stateless && mode != PreviousResponse && mode != Conversation {
		return Session{}, ErrInvalidSession
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	return Session{
		ID: hex.EncodeToString(bytes[:]), Owner: owner, Workspace: filepath.Clean(workspace),
		ProjectID: projectID, ProviderID: providerID, ModelID: modelID, Mode: mode,
		Target: "local", SchemaVersion: SessionSchemaVersion,
		Status: "created", History: []HistoryItem{}, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Store keeps canonical session history outside the project source tree.
type Store struct {
	dir       string
	scoped    bool
	workspace string
	projectID string
	// Test-only fault injection. Production stores always leave this nil.
	// It runs after an immutable-page status edit commits the authoritative
	// journal but before post-commit cleanup, while withLock is still held.
	afterSegmentStatusJournalCommit func()
 // Test-only failure injection for the directory-fsync boundaries. Clients
 // and provider requests never select the sync implementation.
 syncDirectoryOverride func()error
}

func NewStore(dir string) (*Store, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("absolute private state directory required")
	}
	return &Store{dir: filepath.Clean(dir)}, nil
}

// NewProjectStore restricts reads and writes to one authoritative workspace and project.
// An empty project ID represents an as-yet uninitialised project and matches no sessions.
func NewProjectStore(dir, workspace, projectID string) (*Store, error) {
	if workspace == "" || !filepath.IsAbs(workspace) {
		return nil, errors.New("absolute workspace required")
	}
	store, err := NewStore(dir)
	if err != nil {
		return nil, err
	}
	store.scoped = true
	store.workspace = filepath.Clean(workspace)
	store.projectID = projectID
	return store, nil
}

func (s *Store) matchesProject(x Session) bool {
	return !s.scoped || (s.projectID != "" && x.ProjectID == s.projectID && filepath.Clean(x.Workspace) == s.workspace)
}
func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func (s *Store) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(s.dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("unsafe agent session storage directory")
	}
	return nil
}
func (s *Store) file(id string) string { return filepath.Join(s.dir, id+".json") }

// syncDirectory is needed for POSIX persistence of atomic rename directory
// entries. It is distinct from the fsync of file contents and MUST run
// before the first network byte whenever the journal marks SENDING.
func (s *Store) syncDirectory()error{
 if s.syncDirectoryOverride!=nil{return s.syncDirectoryOverride()}
 return syncAgentDirectory(s.dir)
}

// withLock serializes journal mutation both inside this daemon (Go mutex) and
// across TaskDeck processes sharing the same private state directory (OS
// advisory lock). The process mutex is always taken first to keep a single
// lock order and avoid self-deadlock.
func (s *Store) withLock(fn func() error) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	agentJournalMu.Lock()
	defer agentJournalMu.Unlock()
	unlock, err := lockStoreDir(s.dir)
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}
func validateSession(x Session) error {
	if !validProviderRun(x.ProviderRun) {return ErrInvalidSession}
	if err:=ValidateTaskPlan(x.TaskPlan);err!=nil{return err}
	if x.RetryMode != "" && x.RetryMode != RetryOff &&
		x.RetryMode != RetryLimited && x.RetryMode != RetryUnlimited {
		return ErrInvalidSession
	}
	for _, call := range x.ToolCalls {
		if len(call.Attempts) > 8 { return ErrInvalidSession }
	}

	if !validID(x.ID) || x.Owner == "" || len(x.Owner) > 256 || x.Workspace == "" || x.ProjectID == "" || x.ProviderID == "" || x.ModelID == "" {
		return ErrInvalidSession
	}
	if x.Mode != Stateless && x.Mode != PreviousResponse && x.Mode != Conversation {
		return ErrInvalidSession
	}
	if x.Status == "" || len(x.Status) > 64 || len(x.History) > maxItems {
		return ErrInvalidSession
	}
	// Canonical replay integrity: no orphan tool output, no duplicate call ID.
	seenCallIDs := make(map[string]bool, len(x.History))
	completed := make(map[string]bool, len(x.History))
	for _, item := range x.History {
		if err := validHistoryItem(item); err != nil {
			return err
		}
		switch item.Type {
		case "function_call":
			if seenCallIDs[item.CallID] {
				return ErrInvalidSession
			}
			seenCallIDs[item.CallID] = true
		case "function_call_output":
			if !seenCallIDs[item.CallID] || completed[item.CallID] {
				return ErrInvalidSession
			}
			completed[item.CallID] = true
		}
	}
	if len(x.ToolCalls) > 32 {return ErrInvalidSession}
 if len(x.PatchArchiveSummaries)>24 {return ErrInvalidSession}
 for _,summary:=range x.PatchArchiveSummaries {if len(summary)>220{return ErrInvalidSession}}
	seenCalls := make(map[string]bool, len(x.ToolCalls))
	for _, call := range x.ToolCalls {
		if call.CallID == "" || len(call.CallID) > 256 || call.Name != "taskdeck_submit_patch" || seenCalls[call.CallID] || len(call.QueuePath) > 512 || len(call.PackageSHA256) > 64 || call.PackageSize < 0 || call.PackageSize > 2<<20 || len(call.PatchSessionID) > 256 || len(call.Result) > 4096 {
			return ErrInvalidSession
		}
		seenCalls[call.CallID] = true
		switch call.Status {
		case "pending", "rejected", "queuing", "queued", "dispatching", "running", "done", "error", "unknown":
		default:
			return ErrInvalidSession
		}
		if call.Status == "queued" && call.QueuePath == "" {
			return ErrInvalidSession
		}
		if len(call.ModelTurnID) > 256 || len(call.ArgumentsSHA256) > 64 || len(call.ExpectedHead) > 64 ||
			len(call.ExpectedBranch) > 256 || len(call.WorkspaceStatusSHA256) > 64 || len(call.PromptID) > 256 ||
			len(call.RunID) > 256 || call.SelectedIndex < 0 || call.SelectedIndex > maxItems {
			return ErrInvalidSession
		}
		if call.PackageSHA256 != "" && len(call.PackageSHA256) != 64 {
			return ErrInvalidSession
		}
		if call.PackageSHA256 != "" && call.PackageSize == 0 {
			return ErrInvalidSession
		}
		if call.ArgumentsSHA256 != "" && len(call.ArgumentsSHA256) != 64 {
			return ErrInvalidSession
		}
		if call.FeedbackSent && call.Result == "" {
			return ErrInvalidSession
		}
		switch call.FeedbackStatus {
		case "", "prepared", "sending", "acknowledged", "failed", "unknown":
		default:
			return ErrInvalidSession
		}
		if call.FeedbackStatus == "acknowledged" && !call.FeedbackSent {
			return ErrInvalidSession
		}
		found := false
		for _, item := range x.History {
			if item.Type == "function_call" && item.CallID == call.CallID && item.Name == call.Name {
				found = true
				break
			}
		}
		if !found {
			return ErrInvalidSession
		}
	}
	if len(x.PreviousResponseID) > 256 || len(x.ConversationID) > 256 || len(x.AIPackFingerprint) > 128 {
		return ErrInvalidSession
	}
	if len(x.Title) > 120 {
		return ErrInvalidSession
	}
	if len(x.Events) > maxSessionEvents {
		return ErrInvalidSession
	}
	var lastSeq uint64
	for _, ev := range x.Events {
		if ev.Seq <= lastSeq || len(ev.Kind) == 0 || len(ev.Kind) > 32 || len(ev.Detail) > maxEventDetail {
			return ErrInvalidSession
		}
		lastSeq = ev.Seq
	}
	if x.EventSeq < lastSeq {
		return ErrInvalidSession
	}
	if len(x.LastRequestKey) > 128 || len(x.LastRequestResponse) > 32<<10 {
		return ErrInvalidSession
	}
	if x.LastRequestSHA != "" && len(x.LastRequestSHA) != 64 {
		return ErrInvalidSession
	}
	if x.LastRequestKey == "" && (x.LastRequestSHA != "" || x.LastRequestResponse != "" || x.LastRequestCode != 0) {
		return ErrInvalidSession
	}
	if x.LastRequestCode != 0 && (x.LastRequestCode < 200 || x.LastRequestCode > 599) {
		return ErrInvalidSession
	}
	if len(x.ReasoningEffort) > 16 {
		return ErrInvalidSession
	}
	switch x.ReasoningEffort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return ErrInvalidSession
	}
	if x.Target != "" && x.Target != "local" {
		return ErrInvalidSession
	}
	if x.SchemaVersion < 0 || x.SchemaVersion > SessionSchemaVersion {
		return ErrInvalidSession
	}
	return nil
}
func (s *Store) readSessionManifestLocked(id string) (Session, error) {
	if !validID(id) {
		return Session{}, ErrNotFound
	}
	info, err := os.Lstat(s.file(id))
	if os.IsNotExist(err) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > maxSessionBytes {
		return Session{}, errors.New("unsafe agent session file")
	}
	raw, err := os.ReadFile(s.file(id))
	if err != nil {
		return Session{}, err
	}
	var x Session
	if err = json.Unmarshal(raw, &x); err != nil {
		return Session{}, err
	}
	if x.ID != id {
		return Session{}, ErrInvalidSession
	}
	// Pre-versioned records are local-only; do not invent a remote or
	// differently-scoped target for them.
	if x.SchemaVersion == 0 {
		x.SchemaVersion = SessionSchemaVersion
		if x.Target == "" {
			x.Target = "local"
		}
	}
	return x, nil
}
func (s *Store) readLocked(id string) (Session, error) {
 x,err:=s.readSessionManifestLocked(id)
 if err!=nil{return Session{},err}
 if err=s.loadTaskPlanSegments(&x);err!=nil{return Session{},err}
 if err=validateSession(x);err!=nil{return Session{},err}
 return x,nil
}

func (s *Store) Get(id, owner string) (Session, error) {
	var out Session
	err := s.withLock(func() error {
		x, e := s.readLocked(id)
		if e != nil {
			return e
		}
		if x.Owner != owner || !s.matchesProject(x) {
			return ErrNotFound
		}
		out = x
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return out, nil
}

// Save creates a new session when expectedRevision == 0; otherwise it requires
// an exact match to prevent stale model responses overwriting another turn.
func (s *Store) Save(x Session, expectedRevision uint64) (Session, error) {
	var saved Session
	err := s.withLock(func() error {
		// A long-running Collection session can temporarily exceed the active
		// history budget. Archive old completed reads before validating it.
		// Other fields are still validated after the archival transformation.
		if len(x.History) <= maxItems && len(x.ToolCalls) <= 32 {
			if err := validateSession(x); err != nil {return err}
		}
		if !s.matchesProject(x) {
			return ErrInvalidSession
		}
		existing, e := s.readLocked(x.ID)
		if errors.Is(e, ErrNotFound) {
			if expectedRevision != 0 {
				return ErrStaleRevision
			}
		} else if e != nil {
			return e
		} else {
			if existing.Revision != expectedRevision {
				return ErrStaleRevision
			}
			if existing.Owner != x.Owner || existing.Workspace != x.Workspace || existing.ProjectID != x.ProjectID || existing.ProviderID != x.ProviderID || existing.ModelID != x.ModelID || existing.Mode != x.Mode || !existing.CreatedAt.Equal(x.CreatedAt) ||
				x.HistoryArchiveParts!=existing.HistoryArchiveParts || x.ArchivedHistoryItems!=existing.ArchivedHistoryItems ||
            x.PatchArchiveParts!=existing.PatchArchiveParts || x.ArchivedPatchCalls!=existing.ArchivedPatchCalls {
				return ErrInvalidSession
			}
		}
		if err:=s.archivePatchHistory(&x);err!=nil{return err}
        if err:=s.archiveCollectionHistory(&x);err!=nil{return err}
		if err:=validateSession(x);err!=nil{return err}
		// Immutable plan parts must be durable BEFORE the referencing session
		// journal is atomically renamed. CAS and owner checks are complete.
		var old *Session
		if e==nil{old=&existing}
		if err:=s.stageTaskPlanSegments(&x,old,expectedRevision+1);err!=nil{return err}
		x.Revision = expectedRevision + 1
		x.UpdatedAt = time.Now().UTC()
		// Persist the compact manifest, never duplicate segment-backed steps.
		onDisk:=x
		if len(x.TaskPlanSegments)>0{
			planMeta:=*x.TaskPlan
			planMeta.Steps=nil
			onDisk.TaskPlan=&planMeta
		}
		raw, e := json.Marshal(onDisk)
		if e != nil {
			return e
		}
		if len(raw) > maxSessionBytes {
			return errors.New("agent session exceeds size limit")
		}
		temp, e := os.CreateTemp(s.dir, ".agent-session-*")
		if e != nil {
			return e
		}
		defer os.Remove(temp.Name())
		if e = temp.Chmod(0600); e != nil {
			temp.Close()
			return e
		}
		if _, e = temp.Write(raw); e != nil {
			temp.Close()
			return e
		}
		if e = temp.Sync(); e != nil {
			temp.Close()
			return e
		}
		if e = temp.Close(); e != nil {
			return e
		}
		// Archive / immutable page renames are staged first. Flush their
        // directory entries before the canonical journal can reference them.
        // A crash cannot leave a committed manifest referencing missing
        // pages solely because the directory rename was not yet durable.
        if e=s.syncDirectory();e!=nil{return e}
		if e = os.Rename(temp.Name(), s.file(x.ID)); e != nil {
			return e
		}
        // The journal's canonical name is the commit point. A failed fsync
        // AFTER rename is ambiguous; the caller must re-read authoritative
        // state and MUST NOT infer an unsent provider request.
        if e=s.syncDirectory();e!=nil{
         return fmt.Errorf("%w: %v",ErrJournalDurabilityUnknown,e)
        }
		// Do not fail or accidentally repeat a provider/native operation if
		// this derived handoff write fails after the canonical commit.
		// Handoff() can regenerate it from the journal on demand.
		_ = s.writeHandoffLocked(x)
		// Remove only old parts no longer referenced by the committed
		// journal. A crash may leave unreferenced new parts, never lost data.
		if e==nil{s.cleanupTaskPlanSegments(existing.TaskPlanSegments,x.TaskPlanSegments,x.ID)}
		saved = x
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return saved, nil
}
// HasUnresolvedWork identifies native work that would lose its ownership
// journal if deleted. Finished Patch calls may still have feedback pending:
// deleting that completed work is allowed, without re-running or delivering.
func HasUnresolvedWork(x Session) bool {
	if x.Status == "requesting_ai" || x.Status == "tool_queuing" {
		return true
	}
	for _, call := range x.ToolCalls {
		switch call.Status {
		case "pending", "queuing", "queued", "dispatching", "running", "unknown":
			return true
		}
	}
	return false
}

// Delete is owner/project scoped and revision-fenced. Never delete a journal
// with an unresolved native Patch or live provider turn: those outcomes must
// remain recoverable until they reach a verified final state.
func (s *Store) Delete(id, owner string, expectedRevision uint64) error {
	return s.withLock(func() error {
		x, err := s.readLocked(id)
		if err != nil { return err }
		if x.Owner != owner || !s.matchesProject(x) { return ErrNotFound }
		if x.Revision != expectedRevision { return ErrStaleRevision }
		if HasUnresolvedWork(x) {
			return errors.New("conversation contains an active or unresolved Patch operation")
		}
		if err:=os.Remove(s.file(id));err!=nil{return err}
		if err:=os.Remove(s.handoffFile(id));err!=nil&&!os.IsNotExist(err){return err}
		if err:=os.Remove(s.providerRunFile(id));err!=nil&&!os.IsNotExist(err){return err}
		if err:=os.Remove(s.providerHeartbeatFile(id));err!=nil&&!os.IsNotExist(err){return err}
  for _,ref:=range x.TaskPlanSegments{
   if err:=os.Remove(s.taskPlanSegmentPath(id,ref));err!=nil&&!os.IsNotExist(err){return err}
  }
		for _,part:=range s.historyArchiveSequences(x){
   if err:=os.Remove(s.historyArchivePath(id,part));err!=nil&&!os.IsNotExist(err){return err}
  }
  for part:=uint64(1);part<=x.PatchArchiveParts;part++{
   if err:=os.Remove(s.patchArchivePath(id,part));err!=nil&&!os.IsNotExist(err){return err}
  }
		return nil
	})
}

func (s *Store) List(owner string) ([]Session, error) {
	var result []Session
	err := s.withLock(func() error {
		entries, e := os.ReadDir(s.dir)
		if e != nil {
			return e
		}
		// Archive segments have names like <session>.history.<N>.json.
		// Multi-day Collection sessions can legitimately produce thousands of
		// those files. Count actual session journals, not unrelated private
		// history segments, against the session-list admission limit.
		out := make([]Session, 0)
		sessionFiles := 0
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			id := strings.TrimSuffix(name, ".json")
			if !validID(id) || !entry.Type().IsRegular() {
				continue
			}
			sessionFiles++
			if sessionFiles > 1024 {
				return fmt.Errorf("too many agent session journals")
			}
			x, e := s.readLocked(id)
			if e != nil {
				return e
			}
			if x.Owner == owner && s.matchesProject(x) {
				out = append(out, x)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
		result = out
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
