package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalCWDConfigAPIReadsWritesAndPreservesINI(t *testing.T) {
	workspace := t.TempDir()
	for _, dir := range []string{"patch", "tools"} {
		if err := os.MkdirAll(filepath.Join(workspace, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	legacyConfigPath := filepath.Join(workspace, "vscode_tasks_menu.ini")
	configPath := filepath.Join(workspace, ".vscode", "vscode_tasks_menu.ini")
	if err := os.WriteFile(legacyConfigPath, []byte("[server]\nbind = 127.0.0.1\n\n[auth]\npassword = keep-me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}

	put := httptest.NewRequest(http.MethodPut, "/api/config/terminal-cwds", strings.NewReader(`{"selected_cwd":"patch","custom_dirs":["patch","tools"]}`))
	put.Header.Set("Content-Type", "application/json")
	putRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(putRR, put)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", putRR.Code, putRR.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/config/terminal-cwds", nil)
	getRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var got struct {
		SelectedCWD string   `json:"selected_cwd"`
		CustomDirs  []string `json:"custom_dirs"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SelectedCWD != "patch" || len(got.CustomDirs) != 2 {
		t.Fatalf("settings=%+v", got)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacyConfigPath); !os.IsNotExist(err) {
		t.Fatalf("legacy config still exists: %v", err)
	}
	if !strings.Contains(string(data), "password = keep-me") || !strings.Contains(string(data), "[terminal]") {
		t.Fatalf("config not preserved:\n%s", data)
	}
}

func TestTerminalCWDConfigAPIRejectsMissingOrEscapingDirectory(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace: workspace}
	for _, body := range []string{
		`{"selected_cwd":"missing","custom_dirs":["missing"]}`,
		`{"selected_cwd":"../outside","custom_dirs":["../outside"]}`,
	} {
		req := httptest.NewRequest(http.MethodPut, "/api/config/terminal-cwds", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d want 400; response=%s", body, rr.Code, rr.Body.String())
		}
	}
}
