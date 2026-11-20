package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestAgentDebugRequestShowsZeroToolsAndRealTimestamp(t *testing.T) {
	s := &Server{}
	store := s.debugStore()
	store.mu.Lock()
	store.owner("local").State.Enabled = true
	store.mu.Unlock()
	recorder, id := s.startAgentDebug("local", "provider_request", "", agentchat.ProviderConfig{}, "m")
	recorder.RecordProviderDiagnostic(agentchat.DiagnosticEvent{Level: "info", Stage: "request_build", Event: "request.built", APIKeyPresent: true})
	w := httptest.NewRecorder()
	s.agentChatDebugTraceAPI(w, httptest.NewRequest("GET", "/api/agent/debug/traces/"+id, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"tool_count":0`) || strings.Contains(w.Body.String(), `0001-01-01`) || !strings.Contains(w.Body.String(), `"api_key_present":true`) {
		t.Fatalf("misleading request diagnostics: %d %s", w.Code, w.Body)
	}
}

func TestAgentDebugErrorIncludesBlockedSharedIP(t *testing.T) {
	err := &agentchat.ProviderError{Code: "PRIVATE_IP_BLOCKED", Stage: "provider_http", BlockedSharedIP: "100.64.0.19"}
	data := debugErrorJSON(err, "trace", "", "", "")
	detail, ok := data["error"].(map[string]any)
	if !ok || detail["blocked_shared_ip"] != "100.64.0.19" {
		t.Fatalf("blocked address missing from error: %#v", data)
	}
}

func TestAgentDebugRoutesUseSettingsPermissions(t *testing.T) {
	cases := []struct {
		method, path string
		want         []string
	}{
		{"GET", "/api/agent/debug/state", []string{"settings.read"}},
		{"POST", "/api/agent/debug/state", []string{"settings.write"}},
		{"GET", "/api/agent/debug/traces", []string{"settings.read"}},
		{"GET", "/api/agent/debug/traces/abc/payload", []string{"settings.read"}},
		{"POST", "/api/agent/debug/clear", []string{"settings.write"}},
		{"POST", "/api/agent/debug/traces/abc/payload", nil},
	}
	for _, test := range cases {
		got := sharedRoutePermissions(httptest.NewRequest(test.method, test.path, nil))
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s %s permissions=%v want=%v", test.method, test.path, got, test.want)
		}
	}
}

func TestAgentDebugSafeAndRawPayloadBoundaries(t *testing.T) {
	s := &Server{}
	store := s.debugStore()
	store.mu.Lock()
	owner := store.owner("local")
	owner.State.Enabled = true
	store.mu.Unlock()
	recorder, traceID := s.startAgentDebug("local", "connection_test", "", agentchat.ProviderConfig{ID: "p", APIType: "responses"}, "m")
	request := []byte(`{"model":"m","instructions":"private project prompt"}`)
	response := []byte(`{"id":"r","output":[{"text":"private answer"}]}`)
	recorder.RecordProviderDiagnostic(agentchat.DiagnosticEvent{Level: "info", Stage: "provider_http", Event: "http.request", RequestBody: request, RequestBytes: len(request)})
	recorder.RecordProviderDiagnostic(agentchat.DiagnosticEvent{Level: "info", Stage: "provider_http", Event: "http.response", ResponseBody: response, ResponseBytes: len(response)})

	safe := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/agent/debug/traces/"+traceID+"/payload?part=request&view=safe", nil)
	s.agentChatDebugTraceAPI(safe, req)
	if safe.Code != 200 || strings.Contains(safe.Body.String(), "private project prompt") {
		t.Fatalf("safe payload leaked body: status=%d body=%s", safe.Code, safe.Body.String())
	}

	otherOwner := httptest.NewRequest("GET", "/api/agent/debug/traces/"+traceID, nil)
	otherOwner = otherOwner.WithContext(context.WithValue(otherOwner.Context(), sharedPrincipalContextKey{}, identity.Principal{UserID: identity.ID("other-user"), Permissions: map[string]bool{identity.PermissionSettingsRead: true}}))
	otherResult := httptest.NewRecorder()
	s.agentChatDebugTraceAPI(otherResult, otherOwner)
	if otherResult.Code != 404 {
		t.Fatalf("cross-owner trace access status=%d body=%s", otherResult.Code, otherResult.Body.String())
	}

	denied := httptest.NewRecorder()
	s.agentChatDebugTraceAPI(denied, httptest.NewRequest("GET", "/api/agent/debug/traces/"+traceID+"/payload?part=request&view=raw", nil))
	if denied.Code != 403 {
		t.Fatalf("RAW view without grant status=%d", denied.Code)
	}

	state := httptest.NewRecorder()
	stateReq := httptest.NewRequest("POST", "/api/agent/debug/state", strings.NewReader(`{"enabled":true,"raw_consent":true}`))
	s.agentChatDebugStateAPI(state, stateReq)
	if state.Code != 200 {
		t.Fatalf("enable RAW: %d %s", state.Code, state.Body.String())
	}
	recorder.RecordProviderDiagnostic(agentchat.DiagnosticEvent{Level: "info", Stage: "provider_http", Event: "http.request", RequestBody: request, RequestBytes: len(request)})
	raw := httptest.NewRecorder()
	s.agentChatDebugTraceAPI(raw, httptest.NewRequest("GET", "/api/agent/debug/traces/"+traceID+"/payload?part=request&view=raw", nil))
	if raw.Code != 200 {
		t.Fatalf("RAW view status=%d body=%s", raw.Code, raw.Body.String())
	}
	var envelope struct {
		Raw string `json:"raw_base64"`
	}
	if err := json.Unmarshal(raw.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(envelope.Raw)
	if err != nil || string(decoded) != string(request) {
		t.Fatalf("RAW bytes mismatch: %q err=%v", decoded, err)
	}

	store.mu.Lock()
	owner.State.RawUntil = time.Now().Add(-time.Second)
	store.mu.Unlock()
	expired := httptest.NewRecorder()
	s.agentChatDebugTraceAPI(expired, httptest.NewRequest("GET", "/api/agent/debug/traces/"+traceID+"/payload?part=request&view=raw", nil))
	if expired.Code != 403 {
		t.Fatalf("expired RAW grant status=%d", expired.Code)
	}
}

func TestAgentDebugRawRequiresWritePermissionInSharedMode(t *testing.T) {
	s := &Server{}
	s.Config.SharedServerEnabled = true
	ownerID := identity.ID("reader")
	ownerName := "user:" + string(ownerID)
	store := s.debugStore()
	traceID := randomAgentTraceID()
	store.mu.Lock()
	o := store.owner(ownerName)
	o.State = agentDebugState{Enabled: true, RawUntil: time.Now().Add(time.Minute)}
	o.Traces = []*agentDebugTrace{{ID: traceID, UpdatedAt: time.Now(), payload: agentDebugPayload{Request: []byte("private"), RequestBytes: 7}}}
	store.mu.Unlock()
	req := httptest.NewRequest("GET", "/api/agent/debug/traces/"+traceID+"/payload?part=request&view=raw", nil)
	req = req.WithContext(context.WithValue(req.Context(), sharedPrincipalContextKey{}, identity.Principal{
		UserID: ownerID, Permissions: map[string]bool{identity.PermissionSettingsRead: true},
	}))
	w := httptest.NewRecorder()
	s.agentChatDebugTraceAPI(w, req)
	if w.Code != 403 {
		t.Fatalf("RAW read without settings-write permission status=%d body=%s", w.Code, w.Body.String())
	}
}
