package agentchat

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrProviderRouteMissing = errors.New("provider route is not registered")
	ErrProviderModelMissing = errors.New("model is not registered for provider route")
)

type providerRoute struct {
	adapter ProviderAdapter
	models  map[string]struct{}
}

// ProviderRouter resolves requests and cancellation from immutable run specs.
// It has no current-provider field and does not hold credentials.
type ProviderRouter struct {
	mu     sync.RWMutex
	routes map[string]providerRoute
}

func NewProviderRouter() *ProviderRouter {
	return &ProviderRouter{routes: make(map[string]providerRoute)}
}

func (r *ProviderRouter) Register(providerID string, adapter ProviderAdapter, modelIDs []string) error {
	if providerID == "" || adapter == nil || len(modelIDs) == 0 {
		return errors.New("provider ID, adapter, and models are required")
	}
	models := make(map[string]struct{}, len(modelIDs))
	for _, modelID := range modelIDs {
		if modelID == "" {
			return errors.New("empty model ID")
		}
		if _, exists := models[modelID]; exists {
			return errors.New("duplicate model ID")
		}
		models[modelID] = struct{}{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.routes == nil {
		r.routes = make(map[string]providerRoute)
	}
	r.routes[providerID] = providerRoute{adapter: adapter, models: models}
	return nil
}

func (r *ProviderRouter) route(spec RunSpec) (ProviderAdapter, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	route, exists := r.routes[spec.ProviderID]
	r.mu.RUnlock()
	if !exists {
		return nil, ErrProviderRouteMissing
	}
	if _, exists := route.models[spec.ModelID]; !exists {
		return nil, ErrProviderModelMissing
	}
	return route.adapter, nil
}

func (r *ProviderRouter) Execute(ctx context.Context, spec RunSpec, history []HistoryItem, sink EventSink) error {
	adapter, err := r.route(spec)
	if err != nil {
		return err
	}
	for _, item := range history {
		if err := ValidateHistoryItemForRun(item); err != nil {
			return err
		}
	}
	if sink != nil {
		sink = &boundRunEventSink{spec: spec, downstream: sink}
	}
	return adapter.Execute(ctx, spec, append([]HistoryItem(nil), history...), sink)
}

type boundRunEventSink struct {
	mu         sync.Mutex
	spec       RunSpec
	downstream EventSink
	sequence   uint64
}

func (s *boundRunEventSink) Publish(ctx context.Context, event RunEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if (event.RunID != "" && event.RunID != s.spec.RunID) ||
		(event.AttemptID != "" && event.AttemptID != s.spec.AttemptID) ||
		(event.ProviderID != "" && event.ProviderID != s.spec.ProviderID) {
		return errors.New("provider event does not match immutable run binding")
	}
	s.sequence++
	event.RunID = s.spec.RunID
	event.AttemptID = s.spec.AttemptID
	event.ProviderID = s.spec.ProviderID
	event.Sequence = s.sequence
	return s.downstream.Publish(ctx, event)
}

func (r *ProviderRouter) Cancel(ctx context.Context, spec RunSpec) error {
	adapter, err := r.route(spec)
	if err != nil {
		return err
	}
	return adapter.Cancel(ctx, spec)
}
