package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestTaskWorkflowSecretRefsUsesReachableNodesOnly(t *testing.T) {
	items := []tasks.Task{
		{Label:"Root", Raw:map[string]any{"dependsOn":[]any{"Build"}}, SecretEnv:map[string]string{"DEPLOY_TOKEN":"deploy/root"}},
		{Label:"Build", Raw:map[string]any{}, SecretEnv:map[string]string{"DB_PASSWORD":"database/build"}},
		{Label:"Unrelated", Raw:map[string]any{}, SecretEnv:map[string]string{"UNUSED_SECRET":"generic/unrelated"}},
	}
	refs, err := taskWorkflowSecretRefs(items, "Root")
	if err != nil { t.Fatal(err) }
	if len(refs) != 2 || refs["DEPLOY_TOKEN"] != "deploy/root" || refs["DB_PASSWORD"] != "database/build" {
		t.Fatalf("refs=%v", refs)
	}
	if _, ok := refs["UNUSED_SECRET"]; ok {
		t.Fatalf("unreachable task secret included: %v", refs)
	}
}

func TestTaskWorkflowSecretRefsRejectsConflictingEnvironment(t *testing.T) {
	items := []tasks.Task{
		{Label:"Root", Raw:map[string]any{"dependsOn":[]any{"Build"}}, SecretEnv:map[string]string{"TOKEN":"deploy/root"}},
		{Label:"Build", Raw:map[string]any{}, SecretEnv:map[string]string{"TOKEN":"deploy/build"}},
	}
	if _, err := taskWorkflowSecretRefs(items, "Root"); err == nil || !strings.Contains(err.Error(), "maps to multiple secret ids") {
		t.Fatalf("err=%v", err)
	}
}

func TestInjectTaskSecretsOverridesNormalEnvWithoutPreviewLeak(t *testing.T) {
	store, err := secretstore.NewFileStore(filepath.Join(t.TempDir(), "secrets"))
	if err != nil { t.Fatal(err) }
	if err := store.Put("deploy/prod/token", []byte("top-secret-value")); err != nil { t.Fatal(err) }
	s := &Server{ConnectionSecrets:store}
	spec := tasks.Execution{
		Command:"/bin/sh", Args:[]string{"-c","echo ok"}, Preview:"echo ok",
		Env:[]string{"PATH=/usr/bin","DEPLOY_TOKEN=browser-value"},
	}
	if err := s.injectTaskSecrets(&spec, map[string]string{"DEPLOY_TOKEN":"deploy/prod/token"}); err != nil { t.Fatal(err) }
	found := false
	for _, item := range spec.Env {
		if item == "DEPLOY_TOKEN=top-secret-value" { found = true }
		if item == "DEPLOY_TOKEN=browser-value" { t.Fatalf("browser env overrode managed secret: %v", spec.Env) }
	}
	if !found { t.Fatalf("managed secret missing from execution env: %v", spec.Env) }
	if strings.Contains(spec.Preview, "top-secret-value") {
		t.Fatalf("preview leaked secret: %q", spec.Preview)
	}
	public, err := json.Marshal(spec)
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(public), "top-secret-value") {
		t.Fatalf("execution JSON leaked secret: %s", public)
	}
}

func TestAuthorizeTaskSecretUseRequiresExactSharedPermission(t *testing.T) {
	s := &Server{}
	refs := map[string]string{"TOKEN":"deploy/prod/token"}

	denied := identity.Principal{UserID:"alice",ProjectID:"project",Permissions:map[string]bool{identity.PermissionTasksRun:true}}
	req := sharedSessionRequest(http.MethodPost, "/api/sessions", "", denied)
	rr := httptest.NewRecorder()
	if s.authorizeTaskSecretUse(rr, req, refs, "Deploy") {
		t.Fatal("task secret use accepted without secrets.use")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("deny status=%d body=%s", rr.Code, rr.Body.String())
	}

	allowed := denied
	allowed.Permissions = map[string]bool{
		identity.PermissionTasksRun:true,
		identity.PermissionSecretsUse:true,
	}
	req = sharedSessionRequest(http.MethodPost, "/api/sessions", "", allowed)
	rr = httptest.NewRecorder()
	if !s.authorizeTaskSecretUse(rr, req, refs, "Deploy") {
		t.Fatalf("task secret use denied with secrets.use status=%d body=%s", rr.Code, rr.Body.String())
	}
}
