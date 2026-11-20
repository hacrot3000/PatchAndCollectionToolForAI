package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentTaskManifestValidateIsPlanningOnlyAndReadOnly(t *testing.T) {
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := patchtool.EnsureProjectIdentity(s.Workspace); err != nil {
		t.Fatal(err)
	}
	md := "# Plan\n## Task A — API\nDepends on: None\nAcceptance:\n- responds\n## Task B — UI\nDepends on: A\nAcceptance:\n- renders\n"
	calls := 0
	s.AgentChatCall = func(context.Context, agentchat.ProviderConfig, string, []byte) (agentchat.ResponsesResult, error) {
		calls++
		return agentchat.ResponsesResult{}, nil
	}
	w := invokeAI(t, s.agentTaskManifestValidateAPI, http.MethodPost, "/api/agent/task-manifests/validate", map[string]string{"markdown": md})
	if w.Code != 200 {
		t.Fatalf("validate failed: %d %s", w.Code, w.Body)
	}
	for _, want := range []string{`"planning_only":true`, `"execution_enabled":false`, `"valid":true`, `"topological_order":["A","B"]`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("response missing %s: %s", want, w.Body)
		}
	}
	if calls != 0 {
		t.Fatal("validation called an AI provider")
	}
	if req := httptest.NewRequest(http.MethodPost, "/api/agent/task-manifests/validate", nil); len(sharedRoutePermissions(req)) != 1 || sharedRoutePermissions(req)[0] != identity.PermissionSettingsRead {
		t.Fatalf("route permission: %+v", sharedRoutePermissions(req))
	}
}

func TestAgentTaskManifestValidateRejectsMalformedAndOversized(t *testing.T) {
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := patchtool.EnsureProjectIdentity(s.Workspace); err != nil {
		t.Fatal(err)
	}
	bad := invokeAI(t, s.agentTaskManifestValidateAPI, http.MethodPost, "/api/agent/task-manifests/validate", map[string]string{"markdown": "# P\n## Task A — a\nDepends on: MISSING\nAcceptance:\n- x\n"})
	if bad.Code != 200 || !strings.Contains(bad.Body.String(), `"valid":false`) || !strings.Contains(bad.Body.String(), "unknown_dependency") {
		t.Fatalf("unknown dep not diagnosed: %d %s", bad.Code, bad.Body)
	}
	tooBig := invokeAI(t, s.agentTaskManifestValidateAPI, http.MethodPost, "/api/agent/task-manifests/validate", map[string]string{"markdown": strings.Repeat("x", (1<<20)+1)})
	if tooBig.Code != 413 {
		t.Fatalf("oversize markdown accepted: %d", tooBig.Code)
	}
}

func TestAgentTaskManifestValidateCanonicalJSON(t *testing.T) {
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := patchtool.EnsureProjectIdentity(s.Workspace); err != nil {
		t.Fatal(err)
	}
	id, err := patchtool.EnsureProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	canonical := `{"schema_version":1,"name":"p","project_id":"` + id.ID + `","tasks":[{"id":"A","title":"api","dependencies":[],"acceptance_criteria":["ok"]}]}`
	w := invokeAI(t, s.agentTaskManifestValidateAPI, http.MethodPost, "/api/agent/task-manifests/validate", map[string]string{"format": "canonical_json", "canonical_json": canonical})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"valid":true`) {
		t.Fatalf("canonical validation failed: %d %s", w.Code, w.Body)
	}
	wrong := strings.Replace(canonical, id.ID, "other-project", 1)
	w = invokeAI(t, s.agentTaskManifestValidateAPI, http.MethodPost, "/api/agent/task-manifests/validate", map[string]string{"format": "canonical_json", "canonical_json": wrong})
	if w.Code != 400 {
		t.Fatalf("cross-project canonical JSON accepted: %d %s", w.Code, w.Body)
	}
}
