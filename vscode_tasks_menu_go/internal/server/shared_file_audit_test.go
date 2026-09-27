package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func sharedFileAuditServer(t *testing.T) (*Server, identity.Principal) {
	t.Helper()
	ctx := context.Background()
	store, err := identity.OpenSQLiteStore(ctx, filepath.Join(t.TempDir(), "identity", "identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	principal, err := store.BootstrapFirstAdmin(ctx, "project-key", "alice", "$scrypt$v=1,ln=17,r=8,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		Workspace: t.TempDir(),
		Config: config.Config{SharedServerEnabled: true, SharedProjectID: "project-key"},
		Identity: store,
	}, principal
}

func sharedAuditRequest(req *http.Request, principal identity.Principal) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), sharedPrincipalContextKey{}, principal))
}

func TestSharedFileUploadAuditRecordsOnlyMetadata(t *testing.T) {
	s, principal := sharedFileAuditServer(t)
	if err := os.Mkdir(filepath.Join(s.Workspace, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("dir", "artifacts"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "result.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "secret file content"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/files/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = sharedAuditRequest(req, principal)
	recorder := httptest.NewRecorder()
	s.fileUpload(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	events, err := s.Identity.ListAudit(context.Background(), identity.AuditQuery{ProjectID: principal.ProjectID, UserID: principal.UserID, Action: "file.upload", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ResourceID != "artifacts/result.txt" || events[0].Result != "success" {
		t.Fatalf("unexpected upload audit: %+v", events)
	}
	if strings.Contains(events[0].Details, "secret file content") {
		t.Fatalf("file contents leaked into audit: %s", events[0].Details)
	}
}

func TestSharedProjectFileWriteAuditOmitsContents(t *testing.T) {
	s, principal := sharedFileAuditServer(t)
	path := filepath.Join(s.Workspace, "sample.txt")
	original := []byte("before\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(original)
	payload, err := json.Marshal(projectFileSaveRequest{
		Path: "sample.txt", Content: "sensitive replacement\n",
		ExpectedSHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/project/file", bytes.NewReader(payload))
	req = sharedAuditRequest(req, principal)
	recorder := httptest.NewRecorder()
	s.projectFileSave(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	events, err := s.Identity.ListAudit(context.Background(), identity.AuditQuery{ProjectID: principal.ProjectID, UserID: principal.UserID, Action: "file.write", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ResourceID != "sample.txt" || events[0].Result != "success" {
		t.Fatalf("unexpected file write audit: %+v", events)
	}
	if strings.Contains(events[0].Details, "sensitive replacement") {
		t.Fatalf("file contents leaked into audit: %s", events[0].Details)
	}
}
