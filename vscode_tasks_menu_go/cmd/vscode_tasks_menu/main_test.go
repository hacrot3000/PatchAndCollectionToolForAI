package main

import (
	"net"
	"os"
	"path/filepath"
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
	end := strings.Index(src[start:], "logger.Printf(\"shutdown signal=%s; freezing terminal state before broker shutdown\"")
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
		"reloadDaemonConfig(ws, cfg, cfgPath)",
		"func reloadDaemonConfig(ws string, cfg config.Config, cfgPath string) error",
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
	reload := strings.Index(src, "fatalIf(reloadDaemonConfig(ws, cfg, cfgPath))")
	if load < 0 || reload < 0 || load > reload {
		t.Fatal("reload-config must validate vscode_tasks_menu.ini before restarting daemon")
	}
	start := strings.Index(src, "func reloadDaemonConfig(ws string, cfg config.Config, cfgPath string) error")
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
	if err := reloadDaemonConfig(t.TempDir(), config.Default(), "/tmp/vscode_tasks_menu.ini"); err != nil {
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

func TestRemoteFirewallGuidanceSkipsLocalOnlyBind(t *testing.T) {
	for _, bind := range []string{"127.0.0.1", "localhost", "::1"} {
		cfg := config.Default()
		cfg.Bind = bind
		if got := remoteFirewallGuidance(cfg, "127.0.0.1:43127"); len(got) != 0 {
			t.Fatalf("bind=%q unexpectedly produced firewall guidance: %#v", bind, got)
		}
	}
}


func TestStatusIncludesRemoteFirewallGuidance(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"printDaemonStatus(ws, cfg)",
		"func printDaemonStatus(ws string, cfg config.Config)",
		`fmt.Printf("running pid=%d url=%s started=%s\n", st.PID, st.URL, st.StartedAt)`,
		"printRemoteFirewallGuidance(os.Stdout, cfg, st.Address)",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("status remote guidance missing %q", want)
		}
	}
}


func TestRemoteFirewallGuidancePrefersLiveListenerAddress(t *testing.T) {
	localCfg := config.Default()
	lines := remoteFirewallGuidance(localCfg, "0.0.0.0:42882")
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "TCP port 42882") {
		t.Fatalf("live wildcard listener should produce guidance even when current config is local: %s", joined)
	}

	remoteCfg := config.Default()
	remoteCfg.Bind = "0.0.0.0"
	got := strings.Join(remoteFirewallGuidance(remoteCfg, "127.0.0.1:42882"), "\n")
	for _, want := range []string{
		"WARNING: config bind=0.0.0.0 nhưng daemon hiện listen 127.0.0.1:42882",
		"chạy --reload-config",
		"TCP port 42882",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("remote config/live listener mismatch missing %q: %s", want, got)
		}
	}
}


func TestEffectiveInstalledRevisionPrefersEmbeddedBinary(t *testing.T) {
	const remote = "8bdd1ee585c304d205990592c96d45e018f7001e"
	const older = "1234567890abcdef1234567890abcdef12345678"

	if got := effectiveInstalledRevision(older, remote); got != older {
		t.Fatalf("embedded revision must win over stale marker: got %q want %q", got, older)
	}
	if got := effectiveInstalledRevision(remote, older); got != remote {
		t.Fatalf("embedded current revision must win over marker: got %q want %q", got, remote)
	}
	if got := effectiveInstalledRevision("dev", remote); got != remote {
		t.Fatalf("legacy dev binary should fall back to marker: got %q want %q", got, remote)
	}
	if got := effectiveInstalledRevision("", remote); got != remote {
		t.Fatalf("empty embedded revision should fall back to marker: got %q want %q", got, remote)
	}
}

func TestSelfUpdateWarnsWhenMarkerClaimsLatestButBinaryDoesNot(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"markerRevision := selfupdate.InstalledRevision(targetBinary)",
		"installedRevision := effectiveInstalledRevision(buildRevision, markerRevision)",
		"markerRevision == remote && installedRevision != remote",
		"bỏ qua marker cũ và cập nhật lại binary",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("self-update stale marker protection missing %q", want)
		}
	}
}


func TestRemoteFirewallGuidanceRecognizesIPv6WildcardListener(t *testing.T) {
	cfg := config.Default()
	cfg.Bind = "0.0.0.0"
	for _, address := range []string{"0.0.0.0:42882", "[::]:42882"} {
		lines := remoteFirewallGuidance(cfg, address)
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "TCP port 42882") {
			t.Fatalf("wildcard listener %q should produce firewall guidance: %s", address, joined)
		}
	}
}

func TestRemoteWildcardHost(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::", "[::]"} {
		if !remoteWildcardHost(host) {
			t.Fatalf("expected wildcard host %q", host)
		}
	}
	for _, host := range []string{"127.0.0.1", "::1", "localhost", "192.168.1.20"} {
		if remoteWildcardHost(host) {
			t.Fatalf("unexpected wildcard host %q", host)
		}
	}
}


func TestServeForegroundValidatesEffectiveListenerBeforePublishingState(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "func serveForeground")
	end := strings.Index(src[start:], "func acquireDaemonLock")
	if start < 0 || end < 0 {
		t.Fatal("serveForeground not found")
	}
	body := src[start : start+end]
	create := strings.Index(body, "createListener(cfg, handoffFD, listenAddr)")
	validate := strings.Index(body, "cfg.ValidateListenerAddress(ln.Addr().String())")
	stateNew := strings.Index(body, "state.New(ws, url, healthURL, ln.Addr().String())")
	if create < 0 || validate < 0 || stateNew < 0 {
		t.Fatalf("missing effective listener security flow create=%d validate=%d state=%d", create, validate, stateNew)
	}
	if !(create < validate && validate < stateNew) {
		t.Fatal("effective listener must be auth-validated before daemon state is published")
	}
}


func TestProtocolChangesURLSchemeWithoutChangingListenerPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	httpsCfg := config.Default()
	httpsURL := publicURLForListener(httpsCfg, ln)
	if httpsURL != "https://127.0.0.1:"+port {
		t.Fatalf("HTTPS URL=%q", httpsURL)
	}
	if health := healthURLForListener(httpsCfg, ln); health != "https://127.0.0.1:"+port {
		t.Fatalf("HTTPS health URL=%q", health)
	}

	httpCfg := httpsCfg
	httpCfg.Protocol = config.ProtocolHTTP
	httpURL := publicURLForListener(httpCfg, ln)
	if httpURL != "http://127.0.0.1:"+port {
		t.Fatalf("HTTP URL=%q", httpURL)
	}
	if health := healthURLForListener(httpCfg, ln); health != "http://127.0.0.1:"+port {
		t.Fatalf("HTTP health URL=%q", health)
	}
}


func TestShutdownFreezesTerminalStateBeforeBrokerShutdown(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "logger.Printf(\"shutdown signal=%s; freezing terminal state before broker shutdown\"")
	if start < 0 {
		t.Fatal("shutdown signal block not found")
	}
	end := strings.Index(src[start:], "case <-serveDone:")
	if end < 0 {
		t.Fatal("shutdown signal block end not found")
	}
	body := src[start : start+end]
	freeze := strings.Index(body, "srv.FreezeTerminalStatePersistence()")
	closeListener := strings.Index(body, "_ = ln.Close()")
	shutdownBroker := strings.Index(body, "brokerClient.ShutdownBroker()")
	if freeze < 0 || closeListener < 0 || shutdownBroker < 0 {
		t.Fatalf("shutdown preservation flow missing freeze=%d close=%d broker=%d", freeze, closeListener, shutdownBroker)
	}
	if !(freeze < closeListener && closeListener < shutdownBroker) {
		t.Fatal("shutdown must freeze terminal persistence and close web listener before stopping broker")
	}
}

func TestGitTextconvCLIHookRunsBeforeWorkspaceResolution(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	flagPos := strings.Index(src, `flag.Bool("git-textconv"`)
	hookPos := strings.Index(src, "if *gitTextconv {")
	workspacePos := strings.Index(src, "ws, err := resolveWorkspace(*workspace)")
	for name, pos := range map[string]int{"flag": flagPos, "hook": hookPos, "workspace": workspacePos} {
		if pos < 0 {
			t.Fatalf("missing git textconv CLI %s", name)
		}
	}
	if !(flagPos < hookPos && hookPos < workspacePos) {
		t.Fatal("git textconv helper must run before workspace resolution so Git temp blobs work from any cwd")
	}
	for _, want := range []string{
		"gittextconv.NormalizeFile(flag.Arg(0), os.Stdout)",
		"--git-textconv requires exactly one file path",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("git textconv CLI missing %q", want)
		}
	}
}


func TestGlobalTaskdeckIsPreferredForRestartAndHandoff(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"func preferredTaskdeckExecutable()",
		"return selfupdate.PreferredBinary(exe), nil",
		"func handoffToUpdatedDaemon",
		"func startDaemonWithOptions",
		"migrationPending := !selfupdate.SameExecutablePath(exe, global)",
		"targetBinary = global",
		"Đã chuyển daemon sang TaskDeck global:",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("global TaskDeck migration flow missing %q", want)
		}
	}
}

func TestVersionOutputUsesTaskdeckName(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if !strings.Contains(src, "taskdeck revision=%s") {
		t.Fatal("version output must use taskdeck name")
	}
}


func TestLegacyCleanupSkipsTaskdeckSourceRepository(t *testing.T) {
	workspace := t.TempDir()
	gitDir := filepath.Join(workspace, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "[remote \"origin\"]\n\turl = git@github.com:hacrot3000/PatchAndCollectionToolForAI.git\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if !taskdeckSourceRepository(workspace) {
		t.Fatal("TaskDeck source repository was not recognized")
	}
}

func TestLegacyCleanupFindsOnlyOldProjectArtifacts(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "vscode_tasks_menu"), []byte("launcher"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "vscode_tasks_menu_go"), 0o755); err != nil {
		t.Fatal(err)
	}
	found := legacyTaskdeckArtifacts(workspace)
	if len(found) != 2 {
		t.Fatalf("legacy artifacts=%v want 2", found)
	}
}

func TestLegacyCleanupRunsOnlyFromGlobalInteractiveEntry(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"maybeOfferLegacyCleanup(ws)",
		"!selfupdate.SameExecutablePath(exe, global)",
		"stdinInfo.Mode()&os.ModeCharDevice",
		"taskdeckSourceRepository(workspace)",
		"os.RemoveAll(path)",
		"Dọn dẹp và xóa các thành phần cũ này? [y/N]:",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("legacy cleanup guard missing %q", want)
		}
	}
}


func TestDaemonGlobalMigrationDetectionIsLinuxExecutableAware(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		`/proc/%d/exe`,
		"daemonNeedsGlobalMigration(oldState, global)",
		"migrationPending = migrationPending || daemonNeedsGlobalMigration(st, global)",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("daemon migration detection missing %q", want)
		}
	}
}
