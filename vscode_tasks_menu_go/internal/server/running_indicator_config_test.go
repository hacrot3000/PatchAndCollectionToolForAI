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

func TestRunningIndicatorConfigAPIReadsWritesAndPreservesINI(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, ".vscode", "vscode_tasks_menu.ini")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "[server]\nbind = 127.0.0.1\n\n[auth]\npassword = keep-me\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}

	put := httptest.NewRequest(http.MethodPut, "/api/config/running-indicator", strings.NewReader(`{"mode":"braille","rpm":2.5}`))
	put.Header.Set("Content-Type", "application/json")
	putRR := httptest.NewRecorder()
	s.runningIndicatorConfig(putRR, put)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", putRR.Code, putRR.Body.String())
	}
	var saved config.RunningIndicatorSettings
	if err := json.Unmarshal(putRR.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Mode != "braille" || saved.RPM != 3 {
		t.Fatalf("saved=%+v", saved)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"[server]",
		"bind = 127.0.0.1",
		"[auth]",
		"password = keep-me",
		"[appearance]",
		"running_indicator = braille",
		"running_indicator_rpm = 3",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("config missing %q:\n%s", want, text)
		}
	}

	get := httptest.NewRequest(http.MethodGet, "/api/config/running-indicator", nil)
	getRR := httptest.NewRecorder()
	s.runningIndicatorConfig(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var got config.RunningIndicatorSettings
	if err := json.Unmarshal(getRR.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != saved {
		t.Fatalf("GET=%+v want %+v", got, saved)
	}
}

func TestRunningIndicatorConfigAPIRejectsInvalidModeAndRPM(t *testing.T) {
	s := &Server{Workspace: t.TempDir()}
	for _, body := range []string{
		`{"mode":"unknown","rpm":2}`,
		`{"mode":"spinner","rpm":0.01}`,
		`{"mode":"spinner","rpm":121}`,
	} {
		req := httptest.NewRequest(http.MethodPut, "/api/config/running-indicator", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.runningIndicatorConfig(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d want 400 response=%s", body, rr.Code, rr.Body.String())
		}
	}
}

func TestRunningIndicatorConfigDefaultsToBrailleAtTwoRPM(t *testing.T) {
	got, err := config.ReadRunningIndicatorSettings(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != "braille" || got.RPM != 2 {
		t.Fatalf("defaults=%+v", got)
	}
}
