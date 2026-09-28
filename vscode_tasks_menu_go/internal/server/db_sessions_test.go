package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/dbsession"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
	"bletonfc/vscode_tasks_menu/internal/sshtunnel"
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
		var payload dbadapter.ConnectPayload
		if err := json.Unmarshal(request.Payload, &payload); err != nil {
			return nil, &dbadapter.ProtocolError{Code: "INVALID_CONNECT", Message: err.Error()}
		}
		if os.Getenv("TASKDECK_EXPECT_TUNNELED_DB") == "1" {
			if payload.Host != "127.0.0.1" || payload.Port < 1 {
				return nil, &dbadapter.ProtocolError{
					Code:    "WRONG_ENDPOINT",
					Message: "adapter did not receive the tunnel loopback endpoint",
				}
			}
		}
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
	case dbadapter.OpImportSQL:
		var payload dbadapter.ImportSQLPayload
		if err := json.Unmarshal(request.Payload, &payload); err != nil {
			return nil, &dbadapter.ProtocolError{Code: "INVALID_IMPORT", Message: err.Error()}
		}
		info, err := os.Stat(payload.Path)
		if err != nil {
			return nil, &dbadapter.ProtocolError{Code: "IMPORT_MISSING", Message: err.Error()}
		}
		return dbadapter.ImportSQLResult{ImportedBytes: info.Size(), Message: "fixture import complete"}, nil
	case dbadapter.OpBrowseRows:
		var payload dbadapter.BrowseRowsPayload
		if err := json.Unmarshal(request.Payload, &payload); err != nil {
			return nil, &dbadapter.ProtocolError{Code: "INVALID_BROWSE", Message: err.Error()}
		}
		return dbadapter.BrowseRowsResult{
			Columns: []dbadapter.BrowseColumn{{Name: "id", Type: "integer", Identity: true}},
			Rows: []dbadapter.BrowseRow{{Values: []interface{}{1}, Identity: map[string]interface{}{"id": 1}}},
			Offset: payload.Offset, Limit: payload.Limit, Editable: false, EditabilityReason: "fixture",
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
			Connect:    true,
			Ping:       true,
			BrowseRows: true,
			Execute:    true,
			ImportSQL:  true,
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

	script := filepath.Join(s.Workspace, "fixture.sql")
	if err := os.WriteFile(script, []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	importReq := httptest.NewRequest(http.MethodPost, "/api/db/sessions/"+meta.ID+"/import", strings.NewReader(`{"host_path":"fixture.sql","catalog":"main"}`))
	importReq.Header.Set("Content-Type", "application/json")
	importRR := httptest.NewRecorder()
	h.ServeHTTP(importRR, importReq)
	if importRR.Code != http.StatusOK {
		t.Fatalf("import status=%d body=%s", importRR.Code, importRR.Body.String())
	}
	if !strings.Contains(importRR.Body.String(), `"imported_bytes":10`) || !strings.Contains(importRR.Body.String(), "fixture import complete") {
		t.Fatalf("import response=%s", importRR.Body.String())
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
		dbadapter.OpImportSQL,
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
		json.RawMessage(`{"catalog":" main ","statement":"SELECT 1"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	execute, ok := payload.(dbadapter.ExecutePayload)
	if !ok {
		t.Fatalf("payload type=%T", payload)
	}
	if execute.Catalog != "main" {
		t.Fatalf("catalog=%q want main", execute.Catalog)
	}
	if execute.MaxRows != dbadapter.MaxRows {
		t.Fatalf("max rows=%d want %d", execute.MaxRows, dbadapter.MaxRows)
	}
}


func TestBrowserDatabaseWorkbenchOperationsAreNormalized(t *testing.T) {
	browseRaw := json.RawMessage(`{"catalog":" main ","name":" users ","limit":25,"sort":[{"column":" id ","direction":"DESC"}]}`)
	browsePayload, err := normalizeBrowserDBOperation(dbadapter.OpBrowseRows, browseRaw)
	if err != nil {
		t.Fatal(err)
	}
	browse, ok := browsePayload.(dbadapter.BrowseRowsPayload)
	if !ok || browse.Catalog != "main" || browse.Name != "users" || browse.Limit != 25 || browse.Sort[0].Direction != "desc" {
		t.Fatalf("browse payload=%#v", browsePayload)
	}

	mutateRaw := json.RawMessage(`{"catalog":"main","name":"users","mutations":[{"action":"update","identity":{"id":1},"values":{"name":"changed"}}]}`)
	mutatePayload, err := normalizeBrowserDBOperation(dbadapter.OpMutateRows, mutateRaw)
	if err != nil {
		t.Fatal(err)
	}
	mutate, ok := mutatePayload.(dbadapter.MutateRowsPayload)
	if !ok || len(mutate.Mutations) != 1 || mutate.Mutations[0].Action != "update" {
		t.Fatalf("mutate payload=%#v", mutatePayload)
	}

	actionRaw := json.RawMessage(`{"catalog":"main","kind":"table","name":"users","action":"count_rows"}`)
	actionPayload, err := normalizeBrowserDBOperation(dbadapter.OpObjectAction, actionRaw)
	if err != nil {
		t.Fatal(err)
	}
	action, ok := actionPayload.(dbadapter.ObjectActionPayload)
	if !ok || action.Action != "count_rows" || action.Name != "users" {
		t.Fatalf("object action payload=%#v", actionPayload)
	}

	if _, err := normalizeBrowserDBOperation(dbadapter.OpBrowseRows, json.RawMessage(`{"name":"users","limit":1001}`)); err == nil {
		t.Fatal("oversized browser row page unexpectedly accepted")
	}
	if _, err := normalizeBrowserDBOperation(dbadapter.OpMutateRows, json.RawMessage(`{"name":"users","mutations":[{"action":"delete"}]}`)); err == nil {
		t.Fatal("identity-free browser delete unexpectedly accepted")
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
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d want 502 body=%s", rr.Code, rr.Body.String())
	}
}

func TestDatabaseSessionAPIOpensAndCleansSSHTunnel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	t.Setenv("TASKDECK_SERVER_DB_HELPER", "1")
	t.Setenv("TASKDECK_EXPECT_TUNNELED_DB", "1")

	registry := dbadapter.NewRegistry()
	manifest := serverDBTestManifest()
	if err := registry.Register(manifest); err != nil {
		t.Fatal(err)
	}
	dbManager, err := dbsession.NewManager(registry, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer dbManager.Close()

	sshStore, err := sshprofile.NewStore(filepath.Join(t.TempDir(), "ssh_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	sshProfile, err := sshStore.Create(sshprofile.Profile{
		ID:         "ssh-1",
		Name:       "Jump",
		Host:       "jump.example.com",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthAgent,
	})
	if err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	sshExecutable := filepath.Join(binDir, "ssh")
	script := "#!/bin/sh\nexec \"$TASKDECK_SERVER_TUNNEL_HELPER_BIN\" -test.run=TestServerSSHTunnelHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(sshExecutable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TASKDECK_SERVER_TUNNEL_HELPER", "1")
	t.Setenv("TASKDECK_SERVER_TUNNEL_HELPER_BIN", os.Args[0])

	tunnelManager, err := sshtunnel.NewManager(nil, sshtunnel.Options{
		RuntimeDir:    t.TempDir(),
		SSHExecutable: sshExecutable,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tunnelManager.Close()

	profileStore, err := dbprofile.NewStore(filepath.Join(t.TempDir(), "db_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = profileStore.Create(dbprofile.Profile{
		ID:           "profile-tunnel",
		Name:         "Tunnel DB",
		AdapterID:    manifest.ID,
		Transport:    dbprofile.TransportSSHTunnel,
		Host:         "db.internal",
		Port:         3306,
		SSHProfileID: sshProfile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{
		Workspace:  t.TempDir(),
		SSHProfiles: sshStore,
		DBProfiles: profileStore,
		DBAdapters: registry,
		DBSessions: dbManager,
		SSHTunnels: tunnelManager,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/db/sessions", strings.NewReader(`{"profile_id":"profile-tunnel"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("open status=%d body=%s", rr.Code, rr.Body.String())
	}
	var meta dbsession.Metadata
	if err := json.Unmarshal(rr.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if len(tunnelManager.List()) != 1 {
		t.Fatalf("tunnels after open=%+v", tunnelManager.List())
	}

	browseReq := httptest.NewRequest(http.MethodPost, "/api/db/sessions/"+meta.ID+"/request", strings.NewReader(`{
		"operation":"browse_rows",
		"payload":{"catalog":"main","kind":"table","name":"users","limit":25}
	}`))
	browseReq.Header.Set("Content-Type", "application/json")
	browseRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(browseRR, browseReq)
	if browseRR.Code != http.StatusOK {
		t.Fatalf("tunneled browse status=%d body=%s", browseRR.Code, browseRR.Body.String())
	}
	if !strings.Contains(browseRR.Body.String(), `"operation":"browse_rows"`) || !strings.Contains(browseRR.Body.String(), `"rows"`) {
		t.Fatalf("tunneled browse response=%s", browseRR.Body.String())
	}
	if len(tunnelManager.List()) != 1 {
		t.Fatalf("workbench request unexpectedly closed tunnel: %+v", tunnelManager.List())
	}

	closeReq := httptest.NewRequest(http.MethodDelete, "/api/db/sessions/"+meta.ID, nil)
	closeRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(closeRR, closeReq)
	if closeRR.Code != http.StatusNoContent {
		t.Fatalf("close status=%d body=%s", closeRR.Code, closeRR.Body.String())
	}
	deadline := time.Now().Add(time.Second)
	for len(tunnelManager.List()) != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(tunnelManager.List()) != 0 {
		t.Fatalf("database session cleanup leaked tunnel: %+v", tunnelManager.List())
	}
}

func TestServerSSHTunnelHelperProcess(t *testing.T) {
	if os.Getenv("TASKDECK_SERVER_TUNNEL_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) > 0 {
		args = args[1:]
	}
	forward := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-L" {
			forward = args[i+1]
			break
		}
	}
	parts := strings.SplitN(forward, ":", 3)
	if len(parts) != 3 {
		os.Exit(20)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil || port < 1 {
		os.Exit(21)
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		os.Exit(22)
	}
	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			os.Exit(0)
		}
		_ = conn.Close()
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
