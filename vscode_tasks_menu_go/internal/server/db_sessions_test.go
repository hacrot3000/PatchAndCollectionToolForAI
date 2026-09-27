package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/dbsession"
)

type serverDBTestHandler struct {
	manifest dbadapter.Manifest
}

func (h *serverDBTestHandler) Manifest() dbadapter.Manifest {
	return h.manifest
}

func (h *serverDBTestHandler) Handle(_ context.Context, request dbadapter.Envelope) (interface{}, *dbadapter.ProtocolError) {
	switch request.Operation {
	case dbadapter.OpConnect:
		return map[string]bool{"connected": true}, nil
	case dbadapter.OpDisconnect:
		return map[string]bool{"disconnected": true}, nil
	case dbadapter.OpPing:
		return map[string]bool{"pong": true}, nil
	case dbadapter.OpExecute:
		return dbadapter.ExecuteResult{
			Columns: []dbadapter.Column{{Name: "value", Type: "integer"}},
			Rows:    [][]interface{}{{1}},
		}, nil
	default:
		return nil, &dbadapter.ProtocolError{Code: "UNSUPPORTED", Message: "fixture operation unsupported"}
	}
}

func serverDBTestManifest() dbadapter.Manifest {
	return dbadapter.Manifest{
		ID:              "test-adapter",
		Name:            "Test Adapter",
		Kind:            "test",
		ProtocolVersion: dbadapter.ProtocolVersion,
		Command:         os.Args[0],
		Args:            []string{"-test.run=TestServerDBAdapterHelperProcess"},
		Capabilities: dbadapter.CapabilitySet{
			Connect: true,
			Ping:    true,
			Execute: true,
		},
	}
}

func TestDatabaseSessionAPIConnectRequestAndClose(t *testing.T) {
	t.Setenv("TASKDECK_SERVER_DB_HELPER", "1")

	registry := dbadapter.NewRegistry()
	manifest := serverDBTestManifest()
	if err := registry.Register(manifest); err != nil {
		t.Fatal(err)
	}
	manager, err := dbsession.NewManager(registry, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	profileStore, err := dbprofile.NewStore(filepath.Join(t.TempDir(), "db_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := profileStore.Create(dbprofile.Profile{
		ID:        "profile-1",
		Name:      "Fixture DB",
		AdapterID: manifest.ID,
		Transport: dbprofile.TransportDirect,
		Host:      "db.example.com",
		Port:      3306,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Workspace:  t.TempDir(),
		DBProfiles: profileStore,
		DBAdapters: registry,
		DBSessions: manager,
	}
	h := s.Handler()

	open := httptest.NewRequest(http.MethodPost, "/api/db/sessions", strings.NewReader(`{"profile_id":"profile-1"}`))
	open.Header.Set("Content-Type", "application/json")
	openRR := httptest.NewRecorder()
	h.ServeHTTP(openRR, open)
	if openRR.Code != http.StatusCreated {
		t.Fatalf("open status=%d body=%s", openRR.Code, openRR.Body.String())
	}
	var meta dbsession.Metadata
	if err := json.Unmarshal(openRR.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.ID == "" || meta.ProfileID != profile.ID {
		t.Fatalf("metadata=%+v", meta)
	}

	ping := httptest.NewRequest(http.MethodPost, "/api/db/sessions/"+meta.ID+"/request", strings.NewReader(`{"operation":"ping"}`))
	ping.Header.Set("Content-Type", "application/json")
	pingRR := httptest.NewRecorder()
	h.ServeHTTP(pingRR, ping)
	if pingRR.Code != http.StatusOK {
		t.Fatalf("ping status=%d body=%s", pingRR.Code, pingRR.Body.String())
	}
	if !strings.Contains(pingRR.Body.String(), `"pong":true`) {
		t.Fatalf("ping response=%s", pingRR.Body.String())
	}

	execute := httptest.NewRequest(http.MethodPost, "/api/db/sessions/"+meta.ID+"/request", strings.NewReader(`{
		"operation":"execute",
		"payload":{"statement":"SELECT 1","max_rows":1}
	}`))
	execute.Header.Set("Content-Type", "application/json")
	executeRR := httptest.NewRecorder()
	h.ServeHTTP(executeRR, execute)
	if executeRR.Code != http.StatusOK {
		t.Fatalf("execute status=%d body=%s", executeRR.Code, executeRR.Body.String())
	}
	if !strings.Contains(executeRR.Body.String(), `"columns"`) || !strings.Contains(executeRR.Body.String(), `"rows"`) {
		t.Fatalf("execute response=%s", executeRR.Body.String())
	}

	closeReq := httptest.NewRequest(http.MethodDelete, "/api/db/sessions/"+meta.ID, nil)
	closeRR := httptest.NewRecorder()
	h.ServeHTTP(closeRR, closeReq)
	if closeRR.Code != http.StatusNoContent {
		t.Fatalf("close status=%d body=%s", closeRR.Code, closeRR.Body.String())
	}
}

func TestBrowserDatabaseOperationsAreStrictlyAllowlisted(t *testing.T) {
	for _, operation := range []dbadapter.Operation{
		dbadapter.OpHello,
		dbadapter.OpCapabilities,
		dbadapter.OpConnect,
		dbadapter.OpDisconnect,
		dbadapter.OpCancel,
		dbadapter.OpBegin,
		dbadapter.OpCommit,
		dbadapter.OpRollback,
	} {
		if _, err := normalizeBrowserDBOperation(operation, nil); err == nil {
			t.Fatalf("operation %q unexpectedly exposed to browser", operation)
		}
	}

	payload, err := normalizeBrowserDBOperation(
		dbadapter.OpExecute,
		json.RawMessage(`{"statement":"SELECT 1"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	execute, ok := payload.(dbadapter.ExecutePayload)
	if !ok {
		t.Fatalf("payload type=%T", payload)
	}
	if execute.MaxRows != dbadapter.MaxRows {
		t.Fatalf("max rows=%d want %d", execute.MaxRows, dbadapter.MaxRows)
	}
}

func TestDatabaseSessionAPIRejectsTunnelUntilTunnelManagerExists(t *testing.T) {
	registry := dbadapter.NewRegistry()
	manifest := serverDBTestManifest()
	if err := registry.Register(manifest); err != nil {
		t.Fatal(err)
	}
	manager, err := dbsession.NewManager(registry, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	profileStore, err := dbprofile.NewStore(filepath.Join(t.TempDir(), "db_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = profileStore.Create(dbprofile.Profile{
		ID:           "profile-1",
		Name:         "Tunnel DB",
		AdapterID:    manifest.ID,
		Transport:    dbprofile.TransportSSHTunnel,
		Host:         "127.0.0.1",
		Port:         3306,
		SSHProfileID: "ssh-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: t.TempDir(), DBProfiles: profileStore, DBAdapters: registry, DBSessions: manager}
	req := httptest.NewRequest(http.MethodPost, "/api/db/sessions", strings.NewReader(`{"profile_id":"profile-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409 body=%s", rr.Code, rr.Body.String())
	}
}

func TestServerDBAdapterHelperProcess(t *testing.T) {
	if os.Getenv("TASKDECK_SERVER_DB_HELPER") != "1" {
		return
	}
	handler := &serverDBTestHandler{manifest: serverDBTestManifest()}
	handler.manifest.Command = "internal-test-helper"
	handler.manifest.Args = nil
	if err := dbadapter.Serve(context.Background(), os.Stdin, os.Stdout, handler); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
