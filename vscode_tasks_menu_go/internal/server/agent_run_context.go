package server

import (
	"context"
	"errors"
	"os"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// AgentRunInputs is a backend-only view for context builders and executors.
// Workspace paths and snapshot contents must never be returned to the browser
// as a substitute for the opaque IDs in RunSpec.
func (s *Server) agentOwnerWorkspace() string {
	if s != nil && s.OwnerWorkspace != "" {
		return s.OwnerWorkspace
	}
	if s == nil {
		return ""
	}
	return s.Workspace
}

func (s *Server) agentExecutionServer(executionWorkspace string) *Server {
	if s == nil {
		return nil
	}
	return &Server{
		Workspace: executionWorkspace, OwnerWorkspace: s.agentOwnerWorkspace(), Config: s.Config, Log: s.Log,
		Sessions: s.Sessions, Identity: s.Identity, InternalControlToken: s.InternalControlToken,
		ConnectionSecrets: s.ConnectionSecrets, ConnectionAudit: s.ConnectionAudit,
		AgentChatPack: s.AgentChatPack, GitRebaseEditorCommand: s.GitRebaseEditorCommand,
	}
}

type AgentRunInputs struct {
	OwnerWorkspacePath     string
	ExecutionWorkspacePath string
	ProjectID              string
	Binding                agentrun.WorkspaceBinding
	Snapshot               agentrun.ContextSnapshot
}

// ResolveAgentRunInputs revalidates the immutable run binding immediately
// before provider execution. Owner-scoped configuration uses the principal in
// ctx; files and tool operations use the verified execution worktree.
func (s *Server) ResolveAgentRunInputs(ctx context.Context, spec agentchat.RunSpec) (AgentRunInputs, error) {
	var empty AgentRunInputs
	owner, ok := agentrun.PrincipalOwner(ctx)
	if !ok || spec.Validate() != nil {
		return empty, errors.New("authenticated run owner and valid run spec required")
	}
	ownerPath, err := agentrun.WorkspacePath(s.Workspace)
	if err != nil {
		return empty, err
	}
	ownerID, err := agentrun.WorkspaceID(ownerPath)
	if err != nil || ownerID != spec.OwnerWorkspaceID {
		return empty, errors.New("run owner workspace mismatch")
	}
	store, err := s.agentChatStore()
	if err != nil {
		return empty, err
	}
	session, err := store.Get(spec.ConversationID, owner)
	if err != nil {
		return empty, errors.New("run conversation is not owned by the authenticated principal")
	}
	identity, err := patchtool.ReadProjectIdentity(ownerPath)
	if err != nil || identity.ID != session.ProjectID {
		return empty, errors.New("owner project identity changed")
	}
	registry, err := s.agentWorkspaceRegistryForOwner(owner)
	if err != nil {
		return empty, err
	}
	binding, err := registry.ResolveRun(ctx, spec)
	if err != nil {
		return empty, err
	}
	if binding.ProviderConfigurationReference != spec.ProviderID+"@"+spec.ProviderConfigVersion {
		return empty, errors.New("provider config binding mismatch")
	}
	snapshots, err := s.agentContextSnapshotStore(owner)
	if err != nil {
		return empty, err
	}
	snapshot, err := snapshots.Get(spec.ContextSnapshotID, spec.OwnerWorkspaceID, spec.ConversationID, session.ProjectID)
	if err != nil {
		return empty, errors.New("run context snapshot is unavailable or has drifted")
	}
	return AgentRunInputs{OwnerWorkspacePath: ownerPath, ExecutionWorkspacePath: binding.WorktreePath,
		ProjectID: session.ProjectID, Binding: binding, Snapshot: snapshot}, nil
}

// BuildAgentRunWorkspacePack reads project context from the run's verified
// execution workspace, never from whichever workspace is active in the UI.
func (s *Server) BuildAgentRunWorkspacePack(ctx context.Context, spec agentchat.RunSpec) (patchtool.AIPackResult, error) {
	inputs, err := s.ResolveAgentRunInputs(ctx, spec)
	if err != nil {
		return patchtool.AIPackResult{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return patchtool.AIPackResult{}, err
	}
	builder := patchtool.BuildAIPack
	if s.AgentChatPack != nil {
		builder = s.AgentChatPack
	}
	pack, err := builder(inputs.ExecutionWorkspacePath, executable)
	if err != nil {
		return patchtool.AIPackResult{}, err
	}
	if pack.ProjectIdentity.ID != inputs.ProjectID {
		return patchtool.AIPackResult{}, errors.New("execution workspace AI Pack identity mismatch")
	}
	if len(pack.TextContent) > 400<<10 {
		return patchtool.AIPackResult{}, errors.New("execution workspace AI Pack exceeds run context limit")
	}
	pack.TextContent = scrubAIContext(pack.TextContent, 400<<10)
	return pack, nil
}

func (s *Server) AgentRunWorkspaceResolver() agentrun.WorkspaceResolver {
	return serverAgentWorkspaceResolver{server: s}
}

type serverAgentWorkspaceResolver struct{ server *Server }

func (r serverAgentWorkspaceResolver) Resolve(ctx context.Context, spec agentchat.RunSpec) (agentrun.WorkspaceBinding, error) {
	inputs, err := r.server.ResolveAgentRunInputs(ctx, spec)
	if err != nil {
		return agentrun.WorkspaceBinding{}, err
	}
	return inputs.Binding, nil
}
