package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func writeWorkspaceTasksFixture(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".vscode")
	if err := os.MkdirAll(dir, 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(dir, "tasks.json"), []byte(body), 0o600); err != nil { t.Fatal(err) }
}

func TestLoadWorkspaceTasksAggregatesAttachedRoots(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "main")
	attached := filepath.Join(base, "client")
	if err := os.MkdirAll(primary, 0o755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(attached, 0o755); err != nil { t.Fatal(err) }
	writeWorkspaceTasksFixture(t, primary, `{"version":"2.0.0","tasks":[{"label":"Build Main","type":"shell","command":"pwd","group":"build"}]}`)
	writeWorkspaceTasksFixture(t, attached, `{"version":"2.0.0","tasks":[{"label":"Build Client","type":"shell","command":"pwd","group":"build"}]}`)
	s := &Server{Workspace: primary}
	create := callWorkspaceRoots(t, s, http.MethodPost, "/api/workspace-roots", `{"path":"../client","name":"Client App"}`)
	if create.Code != http.StatusCreated { t.Fatalf("attach status=%d body=%s", create.Code, create.Body.String()) }
	var root workspaceRootView
	if err := json.Unmarshal(create.Body.Bytes(), &root); err != nil { t.Fatal(err) }

	items, err := s.loadWorkspaceTasks()
	if err != nil { t.Fatal(err) }
	if len(items) != 2 { t.Fatalf("tasks=%+v", items) }
	seen := map[string]bool{}
	var attachedID int
	for _, item := range items {
		seen[item.Label] = true
		if item.Label == "Build Main" {
			if item.WorkspaceRootID != workspacePrimaryRootID || len(item.Group) != 1 || item.Group[0] != "build" {
				t.Fatalf("primary task=%+v", item)
			}
		}
		if item.Label == "Build Client" {
			attachedID = item.ID
			if item.WorkspaceRootID != root.ID || item.WorkspaceRootName != "Client App" {
				t.Fatalf("attached metadata=%+v", item)
			}
			if len(item.Group) < 2 || item.Group[0] != "Client App" || item.Group[1] != "build" {
				t.Fatalf("attached group=%+v", item.Group)
			}
			if !strings.Contains(item.Detail, "Workspace root: Client App") {
				t.Fatalf("attached detail=%q", item.Detail)
			}
		}
	}
	if !seen["Build Main"] || !seen["Build Client"] || attachedID == 0 {
		t.Fatalf("seen=%v attachedID=%d", seen, attachedID)
	}
	selected, selectedRoot, err := s.workspaceTaskByID(attachedID)
	if err != nil { t.Fatal(err) }
	if selected.Label != "Build Client" || selectedRoot.Path != attached {
		t.Fatalf("selected=%+v root=%+v", selected, selectedRoot)
	}
}

func TestAttachedWorkspaceTaskIDIsStableAndRootSpecific(t *testing.T) {
	a := attachedWorkspaceTaskID("root-a", 12345)
	if a <= 0 || a != attachedWorkspaceTaskID("root-a", 12345) { t.Fatalf("unstable id=%d", a) }
	if b := attachedWorkspaceTaskID("root-b", 12345); b == a { t.Fatalf("root-specific ids collided in fixture: %d", a) }
}

func TestTasksAPIIncludesAttachedRootMetadata(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "main")
	attached := filepath.Join(base, "client")
	if err := os.MkdirAll(primary, 0o755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(attached, 0o755); err != nil { t.Fatal(err) }
	writeWorkspaceTasksFixture(t, attached, `{"tasks":[{"label":"Client Task","type":"shell","command":"echo client"}]}`)
	s := &Server{Workspace: primary}
	create := callWorkspaceRoots(t, s, http.MethodPost, "/api/workspace-roots", `{"path":"../client","name":"Client"}`)
	if create.Code != http.StatusCreated { t.Fatal(create.Body.String()) }

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/tasks", nil))
	if rr.Code != http.StatusOK { t.Fatalf("tasks status=%d body=%s", rr.Code, rr.Body.String()) }
	body := rr.Body.String()
	for _, want := range []string{"Client Task", `"workspace_root_id":"root-`, `"workspace_root_name":"Client"`} {
		if !strings.Contains(body, want) { t.Fatalf("tasks body missing %q: %s", want, body) }
	}
}

func TestWorkspaceTaskExecutionUsesAttachedRootAsWorkspaceFolder(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "main")
	attached := filepath.Join(base, "client")
	if err := os.MkdirAll(primary, 0o755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(attached, "tools"), 0o755); err != nil { t.Fatal(err) }
	fixture := `{"tasks":[{"label":"Client PWD","type":"shell","command":"pwd","options":{"cwd":"` + "$" + `{workspaceFolder}/tools"}}]}`
	writeWorkspaceTasksFixture(t, attached, fixture)
	s := &Server{Workspace: primary}
	create := callWorkspaceRoots(t, s, http.MethodPost, "/api/workspace-roots", `{"path":"../client","name":"Client"}`)
	if create.Code != http.StatusCreated { t.Fatal(create.Body.String()) }
	items, err := s.loadWorkspaceTasks()
	if err != nil { t.Fatal(err) }
	if len(items) != 1 { t.Fatalf("tasks=%+v", items) }
	task, root, err := s.workspaceTaskByID(items[0].ID)
	if err != nil { t.Fatal(err) }
	spec, err := tasks.ResolveExecutionWithInputs(task, root.Path, nil)
	if err != nil { t.Fatal(err) }
	want := filepath.Join(attached, "tools")
	if spec.Cwd != want { t.Fatalf("cwd=%q want=%q", spec.Cwd, want) }
}

func TestLoadWorkspaceTasksSkipsRootsWithoutTasksFile(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "main")
	attached := filepath.Join(base, "client")
	if err := os.MkdirAll(primary, 0o755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(attached, 0o755); err != nil { t.Fatal(err) }
	s := &Server{Workspace: primary}
	create := callWorkspaceRoots(t, s, http.MethodPost, "/api/workspace-roots", `{"path":"../client"}`)
	if create.Code != http.StatusCreated { t.Fatal(create.Body.String()) }
	items, err := s.loadWorkspaceTasks()
	if err != nil { t.Fatal(err) }
	if len(items) != 0 { t.Fatalf("tasks=%+v", items) }
}


func TestLoadWorkspaceTasksExposesDependencyInputsOnWorkflowRoot(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceTasksFixture(t, root, `{
		"version":"2.0.0",
		"inputs":[
			{"id":"target","type":"pickString","options":["debug","release"]},
			{"id":"host","type":"promptString"}
		],
		"tasks":[
			{"label":"Build","type":"shell","command":"echo ${input:target}"},
			{"label":"Deploy","type":"shell","command":"echo ${input:host}","dependsOn":"Build"}
		]
	}`)
	s := &Server{Workspace: root}
	items, err := s.loadWorkspaceTasks()
	if err != nil {
		t.Fatal(err)
	}
	var deploy tasks.Task
	for _, item := range items {
		if item.Label == "Deploy" {
			deploy = item
			break
		}
	}
	if deploy.Label == "" {
		t.Fatalf("workflow root missing: %+v", items)
	}
	if len(deploy.Inputs) != 2 || deploy.Inputs[0].ID != "target" || deploy.Inputs[1].ID != "host" {
		t.Fatalf("workflow inputs=%+v", deploy.Inputs)
	}
	selected, workflowItems, selectedRoot, err := s.workspaceWorkflowByTaskID(deploy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Label != "Deploy" || len(workflowItems) != 2 || selectedRoot.Path != root {
		t.Fatalf("workflow selection selected=%+v items=%+v root=%+v", selected, workflowItems, selectedRoot)
	}
}
