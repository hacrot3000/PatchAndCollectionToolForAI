package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedFileWriteBlocksBehindWorkspaceMutation(t *testing.T) {
	s, principal := sharedFileAuditServer(t)
	path := filepath.Join(s.Workspace, "sample.txt")
	original := []byte("before\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(original)
	payload, err := json.Marshal(projectFileSaveRequest{
		Path: "sample.txt",
		Content: "after\n",
		ExpectedSHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatal(err)
	}

	lease, conflict, err := s.sharedMutation.acquire(principal, "patch.run", "patch-session", time.Now().UTC())
	if err != nil || conflict != nil {
		t.Fatalf("prepare lock conflict=%+v err=%v", conflict, err)
	}

	request := httptest.NewRequest(http.MethodPut, "/api/project/file", bytes.NewReader(payload))
	request = sharedAuditRequest(request, principal)
	recorder := httptest.NewRecorder()
	s.projectFileSave(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("locked write status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Error  string              `json:"error"`
		Holder sharedMutationOwner `json:"holder"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error != "workspace mutation locked" || response.Holder.Operation != "patch.run" || response.Holder.ResourceID != "patch-session" {
		t.Fatalf("unexpected lock response: %+v", response)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("locked write changed file: %q", data)
	}
	events, err := s.Identity.ListAudit(context.Background(), identity.AuditQuery{
		ProjectID: principal.ProjectID,
		UserID: principal.UserID,
		Action: "mutation.lock_conflict",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Result != "denied" {
		t.Fatalf("missing lock conflict audit: %+v", events)
	}

	if !s.sharedMutation.release(lease.token) {
		t.Fatal("failed to release prepared lock")
	}
	request = httptest.NewRequest(http.MethodPut, "/api/project/file", bytes.NewReader(payload))
	request = sharedAuditRequest(request, principal)
	recorder = httptest.NewRecorder()
	s.projectFileSave(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unlocked write status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
