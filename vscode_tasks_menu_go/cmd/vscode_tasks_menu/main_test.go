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


func TestReloadConfigCLIUsesBrokerPreservingRestart(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		`flag.Bool("reload-config"`,
		"reloadDaemonConfig(ws, cfgPath)",
		"func reloadDaemonConfig(ws, cfgPath string) error",
		"process.Signal(reloadConfigSignal)",
		"waitForDaemonStateRelease(ws, st.PID",
		"startDaemon(ws)",
		"waitForDaemon(ws, 8*time.Second, st.PID)",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("reload-config flow missing %q", want)
		}
	}
	load := strings.Index(src, "cfg, cfgPath, err := config.Load(ws)")
	reload := strings.Index(src, "fatalIf(reloadDaemonConfig(ws, cfgPath))")
	if load < 0 || reload < 0 || load > reload {
		t.Fatal("reload-config must validate vscode_tasks_menu.ini before restarting daemon")
	}
	start := strings.Index(src, "func reloadDaemonConfig(ws, cfgPath string) error")
	end := strings.Index(src[start:], "func printDaemonStatus")
	if start < 0 || end < 0 {
		t.Fatal("reloadDaemonConfig function not found")
	}
	body := src[start : start+end]
	if strings.Contains(body, "stopExistingDaemon(") || strings.Contains(body, "ShutdownBroker") {
		t.Fatal("reload-config must not use the broker-destructive daemon stop path")
	}
}

func TestReloadConfigWhenDaemonIsNotRunningIsNoop(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if err := reloadDaemonConfig(t.TempDir(), "/tmp/vscode_tasks_menu.ini"); err != nil {
		t.Fatalf("reload without daemon: %v", err)
	}
}


func TestRemoteFirewallGuidanceUsesActualBoundPort(t *testing.T) {
	cfg := config.Default()
	cfg.Bind = "0.0.0.0"
	lines := remoteFirewallGuidance(cfg, "0.0.0.0:43127")
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"TCP port 43127",
		"sudo ufw allow 43127/tcp",
		"sudo firewall-cmd --permanent --add-port=43127/tcp",
		"sudo firewall-cmd --reload",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("firewall guidance missing %q: %s", want, joined)
		}
	}
}

func TestRemoteFirewallGuidanceOnlyForWildcardIPv4Bind(t *testing.T) {
	for _, bind := range []string{"127.0.0.1", "localhost", "::1", "::"} {
		cfg := config.Default()
		cfg.Bind = bind
		if got := remoteFirewallGuidance(cfg, "127.0.0.1:43127"); len(got) != 0 {
			t.Fatalf("bind=%q unexpectedly produced firewall guidance: %#v", bind, got)
		}
	}
}
