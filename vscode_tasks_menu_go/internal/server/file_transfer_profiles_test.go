package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func fileTransferJSONBody(t *testing.T, value map[string]any) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(data)
}

func newFileTransferProfileAPITestServer(t *testing.T) (*Server, *filetransferprofile.Store, *memorySecretStore) {
	t.Helper()
	store, err := filetransferprofile.NewStore(filepath.Join(t.TempDir(), "file_transfer_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	sshStore, err := sshprofile.NewStore(filepath.Join(t.TempDir(), "ssh_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sshStore.Create(sshprofile.Profile{
		ID: "ssh-prod", Name: "SSH Production", Host: "prod.example.com",
		Username: "deploy", AuthMethod: sshprofile.AuthAgent,
	}); err != nil {
		t.Fatal(err)
	}
	secrets := newMemorySecretStore()
	return &Server{FileTransferProfiles: store, SSHProfiles: sshStore, ConnectionSecrets: secrets}, store, secrets
}

func TestFileTransferProfileCRUDAndSecretProjection(t *testing.T) {
	s, store, secrets := newFileTransferProfileAPITestServer(t)
	h := s.Handler()

	create := httptest.NewRequest(http.MethodPost, "/api/file-transfer/profiles", fileTransferJSONBody(t, map[string]any{
		"name": "FTP Production", "protocol": "ftp", "host": "ftp.example.com",
		"username": "deploy", "secret": "top-secret", "initial_path": "/uploads",
	}))
	create.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, create)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRR.Code, createRR.Body.String())
	}
	if strings.Contains(createRR.Body.String(), "top-secret") || strings.Contains(createRR.Body.String(), "secret_ref") {
		t.Fatalf("create leaked secret data: %s", createRR.Body.String())
	}
	var created struct {
		ID             string `json:"id"`
		HasSecret      bool   `json:"has_secret"`
		MaxConnections int    `json:"max_connections"`
	}
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !created.HasSecret || created.MaxConnections != filetransferprofile.DefaultMaxConnections {
		t.Fatalf("created=%+v", created)
	}
	stored, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Port != 21 || stored.SecretRef == "" {
		t.Fatalf("stored=%+v", stored)
	}
	value, err := secrets.Get(stored.SecretRef)
	if err != nil || string(value) != "top-secret" {
		t.Fatalf("stored secret=%q err=%v", value, err)
	}

	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, httptest.NewRequest(http.MethodGet, "/api/file-transfer/profiles", nil))
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	if strings.Contains(listRR.Body.String(), stored.SecretRef) || strings.Contains(listRR.Body.String(), "top-secret") {
		t.Fatalf("list leaked secret data: %s", listRR.Body.String())
	}

	update := httptest.NewRequest(http.MethodPut, "/api/file-transfer/profiles/"+created.ID, fileTransferJSONBody(t, map[string]any{
		"name": "FTP Renamed", "protocol": "ftp", "host": "ftp.example.com",
		"username": "deploy", "initial_path": "/release", "max_connections": 5,
	}))
	update.Header.Set("Content-Type", "application/json")
	updateRR := httptest.NewRecorder()
	h.ServeHTTP(updateRR, update)
	if updateRR.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updateRR.Code, updateRR.Body.String())
	}
	afterUpdate, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterUpdate.SecretRef != stored.SecretRef || afterUpdate.InitialPath != "/release" || afterUpdate.MaxConnections != 5 {
		t.Fatalf("updated=%+v", afterUpdate)
	}

	clear := httptest.NewRequest(http.MethodPut, "/api/file-transfer/profiles/"+created.ID, fileTransferJSONBody(t, map[string]any{
		"name": "FTP Renamed", "protocol": "ftp", "host": "ftp.example.com",
		"username": "deploy", "initial_path": "/release", "clear_secret": true,
	}))
	clear.Header.Set("Content-Type", "application/json")
	clearRR := httptest.NewRecorder()
	h.ServeHTTP(clearRR, clear)
	if clearRR.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", clearRR.Code, clearRR.Body.String())
	}
	cleared, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.SecretRef != "" {
		t.Fatalf("secret ref not cleared: %q", cleared.SecretRef)
	}
	if _, err := secrets.Get(stored.SecretRef); err == nil {
		t.Fatal("old secret still exists after clear")
	}

	deleteRR := httptest.NewRecorder()
	h.ServeHTTP(deleteRR, httptest.NewRequest(http.MethodDelete, "/api/file-transfer/profiles/"+created.ID, nil))
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleteRR.Code, deleteRR.Body.String())
	}
}

func TestFileTransferSFTPProfileRequiresExistingSSHProfile(t *testing.T) {
	s, _, _ := newFileTransferProfileAPITestServer(t)
	h := s.Handler()

	okReq := httptest.NewRequest(http.MethodPost, "/api/file-transfer/profiles", fileTransferJSONBody(t, map[string]any{
		"name": "SFTP Production", "protocol": "sftp",
		"ssh_profile_id": "ssh-prod", "initial_path": "/srv/app",
	}))
	okReq.Header.Set("Content-Type", "application/json")
	okRR := httptest.NewRecorder()
	h.ServeHTTP(okRR, okReq)
	if okRR.Code != http.StatusCreated {
		t.Fatalf("sftp create status=%d body=%s", okRR.Code, okRR.Body.String())
	}
	if strings.Contains(okRR.Body.String(), "secret_ref") {
		t.Fatalf("sftp response leaked private field: %s", okRR.Body.String())
	}

	badReq := httptest.NewRequest(http.MethodPost, "/api/file-transfer/profiles", fileTransferJSONBody(t, map[string]any{
		"name": "Missing SSH", "protocol": "sftp", "ssh_profile_id": "missing",
	}))
	badReq.Header.Set("Content-Type", "application/json")
	badRR := httptest.NewRecorder()
	h.ServeHTTP(badRR, badReq)
	if badRR.Code != http.StatusBadRequest || !strings.Contains(badRR.Body.String(), "referenced SSH profile not found") {
		t.Fatalf("missing ssh status=%d body=%s", badRR.Code, badRR.Body.String())
	}
}
