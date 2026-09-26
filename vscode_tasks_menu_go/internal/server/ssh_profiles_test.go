package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

type memorySecretStore struct {
	mu      sync.Mutex
	records map[string][]byte
}

func newMemorySecretStore() *memorySecretStore {
	return &memorySecretStore{records: map[string][]byte{}}
}

func (s *memorySecretStore) Put(id string, secret []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[id] = append([]byte(nil), secret...)
	return nil
}

func (s *memorySecretStore) Get(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.records[id]
	if !ok {
		return nil, secretstore.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *memorySecretStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return nil
}

func newSSHProfileAPITestServer(t *testing.T) (*Server, *sshprofile.Store, *memorySecretStore) {
	t.Helper()
	store, err := sshprofile.NewStore(filepath.Join(t.TempDir(), "ssh_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	secrets := newMemorySecretStore()
	return &Server{SSHProfiles: store, ConnectionSecrets: secrets}, store, secrets
}

func TestSSHProfileAPICreateListUpdateDelete(t *testing.T) {
	s, store, secrets := newSSHProfileAPITestServer(t)
	h := s.Handler()

	createBody := `{
		"name":"Production",
		"host":"prod.example.com",
		"port":2222,
		"username":"deploy",
		"auth_method":"password",
		"secret":"top-secret",
		"custom_home_dir":"/srv/app",
		"preset_commands":[{"id":"logs","name":"Logs","command":"pwd"}]
	}`
	create := httptest.NewRequest(http.MethodPost, "/api/ssh/profiles", strings.NewReader(createBody))
	create.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, create)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRR.Code, createRR.Body.String())
	}
	if strings.Contains(createRR.Body.String(), "top-secret") || strings.Contains(createRR.Body.String(), "secret_ref") {
		t.Fatalf("create response leaked secret data: %s", createRR.Body.String())
	}
	var created struct {
		ID        string `json:"id"`
		HasSecret bool   `json:"has_secret"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !created.HasSecret || created.Name != "Production" {
		t.Fatalf("created=%+v", created)
	}

	stored, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SecretRef == "" {
		t.Fatal("stored password profile missing secret reference")
	}
	gotSecret, err := secrets.Get(stored.SecretRef)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotSecret) != "top-secret" {
		t.Fatalf("stored secret = %q", gotSecret)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/ssh/profiles", nil)
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, list)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	if strings.Contains(listRR.Body.String(), "top-secret") || strings.Contains(listRR.Body.String(), stored.SecretRef) {
		t.Fatalf("list response leaked secret data: %s", listRR.Body.String())
	}

	updateBody := `{
		"name":"Production renamed",
		"host":"prod.example.com",
		"port":2222,
		"username":"deploy",
		"auth_method":"password",
		"custom_home_dir":"/srv/app"
	}`
	update := httptest.NewRequest(http.MethodPut, "/api/ssh/profiles/"+created.ID, strings.NewReader(updateBody))
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
	if afterUpdate.Name != "Production renamed" || afterUpdate.SecretRef != stored.SecretRef {
		t.Fatalf("updated profile=%+v", afterUpdate)
	}

	toAgent := `{
		"name":"Production renamed",
		"host":"prod.example.com",
		"port":2222,
		"username":"deploy",
		"auth_method":"agent"
	}`
	agentReq := httptest.NewRequest(http.MethodPut, "/api/ssh/profiles/"+created.ID, strings.NewReader(toAgent))
	agentReq.Header.Set("Content-Type", "application/json")
	agentRR := httptest.NewRecorder()
	h.ServeHTTP(agentRR, agentReq)
	if agentRR.Code != http.StatusOK {
		t.Fatalf("agent update status=%d body=%s", agentRR.Code, agentRR.Body.String())
	}
	afterAgent, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterAgent.SecretRef != "" {
		t.Fatalf("agent profile kept secret ref %q", afterAgent.SecretRef)
	}
	if _, err := secrets.Get(stored.SecretRef); !errors.Is(err, secretstore.ErrNotFound) {
		t.Fatalf("old secret still present: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/ssh/profiles/"+created.ID, nil)
	deleteRR := httptest.NewRecorder()
	h.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleteRR.Code, deleteRR.Body.String())
	}
	if _, err := store.Get(created.ID); !errors.Is(err, sshprofile.ErrProfileNotFound) {
		t.Fatalf("profile still exists after delete: %v", err)
	}
}

func TestSSHProfileAPIRejectsSecretForAgent(t *testing.T) {
	s, _, _ := newSSHProfileAPITestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/ssh/profiles", strings.NewReader(`{
		"name":"Production",
		"host":"prod.example.com",
		"username":"deploy",
		"auth_method":"agent",
		"secret":"must-not-be-used"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
}
