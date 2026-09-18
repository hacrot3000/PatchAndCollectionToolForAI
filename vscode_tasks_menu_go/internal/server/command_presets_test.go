package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func postCommandPreset(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/command-presets", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.commandPresets(rr, req)
	return rr
}

func decodePresetState(t *testing.T, rr *httptest.ResponseRecorder) projectCommandPresetState {
	t.Helper()
	var state projectCommandPresetState
	if err := json.Unmarshal(rr.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode preset state: %v body=%s", err, rr.Body.String())
	}
	return state
}

func TestCommandPresetCRUDPersistsPerProject(t *testing.T) {
	workspace := t.TempDir()
	srv := &Server{Workspace: workspace}

	create := postCommandPreset(t, srv, `{"action":"create","name":"  Pre set 2  ","commands":["git add .","git commit \"Auto commit all\"","git push","git status"]}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	state := decodePresetState(t, create)
	if state.Version != projectCommandPresetVersion || len(state.Presets) != 1 {
		t.Fatalf("unexpected created state: %#v", state)
	}
	preset := state.Presets[0]
	if preset.ID == "" || preset.Name != "Pre set 2" || len(preset.Commands) != 4 {
		t.Fatalf("unexpected created preset: %#v", preset)
	}

	stat, err := os.Stat(projectCommandPresetPath(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("preset state mode=%o want 600", stat.Mode().Perm())
	}

	replacement := &Server{Workspace: workspace}
	get := httptest.NewRequest(http.MethodGet, "/api/command-presets", nil)
	getRR := httptest.NewRecorder()
	replacement.commandPresets(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("reload status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	reloaded := decodePresetState(t, getRR)
	if len(reloaded.Presets) != 1 || reloaded.Presets[0].ID != preset.ID || reloaded.Presets[0].Commands[2] != "git push" {
		t.Fatalf("preset state not restored: %#v", reloaded)
	}

	update := postCommandPreset(t, replacement, `{"action":"update","id":"`+preset.ID+`","name":"Deploy","commands":["git push","git status"]}`)
	if update.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", update.Code, update.Body.String())
	}
	updated := decodePresetState(t, update)
	if updated.Presets[0].Name != "Deploy" || len(updated.Presets[0].Commands) != 2 {
		t.Fatalf("preset not updated: %#v", updated.Presets[0])
	}

	deleteRR := postCommandPreset(t, replacement, `{"action":"delete","id":"`+preset.ID+`"}`)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleteRR.Code, deleteRR.Body.String())
	}
	if got := decodePresetState(t, deleteRR); len(got.Presets) != 0 {
		t.Fatalf("preset not deleted: %#v", got)
	}
}

func TestCommandPresetValidation(t *testing.T) {
	srv := &Server{Workspace: t.TempDir()}
	for _, tc := range []struct {
		name string
		body string
	}{
		{"empty name", `{"action":"create","name":" ","commands":["echo ok"]}`},
		{"empty commands", `{"action":"create","name":"Bad","commands":[]}`},
		{"blank commands", `{"action":"create","name":"Bad","commands":[" ",""]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := postCommandPreset(t, srv, tc.body)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}
