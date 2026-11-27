package server

import (
	"context"
	"errors"
 "fmt"

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

// AdmitProviderAttempt rechecks the immutable automatic-turn grant at the
// last transport admission point, after expensive prompt assembly and before
// each retry. The current owner journal is authoritative, not the prior
// prompt-builder snapshot. No journal lock is held over network I/O.
func(r ownerAgentRunProviderResolver) AdmitProviderAttempt(ctx context.Context,spec agentchat.RunSpec)error{
 if err:=ctx.Err();err!=nil{return err}
 inputs,err:=r.server.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(ctx,r.owner),spec)
 if err!=nil{return err}
 if inputs.Snapshot.Policy==nil{return errors.New("run policy unavailable at provider admission")}
 policy:=inputs.Snapshot.Policy.Automation
 if policy.ContinuationMode==""{return nil} // explicit owner turn or receipt reconciliation
 store,err:=r.server.agentChatStore();if err!=nil{return err}
 x,err:=store.Get(spec.ConversationID,r.owner)
 if err!=nil||x.ProjectID!=inputs.ProjectID||runAutoContinueMode(x,policy)!=policy.ContinuationMode{
  return &agentchat.ProviderError{Code:"AUTOMATIC_CONTINUATION_REVOKED",Stage:"provider_admission",Message:"Automatic run continuation is no longer authorized; no new provider attempt was sent."}
 }
 return ctx.Err()
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


// Unified uses a domain-separated namespace for output-only hints: Legacy's
// input estimate and Unified's conservative byte admission are not equivalent.
// Scope is the authoritative owner/project/conversation, exact execution
// workspace, provider/model version and verified credential fingerprint.
// The existing private bounded TTL store provides integrity and expiry.
func(r ownerAgentRunProviderResolver) outputHintScope(ctx context.Context,spec agentchat.RunSpec,resolved agentchat.ResolvedProvider)(*agentchat.Store,agentchat.Session,string,bool){
 if ctx.Err()!=nil||r.server==nil||r.owner==""||spec.CredentialFingerprint==""||
  resolved.Config.ID!=spec.ProviderID||resolved.ConfigVersion!=spec.ProviderConfigVersion||
  resolved.CredentialFingerprint!=spec.CredentialFingerprint||
  agentchat.RunCredentialFingerprint([]byte(resolved.Credential))!=spec.CredentialFingerprint{return nil,agentchat.Session{},"",false}
 inputs,err:=r.server.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(ctx,r.owner),spec)
 if err!=nil{return nil,agentchat.Session{},"",false}
 store,err:=r.server.agentChatStore();if err!=nil{return nil,agentchat.Session{},"",false}
 x,err:=store.Get(spec.ConversationID,r.owner)
 if err!=nil||x.ProjectID!=inputs.ProjectID{return nil,agentchat.Session{},"",false}
 version:=agentchat.BodyDigest([]byte("taskdeck-unified-output-hint-v1\x00"+spec.ExecutionWorkspaceID+"\x00"+spec.ProviderConfigVersion))
 return store,x,version,true
}

func(r ownerAgentRunProviderResolver) OutputBudgetHint(ctx context.Context,spec agentchat.RunSpec,resolved agentchat.ResolvedProvider)int{
 store,x,version,ok:=r.outputHintScope(ctx,spec,resolved);if !ok{return 0}
 hint,found,err:=store.AdaptiveBudgetHint(x,version,spec.CredentialFingerprint)
 if err!=nil||!found{return 0}
 return hint.Output
}

func(r ownerAgentRunProviderResolver) RememberOutputBudgetHint(ctx context.Context,spec agentchat.RunSpec,resolved agentchat.ResolvedProvider,output int){
 if output<8192||output>200000{return}
 store,x,version,ok:=r.outputHintScope(ctx,spec,resolved);if !ok||ctx.Err()!=nil{return}
 // Input is not used in this domain. Keep the existing store format and
 // its minimum valid input marker; never interpret it as an input limit.
 _=store.RememberAdaptiveBudgetHint(x,version,spec.CredentialFingerprint,4096,output)
}


// Keep each Unified attempt in a separate owner-bound safe trace so concurrent
// runs in the same conversation cannot attach diagnostics to the wrong run.
// No raw capture, secrets, prompt, tool arguments or response are supplied.
func(r ownerAgentRunProviderResolver) ObserveProviderAttempt(spec agentchat.RunSpec,attempt int,m agentchat.ProviderPayloadMeasurement,digest string,outcome agentchat.ProviderAttemptOutcome){
 if r.server==nil||r.owner==""||attempt<=0||m.CanonicalBytes<=0{return}
 recorder,_:=r.server.startAgentDebug(r.owner,"unified_provider_attempt",spec.ConversationID,agentchat.ProviderConfig{ID:spec.ProviderID},spec.ModelID)
 recorder.RecordProviderDiagnostic(agentchat.DiagnosticEvent{
  Level:"info",Stage:"provider_retry",Event:"payload.budget",ProviderID:spec.ProviderID,ModelID:spec.ModelID,
  RequestBytes:m.CanonicalBytes,RequestSHA256:digest,
  SafeMessage:fmt.Sprintf("run=%s attempt=%d outcome=%s canonical_bytes=%d wire_template_bytes=%d estimated_input_tokens=%d output_budget=%d instruction_bytes=%d input_bytes=%d tools_bytes=%d other_bytes=%d stateful=%t",spec.RunID,attempt,outcome,m.CanonicalBytes,m.WireBytes,m.EstimatedInputTokens,m.OutputBudget,m.InstructionsBytes,m.InputBytes,m.ToolsBytes,m.OtherBytes,m.Stateful),
 })
}
