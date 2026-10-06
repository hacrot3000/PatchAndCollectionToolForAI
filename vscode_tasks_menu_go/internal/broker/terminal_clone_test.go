package broker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestBrokerTerminalCloneEndpoint(t *testing.T) {
	root := t.TempDir()
	manager := session.NewManager(1 << 20)
	defer manager.Shutdown(500000000)

	source, err := manager.Start(tasks.Execution{
		Label: "Terminal",
		Command: "/bin/sh",
		Cwd: root,
		Env: append(os.Environ(), "TASKDECK_BROKER_CLONE_TEST=1"),
		TargetType: "local",
	})
	if err != nil {
		t.Fatal(err)
	}

	api := &sessionAPI{manager: manager}
	body := strings.NewReader(`{"owner_user_id":"alice","project_id":"project-1"}`)
	rr := httptest.NewRecorder()
	api.sessionItem(rr, httptest.NewRequest(http.MethodPost, "/v1/sessions/"+source.ID+"/clone", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("clone status=%d body=%s", rr.Code, rr.Body.String())
	}
	var cloned session.Metadata
	if err := json.Unmarshal(rr.Body.Bytes(), &cloned); err != nil {
		t.Fatal(err)
	}
	if cloned.ID == "" || cloned.ID == source.ID {
		t.Fatalf("clone id=%q source=%q", cloned.ID, source.ID)
	}
	if cloned.Kind != tasks.SessionKindTerminal || cloned.OwnerUserID != "alice" || cloned.ProjectID != "project-1" {
		t.Fatalf("clone ownership=%+v", cloned)
	}
	if filepath.Clean(cloned.Cwd) != filepath.Clean(root) {
		t.Fatalf("clone cwd=%q want=%q", cloned.Cwd, root)
	}
}

func TestBrokerTerminalCloneEndpointRejectsBadJSON(t *testing.T) {
	manager := session.NewManager(1 << 20)
	api := &sessionAPI{manager: manager}
	rr := httptest.NewRecorder()
	api.sessionItem(rr, httptest.NewRequest(http.MethodPost, "/v1/sessions/missing/clone", strings.NewReader("{")))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad JSON status=%d body=%s", rr.Code, rr.Body.String())
	}
}
