package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverNestedGitRepositoriesAndSelectRepo(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	nested := filepath.Join(workspace, "projects", "client")
	if err := os.MkdirAll(nested, 0o755); err != nil { t.Fatal(err) }
	gitQuickRun(t, nested, "init")
	gitQuickRun(t, nested, "config", "user.name", "Task Menu Test")
	gitQuickRun(t, nested, "config", "user.email", "task-menu@example.invalid")
	if err := os.WriteFile(filepath.Join(nested, "client.txt"), []byte("client\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, nested, "add", "client.txt")
	gitQuickRun(t, nested, "commit", "-m", "client initial")

	ignored := filepath.Join(workspace, "node_modules", "ignored-repo")
	if err := os.MkdirAll(ignored, 0o755); err != nil { t.Fatal(err) }
	gitQuickRun(t, ignored, "init")

	repos, settings, err := s.discoverGitRepositories(true)
	if err != nil { t.Fatal(err) }
	if !settings.ScanEnabled || settings.ScanDepth != 4 {
		t.Fatalf("settings=%+v", settings)
	}
	seen := map[string]gitRepository{}
	for _, item := range repos {
		seen[item.ID] = item
	}
	if _, ok := seen["."]; !ok {
		t.Fatalf("root repository missing: %+v", repos)
	}
	if got, ok := seen["projects/client"]; !ok || got.Root != nested {
		t.Fatalf("nested repository missing/wrong: %+v", repos)
	}
	if _, ok := seen["node_modules/ignored-repo"]; ok {
		t.Fatalf("ignored repository unexpectedly discovered: %+v", repos)
	}

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?repo=projects%2Fclient", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("selected repo status=%d body=%s", rr.Code, rr.Body.String())
	}
	var status gitStatusResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil { t.Fatal(err) }
	if !status.Repository || status.RepoID != "projects/client" || status.Head == "" {
		t.Fatalf("status=%+v", status)
	}

	if err := os.WriteFile(filepath.Join(nested, "client.txt"), []byte("client changed\n"), 0o644); err != nil { t.Fatal(err) }
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status?repo=projects%2Fclient", `{"action":"stage_all"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("nested stage status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, nested, "diff", "--cached", "--name-only"); got != "client.txt" {
		t.Fatalf("nested staged files=%q", got)
	}
}

func TestGitRepositoryConfigNamesAndRepositoriesView(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	nested := filepath.Join(workspace, "projects", "server")
	if err := os.MkdirAll(nested, 0o755); err != nil { t.Fatal(err) }
	gitQuickRun(t, nested, "init")
	gitQuickRun(t, nested, "config", "user.name", "Task Menu Test")
	gitQuickRun(t, nested, "config", "user.email", "task-menu@example.invalid")
	if err := os.WriteFile(filepath.Join(nested, "server.txt"), []byte("server\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, nested, "add", "server.txt")
	gitQuickRun(t, nested, "commit", "-m", "server initial")

	configPath := filepath.Join(workspace, ".vscode", "vscode_tasks_menu.ini")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil { t.Fatal(err) }
	cfg := "[git]\nscan_enabled = true\nscan_depth = 4\ndefault_repository = projects/server\nauto_select_from_terminal_cwd = true\n\n[git.repositories]\nM3 Server = projects/server\n"
	if err := os.WriteFile(configPath, []byte(cfg), 0o600); err != nil { t.Fatal(err) }

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=repositories&refresh=1", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("repositories status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Repositories []gitRepositoryStatus `json:"repositories"`
		DefaultRepository string `json:"default_repository"`
		AutoSelect bool `json:"auto_select_from_terminal_cwd"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil { t.Fatal(err) }
	if payload.DefaultRepository != "projects/server" || !payload.AutoSelect {
		t.Fatalf("payload=%+v", payload)
	}
	found := false
	for _, item := range payload.Repositories {
		if item.ID == "projects/server" {
			found = true
			if item.Name != "M3 Server" || !item.Default || item.Head == "" {
				t.Fatalf("configured repo=%+v", item)
			}
		}
	}
	if !found {
		t.Fatalf("configured repository missing: %+v", payload.Repositories)
	}
}

func TestUnknownGitRepositoryIDIsRejected(t *testing.T) {
	_, s, _ := setupGitQuickRepo(t)
	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=branches&repo=..%2Foutside", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}


func TestGitWorkspaceRootSurvivesDisabledNestedScan(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	configPath := filepath.Join(workspace, ".vscode", "vscode_tasks_menu.ini")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(configPath, []byte("[git]\nscan_enabled = false\n"), 0o600); err != nil { t.Fatal(err) }

	repos, settings, err := s.discoverGitRepositories(true)
	if err != nil { t.Fatal(err) }
	if settings.ScanEnabled {
		t.Fatalf("scan_enabled=%v, want false", settings.ScanEnabled)
	}
	if len(repos) != 1 || repos[0].ID != "." {
		t.Fatalf("repos=%+v, want workspace root only", repos)
	}
}

func TestInitializedSubmoduleIsDiscoveredWhenNestedScanDisabled(t *testing.T) {
	workspace, s, _, _, _ := setupGitSubmoduleRepo(t)
	configPath := filepath.Join(workspace, ".vscode", "vscode_tasks_menu.ini")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("[git]\nscan_enabled = false\nscan_depth = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	repos, settings, err := s.discoverGitRepositories(true)
	if err != nil {
		t.Fatal(err)
	}
	if settings.ScanEnabled {
		t.Fatalf("scan_enabled=%v want false", settings.ScanEnabled)
	}
	seen := map[string]gitRepository{}
	for _, item := range repos {
		seen[item.ID] = item
	}
	if _, ok := seen["."]; !ok {
		t.Fatalf("root repository missing: %+v", repos)
	}
	child, ok := seen["modules/child"]
	if !ok {
		t.Fatalf("initialized submodule repository missing with scan disabled: %+v", repos)
	}
	if child.Name == "" || child.Root != filepath.Join(workspace, "modules", "child") {
		t.Fatalf("submodule repository=%+v", child)
	}
}
