package agentchat

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type recordingAdapter struct {
	mu        sync.Mutex
	executed  []RunSpec
	cancelled []RunSpec
}

func (a *recordingAdapter) Execute(_ context.Context, spec RunSpec, _ []HistoryItem, _ EventSink) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.executed = append(a.executed, spec)
	return nil
}

func (a *recordingAdapter) Cancel(_ context.Context, spec RunSpec) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cancelled = append(a.cancelled, spec)
	return nil
}

func runSpecFor(providerID, modelID, runID string) RunSpec {
	return RunSpec{
		RunID: runID, ConversationID: "conv-1", OwnerWorkspaceID: "owner-ws",
		ExecutionWorkspaceID: "exec-ws", ProviderID: providerID, ModelID: modelID,
		ProviderConfigVersion: "cfg-v1", ContextSnapshotID: "ctx-1",
		AttemptID: runID + "-a1", IdempotencyKey: "req-" + runID,
	}
}

func TestProviderRouterBindsExecutionToRunProviderNotLatestSelection(t *testing.T) {
	router := NewProviderRouter()
	composer, gpt := &recordingAdapter{}, &recordingAdapter{}
	if err := router.Register("composer", composer, []string{"composer-2.5"}); err != nil {
		t.Fatal(err)
	}
	if err := router.Register("gpt", gpt, []string{"gpt-6.1-sol"}); err != nil {
		t.Fatal(err)
	}
	run101 := runSpecFor("composer", "composer-2.5", "run-101")
	run102 := runSpecFor("gpt", "gpt-6.1-sol", "run-102")
	if err := router.Execute(context.Background(), run101, nil, nil); err != nil {
		t.Fatal(err)
	}
	// The UI selection may now be GPT, but the original run keeps Composer.
	if err := router.Execute(context.Background(), run102, nil, nil); err != nil {
		t.Fatal(err)
	}
	retry101 := run101
	retry101.AttemptID = "run-101-a2"
	retry101.IdempotencyKey = "req-run-101-a2"
	if err := router.Execute(context.Background(), retry101, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(composer.executed) != 2 || composer.executed[0].RunID != "run-101" || composer.executed[1].ProviderID != run101.ProviderID || composer.executed[1].ModelID != run101.ModelID || composer.executed[1].ProviderConfigVersion != run101.ProviderConfigVersion {
		t.Fatalf("composer retry did not retain original binding: %+v", composer.executed)
	}
	if len(gpt.executed) != 1 || gpt.executed[0].RunID != "run-102" {
		t.Fatalf("gpt did not own run #102: %+v", gpt.executed)
	}
	if err := router.Cancel(context.Background(), run101); err != nil {
		t.Fatal(err)
	}
	if len(gpt.cancelled) != 0 || len(composer.cancelled) != 1 {
		t.Fatalf("stop #101 crossed provider boundary: composer=%+v gpt=%+v", composer.cancelled, gpt.cancelled)
	}
}

type emittingAdapter struct{}

func (emittingAdapter) Execute(ctx context.Context, spec RunSpec, _ []HistoryItem, sink EventSink) error {
	return sink.Publish(ctx, RunEvent{Type: RunEventTextDelta, Payload: []byte("safe visible output")})
}
func (emittingAdapter) Cancel(context.Context, RunSpec) error { return nil }

type eventCollector struct{ events []RunEvent }

func (c *eventCollector) Publish(_ context.Context, event RunEvent) error {
	c.events = append(c.events, event)
	return nil
}

func TestProviderRouterBindsNormalizedEventsToImmutableSpec(t *testing.T) {
	router := NewProviderRouter()
	if err := router.Register("gpt", emittingAdapter{}, []string{"gpt-6.1-sol"}); err != nil {
		t.Fatal(err)
	}
	spec := runSpecFor("gpt", "gpt-6.1-sol", "run-102")
	collector := &eventCollector{}
	if err := router.Execute(context.Background(), spec, nil, collector); err != nil {
		t.Fatal(err)
	}
	if len(collector.events) != 1 {
		t.Fatalf("expected one normalized event, got %d", len(collector.events))
	}
	event := collector.events[0]
	if event.RunID != spec.RunID || event.AttemptID != spec.AttemptID || event.ProviderID != spec.ProviderID || event.Sequence != 1 {
		t.Fatalf("event did not inherit pinned run binding: %+v", event)
	}
}

func TestProviderRouterRoutesIndependentRunsConcurrently(t *testing.T) {
	router := NewProviderRouter()
	composer, gpt := &recordingAdapter{}, &recordingAdapter{}
	if err := router.Register("composer", composer, []string{"composer-2.5"}); err != nil {
		t.Fatal(err)
	}
	if err := router.Register("gpt", gpt, []string{"gpt-6.1-sol"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, spec := range []RunSpec{
		runSpecFor("composer", "composer-2.5", "run-101"),
		runSpecFor("gpt", "gpt-6.1-sol", "run-102"),
	} {
		wg.Add(1)
		go func(spec RunSpec) {
			defer wg.Done()
			errs <- router.Execute(context.Background(), spec, nil, nil)
		}(spec)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(composer.executed) != 1 || composer.executed[0].RunID != "run-101" || len(gpt.executed) != 1 || gpt.executed[0].RunID != "run-102" {
		t.Fatalf("concurrent runs crossed provider adapters: composer=%+v gpt=%+v", composer.executed, gpt.executed)
	}
}

func TestProviderRouterRejectsUnknownOrMismatchedBinding(t *testing.T) {
	router := NewProviderRouter()
	if err := router.Register("gpt", &recordingAdapter{}, []string{"gpt-6.1-sol"}); err != nil {
		t.Fatal(err)
	}
	if err := router.Execute(context.Background(), runSpecFor("missing", "gpt-6.1-sol", "run-1"), nil, nil); !errors.Is(err, ErrProviderRouteMissing) {
		t.Fatalf("unknown provider: %v", err)
	}
	if err := router.Execute(context.Background(), runSpecFor("gpt", "other-model", "run-2"), nil, nil); !errors.Is(err, ErrProviderModelMissing) {
		t.Fatalf("unregistered model: %v", err)
	}
	noWorkspace := runSpecFor("gpt", "gpt-6.1-sol", "run-3")
	noWorkspace.ExecutionWorkspaceID = ""
	if err := router.Execute(context.Background(), noWorkspace, nil, nil); err == nil {
		t.Fatal("missing execution workspace binding accepted")
	}
}
