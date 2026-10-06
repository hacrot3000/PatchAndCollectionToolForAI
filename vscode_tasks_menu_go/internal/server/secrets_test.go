package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func newSecretsAPITestServer(t *testing.T) (*Server, *secretstore.FileStore, *sshprofile.Store) {
	t.Helper()
	dir := t.TempDir()
	secrets, err := secretstore.NewFileStore(filepath.Join(dir, "secrets"))
	if err != nil { t.Fatal(err) }
	sshStore, err := sshprofile.NewStore(filepath.Join(dir, "ssh_profiles.json"))
	if err != nil { t.Fatal(err) }
	dbStore, err := dbprofile.NewStore(filepath.Join(dir, "db_profiles.json"))
	if err != nil { t.Fatal(err) }
	transferStore, err := filetransferprofile.NewStore(filepath.Join(dir, "file_transfer_profiles.json"))
	if err != nil { t.Fatal(err) }
	return &Server{
		SSHProfiles:sshStore, DBProfiles:dbStore, FileTransferProfiles:transferStore,
		ConnectionSecrets:secrets,
	}, secrets, sshStore
}

func TestSecretsAPICreateListRotateAndDeleteWithoutPlaintextResponse(t *testing.T) {
	s, secrets, _ := newSecretsAPITestServer(t)
	h := s.Handler()

	create := httptest.NewRequest(http.MethodPost, "/api/secrets", strings.NewReader(`{"action":"create","kind":"deploy","value":"deploy-top-secret"}`))
	create.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, create)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRR.Code, createRR.Body.String())
	}
	if strings.Contains(createRR.Body.String(), "deploy-top-secret") {
		t.Fatalf("create response leaked plaintext: %s", createRR.Body.String())
	}
	var created managedSecretView
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil { t.Fatal(err) }
	if created.ID == "" || created.Kind != "deploy" || !created.Present {
		t.Fatalf("created=%+v", created)
	}

	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, httptest.NewRequest(http.MethodGet, "/api/secrets", nil))
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	if strings.Contains(listRR.Body.String(), "deploy-top-secret") {
		t.Fatalf("list response leaked plaintext: %s", listRR.Body.String())
	}
	if !strings.Contains(listRR.Body.String(), created.ID) {
		t.Fatalf("list response missing secret reference: %s", listRR.Body.String())
	}

	rotate := httptest.NewRequest(http.MethodPost, "/api/secrets", strings.NewReader(`{"action":"rotate","id":"`+created.ID+`","value":"rotated-top-secret"}`))
	rotate.Header.Set("Content-Type", "application/json")
	rotateRR := httptest.NewRecorder()
	h.ServeHTTP(rotateRR, rotate)
	if rotateRR.Code != http.StatusOK {
		t.Fatalf("rotate status=%d body=%s", rotateRR.Code, rotateRR.Body.String())
	}
	if strings.Contains(rotateRR.Body.String(), "rotated-top-secret") {
		t.Fatalf("rotate response leaked plaintext: %s", rotateRR.Body.String())
	}
	got, err := secrets.Get(created.ID)
	if err != nil { t.Fatal(err) }
	if string(got) != "rotated-top-secret" {
		t.Fatalf("rotated secret=%q", got)
	}

	deleteRR := httptest.NewRecorder()
	h.ServeHTTP(deleteRR, httptest.NewRequest(http.MethodDelete, "/api/secrets?id="+created.ID, nil))
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleteRR.Code, deleteRR.Body.String())
	}
	if _, err := secrets.Get(created.ID); err == nil {
		t.Fatal("deleted secret still exists")
	}
}

func TestSecretsAPIDeleteRefusesReferencedConnectionSecret(t *testing.T) {
	s, secrets, sshStore := newSecretsAPITestServer(t)
	const ref = "ssh/profile-1/auth/test"
	if err := secrets.Put(ref, []byte("ssh-password")); err != nil { t.Fatal(err) }
	if _, err := sshStore.Create(sshprofile.Profile{
		ID:"profile-1", Name:"Production SSH", Host:"prod.example.com", Username:"deploy",
		AuthMethod:sshprofile.AuthPassword, SecretRef:ref,
	}); err != nil { t.Fatal(err) }

	listRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRR, httptest.NewRequest(http.MethodGet, "/api/secrets", nil))
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	if strings.Contains(listRR.Body.String(), "ssh-password") {
		t.Fatalf("list leaked referenced plaintext: %s", listRR.Body.String())
	}
	if !strings.Contains(listRR.Body.String(), "Production SSH") || !strings.Contains(listRR.Body.String(), ref) {
		t.Fatalf("list missing safe usage metadata: %s", listRR.Body.String())
	}

	deleteRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(deleteRR, httptest.NewRequest(http.MethodDelete, "/api/secrets?id="+ref, nil))
	if deleteRR.Code != http.StatusConflict {
		t.Fatalf("referenced delete status=%d body=%s", deleteRR.Code, deleteRR.Body.String())
	}
	if _, err := secrets.Get(ref); err != nil {
		t.Fatalf("referenced secret was deleted: %v", err)
	}
}

func TestSecretsAPINeverProvidesPlaintextReadOperation(t *testing.T) {
	s, _, _ := newSecretsAPITestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/secrets", strings.NewReader(`{"action":"get","id":"anything","value":"unused"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "unsupported secret action") {
		t.Fatalf("plaintext get action unexpectedly accepted status=%d body=%s", rr.Code, rr.Body.String())
	}
}
