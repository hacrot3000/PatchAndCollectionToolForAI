package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	updater "bletonfc/vscode_tasks_menu/internal/selfupdate"
)

func TestSharedSelfUpdateStartHoldsWorkspaceMutationLock(t *testing.T) {
	s, principal := sharedFileAuditServer(t)
	RegisterSelfUpdateStart(s, func() error { return nil })
	t.Cleanup(func() { RegisterSelfUpdateStart(s, nil) })

	req := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=start", strings.NewReader(`{}`))
	req = sharedAuditRequest(req, principal)
	recorder := httptest.NewRecorder()
	s.selfUpdateState(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("start status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	holder, ok := s.sharedMutation.snapshot()
	if !ok || holder.Operation != "selfupdate.run" || holder.UserID != principal.UserID {
		t.Fatalf("unexpected self-update lock: %+v ok=%v", holder, ok)
	}
}

func TestSharedSelfUpdateLifecycleRecoversAndReleasesLock(t *testing.T) {
	s, principal := sharedFileAuditServer(t)
	update, err := updater.CreateRequest(s.Workspace, "0123456789abcdef", "", true)
	if err != nil {
		t.Fatal(err)
	}
	lease, conflict, err := s.sharedMutation.acquire(principal, "selfupdate.run", "", time.Now().UTC())
	if err != nil || conflict != nil || lease.token == "" {
		t.Fatalf("prepare self-update lock lease=%+v conflict=%+v err=%v", lease, conflict, err)
	}
	s.refreshSharedMutationLock()
	holder, ok := s.sharedMutation.snapshot()
	if !ok || holder.ResourceID != update.ID {
		t.Fatalf("active update was not bound to lock: %+v ok=%v", holder, ok)
	}

	if _, err := updater.Update(s.Workspace, update.ID, "failed", "failed", "", "test"); err != nil {
		t.Fatal(err)
	}
	s.refreshSharedMutationLock()
	if _, ok := s.sharedMutation.snapshot(); ok {
		t.Fatal("terminal self-update state kept mutation lock")
	}

	if err := os.Remove(updater.RequestPath(s.Workspace)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	oldLease, conflict, err := s.sharedMutation.acquire(principal, "selfupdate.run", "", time.Now().Add(-time.Minute))
	if err != nil || conflict != nil || oldLease.token == "" {
		t.Fatalf("prepare stale start lease=%+v conflict=%+v err=%v", oldLease, conflict, err)
	}
	s.refreshSharedMutationLock()
	if _, ok := s.sharedMutation.snapshot(); ok {
		t.Fatal("stale self-update start without request kept mutation lock")
	}
}

func TestSharedSelfUpdateConfirmRecoversLockAfterRestart(t *testing.T) {
	s, principal := sharedFileAuditServer(t)
	update, err := updater.CreateRequest(s.Workspace, "0123456789abcdef", "", true)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=confirm", strings.NewReader(`{"id":"`+update.ID+`"}`))
	req = sharedAuditRequest(req, principal)
	recorder := httptest.NewRecorder()
	s.selfUpdateState(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	holder, ok := s.sharedMutation.snapshot()
	if !ok || holder.Operation != "selfupdate.run" || holder.ResourceID != update.ID {
		t.Fatalf("confirm did not recover self-update lock: %+v ok=%v", holder, ok)
	}

	conflictReq := httptest.NewRequest(http.MethodPut, "/api/project/file", nil)
	conflictReq = sharedAuditRequest(conflictReq, principal)
	conflictRecorder := httptest.NewRecorder()
	if _, ok := s.acquireSharedMutation(conflictRecorder, conflictReq, "file.write", "sample.txt"); ok || conflictRecorder.Code != http.StatusConflict {
		t.Fatalf("conflicting mutation status=%d ok=%v body=%s", conflictRecorder.Code, ok, conflictRecorder.Body.String())
	}
}
