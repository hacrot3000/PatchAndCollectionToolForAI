package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func autoFeedbackReadySession(t *testing.T) (*Server, string) {
	t.Helper()
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	x.ToolCalls[0].Status = "done"
	x.ToolCalls[0].Result = `{"status":"PASS","run_id":"run-auto"}`
	until := time.Now().UTC().Add(10 * time.Minute)
	x.AutoFeedback = true
	x.AutoFeedbackUntil = &until
	x, err = store.Save(x, x.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return s, id
}

func TestAgentAutoFeedbackRequiresConsentAndDeliversOnce(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	noConsent := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/auto-feedback",
		map[string]any{"enabled": true, "expected_revision": x.Revision})
	if noConsent.Code != 403 {
		t.Fatalf("auto feedback enabled without consent: %d", noConsent.Code)
	}
	ok := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/auto-feedback",
		map[string]any{"enabled": true, "confirm_external": true, "expected_revision": x.Revision})
	if ok.Code != 200 || !strings.Contains(ok.Body.String(), `"auto_feedback":true`) {
		t.Fatalf("auto feedback opt-in failed: %d %s", ok.Code, ok.Body)
	}
	y, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if y.AutoFeedbackUntil == nil || y.AutoFeedbackUntil.After(time.Now().UTC().Add(agentAutoFeedbackMaxAge+time.Minute)) {
		t.Fatalf("trust window not bounded: %+v", y.AutoFeedbackUntil)
	}
	// A pending (not final) patch must never be delivered by the scheduler.
	if _, ready := agentAutoFeedbackCandidate(y); ready {
		t.Fatal("pending patch treated as ready")
	}
	y.ToolCalls[0].Status = "done"
	y.ToolCalls[0].Result = `{"status":"PASS","run_id":"run-auto"}`
	idx, ready := agentAutoFeedbackCandidate(y)
	if !ready || idx != 0 {
		t.Fatalf("final result not eligible: %d %v", idx, ready)
	}
}

func TestAgentAutoFeedbackSchedulerDeliversVerifiedResultOnce(t *testing.T) {
	s, id := autoFeedbackReadySession(t)
	calls := 0
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		if !strings.Contains(string(payload), `"call_id":"call_agent_001"`) {
			t.Fatal("wrong call_id delivered")
		}
		return agentchat.ResponsesResult{ResponseID: "resp_auto", Text: "ack"}, nil
	}
	s.runAgentAutoFeedback(context.Background())
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !x.ToolCalls[0].FeedbackSent || x.ToolCalls[0].FeedbackStatus != "acknowledged" {
		t.Fatalf("auto delivery not applied once: calls=%d %+v", calls, x.ToolCalls[0])
	}
	// A later pass must not resend.
	s.runAgentAutoFeedback(context.Background())
	if calls != 1 {
		t.Fatalf("auto feedback resent the same result: %d", calls)
	}
}

func TestAgentAutoFeedbackStopsAfterExpiryAndTransportFailure(t *testing.T) {
	s, id := autoFeedbackReadySession(t)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	x.AutoFeedbackUntil = &past
	if _, err = store.Save(x, x.Revision); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		return agentchat.ResponsesResult{}, nil
	}
	s.runAgentAutoFeedback(context.Background())
	if calls != 0 {
		t.Fatal("expired consent still delivered")
	}
	y, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(10 * time.Minute)
	y.AutoFeedbackUntil = &future
	y.ToolCalls[0].FeedbackSent = false
	y.ToolCalls[0].FeedbackStatus = ""
	y.Status = "waiting_tool"
	if y, err = store.Save(y, y.Revision); err != nil {
		t.Fatal(err)
	}
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		return agentchat.ResponsesResult{}, context.Canceled
	}
	s.runAgentAutoFeedback(context.Background())
	z, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if z.ToolCalls[0].FeedbackStatus != "unknown" {
		t.Fatalf("failed delivery not marked unknown: %q", z.ToolCalls[0].FeedbackStatus)
	}
	before := calls
	s.runAgentAutoFeedback(context.Background())
	if calls != before {
		t.Fatal("ambiguous delivery was retried automatically")
	}
}

func TestAgentAutoFeedbackSharedPermission(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/agent/sessions/id/auto-feedback", nil)
	perms := sharedRoutePermissions(req)
	if len(perms) != 3 || perms[2] != identity.PermissionFilesRead {
		t.Fatalf("auto-feedback permission mapping: %+v", perms)
	}
}
