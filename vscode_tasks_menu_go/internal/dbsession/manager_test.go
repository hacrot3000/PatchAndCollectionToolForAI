package dbsession

import (
	"context"
	"os"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

type helperHandler struct {
	manifest dbadapter.Manifest
}

func (h *helperHandler) Manifest() dbadapter.Manifest {
	return h.manifest
}

func (h *helperHandler) Handle(_ context.Context, request dbadapter.Envelope) (interface{}, *dbadapter.ProtocolError) {
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
		return nil, &dbadapter.ProtocolError{Code: "UNSUPPORTED", Message: "unsupported fixture operation"}
	}
}

func sessionTestManifest(id string) dbadapter.Manifest {
	return dbadapter.Manifest{
		ID:              id,
		Name:            "Session Test Adapter",
		Kind:            "test",
		ProtocolVersion: dbadapter.ProtocolVersion,
		Command:         os.Args[0],
		Args:            []string{"-test.run=TestDBSessionHelperProcess"},
		Capabilities: dbadapter.CapabilitySet{
			Connect: true,
			Ping:    true,
			Execute: true,
		},
	}
}

func sessionHelperEnv(adapterID string) []string {
	env := append([]string(nil), os.Environ()...)
	return append(env,
		"TASKDECK_DBSESSION_HELPER=1",
		"TASKDECK_DBSESSION_ADAPTER_ID="+adapterID,
	)
}

func TestManagerOpenRequestAndClose(t *testing.T) {
	registry := dbadapter.NewRegistry()
	manifest := sessionTestManifest("test-adapter")
	if err := registry.Register(manifest); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(registry, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	meta, err := manager.Open(
		ctx,
		"profile-1",
		manifest.ID,
		dbadapter.ProcessOptions{Env: sessionHelperEnv(manifest.ID)},
		dbadapter.ConnectPayload{Host: "db.example.com", Port: 3306, Username: "app", Secret: "not-echoed"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ProfileID != "profile-1" || meta.AdapterID != manifest.ID || meta.ID == "" {
		t.Fatalf("metadata=%+v", meta)
	}
	if len(manager.List()) != 1 {
		t.Fatalf("sessions=%+v", manager.List())
	}

	response, err := manager.Request(ctx, meta.ID, dbadapter.OpPing, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Operation != dbadapter.OpPing || response.Error != nil {
		t.Fatalf("response=%+v", response)
	}

	if err := manager.CloseSession(meta.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(meta.ID); err != ErrSessionNotFound {
		t.Fatalf("Get after close error=%v", err)
	}
}

func TestManagerRejectsAdapterIdentityMismatch(t *testing.T) {
	registry := dbadapter.NewRegistry()
	manifest := sessionTestManifest("registered-adapter")
	if err := registry.Register(manifest); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(registry, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = manager.Open(
		ctx,
		"profile-1",
		manifest.ID,
		dbadapter.ProcessOptions{Env: sessionHelperEnv("different-adapter")},
		dbadapter.ConnectPayload{},
	)
	if err == nil {
		t.Fatal("expected adapter identity mismatch")
	}
	if len(manager.List()) != 0 {
		t.Fatalf("failed adapter was retained: %+v", manager.List())
	}
}

func TestManagerEnforcesSessionLimit(t *testing.T) {
	registry := dbadapter.NewRegistry()
	manifest := sessionTestManifest("test-adapter")
	if err := registry.Register(manifest); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(registry, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first, err := manager.Open(ctx, "profile-1", manifest.ID, dbadapter.ProcessOptions{Env: sessionHelperEnv(manifest.ID)}, dbadapter.ConnectPayload{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.CloseSession(first.ID)

	if _, err := manager.Open(ctx, "profile-2", manifest.ID, dbadapter.ProcessOptions{Env: sessionHelperEnv(manifest.ID)}, dbadapter.ConnectPayload{}); err == nil {
		t.Fatal("expected session limit failure")
	}
}

func TestDBSessionHelperProcess(t *testing.T) {
	if os.Getenv("TASKDECK_DBSESSION_HELPER") != "1" {
		return
	}
	id := os.Getenv("TASKDECK_DBSESSION_ADAPTER_ID")
	handler := &helperHandler{manifest: sessionTestManifest(id)}
	handler.manifest.Command = "internal-test-helper"
	handler.manifest.Args = nil
	if err := dbadapter.Serve(context.Background(), os.Stdin, os.Stdout, handler); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
