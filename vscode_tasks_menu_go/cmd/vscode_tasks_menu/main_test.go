package main

import (
	"net"
	"os"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
)

func TestCreateListenerFromInheritedFDKeepsAddress(t *testing.T) {
	original, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	defer original.Close()
	provider, ok := original.(interface{ File() (*os.File, error) })
	if !ok { t.Skip("listener has no File method") }
	file, err := provider.File()
	if err != nil { t.Fatal(err) }
	fd := int(file.Fd())
	inherited, err := createListener(config.Default(), fd, "")
	if err != nil { file.Close(); t.Fatal(err) }
	defer inherited.Close()
	// createListener takes ownership of the descriptor represented by file.
	if inherited.Addr().String() != original.Addr().String() {
		t.Fatalf("inherited addr=%s want %s", inherited.Addr(), original.Addr())
	}
}

func TestSelfUpdateReconnectsBrokerBeforeTerminalFallback(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	discover := strings.Index(src, "brokerClient.ListWithError()")
	reconnect := strings.Index(src, "self-update reconnected broker sessions=")
	restore := strings.Index(src, "RestoreProjectTerminalsForStartup()")
	if discover < 0 || reconnect < 0 || restore < 0 {
		t.Fatalf("missing self-update broker reconnect flow: discover=%d reconnect=%d restore=%d", discover, reconnect, restore)
	}
	if !(discover < reconnect && reconnect < restore) {
		t.Fatal("self-update must discover/reuse broker sessions before terminal recreation fallback")
	}
}

func TestSelfUpdateHandoffDoesNotShutdownBroker(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "func handoffToUpdatedDaemon")
	end := strings.Index(src[start:], "func publicURLForListener")
	if start < 0 || end < 0 {
		t.Fatal("handoffToUpdatedDaemon not found")
	}
	body := src[start : start+end]
	if strings.Contains(body, "ShutdownBroker") || strings.Contains(body, ".Shutdown(") {
		t.Fatal("self-update handoff must not shut down the independent session broker")
	}
}

func TestSelfUpdateFallbackPrefersBrokerPreservingDetach(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "func fallbackRestartAfterUpdate")
	end := strings.Index(src[start:], "func waitForDaemon(")
	if start < 0 || end < 0 {
		t.Fatal("fallbackRestartAfterUpdate not found")
	}
	body := src[start : start+end]
	detach := strings.Index(body, "requestDaemonDetach")
	legacyStop := strings.Index(body, "stopExistingDaemon")
	if detach < 0 || legacyStop < 0 || detach > legacyStop {
		t.Fatal("self-update fallback must try broker-preserving detach before legacy daemon stop")
	}
}


func TestConfigReloadSignalPreservesSessionBroker(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	want := "if sig == reloadConfigSignal"
	start := strings.Index(src, want)
	if start < 0 {
		t.Fatalf("missing reload signal branch %q", want)
	}
	end := strings.Index(src[start:], "logger.Printf(\"shutdown signal=%s\"")
	if end < 0 {
		t.Fatal("reload signal branch end not found")
	}
	body := src[start : start+end]
	if strings.Contains(body, "ShutdownBroker") {
		t.Fatal("config reload signal must preserve session broker")
	}
	if !strings.Contains(body, "_ = ln.Close()") {
		t.Fatal("config reload signal must close the old web listener")
	}
}
