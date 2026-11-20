package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestAgentChatSessionsCreateListAndGet(t *testing.T) {
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.saveAISettings(aiSettings{Enabled: true, Consent: true, Provider: "openai", Model: "gpt-4.1-mini"}); err != nil {
		t.Fatal(err)
	}
	created := invokeAI(t, s.agentChatSessionsAPI, http.MethodPost, "/api/agent/sessions", map[string]string{"mode": "stateless"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create %d %s", created.Code, created.Body)
	}
	var c struct {
		Session struct {
			ID        string `json:"id"`
			ProjectID string `json:"project_id"`
			Revision  uint64 `json:"revision"`
		} `json:"session"`
		ExecutionEnabled bool `json:"execution_enabled"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if c.Session.ID == "" || c.Session.ProjectID == "" || c.Session.Revision != 1 || c.ExecutionEnabled {
		t.Fatalf("bad session: %+v", c)
	}
	list := invokeAI(t, s.agentChatSessionsAPI, http.MethodGet, "/api/agent/sessions", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), c.Session.ID) {
		t.Fatalf("list %d %s", list.Code, list.Body)
	}
	get := invokeAI(t, s.agentChatSessionItemAPI, http.MethodGet, "/api/agent/sessions/"+c.Session.ID, nil)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), c.Session.ProjectID) {
		t.Fatalf("get %d %s", get.Code, get.Body)
	}
	other := invokeAI(t, s.agentChatSessionItemAPI, http.MethodGet, "/api/agent/sessions/../../etc/passwd", nil)
	if other.Code != http.StatusBadRequest {
		t.Fatalf("path traversal accepted: %d", other.Code)
	}
}

func TestAgentChatSessionsDisabledAndUnknownFields(t *testing.T) {
	s := newAIServerForTest(t)
	w := invokeAI(t, s.agentChatSessionsAPI, http.MethodPost, "/api/agent/sessions", map[string]string{"mode": "stateless"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected opt-in rejection, got %d", w.Code)
	}
	if err := s.saveAISettings(aiSettings{Enabled: true, Consent: true, Provider: "openai"}); err != nil {
		t.Fatal(err)
	}
	w = invokeAI(t, s.agentChatSessionsAPI, http.MethodPost, "/api/agent/sessions", map[string]string{"mode": "unknown"})
	if w.Code != http.StatusBadRequest && w.Code != http.StatusConflict {
		t.Fatalf("invalid mode accepted: %d", w.Code)
	}
	w = invokeAI(t, s.agentChatSessionsAPI, http.MethodPost, "/api/agent/sessions", map[string]string{"unexpected": "value"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown fields accepted: %d", w.Code)
	}
}

func TestAgentChatRoutesExplicitSharedPermission(t *testing.T) {
	routes := []struct {
		method, path string
		perms        []string
	}{
		{"GET", "/api/agent/sessions", []string{identity.PermissionSettingsRead}},
		{"POST", "/api/agent/sessions", []string{identity.PermissionSettingsWrite}},
		{"GET", "/api/agent/sessions/123", []string{identity.PermissionSettingsRead}},
	}
	for _, tc := range routes {
		r := &http.Request{Method: tc.method, URL: mustAgentTestURL(t, tc.path)}
		p := sharedRoutePermissions(r)
		if len(p) != len(tc.perms) || len(p) != 1 || p[0] != tc.perms[0] {
			t.Fatalf("missing shared permission %s %s: %v", tc.method, tc.path, p)
		}
	}
}

func mustAgentTestURL(t *testing.T, path string) *url.URL {
	t.Helper()
	u, err := url.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestAgentChatSessionRenameArchiveAndActiveLimit(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if x.Target != "local" || x.SchemaVersion != agentchat.SessionSchemaVersion {
		t.Fatalf("schema not initialised: %+v", x)
	}
	path := "/api/agent/sessions/" + id + "/metadata"
	stale := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPatch, path, map[string]any{"title": "renamed", "expected_revision": x.Revision + 9})
	if stale.Code != 409 {
		t.Fatalf("stale metadata revision accepted: %d", stale.Code)
	}
	renamed := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPatch, path, map[string]any{"title": "  my chat  ", "expected_revision": x.Revision})
	if renamed.Code != 200 {
		t.Fatalf("rename failed: %d %s", renamed.Code, renamed.Body)
	}
	y, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if y.Title != "my chat" {
		t.Fatalf("title not normalised: %q", y.Title)
	}
	// An unresolved patch call must block archiving instead of hiding the job.
	y.ToolCalls = append(y.ToolCalls, agentchat.ToolCallRecord{
		CallID: "call_running", Name: "taskdeck_submit_patch", Status: "running",
	})
	y.History = append(y.History, agentchat.HistoryItem{Type: "function_call", CallID: "call_running", Name: "taskdeck_submit_patch", Arguments: `{"name":"x","patch_python":"y"}`})
	y, err = store.Save(y, y.Revision)
	if err != nil {
		t.Fatal(err)
	}
	blocked := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPatch, path, map[string]any{"archived": true, "expected_revision": y.Revision})
	if blocked.Code != 409 {
		t.Fatalf("unresolved patch session archived: %d", blocked.Code)
	}
	// Resvolving to a final status allows archiving; archived sessions do not
	// count against the active limit.
	y.ToolCalls[len(y.ToolCalls)-1].Status = "done"
	y.ToolCalls[len(y.ToolCalls)-1].Result = `{"status":"PASS"}`
	y, err = store.Save(y, y.Revision)
	if err != nil {
		t.Fatal(err)
	}
	ok := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPatch, path, map[string]any{"archived": true, "expected_revision": y.Revision})
	if ok.Code != 200 {
		t.Fatalf("archive failed: %d %s", ok.Code, ok.Body)
	}
	z, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if !z.Archived {
		t.Fatal("archive not persisted")
	}
	list := invokeAI(t, s.agentChatSessionsAPI, http.MethodGet, "/api/agent/sessions", nil)
	if list.Code != 200 || !strings.Contains(list.Body.String(), `"archived":true`) {
		t.Fatalf("archive not listed: %s", list.Body)
	}
}

func TestAgentChatCancelStopsActiveProviderRequest(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		<-ctx.Done()
		return agentchat.ResponsesResult{}, ctx.Err()
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/messages",
			map[string]any{"message": "long request", "confirm_external": true, "expected_revision": 1})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !agentRequestActive(s.Workspace, "local", id) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !agentRequestActive(s.Workspace, "local", id) {
		t.Fatal("provider request was not registered")
	}
	cancel := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/cancel", map[string]any{})
	if cancel.Code != 200 || !strings.Contains(cancel.Body.String(), `"cancelled":true`) {
		t.Fatalf("cancel failed: %d %s", cancel.Code, cancel.Body)
	}
	select {
	case w := <-done:
		if w.Code != 409 {
			t.Fatalf("cancelled turn did not report failure: %d %s", w.Code, w.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled provider call did not return")
	}
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if x.Status != "cancelled" {
		t.Fatalf("cancelled turn recorded as %q", x.Status)
	}
	// A second cancel has nothing to stop and must say so, not claim success.
	again := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/cancel", map[string]any{})
	if again.Code != 200 || !strings.Contains(again.Body.String(), `"cancelled":false`) {
		t.Fatalf("idle cancel misreported: %d %s", again.Code, again.Body)
	}
}

func TestAgentChatSessionReasoningEffortSelection(t *testing.T) {
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	p := agentProviderFixture()
	providers, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	if err := providers.Put("local", p); err != nil {
		t.Fatal(err)
	}
	if err := s.ConnectionSecrets.Put(agentProviderSecretID(s.Workspace, "local", p.ID), []byte("K")); err != nil {
		t.Fatal(err)
	}
	bad := invokeAI(t, s.agentChatSessionsAPI, http.MethodPost, "/api/agent/sessions", map[string]any{
		"provider_id": p.ID, "model_id": p.Models[0].ID, "reasoning_effort": "turbo"})
	if bad.Code != 400 {
		t.Fatalf("unadvertised reasoning effort accepted: %d", bad.Code)
	}
	ok := invokeAI(t, s.agentChatSessionsAPI, http.MethodPost, "/api/agent/sessions", map[string]any{
		"provider_id": p.ID, "model_id": p.Models[0].ID, "reasoning_effort": "high"})
	if ok.Code != 201 {
		t.Fatalf("advertised effort refused: %d %s", ok.Code, ok.Body)
	}
	var view struct {
		Session agentchat.Session `json:"session"`
	}
	if err := json.Unmarshal(ok.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Session.ReasoningEffort != "high" {
		t.Fatalf("effort not persisted: %q", view.Session.ReasoningEffort)
	}
	list := invokeAI(t, s.agentChatSessionsAPI, http.MethodGet, "/api/agent/sessions", nil)
	if !strings.Contains(list.Body.String(), `"reasoning_effort":"high"`) {
		t.Fatalf("effort missing from list: %s", list.Body)
	}
}

func TestAgentPackPreviewDoesNotContactProviderOrPersist(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		return agentchat.ResponsesResult{}, nil
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, http.MethodGet, "/api/agent/sessions/"+id+"/pack-preview", nil)
	if w.Code != 200 {
		t.Fatalf("preview failed: %d %s", w.Code, w.Body)
	}
	if calls != 0 {
		t.Fatal("preview contacted the provider")
	}
	body := w.Body.String()
	for _, want := range []string{`"redaction":"best_effort"`, `"preview"`, `"fingerprint":"fingerprint-001"`, "No provider request was made"} {
		if !strings.Contains(body, want) {
			t.Fatalf("preview missing %q: %s", want, body)
		}
	}
	after, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || len(after.History) != 0 {
		t.Fatal("preview mutated the durable session")
	}
	if req := httptest.NewRequest(http.MethodGet, "/api/agent/sessions/x/pack-preview", nil); len(sharedRoutePermissions(req)) != 2 {
		// The preview renders Patch Tool documentation, so it needs patch.view
		// alongside settings.read.
		t.Fatalf("preview permission mapping: %+v", sharedRoutePermissions(req))
	}
}

func TestAgentChatSessionTargetIsLocalOnly(t *testing.T) {
	s, _ := newAgentConversationFixture(t, "stateless")
	p := agentProviderFixture()
	bad := invokeAI(t, s.agentChatSessionsAPI, http.MethodPost, "/api/agent/sessions", map[string]any{
		"provider_id": p.ID, "model_id": p.Models[0].ID, "target": "remote-host-1"})
	if bad.Code != 400 {
		t.Fatalf("unsupported remote target accepted: %d %s", bad.Code, bad.Body)
	}
	ok := invokeAI(t, s.agentChatSessionsAPI, http.MethodPost, "/api/agent/sessions", map[string]any{
		"provider_id": p.ID, "model_id": p.Models[0].ID, "target": "local"})
	if ok.Code != 201 {
		t.Fatalf("local target refused: %d %s", ok.Code, ok.Body)
	}
	var view struct {
		Session agentchat.Session `json:"session"`
	}
	if err := json.Unmarshal(ok.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Session.Target != "local" || view.Session.Workspace != s.Workspace {
		t.Fatalf("target/workspace scope not bound: %+v", view.Session)
	}
}

func TestAgentRefreshDuringRequestOffersCancelNotDuplicate(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	x.Status = "requesting_ai"
	if _, err = store.Save(x, x.Revision); err != nil {
		t.Fatal(err)
	}
	// A duplicate send while a request is registered must be refused, and the
	// cancel endpoint must report it as active rather than "nothing".
	key := agentRequestKey(s.Workspace, "local", id)
	ctx, cancel := context.WithCancel(context.Background())
	release := registerAgentRequest(key, cancel)
	dup := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "again", "confirm_external": true, "expected_revision": x.Revision + 1})
	if dup.Code != 409 {
		t.Fatalf("duplicate send during request allowed: %d %s", dup.Code, dup.Body)
	}
	stop := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/cancel", map[string]any{})
	if stop.Code != 200 || !strings.Contains(stop.Body.String(), `"cancelled":true`) {
		t.Fatalf("refresh-time cancel did not stop the registered request: %d %s", stop.Code, stop.Body)
	}
	<-ctx.Done()
	release()
	after, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != "cancelled" {
		t.Fatalf("cancel not persisted after refresh: %q", after.Status)
	}
}
