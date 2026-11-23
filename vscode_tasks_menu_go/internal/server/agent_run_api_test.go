package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

type fakeAgentRunExecutor struct {
	started chan agentchat.RunSpec
	owners  chan string
}

func (e *fakeAgentRunExecutor) Execute(ctx context.Context, spec agentchat.RunSpec, _ []agentchat.HistoryItem, _ agentchat.EventSink) error {
	e.started <- spec
	if e.owners != nil {
		owner, _ := agentrun.PrincipalOwner(ctx)
		e.owners <- owner
	}
	return nil
}
func (*fakeAgentRunExecutor) Cancel(context.Context, agentchat.RunSpec) error { return nil }

func prepareRunAPIServer(t *testing.T, gitRepo bool) (*Server, agentchat.Session, agentchat.ProviderConfig) {
	t.Helper()
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	identity, err := patchtool.EnsureProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	if gitRepo {
		commands := [][]string{{"init", "-q"}, {"config", "user.email", "test@example.invalid"}, {"config", "user.name", "TaskDeck Test"}}
		for _, args := range commands {
			cmd := exec.Command("git", append([]string{"-C", s.Workspace}, args...)...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
		if err := os.WriteFile(filepath.Join(s.Workspace, "source.txt"), []byte("source\n"), 0600); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "initial"}} {
			cmd := exec.Command("git", append([]string{"-C", s.Workspace}, args...)...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
	}
	provider := agentchat.ProviderConfig{ID: "test-provider", Name: "Test", APIType: "responses", BaseURL: "https://api.example.test", Models: []agentchat.ModelConfig{{ID: "model-a", Name: "Model A"}}}
	providers, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	if err := providers.Put("local", provider); err != nil {
		t.Fatal(err)
	}
	if err := s.ConnectionSecrets.Put(agentProviderSecretID(s.Workspace, "local", provider.ID), []byte("TEST-API-KEY")); err != nil {
		t.Fatal(err)
	}
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	session, err := agentchat.NewSession("local", s.Workspace, identity.ID, provider.ID, provider.Models[0].ID, agentchat.Stateless)
	if err != nil {
		t.Fatal(err)
	}
	session, err = store.Save(session, 0)
	if err != nil {
		t.Fatal(err)
	}
	s.AgentRunExecutor = &fakeAgentRunExecutor{started: make(chan agentchat.RunSpec, 4), owners: make(chan string, 4)}
	return s, session, provider
}

func assertWorktreeDoesNotContainSecret(t *testing.T, root, secret string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(contents), secret) {
			t.Errorf("secret was copied into worktree file %s", filepath.Base(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func runSpecFor(s *Server, session agentchat.Session, provider agentchat.ProviderConfig) agentchat.RunSpec {
	ownerID, _ := agentrun.WorkspaceID(s.Workspace)
	return agentchat.RunSpec{RunID: "run-1", ConversationID: session.ID, OwnerWorkspaceID: ownerID,
		ExecutionWorkspaceID: ownerID, ProviderID: provider.ID, ModelID: provider.Models[0].ID,
		ProviderConfigVersion: agentchat.RunProviderConfigVersion(provider, provider.Models[0]), ContextSnapshotID: "ctx-1", AttemptID: "attempt-1", IdempotencyKey: "idem-1"}
}

func waitForAgentRunStatus(t *testing.T, manager *agentrun.Manager, runID string, want agentrun.Status) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if record, ok := manager.Get(runID); ok && record.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	record, _ := manager.Get(runID)
	t.Fatalf("Agent Run status=%s, want %s", record.Status, want)
}

func TestAgentRunAPIUsesOwnerConversationAndImmutableRunBinding(t *testing.T) {
	s, session, provider := prepareRunAPIServer(t, false)
	spec := runSpecFor(s, session, provider)
	snapshotResponse := invokeAI(t, s.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{"conversation_id": session.ID, "prompt": "inspect this task"})
	if snapshotResponse.Code != http.StatusCreated {
		t.Fatalf("snapshot create: %d %s", snapshotResponse.Code, snapshotResponse.Body)
	}
	var snapshot struct {
		ID string `json:"context_snapshot_id"`
	}
	if err := json.Unmarshal(snapshotResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	spec.ContextSnapshotID = snapshot.ID
	w := invokeAI(t, s.agentRunAPI, http.MethodPost, "/api/agent/runs", map[string]any{"spec": spec, "confirm_external": true})
	if w.Code != http.StatusAccepted {
		t.Fatalf("run submit: %d %s", w.Code, w.Body)
	}
	select {
	case started := <-s.AgentRunExecutor.(*fakeAgentRunExecutor).started:
		if started != spec {
			t.Fatalf("executor binding changed: %+v", started)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run executor was not invoked")
	}
	select {
	case owner := <-s.AgentRunExecutor.(*fakeAgentRunExecutor).owners:
		if owner != "local" {
			t.Fatalf("provider owner context=%q, want local", owner)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("principal owner was not bound to provider context")
	}
	manager := s.agentRunManagers["local"]
	if manager == nil {
		t.Fatal("run manager was not created")
	}
	defer manager.Close()
	waitForAgentRunStatus(t, manager, spec.RunID, agentrun.Completed)
	if _, ok := manager.Get(spec.RunID); !ok {
		t.Fatal("run was not persisted")
	}
	inputs, err := s.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(context.Background(), "local"), spec)
	if err != nil || inputs.ExecutionWorkspacePath != s.Workspace || inputs.Snapshot.ID != spec.ContextSnapshotID {
		t.Fatalf("owner workspace binding did not resolve: %+v %v", inputs, err)
	}
	badSpec := spec
	badSpec.RunID = "run-foreign"
	badSpec.ExecutionWorkspaceID = "unregistered"
	w = invokeAI(t, s.agentRunAPI, http.MethodPost, "/api/agent/runs", map[string]any{"spec": badSpec, "confirm_external": true})
	if w.Code != http.StatusForbidden {
		t.Fatalf("unregistered execution workspace accepted: %d %s", w.Code, w.Body)
	}
}

func TestAgentRunRequiresExternalConsentAndOwnerSession(t *testing.T) {
	s, session, provider := prepareRunAPIServer(t, false)
	spec := runSpecFor(s, session, provider)
	w := invokeAI(t, s.agentRunAPI, http.MethodPost, "/api/agent/runs", map[string]any{"spec": spec})
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing consent accepted: %d", w.Code)
	}
	spec.ConversationID = "missing-conversation"
	w = invokeAI(t, s.agentRunAPI, http.MethodPost, "/api/agent/runs", map[string]any{"spec": spec, "confirm_external": true})
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown owner conversation accepted: %d", w.Code)
	}
}

func TestAgentRunWorktreeEndpointRegistersDetachedWorkspace(t *testing.T) {
	s, session, provider := prepareRunAPIServer(t, true)
	w := invokeAI(t, func(w http.ResponseWriter, r *http.Request) { s.agentRunWorktreeAPI(w, r, "local") }, http.MethodPost, "/api/agent/runs/worktrees", map[string]any{
		"conversation_id": session.ID, "run_id": "task-one", "provider_id": provider.ID,
		"model_id": provider.Models[0].ID, "provider_config_version": agentchat.RunProviderConfigVersion(provider, provider.Models[0]),
		"permission_scope": []string{"files.read", "patch.propose", "patch.execute.approval_required"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("worktree creation: %d %s", w.Code, w.Body)
	}
	var result struct {
		WorkspaceID string `json:"workspace_id"`
		BaseHEAD    string `json:"base_head"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.WorkspaceID == "" || result.BaseHEAD == "" {
		t.Fatalf("incomplete worktree response: %+v", result)
	}
	registry, err := s.agentWorkspaceRegistryForOwner("local")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := registry.Resolve(context.Background(), result.WorkspaceID, session.ID, "task-one", session.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.WorktreePath == s.Workspace {
		t.Fatal("task worktree resolved to owner source directory")
	}
	if _, err := os.Stat(filepath.Join(binding.WorktreePath, ".python_patch_tool.json")); err != nil {
		t.Fatal("owner project identity was not bound to child worktree")
	}
	assertWorktreeDoesNotContainSecret(t, binding.WorktreePath, "TEST-API-KEY")
	snapshotResponse := invokeAI(t, s.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{"conversation_id": session.ID, "prompt": "work in the child worktree"})
	if snapshotResponse.Code != http.StatusCreated {
		t.Fatalf("snapshot create: %d %s", snapshotResponse.Code, snapshotResponse.Body)
	}
	var snapshot struct {
		ID string `json:"context_snapshot_id"`
	}
	if err := json.Unmarshal(snapshotResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	spec := runSpecFor(s, session, provider)
	spec.RunID, spec.ExecutionWorkspaceID, spec.ContextSnapshotID = "task-one", result.WorkspaceID, snapshot.ID
	inputs, err := s.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(context.Background(), "local"), spec)
	if err != nil || inputs.ExecutionWorkspacePath != binding.WorktreePath || inputs.OwnerWorkspacePath != s.Workspace {
		t.Fatalf("child worktree binding did not resolve: %+v %v", inputs, err)
	}
	var packPath string
	s.AgentChatPack = func(workspace, _ string) (patchtool.AIPackResult, error) {
		packPath = workspace
		identity, err := patchtool.ReadProjectIdentity(workspace)
		return patchtool.AIPackResult{ProjectIdentity: identity, TextContent: "child context"}, err
	}
	if _, err := s.BuildAgentRunWorkspacePack(agentrun.WithPrincipalOwner(context.Background(), "local"), spec); err != nil {
		t.Fatal(err)
	}
	if packPath != binding.WorktreePath {
		t.Fatalf("AI Pack read %q; want execution worktree %q", packPath, binding.WorktreePath)
	}
	if err := os.WriteFile(filepath.Join(binding.WorktreePath, "source.txt"), []byte("worktree-only change\\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if _, err := manager.Submit(spec, inputs.Snapshot.History); err != nil {
		t.Fatal(err)
	}
	waitForAgentRunStatus(t, manager, spec.RunID, agentrun.Completed)
	reportResponse := invokeAI(t, s.agentRunItemAPI, http.MethodGet, "/api/agent/runs/task-one/integration-report", nil)
	if reportResponse.Code != http.StatusOK || !strings.Contains(reportResponse.Body.String(), `"approval_required":true`) || !strings.Contains(reportResponse.Body.String(), `"branch_mutation_performed":false`) || !strings.Contains(reportResponse.Body.String(), `source.txt`) {
		t.Fatalf("integration report response: %d %s", reportResponse.Code, reportResponse.Body)
	}
}
