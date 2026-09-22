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

func TestPageTitleAPIReadsAndWritesWorkspaceConfig(t *testing.T) {
	workspace := t.TempDir()
	legacyConfigPath := filepath.Join(workspace, "vscode_tasks_menu.ini")
	configPath := filepath.Join(workspace, ".vscode", "vscode_tasks_menu.ini")
	initial := "[server]\nbind = 127.0.0.1\n\n[auth]\npassword = keep-me\n"
	if err := os.WriteFile(legacyConfigPath, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}

	putReq := httptest.NewRequest(http.MethodPut, "/api/config/page-title", strings.NewReader(`{"title":"BLE Secure OTA"}`))
	putReq.Header.Set("Content-Type", "application/json")
	putRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(putRR, putReq)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", putRR.Code, putRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/config/page-title", nil)
	getRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var got struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != "BLE Secure OTA" {
		t.Fatalf("title=%q", got.Title)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacyConfigPath); !os.IsNotExist(err) {
		t.Fatalf("legacy config still exists: %v", err)
	}
	if !strings.Contains(string(data), "password = keep-me") || !strings.Contains(string(data), "page_title = BLE Secure OTA") {
		t.Fatalf("config not preserved/updated:\n%s", data)
	}
}

func TestPageTitleAPIRejectsCrossOriginMutation(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "vscode_tasks_menu.ini"), []byte("[server]\nbind = 127.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	req := httptest.NewRequest(http.MethodPut, "http://tool.local/api/config/page-title", strings.NewReader(`{"title":"Bad"}`))
	req.Host = "tool.local"
	req.Header.Set("Origin", "http://evil.local")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusForbidden)
	}
}
