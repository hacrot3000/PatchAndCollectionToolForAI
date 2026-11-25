package server

import (
	"context"
	"errors"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

type ownerAgentRunProviderResolver struct {
	server *Server
	owner  string
}

func (r ownerAgentRunProviderResolver) Resolve(ctx context.Context, spec agentchat.RunSpec) (agentchat.ResolvedProvider, error) {
	if r.server == nil || r.owner == "" {
		return agentchat.ResolvedProvider{}, errors.New("provider owner is unavailable")
	}
	ownerWorkspace := r.server.agentOwnerWorkspace()
	ownerWorkspaceID, err := agentrun.WorkspaceID(ownerWorkspace)
	if err != nil || ownerWorkspaceID != spec.OwnerWorkspaceID {
		return agentchat.ResolvedProvider{}, errors.New("run owner workspace binding mismatch")
	}
	store, err := r.server.agentChatProviders()
	if err != nil {
		return agentchat.ResolvedProvider{}, err
	}
	provider, model, err := store.GetVersion(r.owner, spec.ProviderID, spec.ModelID, spec.ProviderConfigVersion)
	if err != nil {
		return agentchat.ResolvedProvider{}, agentchat.ErrProviderBindingMismatch
	}
	version := spec.ProviderConfigVersion
	if r.server.ConnectionSecrets == nil {
		return agentchat.ResolvedProvider{}, agentchat.ErrProviderKeyUnavailable
	}
	credential, err := r.server.ConnectionSecrets.Get(agentProviderSecretID(ownerWorkspace, r.owner, provider.ID))
	if err != nil || len(credential) == 0 {
		return agentchat.ResolvedProvider{}, agentchat.ErrProviderKeyUnavailable
	}
	credentialFingerprint := agentchat.RunCredentialFingerprint(credential)
	if spec.CredentialFingerprint == "" || credentialFingerprint != spec.CredentialFingerprint {
		return agentchat.ResolvedProvider{}, agentchat.ErrProviderCredentialChanged
	}
	streamingVerified := false
	if provider.Stream {
		caps, capsErr := r.server.agentChatCapabilities()
		if capsErr == nil {
			record, exists, getErr := caps.Get(r.owner, provider.ID, model.ID, version)
			streamingVerified = getErr == nil && exists && record.Capabilities[agentchat.ProbeStreaming] == agentchat.CapabilityVerified
		}
	}
	return agentchat.ResolvedProvider{
		Config: provider, ConfigVersion: version, Credential: string(credential),
		CredentialFingerprint: credentialFingerprint, StreamingVerified: streamingVerified,
	}, nil
}

// serverAgentRunProviderInvoker reuses the existing Agent Chat transport seam
// so tests can inject a provider double without a second production HTTP path.
// Production behavior is identical to the hardened HTTP invoker.
type serverAgentRunProviderInvoker struct{ server *Server }

func (i serverAgentRunProviderInvoker) Invoke(ctx context.Context, provider agentchat.ProviderConfig, credential string, payload []byte) (agentchat.ResponsesResult, error) {
	if i.server != nil && i.server.AgentChatCall != nil {
		return i.server.AgentChatCall(ctx, provider, credential, payload)
	}
	return agentchat.HTTPProviderInvoker{}.Invoke(ctx, provider, credential, payload)
}

func (i serverAgentRunProviderInvoker) InvokeStream(ctx context.Context, provider agentchat.ProviderConfig, credential string, payload []byte, onEvent func(agentchat.StreamEvent) error) (agentchat.ResponsesResult, error) {
	if i.server != nil && i.server.AgentChatCall != nil {
		return i.server.AgentChatCall(ctx, provider, credential, payload)
	}
	return agentchat.HTTPProviderInvoker{}.InvokeStream(ctx, provider, credential, payload, onEvent)
}

// NewAgentRunProviderRouter creates an owner-bound executor for one run-manager
// instance. The caller must bind the same owner to the manager; do not share this
// router across owners. Credentials are resolved only when a pinned run executes.
func (s *Server) NewAgentRunProviderRouterForRun(owner string, options agentchat.ProviderPromptOptions, spec agentchat.RunSpec) (*agentchat.ProviderRouter, error) {
	if s == nil || s.Workspace == "" || owner == "" || options == nil {
		return nil, errors.New("owner workspace, owner identity, and prompt options are required")
	}
	providers, err := s.agentChatProviders()
	if err != nil {
		return nil, err
	}
	provider, model, err := providers.GetVersion(owner, spec.ProviderID, spec.ModelID, spec.ProviderConfigVersion)
	if err != nil {
		return nil, agentchat.ErrProviderBindingMismatch
	}
	resolver := ownerAgentRunProviderResolver{server: s, owner: owner}
	adapter := agentchat.ConfiguredProviderAdapter{
		ProviderID: provider.ID, Resolver: resolver, Builder: agentchat.CanonicalProviderPayloadBuilder{Options: options},
		Invoker: serverAgentRunProviderInvoker{server: s},
	}
	router := agentchat.NewProviderRouter()
	if err := router.Register(provider.ID, adapter, []string{model.ID}); err != nil {
		return nil, err
	}
	return router, nil
}

func (s *Server) NewAgentRunProviderRouter(owner string, options agentchat.ProviderPromptOptions) (*agentchat.ProviderRouter, error) {
	if s == nil || s.Workspace == "" || owner == "" || options == nil {
		return nil, errors.New("owner workspace, owner identity, and prompt options are required")
	}
	providers, err := s.agentChatProviders()
	if err != nil {
		return nil, err
	}
	configs, err := providers.List(owner)
	if err != nil {
		return nil, err
	}
	resolver := ownerAgentRunProviderResolver{server: s, owner: owner}
	builder := agentchat.CanonicalProviderPayloadBuilder{Options: options}
	invoker := serverAgentRunProviderInvoker{server: s}
	router := agentchat.NewProviderRouter()
	for _, provider := range configs {
		models := make([]string, 0, len(provider.Models))
		for _, model := range provider.Models {
			models = append(models, model.ID)
		}
		adapter := agentchat.ConfiguredProviderAdapter{
			ProviderID: provider.ID, Resolver: resolver, Builder: builder,
			Invoker: invoker,
		}
		if err := router.Register(provider.ID, adapter, models); err != nil {
			return nil, err
		}
	}
	return router, nil
}
