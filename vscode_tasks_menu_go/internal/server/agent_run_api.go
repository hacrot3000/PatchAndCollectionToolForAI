package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

const maxAgentRunRequestBytes = 1 << 20

type ownerBoundRunExecutor struct {
	owner string
	base  agentrun.Executor
}

func (e ownerBoundRunExecutor) Execute(ctx context.Context, spec agentchat.RunSpec, history []agentchat.HistoryItem, sink agentchat.EventSink) error {
	return e.base.Execute(agentrun.WithPrincipalOwner(ctx, e.owner), spec, history, sink)
}

func (e ownerBoundRunExecutor) Cancel(ctx context.Context, spec agentchat.RunSpec) error {
	return e.base.Cancel(agentrun.WithPrincipalOwner(ctx, e.owner), spec)
}

var safeAgentRunID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func (s *Server) agentRunJournalDir(owner string) (string, error) {
	ownerWorkspace := s.agentOwnerWorkspace()
	privatePath, err := aiPrivatePath(ownerWorkspace)
	if err != nil {
		return "", err
	}
	ownerHash := aiWorkspaceHash(ownerWorkspace) + "-" + aiOwnerHash(owner)
	return filepath.Join(filepath.Dir(privatePath), "agent-runs-"+ownerHash), nil
}

func (s *Server) agentRunManager(owner string) (*agentrun.Manager, error) {
	s.agentRunManagersMu.Lock()
	defer s.agentRunManagersMu.Unlock()
	if s.agentRunManagers == nil {
		s.agentRunManagers = map[string]*agentrun.Manager{}
	}
	if manager := s.agentRunManagers[owner]; manager != nil {
		return manager, nil
	}
	runDir, err := s.agentRunJournalDir(owner)
	if err != nil {
		return nil, err
	}
	executor := s.AgentRunExecutor
	if executor == nil {
		executor, err = s.NewAgentRunProviderExecutor(owner)
		if err != nil {
			return nil, err
		}
	} else {
		executor = ownerBoundRunExecutor{owner: owner, base: executor}
	}
	manager, err := agentrun.NewManager(runDir, agentrun.MaxConcurrentRuns, executor)
	if err != nil {
		return nil, err
	}
	s.agentRunManagers[owner] = manager
	return manager, nil
}

func aiOwnerHash(owner string) string {
	sum := sha256.Sum256([]byte(owner))
	return hex.EncodeToString(sum[:12])
}

func (s *Server) agentRunAPI(w http.ResponseWriter, r *http.Request) {
	owner := aiPlanOwner(r)
	if !s.requireSharedActionPermission(w, r, identity.PermissionSettingsRead, "agent.run", "agent_runs") {
		return
	}
	if r.Method == http.MethodPost {
		http.Error(w, "Agent Run creation is temporarily disabled.\nUse Legacy Agent Chat.", http.StatusConflict)
		return
	}
	manager, err := s.agentRunManager(owner)
	if err != nil {
		http.Error(w, "Agent Run Manager unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"runs": manager.List(), "max_concurrent": agentrun.MaxConcurrentRuns})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAgentRunRequestBytes)
	var request struct {
		Spec            agentchat.RunSpec `json:"spec"`
		ConfirmExternal bool              `json:"confirm_external"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		http.Error(w, "invalid Agent Run request", 400)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one Agent Run object required", 400)
		return
	}
	if !request.ConfirmExternal {
		http.Error(w, "external provider consent required", http.StatusForbidden)
		return
	}
	if err := request.Spec.Validate(); err != nil {
		http.Error(w, "invalid immutable run binding", 400)
		return
	}
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil || request.Spec.OwnerWorkspaceID != ownerWorkspaceID {
		http.Error(w, "run owner workspace mismatch", 403)
		return
	}
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "Agent conversation store unavailable", 500)
		return
	}
	session, err := store.Get(request.Spec.ConversationID, owner)
	if err != nil {
		http.Error(w, "conversation not found for owner workspace", 404)
		return
	}
	project, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil || project.ID != session.ProjectID {
		http.Error(w, "conversation project identity mismatch", 409)
		return
	}
	providerStore, err := s.agentChatProviders()
	if err != nil {
		http.Error(w, "owner provider registry unavailable", 503)
		return
	}
	provider, err := providerStore.Get(owner, request.Spec.ProviderID)
	if err != nil {
		http.Error(w, "provider is not configured in owner workspace", 400)
		return
	}
	model, ok := provider.FindModel(request.Spec.ModelID)
	if !ok {
		http.Error(w, "model is not configured for provider", 400)
		return
	}
	if agentchat.RunProviderConfigVersion(provider, model) != request.Spec.ProviderConfigVersion {
		http.Error(w, "provider configuration changed since run creation", 409)
		return
	}
	if request.Spec.ExecutionWorkspaceID != ownerWorkspaceID {
		registry, err := s.agentWorkspaceRegistryForOwner(owner)
		if err != nil {
			http.Error(w, "workspace registry unavailable", 500)
			return
		}
		binding, err := registry.Resolve(r.Context(), request.Spec.ExecutionWorkspaceID, request.Spec.ConversationID, request.Spec.RunID, project.ID)
		if err != nil || binding.ProviderConfigurationReference != request.Spec.ProviderID+"@"+request.Spec.ProviderConfigVersion {
			http.Error(w, "execution workspace is not authorized for this provider-bound run", 403)
			return
		}
	}
	snapshots, err := s.agentContextSnapshotStore(owner)
	if err != nil {
		http.Error(w, "context snapshot store unavailable", 500)
		return
	}
	snapshot, err := snapshots.Get(request.Spec.ContextSnapshotID, ownerWorkspaceID, request.Spec.ConversationID, session.ProjectID)
	if err != nil {
		http.Error(w, "context snapshot is missing, changed, or belongs to another conversation", 409)
		return
	}
	if request.Spec.CredentialFingerprint != "" || snapshot.Policy != nil && snapshot.Policy.RunBinding != nil {
		if err := verifyAgentRunSnapshotBinding(request.Spec, snapshot); err != nil {
			http.Error(w, "run binding does not match its immutable context snapshot", 409)
			return
		}
	}
	history := append([]agentchat.HistoryItem(nil), snapshot.History...)
	record, err := manager.Submit(request.Spec, history)
	if err != nil {
		http.Error(w, "Agent Run was not admitted: "+err.Error(), 409)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, map[string]any{"run": record, "execution_enabled": true})
}

func (s *Server) agentWorkspaceRegistryForOwner(owner string) (*agentrun.WorkspaceRegistry, error) {
	ownerWorkspace := s.agentOwnerWorkspace()
	privatePath, err := aiPrivatePath(ownerWorkspace)
	if err != nil {
		return nil, err
	}
	return agentrun.NewWorkspaceRegistry(filepath.Join(filepath.Dir(privatePath), "agent-workspaces-"+aiWorkspaceHash(ownerWorkspace)+"-"+aiOwnerHash(owner)+".json"), ownerWorkspace)
}

func (s *Server) agentRunItemAPI(w http.ResponseWriter, r *http.Request) {
	owner := aiPlanOwner(r)
	path := strings.TrimPrefix(r.URL.Path, "/api/agent/runs/")
	if path == "worktrees" {
		s.agentRunWorktreeAPI(w, r, owner)
		return
	}
	segments := strings.Split(path, "/")
	if len(segments) == 0 || !safeAgentRunID.MatchString(segments[0]) {
		http.Error(w, "invalid run ID", 400)
		return
	}
	if len(segments) == 4 && segments[1] == "tools" {
		if segments[2] == "" {
			http.Error(w, "invalid tool call ID", 400)
			return
		}
		switch segments[3] {
		case "decision":
			s.agentRunToolDecisionAPI(w, r, owner, segments[0], segments[2])
			return
		case "continue":
			s.agentRunToolContinueAPI(w, r, owner, segments[0])
			return
		}
	}
	if !s.requireSharedActionPermission(w, r, identity.PermissionSettingsRead, "agent.run.inspect", segments[0]) {
		return
	}
	manager, err := s.agentRunManager(owner)
	if err != nil {
		http.Error(w, "Agent Run Manager unavailable", 503)
		return
	}
	if len(segments) == 2 && segments[1] == "integration-report" && r.Method == http.MethodGet {
		record, ok := manager.Get(segments[0])
		if !ok {
			http.Error(w, "Agent Run not found", 404)
			return
		}
		registry, err := s.agentWorkspaceRegistryForOwner(owner)
		if err != nil {
			http.Error(w, "workspace registry unavailable", 500)
			return
		}
		report, err := registry.IntegrationReport(r.Context(), record.Spec)
		if err != nil {
			http.Error(w, "worktree integration report unavailable", 409)
			return
		}
		report.DiffPreview = scrubAIContext(report.DiffPreview, 64<<10)
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"report": report})
		return
	}
	if len(segments) == 1 && r.Method == http.MethodGet {
		record, ok := manager.Get(segments[0])
		if !ok {
			http.Error(w, "Agent Run not found", 404)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"run": record})
		return
	}
	if len(segments) != 2 || r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	switch segments[1] {
	case "stop":
		if err := manager.Stop(r.Context(), segments[0]); err != nil {
			http.Error(w, "cannot stop Agent Run", 409)
			return
		}
	case "retry", "resume":
		if !s.requireSharedActionPermission(w, r, identity.PermissionSettingsWrite, "agent.run.retry", segments[0]) {
			return
		}
		var record agentrun.Record
		if segments[1] == "retry" {
			record, err = manager.Retry(segments[0])
		} else {
			current, ok := manager.Get(segments[0])
			if !ok {
				http.Error(w, "Agent Run not found", 404)
				return
			}
			if current.Status == agentrun.Queued {
				record, err = manager.ResumeQueued(segments[0])
			} else {
				r.Body = http.MaxBytesReader(w, r.Body, 2048)
				var confirmation struct {
					ConfirmNoSideEffect bool `json:"confirm_no_side_effect"`
				}
				dec := json.NewDecoder(r.Body)
				dec.DisallowUnknownFields()
				if dec.Decode(&confirmation) != nil {
					http.Error(w, "explicit resume confirmation required", 400)
					return
				}
				var extra any
				if !errors.Is(dec.Decode(&extra), io.EOF) {
					http.Error(w, "exactly one resume object required", 400)
					return
				}
				record, err = manager.ResumeAfterReview(segments[0], confirmation.ConfirmNoSideEffect)
			}
		}
		if err != nil {
			http.Error(w, "run cannot be retried/resumed safely", 409)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusAccepted, map[string]any{"run": record})
		return
	default:
		http.Error(w, "unknown Agent Run action", 404)
		return
	}
	if record, ok := manager.Get(segments[0]); ok {
		writeJSON(w, 200, map[string]any{"run": record})
		return
	}
	http.Error(w, "Agent Run not found", 404)
}

func (s *Server) agentRunWorktreeAPI(w http.ResponseWriter, r *http.Request, owner string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.requireSharedActionPermission(w, r, identity.PermissionSettingsRead, "agent.run.worktree.create", "agent_worktrees") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var request struct {
		ConversationID        string   `json:"conversation_id"`
		RunID                 string   `json:"run_id"`
		ProviderID            string   `json:"provider_id"`
		ModelID               string   `json:"model_id"`
		ProviderConfigVersion string   `json:"provider_config_version"`
		PermissionScope       []string `json:"permission_scope"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		http.Error(w, "invalid worktree request", 400)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one worktree object required", 400)
		return
	}
	if !safeAgentRunID.MatchString(request.RunID) || request.ConversationID == "" || request.ProviderID == "" || request.ModelID == "" || request.ProviderConfigVersion == "" || len(request.PermissionScope) == 0 || len(request.PermissionScope) > 4 {
		http.Error(w, "worktree binding is incomplete", 400)
		return
	}
	allowedScope := map[string]bool{"files.read": true, "patch.propose": true, "patch.execute.approval_required": true}
	seenScope := map[string]bool{}
	for _, permission := range request.PermissionScope {
		if !allowedScope[permission] || seenScope[permission] {
			http.Error(w, "worktree permission scope is unsupported", 400)
			return
		}
		seenScope[permission] = true
	}
	for _, required := range []string{"files.read", "patch.propose", "patch.execute.approval_required"} {
		if !seenScope[required] {
			http.Error(w, "worktree permission scope omits a required run permission", 400)
			return
		}
	}
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "Agent conversation store unavailable", 500)
		return
	}
	session, err := store.Get(request.ConversationID, owner)
	if err != nil {
		http.Error(w, "conversation not found for owner", 404)
		return
	}
	providers, err := s.agentChatProviders()
	if err != nil {
		http.Error(w, "owner provider registry unavailable", 503)
		return
	}
	provider, err := providers.Get(owner, request.ProviderID)
	if err != nil {
		http.Error(w, "provider is not configured in owner workspace", 400)
		return
	}
	model, ok := provider.FindModel(request.ModelID)
	if !ok || agentchat.RunProviderConfigVersion(provider, model) != request.ProviderConfigVersion {
		http.Error(w, "provider model/config version is unavailable", 409)
		return
	}
	ownerWorkspace := s.agentOwnerWorkspace()
	privatePath, err := aiPrivatePath(ownerWorkspace)
	if err != nil {
		http.Error(w, "private state directory unavailable", 500)
		return
	}
	worktreeRoot := filepath.Join(filepath.Dir(privatePath), "agent-worktrees-"+aiWorkspaceHash(ownerWorkspace)+"-"+aiOwnerHash(owner))
	if err := os.MkdirAll(worktreeRoot, 0700); err != nil {
		http.Error(w, "worktree storage unavailable", 500)
		return
	}
	registry, err := s.agentWorkspaceRegistryForOwner(owner)
	if err != nil {
		http.Error(w, "workspace registry unavailable", 500)
		return
	}
	project, err := patchtool.ReadProjectIdentity(ownerWorkspace)
	if err != nil || project.ID != session.ProjectID {
		http.Error(w, "owner project identity unavailable", 409)
		return
	}
	target := filepath.Join(worktreeRoot, request.RunID)
	binding, err := registry.CreateDetached(r.Context(), target, project.ID, request.ConversationID, request.RunID, request.ProviderID+"@"+request.ProviderConfigVersion, request.PermissionScope)
	if err != nil {
		http.Error(w, "cannot create authorized detached worktree", 409)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"workspace_id": binding.ExecutionWorkspaceID, "run_id": binding.RunID, "base_head": binding.BaseHEAD, "lifecycle_status": binding.LifecycleStatus})
}
