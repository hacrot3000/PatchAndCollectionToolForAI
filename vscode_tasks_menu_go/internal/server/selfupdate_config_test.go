package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
)

func TestSelfUpdateConfigAPIReadsWritesBranchAndPreservesINI(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, ".vscode", "vscode_tasks_menu.ini")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "[server]\nprotocol = https\nbind = 127.0.0.1\n\n[auth]\npassword = keep-me\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}

	get := httptest.NewRequest(http.MethodGet, "/api/config/self-update", nil)
	getRR := httptest.NewRecorder()
	s.selfUpdateConfig(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var initial config.SelfUpdateSettings
	if err := json.Unmarshal(getRR.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Branch != "main" {
		t.Fatalf("default branch=%q want main", initial.Branch)
	}

	put := httptest.NewRequest(http.MethodPut, "/api/config/self-update", strings.NewReader(`{"branch":"feat/update-test","run_full_validation_tests":true}`))
	put.Header.Set("Content-Type", "application/json")
	putRR := httptest.NewRecorder()
	s.selfUpdateConfig(putRR, put)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", putRR.Code, putRR.Body.String())
	}
	var saved config.SelfUpdateSettings
	if err := json.Unmarshal(putRR.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Branch != "feat/update-test" || !saved.RunFullValidationTests {
		t.Fatalf("saved settings=%+v", saved)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"protocol = https",
		"password = keep-me",
		"[self_update]",
		"branch = feat/update-test",
		"run_full_validation_tests = true",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("config missing %q:\n%s", want, text)
		}
	}
}

func TestSelfUpdateConfigAPIRejectsUnsafeBranch(t *testing.T) {
	s := &Server{Workspace: t.TempDir()}
	req := httptest.NewRequest(http.MethodPut, "/api/config/self-update", strings.NewReader(`{"branch":"../main"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.selfUpdateConfig(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unsafe branch status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
}

func TestSelfUpdateConfigAPIRejectsUnknownAction(t *testing.T) {
	s := &Server{Workspace: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/config/self-update?action=unknown", nil)
	rr := httptest.NewRecorder()
	s.selfUpdateConfig(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown action status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
}
