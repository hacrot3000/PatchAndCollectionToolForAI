package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func callWorkspaceSnapshots(t *testing.T, s *Server, method, target, body string, principal *identity.Principal) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if principal != nil {
		req = req.WithContext(context.WithValue(req.Context(), sharedPrincipalContextKey{}, *principal))
	}
	rr := httptest.NewRecorder()
	s.workspaceSnapshots(rr, req)
	return rr
}

func TestWorkspaceSnapshotSaveNormalizesContextAndCapturesTerminalState(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace: workspace}
	terminal := projectTerminalState{
		Version: 5,
		Terminals: []terminalStateItem{
			{SessionID: "live-a", Cwd: workspace, Title: "Debug", BroadcastGroupID: "group-a"},
		},
		ActiveIndex: 0,
		Splits: []terminalSplitState{},
		LiveSplits: []terminalSnapshotSplitRequest{{LeftSessionID: "live-a", RightSessionID: "live-b", Ratio: .5}},
	}
	if err := writeProjectTerminalState(workspace, terminal); err != nil {
		t.Fatal(err)
	}
	longSQL := strings.Repeat("x", (64<<10)+100)
	payload := map[string]any{
		"name": "Debug NFC issue",
		"state": map[string]any{
			"tab_order": []string{"terminal:a", "editor:src/main.go", "terminal:a", strings.Repeat("z", 300)},
			"active_tab": "editor:src/main.go",
			"editor_files": []string{"src/main.go", "../escape", "/tmp/host", "src/main.go"},
			"active_editor": "src/main.go",
			"explorer_expanded": []string{"src", "../escape"},
			"databases": []any{
				map[string]any{
					"profile_id": "db-main",
					"active_query_key": "query-2",
					"queries": []any{
						map[string]any{"key": "query-1", "title": "A", "text": longSQL, "file_path": "sql/a.sql"},
					},
				},
			},
			"transfers": []any{
				map[string]any{"profile_id": "sftp-prod", "local_mode": "project", "local_path": "deploy", "remote_path": "/srv/app"},
			},
			"git_repository_id": ".",
		},
	}
	raw, _ := json.Marshal(payload)
	rr := callWorkspaceSnapshots(t, s, http.MethodPost, "/api/workspace-snapshots", string(raw), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got workspaceSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.Name != "Debug NFC issue" || got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Fatalf("snapshot metadata=%+v", got)
	}
	if len(got.State.EditorFiles) != 1 || got.State.EditorFiles[0] != "src/main.go" || got.State.ActiveEditor != "src/main.go" {
		t.Fatalf("editor state=%+v", got.State)
	}
	if len(got.State.ExplorerExpanded) != 1 || got.State.ExplorerExpanded[0] != "src" {
		t.Fatalf("explorer state=%+v", got.State.ExplorerExpanded)
	}
	if len(got.State.Databases) != 1 || len(got.State.Databases[0].Queries) != 1 || len(got.State.Databases[0].Queries[0].Text) != 64<<10 {
		t.Fatalf("database state=%+v", got.State.Databases)
	}
	if len(got.State.Transfers) != 1 || got.State.Transfers[0].RemotePath != "/srv/app" || got.State.Transfers[0].LocalPath != "deploy" {
		t.Fatalf("transfer state=%+v", got.State.Transfers)
	}
	if len(got.Terminal.Terminals) != 1 || got.Terminal.Terminals[0].SessionID != "" || got.Terminal.Terminals[0].Cwd != workspace {
		t.Fatalf("terminal state=%+v", got.Terminal)
	}
	if len(got.Terminal.LiveSplits) != 0 {
		t.Fatalf("live session split IDs must not be snapshotted: %+v", got.Terminal.LiveSplits)
	}

	path, err := workspaceSnapshotsPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot file mode=%o", info.Mode().Perm())
	}
}

func TestWorkspaceSnapshotCRUDPreservesCreatedAt(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace: workspace}
	first := callWorkspaceSnapshots(t, s, http.MethodPost, "/api/workspace-snapshots",
		`{"name":"One","state":{"editor_files":["a.txt"],"active_editor":"a.txt"}}`, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", first.Code, first.Body.String())
	}
	var created workspaceSnapshot
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	updateBody, _ := json.Marshal(workspaceSnapshotSaveRequest{
		ID: created.ID, Name: "Renamed",
		State: workspaceSnapshotClientState{EditorFiles: []string{"b.txt"}, ActiveEditor: "b.txt"},
	})
	updatedRR := callWorkspaceSnapshots(t, s, http.MethodPut, "/api/workspace-snapshots", string(updateBody), nil)
	if updatedRR.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updatedRR.Code, updatedRR.Body.String())
	}
	var updated workspaceSnapshot
	if err := json.Unmarshal(updatedRR.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.CreatedAt != created.CreatedAt || updated.ID != created.ID || updated.Name != "Renamed" {
		t.Fatalf("updated=%+v created=%+v", updated, created)
	}

	list := callWorkspaceSnapshots(t, s, http.MethodGet, "/api/workspace-snapshots", "", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), created.ID) || !strings.Contains(list.Body.String(), "Renamed") {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}

	del := callWorkspaceSnapshots(t, s, http.MethodDelete, "/api/workspace-snapshots?id="+created.ID, "", nil)
	if del.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", del.Code, del.Body.String())
	}
	list = callWorkspaceSnapshots(t, s, http.MethodGet, "/api/workspace-snapshots", "", nil)
	if strings.Contains(list.Body.String(), created.ID) {
		t.Fatalf("deleted snapshot remains: %s", list.Body.String())
	}
}

func TestWorkspaceSnapshotsAreIsolatedPerSharedUser(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace: workspace}
	s.Config.SharedServerEnabled = true
	alice := identity.Principal{UserID: "alice", ProjectID: "project"}
	bob := identity.Principal{UserID: "bob", ProjectID: "project"}

	saveAlice := callWorkspaceSnapshots(t, s, http.MethodPost, "/api/workspace-snapshots",
		`{"name":"Alice private","state":{"editor_files":["alice.txt"]}}`, &alice)
	if saveAlice.Code != http.StatusOK {
		t.Fatalf("alice save status=%d body=%s", saveAlice.Code, saveAlice.Body.String())
	}
	saveBob := callWorkspaceSnapshots(t, s, http.MethodPost, "/api/workspace-snapshots",
		`{"name":"Bob private","state":{"editor_files":["bob.txt"]}}`, &bob)
	if saveBob.Code != http.StatusOK {
		t.Fatalf("bob save status=%d body=%s", saveBob.Code, saveBob.Body.String())
	}

	aliceList := callWorkspaceSnapshots(t, s, http.MethodGet, "/api/workspace-snapshots", "", &alice)
	if !strings.Contains(aliceList.Body.String(), "Alice private") || strings.Contains(aliceList.Body.String(), "Bob private") {
		t.Fatalf("alice list leaked bob snapshot: %s", aliceList.Body.String())
	}
	bobList := callWorkspaceSnapshots(t, s, http.MethodGet, "/api/workspace-snapshots", "", &bob)
	if !strings.Contains(bobList.Body.String(), "Bob private") || strings.Contains(bobList.Body.String(), "Alice private") {
		t.Fatalf("bob list leaked alice snapshot: %s", bobList.Body.String())
	}

	var aliceSaved workspaceSnapshot
	if err := json.Unmarshal(saveAlice.Body.Bytes(), &aliceSaved); err != nil {
		t.Fatal(err)
	}
	deleteAsBob := callWorkspaceSnapshots(t, s, http.MethodDelete, "/api/workspace-snapshots?id="+aliceSaved.ID, "", &bob)
	if deleteAsBob.Code != http.StatusNotFound {
		t.Fatalf("bob delete alice status=%d body=%s", deleteAsBob.Code, deleteAsBob.Body.String())
	}

	path, _ := workspaceSnapshotsPath(workspace)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("project:project:user:alice")) || !bytes.Contains(data, []byte("project:project:user:bob")) {
		t.Fatalf("stored owner keys missing: %s", data)
	}
}

func TestWorkspaceSnapshotRejectsInvalidNameUnknownUpdateAndMissingOwner(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace: workspace}
	for _, body := range []string{
		`{"name":"","state":{}}`,
		"{\"name\":\"bad\\nname\",\"state\":{}}",
	} {
		rr := callWorkspaceSnapshots(t, s, http.MethodPost, "/api/workspace-snapshots", body, nil)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("invalid name status=%d body=%s request=%s", rr.Code, rr.Body.String(), body)
		}
	}
	unknown := callWorkspaceSnapshots(t, s, http.MethodPut, "/api/workspace-snapshots",
		`{"id":"missing","name":"Missing","state":{}}`, nil)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown update status=%d body=%s", unknown.Code, unknown.Body.String())
	}

	s.Config.SharedServerEnabled = true
	missingOwner := callWorkspaceSnapshots(t, s, http.MethodGet, "/api/workspace-snapshots", "", nil)
	if missingOwner.Code != http.StatusUnauthorized {
		t.Fatalf("missing shared owner status=%d body=%s", missingOwner.Code, missingOwner.Body.String())
	}
}
