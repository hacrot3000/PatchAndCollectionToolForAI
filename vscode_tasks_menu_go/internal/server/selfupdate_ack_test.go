package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	updater "bletonfc/vscode_tasks_menu/internal/selfupdate"
)

func TestSelfUpdateAckRemovesCompletedRequest(t *testing.T) {
	workspace := t.TempDir()
	req, err := updater.CreateRequest(workspace, "0123456789abcdef", "http://127.0.0.1:1234", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updater.Update(workspace, req.ID, "completed", "done", "http://127.0.0.1:1234", ""); err != nil {
		t.Fatal(err)
	}

	s := &Server{Workspace: workspace}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=ack", strings.NewReader(`{"id":"`+req.ID+`"}`))
	rr := httptest.NewRecorder()
	s.selfUpdateState(rr, httpReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("ack status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(updater.RequestPath(workspace)); !os.IsNotExist(err) {
		t.Fatalf("completed request still exists after ack: %v", err)
	}
}

func TestSelfUpdateAckRemovesFailedRequest(t *testing.T) {
	workspace := t.TempDir()
	req, err := updater.CreateRequest(workspace, "0123456789abcdef", "http://127.0.0.1:1234", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updater.Update(workspace, req.ID, "failed", "validation failed", "", "test failure"); err != nil {
		t.Fatal(err)
	}

	s := &Server{Workspace: workspace}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=ack", strings.NewReader(`{"id":"`+req.ID+`"}`))
	rr := httptest.NewRecorder()
	s.selfUpdateState(rr, httpReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("ack failed status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(updater.RequestPath(workspace)); !os.IsNotExist(err) {
		t.Fatalf("failed request still exists after ack: %v", err)
	}
}

func TestSelfUpdateAckRejectsIncompleteRequest(t *testing.T) {
	workspace := t.TempDir()
	req, err := updater.CreateRequest(workspace, "0123456789abcdef", "http://127.0.0.1:1234", true)
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{Workspace: workspace}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=ack", strings.NewReader(`{"id":"`+req.ID+`"}`))
	rr := httptest.NewRecorder()
	s.selfUpdateState(rr, httpReq)
	if rr.Code != http.StatusConflict {
		t.Fatalf("early ack status=%d want %d body=%s", rr.Code, http.StatusConflict, rr.Body.String())
	}
	if _, err := os.Stat(updater.RequestPath(workspace)); err != nil {
		t.Fatalf("request disappeared after rejected ack: %v", err)
	}
}
