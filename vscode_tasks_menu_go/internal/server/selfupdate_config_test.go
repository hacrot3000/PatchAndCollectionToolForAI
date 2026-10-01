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

func TestSelfUpdateConfigAPIReadsWritesAndPreservesINI(t *testing.T) {
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

	put := httptest.NewRequest(http.MethodPut, "/api/config/self-update", strings.NewReader(`{"run_full_validation_tests":true}`))
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
	if !saved.RunFullValidationTests {
		t.Fatal("full validation preference was not saved")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"[server]",
		"protocol = https",
		"[auth]",
		"password = keep-me",
		"[self_update]",
		"run_full_validation_tests = true",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("config missing %q:\n%s", want, text)
		}
	}

	get := httptest.NewRequest(http.MethodGet, "/api/config/self-update", nil)
	getRR := httptest.NewRecorder()
	s.selfUpdateConfig(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var got config.SelfUpdateSettings
	if err := json.Unmarshal(getRR.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != saved {
		t.Fatalf("GET=%+v want %+v", got, saved)
	}

	putOff := httptest.NewRequest(http.MethodPut, "/api/config/self-update", strings.NewReader(`{"run_full_validation_tests":false}`))
	putOff.Header.Set("Content-Type", "application/json")
	putOffRR := httptest.NewRecorder()
	s.selfUpdateConfig(putOffRR, putOff)
	if putOffRR.Code != http.StatusOK {
		t.Fatalf("PUT false status=%d body=%s", putOffRR.Code, putOffRR.Body.String())
	}
	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text = string(data)
	if strings.Count(text, "run_full_validation_tests") != 1 || !strings.Contains(text, "run_full_validation_tests = false") {
		t.Fatalf("self-update setting not replaced cleanly:\n%s", text)
	}
}

func TestSelfUpdateConfigAPIDefaultsFullValidationOff(t *testing.T) {
	s := &Server{Workspace: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/config/self-update", nil)
	rr := httptest.NewRecorder()
	s.selfUpdateConfig(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET default status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got config.SelfUpdateSettings
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RunFullValidationTests {
		t.Fatal("full validation must default to disabled")
	}
}
