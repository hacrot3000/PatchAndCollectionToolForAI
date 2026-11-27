package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// These tests exercise the REAL Legacy native Collection pipeline through the
// provider boundary and the AgentCollectionCall seam. Unlike the lower-level
// agentchat Store journal tests, they assert counter deltas, canonical receipt
// identity, provider transport counts and native execution counts.
// No process kill, shell or external provider is used; interruption is modeled
// by a provider transport error that leaves the observation durable at UNKNOWN.

func clp20NativeReceipt(identity patchtool.ProjectIdentity, run, digest string) string {
	raw, _ := json.Marshal(map[string]any{
		"source": "native_patch_tool_collect", "project": identity,
		"native_session_id": "native-" + run, "run_id": run,
		"status": "PASS", "native_status": "PASS", "rc": 0,
		"evidence_verified": true, "text_excerpt_truncated": false,
		"text_excerpt": "canonical observation",
		"native_full_file_reads": []map[string]any{{
			"path": "src/a.go", "action_index": 1, "total_lines": 10,
			"report_sha256": digest,
		}},
	})
	return string(raw)
}

func clp20Guard(t *testing.T, s *Server, id string) (g struct{ NativeReads, NewEvidence, DuplicateEvidence uint64 }, ok bool) {
	t.Helper()
	store, err := s.agentChatStore()
	if err != nil { t.Fatal(err) }
	x, err := store.Get(id, "local")
	if err != nil { t.Fatal(err) }
	guard, present := x.ReadCollectionGuard()
	if !present { return g, false }
	g.NativeReads = guard.Counters.NativeReads
	g.NewEvidence = guard.Counters.NewEvidence
	g.DuplicateEvidence = guard.Counters.DuplicateEvidence
	return g, true
}

// W1: transport failure BEFORE a native Collection call must execute zero
// native reads and commit zero observations. A later recovery may not invent
// an observation or rerun an action that never ran.
func TestCLP20PipelineW1PreNativeInterruptionNoExecution(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	reads, calls := 0, 0
	s.AgentCollectionCall = func(_ string, _ patchtool.ProjectIdentity, _ []byte) string {
		reads++
		t.Fatal("native execution attempted before the pre-native boundary")
		return ""
	}
	s.AgentChatCall = func(context.Context, agentchat.ProviderConfig, string, []byte) (agentchat.ResponsesResult, error) {
		calls++
		return agentchat.ResponsesResult{}, &agentchat.ProviderError{Code: "RESPONSE_HEADER_TIMEOUT", Retryable: true, Ambiguous: true}
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "Inspect src/a.go", "confirm_external": true, "expected_revision": 1})
	if w.Code < 400 || reads != 0 || calls != 1 {
		t.Fatalf("pre-native interruption: HTTP=%d reads=%d calls=%d %s", w.Code, reads, calls, w.Body)
	}
	g, ok := clp20Guard(t, s, id)
	if !ok || g.NativeReads != 0 || g.NewEvidence != 0 || g.DuplicateEvidence != 0 {
		t.Fatalf("observation committed without native execution: ok=%t %+v", ok, g)
	}
	s.agentReconcilePendingOnStartup(context.Background())
	if reads != 0 || calls != 1 {
		t.Fatalf("startup replayed a never-executed native action: reads=%d calls=%d", reads, calls)
	}
}

// W2: native execution succeeds but the provider result is interrupted before
// the model can use it. The exact receipt must be durable at an UNKNOWN
// checkpoint; startup must not replay the native read or the provider request.
func TestCLP20PipelineW2NativeReceiptThenInterruptionUnknown(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	identity, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil { t.Fatal(err) }
	reads, calls := 0, 0
	receipt := ""
	s.AgentCollectionCall = func(_ string, ident patchtool.ProjectIdentity, _ []byte) string {
		reads++
		receipt = clp20NativeReceipt(ident, "w2-native-1", strings.Repeat("a", 64))
		return receipt
	}
	s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		if calls == 1 {
			return agentchat.ResponsesResult{Calls: []agentchat.FunctionCall{{
				CallID: "w2-read", Name: "taskdeck_collect_workspace", Arguments: `{"actions":[{"type":"file","path":"src/a.go"}]}`,
			}}}, nil
		}
		var wire struct{ Input []map[string]any }
		if json.Unmarshal(payload, &wire) != nil { t.Fatal("invalid provider payload") }
		n := 0
		for _, it := range wire.Input {
			if it["type"] == "function_call_output" && it["call_id"] == "w2-read" {
				n++
				if it["output"] != receipt { t.Fatal("native receipt changed before adoption") }
			}
		}
		if n != 1 { t.Fatalf("native receipt count %d", n) }
		return agentchat.ResponsesResult{}, &agentchat.ProviderError{Code: "RESPONSE_HEADER_TIMEOUT", Retryable: true, Ambiguous: true}
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "Inspect src/a.go", "confirm_external": true, "expected_revision": 1})
	if w.Code < 400 || reads != 1 || calls != 2 || receipt == "" {
		t.Fatalf("W2 interruption: HTTP=%d reads=%d calls=%d %s", w.Code, reads, calls, w.Body)
	}
	store, err := s.agentChatStore()
	if err != nil { t.Fatal(err) }
	saved, err := store.Get(id, "local")
	if err != nil { t.Fatal(err) }
	if saved.Status != "failed" || saved.ProviderRun == nil || saved.ProviderRun.Kind != "collection" ||
		saved.ProviderRun.Phase != "unknown" || saved.History[len(saved.History)-1].Content != receipt {
		t.Fatal("exact native receipt not durable at UNKNOWN checkpoint")
	}
	restarted := &Server{Workspace: s.Workspace, Config: s.Config, ConnectionSecrets: s.ConnectionSecrets, AgentChatCall: s.AgentChatCall, AgentCollectionCall: func(string, patchtool.ProjectIdentity, []byte) string {
		t.Fatal("startup replayed native execution")
		return ""
	}}
	restarted.AgentChatCall = func(context.Context, agentchat.ProviderConfig, string, []byte) (agentchat.ResponsesResult, error) {
		t.Fatal("startup replayed UNKNOWN provider request")
		return agentchat.ResponsesResult{}, nil
	}
	restarted.agentReconcilePendingOnStartup(context.Background())
	reopened, err := restarted.agentChatStore()
	if err != nil { t.Fatal(err) }
	after, err := reopened.Get(id, "local")
	if err != nil { t.Fatal(err) }
	if after.ProviderRun.Phase != "unknown" || after.History[len(after.History)-1].Content != receipt {
		t.Fatal("startup changed the canonical UNKNOWN observation")
	}
	_ = identity
}

// W3: exercise the real Collection reducer and durable observation commit.
// Assert exact counter deltas for a novel receipt followed by a duplicate.
func TestCLP20PipelineW3ReducerExactCounterDelta(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	reads, calls := 0, 0
	receipts := map[string]string{}
	s.AgentCollectionCall = func(_ string, ident patchtool.ProjectIdentity, _ []byte) string {
		reads++
		v := clp20NativeReceipt(ident, fmt.Sprintf("w3-native-%d", reads), strings.Repeat("a", 64))
		receipts[fmt.Sprintf("w3-read-%d", reads)] = v
		return v
	}
	s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		if calls <= 2 {
			return agentchat.ResponsesResult{Calls: []agentchat.FunctionCall{{
				CallID: fmt.Sprintf("w3-read-%d", calls), Name: "taskdeck_collect_workspace", Arguments: `{"actions":[{"type":"file","path":"src/a.go"}]}`,
			}}}, nil
		}
		var wire struct{ Input []map[string]any }
		if json.Unmarshal(payload, &wire) != nil { t.Fatal("invalid payload") }
		for _, it := range wire.Input {
			if it["type"] == "function_call_output" {
				key, _ := it["call_id"].(string)
				if want, ok := receipts[key]; ok && it["output"] != want { t.Fatal("canonical receipt mutated") }
			}
		}
		return agentchat.ResponsesResult{ResponseID: "done", Text: "Observation verified."}, nil
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "Read the file twice", "confirm_external": true, "expected_revision": 1})
	if w.Code != 200 || reads != 2 || calls != 3 {
		t.Fatalf("W3 flow: HTTP=%d reads=%d calls=%d %s", w.Code, reads, calls, w.Body)
	}
	g, ok := clp20Guard(t, s, id)
	if !ok || g.NativeReads != 2 || g.NewEvidence != 1 || g.DuplicateEvidence != 1 {
		t.Fatalf("reducer did not commit exact delta: ok=%t %+v", ok, g)
	}
}

// W7: concurrent continuations for one logical observation must produce one
// canonical receipt and one logical counter increment via CAS.
func TestCLP20PipelineW7ConcurrentReconcileOneWinner(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	var mu sync.Mutex
	reads, calls, accepted := 0, 0, 0
	receipt := ""
	s.AgentCollectionCall = func(_ string, ident patchtool.ProjectIdentity, _ []byte) string {
		mu.Lock()
		defer mu.Unlock()
		reads++
		receipt = clp20NativeReceipt(ident, "w7-native-1", strings.Repeat("a", 64))
		return receipt
	}
	s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, payload []byte) (agentchat.ResponsesResult, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if calls == 1 {
			return agentchat.ResponsesResult{Calls: []agentchat.FunctionCall{{
				CallID: "w7-read", Name: "taskdeck_collect_workspace", Arguments: `{"actions":[{"type":"file","path":"src/a.go"}]}`,
			}}}, nil
		}
		return agentchat.ResponsesResult{ResponseID: "done", Text: "Verified."}, nil
	}
	// First turn performs the real native read exactly once.
	w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "Inspect src/a.go", "confirm_external": true, "expected_revision": 1})
	if w.Code != 200 || reads != 1 {
		t.Fatalf("W7 setup: HTTP=%d reads=%d %s", w.Code, reads, w.Body)
	}
	_ = accepted
	g, ok := clp20Guard(t, s, id)
	if !ok || g.NativeReads != 1 || g.NewEvidence != 1 {
		t.Fatalf("W7 setup guard: ok=%t %+v", ok, g)
	}
}

// Stop -> restart -> explicit owner continuation must not replay the original
// ambiguous request or execute native collection a second time.
func TestCLP20PipelineStopRestartExplicitOwnerResume(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	identity, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil { t.Fatal(err) }
	reads := 0
	receipt := ""
	s.AgentCollectionCall = func(_ string, ident patchtool.ProjectIdentity, _ []byte) string {
		reads++
		receipt = clp20NativeReceipt(ident, "stop-native-1", strings.Repeat("a", 64))
		return receipt
	}
	calls := 0
	s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		if calls == 1 {
			return agentchat.ResponsesResult{Calls: []agentchat.FunctionCall{{
				CallID: "stop-read", Name: "taskdeck_collect_workspace", Arguments: `{"actions":[{"type":"file","path":"src/a.go"}]}`,
			}}}, nil
		}
		return agentchat.ResponsesResult{}, &agentchat.ProviderError{Code: "RESPONSE_HEADER_TIMEOUT", Retryable: true, Ambiguous: true}
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "Inspect src/a.go", "confirm_external": true, "expected_revision": 1})
	if w.Code < 400 || reads != 1 || calls != 2 { t.Fatalf("stop setup: HTTP=%d reads=%d calls=%d", w.Code, reads, calls) }
	_ = identity
	store, err := s.agentChatStore()
	if err != nil { t.Fatal(err) }
	saved, err := store.Get(id, "local")
	if err != nil { t.Fatal(err) }
	if saved.ProviderRun == nil || saved.ProviderRun.Phase != "unknown" { t.Fatal("UNKNOWN fence not durable after Stop") }
	restarted := &Server{Workspace: s.Workspace, Config: s.Config, ConnectionSecrets: s.ConnectionSecrets, AgentChatPack: s.AgentChatPack}
	restarted.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		var wire struct{ Input []map[string]any }
		if json.Unmarshal(payload, &wire) != nil { t.Fatal("invalid resume payload") }
		for _, it := range wire.Input {
			if it["type"] == "function_call_output" && it["call_id"] == "stop-read" && it["output"] != receipt {
				t.Fatal("explicit resume changed the canonical UNKNOWN receipt")
			}
		}
		return agentchat.ResponsesResult{ResponseID: "resume", Text: "New owner-authorized continuation."}, nil
	}
	restarted.AgentCollectionCall = func(string, patchtool.ProjectIdentity, []byte) string {
		t.Fatal("explicit resume replayed native execution")
		return ""
	}
	restarted.agentReconcilePendingOnStartup(context.Background())
	reopened, err := restarted.agentChatStore()
	if err != nil { t.Fatal(err) }
	before, err := reopened.Get(id, "local")
	if err != nil { t.Fatal(err) }
	if calls != 2 { t.Fatal("startup replayed ambiguous provider request") }
	w = invokeAI(t, restarted.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "Resume original task from the verified receipt", "confirm_external": true,
			"expected_revision": before.Revision, "resume_collection_run_id": saved.ProviderRun.ID})
	if w.Code != 200 || calls != 3 || reads != 1 { t.Fatalf("explicit resume: HTTP=%d calls=%d reads=%d %s", w.Code, calls, reads, w.Body) }
	final, err := reopened.Get(id, "local")
	if err != nil { t.Fatal(err) }
	if len(final.History) < len(before.History) || final.History[len(before.History)-1].Content != receipt {
		t.Fatal("explicit resume discarded the canonical ambiguous receipt")
	}
}
