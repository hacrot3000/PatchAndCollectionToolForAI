package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	updater "bletonfc/vscode_tasks_menu/internal/selfupdate"
)

func TestSelfUpdateConfirmAndHandoff(t *testing.T) {
	workspace := t.TempDir()
	req, err := updater.CreateRequest(workspace, "0123456789abcdef", "http://127.0.0.1:1234", false)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}

	confirm := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=confirm", strings.NewReader(`{"id":"`+req.ID+`"}`))
	confirm.RemoteAddr = "127.0.0.1:50000"
	rr := httptest.NewRecorder()
	s.selfUpdateState(rr, confirm)
	if rr.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", rr.Code, rr.Body.String())
	}
	loaded, err := updater.Load(workspace)
	if err != nil || loaded.Status != "confirmed" {
		t.Fatalf("confirmed state=%#v err=%v", loaded, err)
	}

	if _, err := updater.Update(workspace, req.ID, "ready_restart", "ready", "", ""); err != nil {
		t.Fatal(err)
	}
	called := make(chan string, 1)
	RegisterSelfUpdateHandoff(s, func(id string) error { called <- id; return nil })
	defer RegisterSelfUpdateHandoff(s, nil)

	handoff := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=handoff", strings.NewReader(`{"id":"`+req.ID+`"}`))
	handoff.RemoteAddr = "127.0.0.1:50001"
	handoffRR := httptest.NewRecorder()
	s.selfUpdateState(handoffRR, handoff)
	if handoffRR.Code != http.StatusAccepted {
		t.Fatalf("handoff status=%d body=%s", handoffRR.Code, handoffRR.Body.String())
	}
	select {
	case id := <-called:
		if id != req.ID { t.Fatalf("handoff id=%q", id) }
	case <-time.After(time.Second):
		t.Fatal("handoff callback not invoked")
	}
}

func TestSelfUpdateHandoffRejectsRemoteClient(t *testing.T) {
	workspace := t.TempDir()
	req, err := updater.CreateRequest(workspace, "0123456789abcdef", "", true)
	if err != nil { t.Fatal(err) }
	if _, err := updater.Update(workspace, req.ID, "ready_restart", "ready", "", ""); err != nil { t.Fatal(err) }
	s := &Server{Workspace: workspace}
	request := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=handoff", strings.NewReader(`{"id":"`+req.ID+`"}`))
	request.RemoteAddr = "192.0.2.10:50000"
	rr := httptest.NewRecorder()
	s.selfUpdateState(rr, request)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("remote handoff status=%d body=%s", rr.Code, rr.Body.String())
	}
}
