package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedSettingsMutationsAreAuditedWithoutValues(t *testing.T) {
	s, principal := sharedFileAuditServer(t)
	if err := os.MkdirAll(filepath.Join(s.Workspace, ".vscode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.Workspace, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Workspace, ".vscode", "vscode_tasks_menu.ini"), []byte("[server]\nbind = 127.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(method, path, body string, handler func(http.ResponseWriter, *http.Request), want int) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = sharedAuditRequest(req, principal)
		recorder := httptest.NewRecorder()
		handler(recorder, req)
		if recorder.Code != want {
			t.Fatalf("%s %s status=%d body=%s", method, path, recorder.Code, recorder.Body.String())
		}
	}

	run(http.MethodPut, "/api/config/page-title", `{"title":"Sensitive Project Name"}`, s.pageTitle, http.StatusOK)
	run(http.MethodPut, "/api/config/terminal-cwds", `{"selected_cwd":"tools","custom_dirs":["tools"]}`, s.terminalCWDConfig, http.StatusOK)
	run(http.MethodPost, "/api/command-presets", `{"action":"create","name":"Private deployment","commands":["echo top-secret"]}`, s.commandPresets, http.StatusCreated)
	run(http.MethodPut, "/api/state/tasks", `{"version":1,"favorites":[1],"recent":[2],"history":[]}`, s.taskState, http.StatusOK)

	events, err := s.Identity.ListAudit(context.Background(), identity.AuditQuery{ProjectID: principal.ProjectID, UserID: principal.UserID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]identity.AuditEvent{}
	for _, event := range events {
		actions[event.Action] = event
	}
	for _, action := range []string{
		"settings.page_title.update",
		"settings.terminal_cwd.update",
		"settings.command_preset.create",
		"settings.task_state.update",
	} {
		event, ok := actions[action]
		if !ok || event.Result != "success" {
			t.Fatalf("missing settings audit %s in %+v", action, events)
		}
		if strings.Contains(event.Details, "Sensitive Project Name") ||
			strings.Contains(event.Details, "Private deployment") ||
			strings.Contains(event.Details, "top-secret") ||
			strings.Contains(event.Details, "tools") {
			t.Fatalf("%s leaked setting value: %s", action, event.Details)
		}
	}
}
