package server

import (
	"context"
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

func TestSelfUpdateDetachPreservesBrokerControlPath(t *testing.T) {
	workspace := t.TempDir()
	req, err := updater.CreateRequest(workspace, "0123456789abcdef", "http://127.0.0.1:1234", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updater.Update(workspace, req.ID, "ready_restart", "ready", "", ""); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	called := make(chan string, 1)
	RegisterSelfUpdateDetach(s, func(id string) error { called <- id; return nil })
	defer RegisterSelfUpdateDetach(s, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=detach", strings.NewReader(`{"id":"`+req.ID+`"}`))
	request.RemoteAddr = "127.0.0.1:50100"
	rr := httptest.NewRecorder()
	s.selfUpdateState(rr, request)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("detach status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case id := <-called:
		if id != req.ID {
			t.Fatalf("detach id=%q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("detach callback not invoked")
	}
	loaded, err := updater.Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "restarting" {
		t.Fatalf("detach update status=%q want restarting", loaded.Status)
	}
}

func TestSelfUpdateDetachRejectsRemoteClient(t *testing.T) {
	workspace := t.TempDir()
	req, err := updater.CreateRequest(workspace, "0123456789abcdef", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updater.Update(workspace, req.ID, "ready_restart", "ready", "", ""); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	RegisterSelfUpdateDetach(s, func(string) error { return nil })
	defer RegisterSelfUpdateDetach(s, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=detach", strings.NewReader(`{"id":"`+req.ID+`"}`))
	request.RemoteAddr = "192.0.2.10:50100"
	rr := httptest.NewRecorder()
	s.selfUpdateState(rr, request)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("remote detach status=%d body=%s", rr.Code, rr.Body.String())
	}
}


func TestSelfUpdateCheckAndStartHooks(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace: workspace}
	RegisterSelfUpdateCheck(s, func(context.Context) (SelfUpdateCheckResult, error) {
		return SelfUpdateCheckResult{
			Available: true,
			InstalledRevision: "old-revision",
			RemoteRevision: "new-revision",
		}, nil
	})
	defer RegisterSelfUpdateCheck(s, nil)

	check := httptest.NewRequest(http.MethodGet, "/api/state/tasks?scope=self-update&action=check", nil)
	checkRR := httptest.NewRecorder()
	s.selfUpdateState(checkRR, check)
	if checkRR.Code != http.StatusOK {
		t.Fatalf("check status=%d body=%s", checkRR.Code, checkRR.Body.String())
	}
	body := checkRR.Body.String()
	for _, want := range []string{`"available":true`, `"installed_revision":"old-revision"`, `"remote_revision":"new-revision"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("check response missing %q: %s", want, body)
		}
	}

	started := make(chan struct{}, 1)
	RegisterSelfUpdateStart(s, func() error {
		started <- struct{}{}
		return nil
	})
	defer RegisterSelfUpdateStart(s, nil)

	start := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=start", strings.NewReader(`{}`))
	startRR := httptest.NewRecorder()
	s.selfUpdateState(startRR, start)
	if startRR.Code != http.StatusAccepted {
		t.Fatalf("start status=%d body=%s", startRR.Code, startRR.Body.String())
	}
	select {
	case <-started:
	default:
		t.Fatal("self-update start callback was not invoked")
	}
}

func TestSelfUpdateStartRejectsConcurrentActiveRequest(t *testing.T) {
	workspace := t.TempDir()
	if _, err := updater.CreateRequest(workspace, "0123456789abcdef", "", true); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	called := false
	RegisterSelfUpdateStart(s, func() error { called = true; return nil })
	defer RegisterSelfUpdateStart(s, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=start", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.selfUpdateState(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("concurrent start status=%d body=%s", rr.Code, rr.Body.String())
	}
	if called {
		t.Fatal("start callback must not run while update request is active")
	}
}
