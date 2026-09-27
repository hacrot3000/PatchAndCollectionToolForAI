package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func newDBProfileAPITestServer(t *testing.T) (*Server, *dbprofile.Store, *memorySecretStore) {
	t.Helper()
	store, err := dbprofile.NewStore(filepath.Join(t.TempDir(), "db_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	registry := dbadapter.NewRegistry()
	if err := registry.Register(dbadapter.Manifest{
		ID:              "mysql-cli",
		Name:            "MySQL CLI",
		Kind:            "mysql",
		ProtocolVersion: dbadapter.ProtocolVersion,
		Command:         "/private/taskdeck",
		Args:            []string{"--db-adapter", "mysql"},
		Capabilities: dbadapter.CapabilitySet{
			Connect:        true,
			Ping:           true,
			ListCatalogs:   true,
			ListObjects:    true,
			DescribeObject: true,
			Execute:        true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	secrets := newMemorySecretStore()
	return &Server{
		Workspace:         t.TempDir(),
		DBProfiles:        store,
		DBAdapters:        registry,
		ConnectionSecrets: secrets,
	}, store, secrets
}

func TestDBAdaptersAPIHidesProcessDetails(t *testing.T) {
	s, _, _ := newDBProfileAPITestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/db/adapters", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "mysql-cli") {
		t.Fatalf("adapter missing: %s", body)
	}
	for _, forbidden := range []string{"/private/taskdeck", "--db-adapter"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("adapter process detail leaked: %s", body)
		}
	}
}

func TestDBProfileAPICreateUpdateClearSecretDelete(t *testing.T) {
	s, store, secrets := newDBProfileAPITestServer(t)
	h := s.Handler()

	createBody := `{
		"name":"Production DB",
		"adapter_id":"mysql-cli",
		"transport":"direct",
		"host":"db.example.com",
		"port":3306,
		"username":"app",
		"database":"main",
		"read_only":true,
		"secret":"top-secret"
	}`
	create := httptest.NewRequest(http.MethodPost, "/api/db/profiles", strings.NewReader(createBody))
	create.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, create)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRR.Code, createRR.Body.String())
	}
	if strings.Contains(createRR.Body.String(), "top-secret") || strings.Contains(createRR.Body.String(), "secret_ref") {
		t.Fatalf("create response leaked secret: %s", createRR.Body.String())
	}
	var created struct {
		ID        string `json:"id"`
		HasSecret bool   `json:"has_secret"`
	}
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !created.HasSecret {
		t.Fatalf("created=%+v", created)
	}

	stored, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SecretRef == "" {
		t.Fatal("stored profile missing secret reference")
	}
	secret, err := secrets.Get(stored.SecretRef)
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != "top-secret" {
		t.Fatalf("secret=%q", secret)
	}

	updateBody := `{
		"name":"Production DB renamed",
		"adapter_id":"mysql-cli",
		"transport":"direct",
		"host":"db.example.com",
		"port":3306,
		"username":"app",
		"database":"main",
		"read_only":true
	}`
	update := httptest.NewRequest(http.MethodPut, "/api/db/profiles/"+created.ID, strings.NewReader(updateBody))
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
	if afterUpdate.SecretRef != stored.SecretRef {
		t.Fatalf("nil secret update replaced secret ref: before=%q after=%q", stored.SecretRef, afterUpdate.SecretRef)
	}

	clearBody := `{
		"name":"Production DB renamed",
		"adapter_id":"mysql-cli",
		"transport":"direct",
		"host":"db.example.com",
		"port":3306,
		"username":"app",
		"database":"main",
		"read_only":true,
		"secret":""
	}`
	clearReq := httptest.NewRequest(http.MethodPut, "/api/db/profiles/"+created.ID, strings.NewReader(clearBody))
	clearReq.Header.Set("Content-Type", "application/json")
	clearRR := httptest.NewRecorder()
	h.ServeHTTP(clearRR, clearReq)
	if clearRR.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", clearRR.Code, clearRR.Body.String())
	}
	afterClear, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterClear.SecretRef != "" {
		t.Fatalf("secret ref not cleared: %q", afterClear.SecretRef)
	}
	if _, err := secrets.Get(stored.SecretRef); err == nil {
		t.Fatal("old database secret still exists after clear")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/db/profiles/"+created.ID, nil)
	deleteRR := httptest.NewRecorder()
	h.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleteRR.Code, deleteRR.Body.String())
	}
}

func TestDBProfileAPIRejectsUnknownAdapter(t *testing.T) {
	s, store, _ := newDBProfileAPITestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/db/profiles", strings.NewReader(`{
		"name":"Unknown DB",
		"adapter_id":"not-installed",
		"transport":"direct"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	profiles, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Fatalf("unknown adapter created profiles: %+v", profiles)
	}
}

func TestDBProfileAPITunnelRequiresExistingSSHProfile(t *testing.T) {
	s, store, _ := newDBProfileAPITestServer(t)
	sshStore, err := sshprofile.NewStore(filepath.Join(t.TempDir(), "ssh_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	sshProfile, err := sshStore.Create(sshprofile.Profile{
		ID:         "jump-1",
		Name:       "Jump",
		Host:       "jump.example.com",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthAgent,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.SSHProfiles = sshStore

	valid := httptest.NewRequest(http.MethodPost, "/api/db/profiles", strings.NewReader(`{
		"name":"Tunnel DB",
		"adapter_id":"mysql-cli",
		"transport":"ssh_tunnel",
		"host":"db.internal",
		"port":3306,
		"ssh_profile_id":"jump-1"
	}`))
	valid.Header.Set("Content-Type", "application/json")
	validRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(validRR, valid)
	if validRR.Code != http.StatusCreated {
		t.Fatalf("valid tunnel profile status=%d body=%s", validRR.Code, validRR.Body.String())
	}
	profiles, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Transport != dbprofile.TransportSSHTunnel || profiles[0].SSHProfileID != sshProfile.ID {
		t.Fatalf("stored tunnel profile=%+v", profiles)
	}

	invalid := httptest.NewRequest(http.MethodPost, "/api/db/profiles", strings.NewReader(`{
		"name":"Missing Jump",
		"adapter_id":"mysql-cli",
		"transport":"ssh_tunnel",
		"host":"db.internal",
		"port":3306,
		"ssh_profile_id":"missing"
	}`))
	invalid.Header.Set("Content-Type", "application/json")
	invalidRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(invalidRR, invalid)
	if invalidRR.Code != http.StatusBadRequest {
		t.Fatalf("missing SSH profile status=%d want 400 body=%s", invalidRR.Code, invalidRR.Body.String())
	}
	profiles, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("invalid tunnel profile was persisted: %+v", profiles)
	}
}
