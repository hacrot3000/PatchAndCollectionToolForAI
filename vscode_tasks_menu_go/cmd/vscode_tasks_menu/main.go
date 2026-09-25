package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"bletonfc/vscode_tasks_menu/internal/broker"
	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/gittextconv"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
	"bletonfc/vscode_tasks_menu/internal/selfupdate"
	"bletonfc/vscode_tasks_menu/internal/server"
	"bletonfc/vscode_tasks_menu/internal/state"
	"bletonfc/vscode_tasks_menu/internal/tlscert"
	terminalui "bletonfc/vscode_tasks_menu/internal/terminal"
)

var buildRevision = "dev"
var activeWorkspaceForUpdateCheck string

const reloadConfigSignal = syscall.Signal(1)

func main() {
	workspace := flag.String("workspace", "", "workspace chứa .vscode/tasks.json")
	serve := flag.Bool("serve", false, "chạy HTTP server foreground (internal)")
	terminal := flag.Bool("terminal", false, "mở terminal menu native Go")
	noBrowser := flag.Bool("no-browser", false, "không tự mở trình duyệt")
	statusOnly := flag.Bool("status", false, "in trạng thái daemon rồi thoát")
	stopDaemonFlag := flag.Bool("stop-daemon", false, "dừng daemon của workspace rồi thoát")
	restartDaemon := flag.Bool("restart-daemon", false, "dừng daemon cũ rồi khởi động lại")
	reloadConfigFlag := flag.Bool("reload-config", false, "nạp lại .vscode/vscode_tasks_menu.ini và restart web daemon, giữ nguyên session broker")
	selfUpdateFlag := flag.Bool("self-update", false, "kiểm tra, xác nhận và cài bản mới nhất từ GitHub")
	selfUpdateAuto := flag.Bool("self-update-auto", false, "tự xác nhận self-update (internal)")
	sessionBroker := flag.Bool("session-broker", false, "chạy session broker foreground (internal)")
	versionFlag := flag.Bool("version", false, "in revision của binary rồi thoát")
	gitTextconv := flag.Bool("git-textconv", false, "normalize standalone CR for Git textconv (internal)")
	handoffFD := flag.Int("handoff-fd", -1, "inherited listener fd (internal)")
	listenAddr := flag.String("listen-addr", "", "listener address override (internal)")
	selfUpdateID := flag.String("self-update-id", "", "self-update handoff id (internal)")
	cleanupLegacy := flag.Bool("cleanup-legacy", false, "dọn thành phần TaskDeck/Patch Tool legacy đã xác minh trong workspace")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("taskdeck revision=%s\n", buildRevision)
		return
	}
	if *gitTextconv {
		if flag.NArg() != 1 {
			fatalIf(fmt.Errorf("--git-textconv requires exactly one file path"))
		}
		fatalIf(gittextconv.NormalizeFile(flag.Arg(0), os.Stdout))
		return
	}

	patchCommand := flag.NArg() > 0 && flag.Arg(0) == "patch"
	ws, err := resolveWorkspace(*workspace)
	fatalIf(err)
	if *cleanupLegacy {
		if taskdeckSourceRepository(ws) {
			fmt.Println("Bỏ qua cleanup: workspace hiện tại là source repo TaskDeck.")
			return
		}
		fatalIf(cleanupLegacyTaskdeckArtifacts(ws, true))
		fatalIf(cleanupLegacyPatchRuntime(ws, true))
		return
	}
	if !*serve && !*sessionBroker && !*gitTextconv && !*selfUpdateAuto {
		maybeOfferLegacyCleanup(ws)
	}
	if patchCommand {
		os.Exit(runPatchCLI(ws, flag.Args()[1:]))
	}
	if *sessionBroker {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		fatalIf(broker.Run(ctx, ws, log.New(os.Stdout, "", log.LstdFlags)))
		return
	}
	if *terminal {
		fatalIf(terminalui.Run(ws))
		return
	}
	cfg, cfgPath, err := config.Load(ws)
	fatalIf(err)
	if *serve {
		fatalIf(serveForeground(ws, cfg, cfgPath, *handoffFD, *listenAddr, *selfUpdateID))
		return
	}
	if *selfUpdateFlag || *selfUpdateAuto {
		fatalIf(runSelfUpdate(ws, cfg, *selfUpdateAuto))
		return
	}

	startLock, err := state.AcquireStartLock(ws)
	fatalIf(err)
	defer startLock.Close()

	if *reloadConfigFlag {
		fatalIf(reloadDaemonConfig(ws, cfg, cfgPath))
		return
	}
	if *statusOnly {
		printDaemonStatus(ws, cfg)
		return
	}
	if *stopDaemonFlag || *restartDaemon {
		fatalIf(stopExistingDaemon(ws))
		if *stopDaemonFlag && !*restartDaemon {
			return
		}
	}

	if st, err := state.Load(ws); err == nil && state.Healthy(st) {
		fmt.Println(st.URL)
		printRemoteFirewallGuidance(os.Stdout, cfg, st.Address)
		printRemoteSecurityWarning(cfg)
		printTLSStatus(ws, cfg)
		if cfg.OpenBrowser && !*noBrowser {
			_ = openBrowser(st.URL)
		}
		return
	}
	state.Remove(ws)
	fatalIf(startDaemon(ws))
	st, err := waitForDaemon(ws, 5*time.Second, 0)
	fatalIf(err)
	fmt.Println(st.URL)
	printRemoteFirewallGuidance(os.Stdout, cfg, st.Address)
	printRemoteSecurityWarning(cfg)
	printTLSStatus(ws, cfg)
	if cfg.OpenBrowser && !*noBrowser {
		_ = openBrowser(st.URL)
	}
}

func runPatchCLI(workspace string, args []string) int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TaskDeck Patch: cannot resolve executable: %v\n", err)
		return 2
	}
	runtimeSpec, err := patchtool.Resolve(workspace, exe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TaskDeck Patch: %v\n", err)
		return 2
	}
	command, commandArgs, err := runtimeSpec.Command(workspace, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TaskDeck Patch: %v\n", err)
		return 2
	}
	cmd := exec.Command(command, commandArgs...)
	cmd.Dir = workspace
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "TaskDeck Patch: start failed: %v\n", err)
		return 2
	}
	return 0
}

func taskdeckRepositoryRemote(text string) bool {
	normalized := strings.ToLower(text)
	normalized = strings.ReplaceAll(normalized, "git@github.com:", "github.com/")
	normalized = strings.ReplaceAll(normalized, "ssh://git@github.com/", "github.com/")
	normalized = strings.ReplaceAll(normalized, "https://github.com/", "github.com/")
	return strings.Contains(normalized, "github.com/hacrot3000/patchandcollectiontoolforai")
}

func taskdeckSourceRepository(workspace string) bool {
	if _, err := os.Stat(filepath.Join(workspace, "vscode_tasks_menu_go", "go.mod")); err != nil {
		return false
	}

	gitConfig := filepath.Join(workspace, ".git", "config")
	if data, err := os.ReadFile(gitConfig); err == nil {
		return taskdeckRepositoryRemote(string(data))
	}

	// Git worktrees store .git as a file, so fall back to git only for that
	// layout. The Git root must still be exactly the active workspace.
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		workspaceAbs = filepath.Clean(workspace)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return false
	}
	rootCmd := exec.Command(gitPath, "-C", workspace, "rev-parse", "--show-toplevel")
	out, err := rootCmd.Output()
	if err != nil {
		return false
	}
	rootAbs, err := filepath.Abs(strings.TrimSpace(string(out)))
	if err != nil {
		rootAbs = filepath.Clean(strings.TrimSpace(string(out)))
	}
	if filepath.Clean(rootAbs) != filepath.Clean(workspaceAbs) {
		return false
	}
	remoteCmd := exec.Command(gitPath, "-C", workspace, "remote", "-v")
	remotes, err := remoteCmd.Output()
	return err == nil && taskdeckRepositoryRemote(string(remotes))
}

func legacyTaskdeckArtifacts(workspace string) []string {
	candidates := []string{
		filepath.Join(workspace, "vscode_tasks_menu"),
		filepath.Join(workspace, "vscode_tasks_menu_go"),
	}
	found := make([]string, 0, len(candidates))
	for _, path := range candidates {
		if _, err := os.Lstat(path); err == nil {
			found = append(found, path)
		}
	}
	return found
}

func cleanupLegacyTaskdeckArtifacts(workspace string, forcePrompt bool) error {
	if taskdeckSourceRepository(workspace) {
		if forcePrompt {
			fmt.Println("Bỏ qua cleanup: workspace hiện tại là source repo TaskDeck.")
		}
		return nil
	}
	artifacts := legacyTaskdeckArtifacts(workspace)
	if len(artifacts) == 0 {
		if forcePrompt {
			fmt.Println("Không còn thành phần TaskDeck legacy trong workspace.")
		}
		return nil
	}
	stdinInfo, err := os.Stdin.Stat()
	if err != nil || stdinInfo.Mode()&os.ModeCharDevice == 0 {
		if forcePrompt {
			return fmt.Errorf("cleanup cần chạy từ terminal tương tác")
		}
		return nil
	}
	fmt.Println("Phát hiện thành phần TaskDeck legacy trong project:")
	for _, path := range artifacts {
		fmt.Println(" -", filepath.Base(path))
	}
	fmt.Print("Dọn dẹp và xóa các thành phần cũ này? [y/N]: ")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
	default:
		fmt.Println("Đã bỏ qua cleanup.")
		return nil
	}
	for _, path := range artifacts {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("không xóa được %s: %w", path, err)
		}
		fmt.Println("Đã xóa:", path)
	}
	return nil
}

func printLegacyPatchPaths(label string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Printf("%s (%d):\n", label, len(values))
	limit := len(values)
	if limit > 12 {
		limit = 12
	}
	for _, value := range values[:limit] {
		fmt.Println(" -", value)
	}
	if len(values) > limit {
		fmt.Printf(" - ... và %d file khác\n", len(values)-limit)
	}
}

func cleanupLegacyPatchRuntime(workspace string, forcePrompt bool) error {
	if taskdeckSourceRepository(workspace) {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		if forcePrompt {
			return fmt.Errorf("resolve executable for Patch migration: %w", err)
		}
		return nil
	}
	plan, err := patchtool.PlanLegacyMigration(workspace, exe)
	if err != nil {
		if forcePrompt {
			return fmt.Errorf("kiểm tra Patch Tool legacy: %w", err)
		}
		return nil
	}
	if !plan.Present {
		if forcePrompt {
			fmt.Println("Không còn Patch Tool runtime legacy cần migrate trong workspace.")
		}
		return nil
	}
	if !plan.Safe {
		if forcePrompt {
			fmt.Println("Giữ nguyên Patch Tool runtime legacy vì không thể xác minh an toàn toàn bộ nội dung.")
			printLegacyPatchPaths("File đã sửa hoặc khác checksum", plan.Modified)
			printLegacyPatchPaths("File lạ/không thuộc bundled runtime", plan.Unknown)
		}
		return nil
	}

	stdinInfo, err := os.Stdin.Stat()
	if err != nil || stdinInfo.Mode()&os.ModeCharDevice == 0 {
		if forcePrompt {
			return fmt.Errorf("Patch runtime migration cần chạy từ terminal tương tác")
		}
		return nil
	}
	fmt.Printf("Phát hiện Patch Tool runtime legacy đã xác minh (%d file): %s\n", len(plan.Verified), plan.LegacyRoot)
	fmt.Print("Chuyển sang TaskDeck global và giữ launcher tương thích taskdeck patch? [y/N]: ")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
	default:
		fmt.Println("Đã bỏ qua Patch runtime migration.")
		return nil
	}
	applied, err := patchtool.ApplyLegacyMigration(workspace, exe)
	if err != nil {
		return fmt.Errorf("Patch runtime migration thất bại: %w", err)
	}
	fmt.Printf("Đã migrate Patch Tool legacy: %d file đã xác minh; launcher cũ (nếu có) nay chuyển tiếp sang taskdeck patch.\n", len(applied.Verified))
	return nil
}

func maybeOfferLegacyCleanup(workspace string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	global, err := selfupdate.GlobalBinaryPath()
	if err != nil {
		return
	}
	runningGlobal := selfupdate.SameExecutablePath(exe, global)
	if !runningGlobal && filepath.Base(exe) == "taskdeck" && selfupdate.ExecutableExists(global) {
		runningGlobal = true
	}
	if !runningGlobal {
		return
	}
	fmt.Println("TaskDeck đang chạy từ global app:", global)
	if err := cleanupLegacyTaskdeckArtifacts(workspace, false); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: legacy cleanup: %v\n", err)
	}
	if err := cleanupLegacyPatchRuntime(workspace, false); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Patch runtime migration: %v\n", err)
	}
}

func resolveWorkspace(value string) (string, error) {
	if value == "" {
		var err error
		value, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(abs, ".vscode", "tasks.json")); err != nil {
		return "", fmt.Errorf("không tìm thấy %s/.vscode/tasks.json", abs)
	}
	return abs, nil
}

func resolveTLSForServe(workspace string, cfg config.Config, updateID string) (tlscert.Result, error) {
	if strings.TrimSpace(updateID) != "" {
		return tlscert.ResolveForSelfUpdate(workspace, cfg)
	}
	return tlscert.Resolve(workspace, cfg)
}

func serveForeground(ws string, cfg config.Config, cfgPath string, handoffFD int, listenAddr, updateID string) error {
	activeWorkspaceForUpdateCheck = ws
	defer func() { activeWorkspaceForUpdateCheck = "" }()
	if err := state.EnsureDir(ws); err != nil {
		return err
	}
	daemonLock, err := acquireDaemonLock(ws, handoffFD >= 3)
	if err != nil {
		return err
	}
	defer daemonLock.Close()

	tlsResult, err := resolveTLSForServe(ws, cfg, updateID)
	if err != nil {
		return err
	}
	if cfg.TLS() {
		cfg.TLSCert = tlsResult.CertPath
		cfg.TLSKey = tlsResult.KeyPath
	}

	ln, err := createListener(cfg, handoffFD, listenAddr)
	if err != nil {
		return err
	}
	if err := cfg.ValidateListenerAddress(ln.Addr().String()); err != nil {
		_ = ln.Close()
		return fmt.Errorf("listener security validation: %w", err)
	}
	url := publicURLForListener(cfg, ln)
	healthURL := healthURLForListener(cfg, ln)
	st := state.New(ws, url, healthURL, ln.Addr().String())
	if err := state.Save(st); err != nil {
		_ = ln.Close()
		return err
	}
	defer state.RemoveIfPID(ws, os.Getpid())

	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf("workspace=%s", ws)
	logger.Printf("config=%s", cfgPath)
	logger.Printf("url=%s", url)
	if cfg.TLS() {
		if tlsResult.Auto {
			mode := "reused"
			if tlsResult.Created {
				mode = "created"
			}
			logger.Printf("https=self-signed mode=%s cert=%s", mode, tlsResult.CertPath)
		} else {
			logger.Printf("https=configured cert=%s", tlsResult.CertPath)
		}
	}
	for _, line := range remoteFirewallGuidance(cfg, ln.Addr().String()) {
		logger.Print(line)
	}
	if updateID != "" {
		logger.Printf("self-update handoff=%s revision=%s", updateID, buildRevision)
	}
	if warning := server.RemoteWarning(cfg); warning != "" {
		logger.Print(warning)
	}
	brokerClient, err := broker.EnsureClient(ws, logger)
	if err != nil {
		_ = ln.Close()
		return err
	}
	defer brokerClient.Close()
	sessionService := broker.NewCompatibilityService(brokerClient)
	defer sessionService.Close()
	if sessionService.NeedsPatchProtocolFallback() {
		logger.Printf("session broker predates Patch protocol capabilities; preserving broker-owned terminals and using daemon-local Patch protocol sessions")
	}
	srv := &server.Server{Workspace: ws, Config: cfg, Log: logger, Sessions: sessionService}
	server.RegisterSelfUpdateCheck(srv, checkSelfUpdate)
	defer server.RegisterSelfUpdateCheck(srv, nil)
	server.RegisterSelfUpdateStart(srv, func() error { return startAutoSelfUpdate(ws) })
	defer server.RegisterSelfUpdateStart(srv, nil)

	// During self-update the independent broker normally survives the web-daemon
	// replacement. Reuse those exact sessions first so task processes, PTYs,
	// session IDs and scrollback remain intact. Only recreate saved terminals when
	// the broker is genuinely empty (for example the first migration from an old
	// non-broker build).
	if updateID != "" {
		liveSessions, listErr := brokerClient.ListWithError()
		switch {
		case listErr != nil:
			logger.Printf("self-update broker session discovery warning: %v", listErr)
			_, _ = selfupdate.Update(ws, updateID, "completed", "Update completed; daemon is ready and will retry broker session discovery.", url, listErr.Error())
		case len(liveSessions) > 0:
			running := 0
			for _, meta := range liveSessions {
				if meta.Status == "running" {
					running++
				}
			}
			logger.Printf("self-update reconnected broker sessions=%d running=%d", len(liveSessions), running)
			_, _ = selfupdate.Update(ws, updateID, "completed", fmt.Sprintf("Update completed; reconnected %d broker sessions (%d running).", len(liveSessions), running), url, "")
		default:
			restored, warnings, restoreErr := srv.RestoreProjectTerminalsForStartup()
			if restoreErr != nil {
				logger.Printf("self-update terminal restore failed: %v", restoreErr)
				_, _ = selfupdate.Update(ws, updateID, "completed", "Update completed; daemon is ready but saved terminals could not be restored: "+restoreErr.Error(), url, "")
			} else {
				logger.Printf("self-update restored terminal sessions=%d", restored)
				for _, warning := range warnings {
					logger.Printf("terminal restore warning: %s", warning)
				}
				message := "Update completed; daemon is ready."
				if restored > 0 {
					message = fmt.Sprintf("Update completed; restored %d terminals.", restored)
				}
				_, _ = selfupdate.Update(ws, updateID, "completed", message, url, "")
			}
		}
	}

	server.RegisterSelfUpdateHandoff(srv, func(id string) error {
		return handoffToUpdatedDaemon(ws, ln, id)
	})
	defer server.RegisterSelfUpdateHandoff(srv, nil)
	server.RegisterSelfUpdateDetach(srv, func(id string) error {
		logger.Printf("self-update detach=%s; preserving session broker", id)
		return ln.Close()
	})
	defer server.RegisterSelfUpdateDetach(srv, nil)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, reloadConfigSignal)
	serveDone := make(chan struct{})
	go func() {
		select {
		case sig := <-sigCh:
			if sig == reloadConfigSignal {
				logger.Printf("config reload signal=%s; preserving session broker", sig)
				_ = ln.Close()
				return
			}
			logger.Printf("shutdown signal=%s; freezing terminal state before broker shutdown", sig)
			srv.FreezeTerminalStatePersistence()
			_ = ln.Close()
			if err := brokerClient.ShutdownBroker(); err != nil {
				logger.Printf("session broker shutdown warning: %v", err)
			}
		case <-serveDone:
		}
	}()
	err = srv.Serve(ln)
	close(serveDone)
	signal.Stop(sigCh)
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func acquireDaemonLock(ws string, wait bool) (*os.File, error) {
	if !wait {
		return state.AcquireDaemonLock(ws)
	}
	deadline := time.Now().Add(8 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		f, err := state.AcquireDaemonLock(ws)
		if err == nil {
			return f, nil
		}
		last = err
		time.Sleep(40 * time.Millisecond)
	}
	return nil, fmt.Errorf("waiting for previous daemon lock: %w", last)
}

func createListener(cfg config.Config, handoffFD int, listenAddr string) (net.Listener, error) {
	if handoffFD >= 3 {
		file := os.NewFile(uintptr(handoffFD), "vscode_tasks_menu-handoff-listener")
		if file == nil {
			return nil, fmt.Errorf("invalid inherited listener fd %d", handoffFD)
		}
		ln, err := net.FileListener(file)
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("restore inherited listener: %w", err)
		}
		return ln, nil
	}
	address := strings.TrimSpace(listenAddr)
	if address == "" {
		address = cfg.Address()
	}
	return net.Listen("tcp", address)
}

func handoffToUpdatedDaemon(ws string, ln net.Listener, updateID string) error {
	provider, ok := ln.(interface{ File() (*os.File, error) })
	if !ok {
		return fmt.Errorf("listener does not support file-descriptor handoff")
	}
	listenerFile, err := provider.File()
	if err != nil {
		return fmt.Errorf("duplicate listener: %w", err)
	}
	defer listenerFile.Close()
	exe, err := preferredTaskdeckExecutable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(state.LogPath(ws), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--workspace", ws, "--serve", "--handoff-fd", "3", "--self-update-id", updateID)
	cmd.ExtraFiles = []*os.File{listenerFile}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Stdin = nil
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start replacement daemon: %w", err)
	}
	_ = cmd.Process.Release()
	_ = logFile.Close()
	// The child now owns a duplicated listening socket and waits for the old
	// daemon lock. The independent session broker deliberately remains alive,
	// preserving PTYs while the web daemon process is replaced.
	return ln.Close()
}

func publicURLForListener(cfg config.Config, ln net.Listener) string {
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	host := strings.TrimSpace(cfg.AdvertiseHost)
	if host == "" {
		host = cfg.Bind
	}
	if host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	scheme := "http"
	if cfg.TLS() {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(strings.Trim(host, "[]"), port))
}

func healthURLForListener(cfg config.Config, ln net.Listener) string {
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	scheme := "http"
	if cfg.TLS() {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, port))
}

func startDaemon(ws string) error {
	return startDaemonWithOptions(ws, "", "")
}

func startDaemonWithOptions(ws, listenAddr, updateID string) error {
	exe, err := preferredTaskdeckExecutable()
	if err != nil {
		return err
	}
	return startDaemonExecutableWithOptions(exe, ws, listenAddr, updateID)
}

func startDaemonExecutableWithOptions(exe, ws, listenAddr, updateID string) error {
	if err := state.EnsureDir(ws); err != nil {
		return err
	}
	if !selfupdate.ExecutableExists(exe) {
		return fmt.Errorf("TaskDeck executable is unavailable: %s", exe)
	}
	logFile, err := os.OpenFile(state.LogPath(ws), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	args := []string{"--workspace", ws, "--serve"}
	if strings.TrimSpace(listenAddr) != "" {
		args = append(args, "--listen-addr", listenAddr)
	}
	if updateID != "" {
		args = append(args, "--self-update-id", updateID)
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Stdin = nil
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	_ = cmd.Process.Release()
	return logFile.Close()
}

func reloadDaemonConfig(ws string, cfg config.Config, cfgPath string) error {
	st, err := state.Load(ws)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Config hợp lệ: %s\nDaemon chưa chạy; không cần reload.\n", cfgPath)
			return nil
		}
		return err
	}
	if !state.Healthy(st) {
		state.Remove(ws)
		fmt.Printf("Config hợp lệ: %s\nDaemon không còn hoạt động; đã dọn state cũ.\n", cfgPath)
		return nil
	}
	if runtime.GOOS == "windows" {
		return fmt.Errorf("-reload-config chưa hỗ trợ restart giữ session broker trên Windows; dùng -restart-daemon")
	}
	process, err := os.FindProcess(st.PID)
	if err != nil {
		return fmt.Errorf("find daemon pid %d: %w", st.PID, err)
	}
	if err := process.Signal(reloadConfigSignal); err != nil {
		return fmt.Errorf("gửi tín hiệu reload config tới daemon pid %d: %w", st.PID, err)
	}
	if err := waitForDaemonStateRelease(ws, st.PID, 5*time.Second); err != nil {
		return fmt.Errorf("reload config: %w", err)
	}
	if err := startDaemon(ws); err != nil {
		return fmt.Errorf("khởi động lại daemon sau reload config: %w", err)
	}
	next, err := waitForDaemon(ws, 8*time.Second, st.PID)
	if err != nil {
		return fmt.Errorf("reload config: %w", err)
	}
	fmt.Printf("Đã reload config: %s\n%s\n", cfgPath, next.URL)
	printRemoteFirewallGuidance(os.Stdout, cfg, next.Address)
	printRemoteSecurityWarning(cfg)
	printTLSStatus(ws, cfg)
	return nil
}

func remoteWildcardHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	return host == "0.0.0.0" || host == "::"
}

func remoteFirewallGuidance(cfg config.Config, address string) []string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	configWildcard := remoteWildcardHost(cfg.Bind)
	liveWildcard := err == nil && remoteWildcardHost(host)
	if !configWildcard && !liveWildcard {
		return nil
	}

	lines := make([]string, 0, 4)
	if configWildcard && err == nil && !liveWildcard {
		lines = append(lines, fmt.Sprintf("WARNING: config bind=%s nhưng daemon hiện listen %s; chạy --reload-config để áp dụng remote access.", cfg.Bind, address))
	}

	if err != nil || port == "" || port == "0" {
		if cfg.Port > 0 {
			port = fmt.Sprintf("%d", cfg.Port)
		} else {
			lines = append(lines, "Remote access wildcard bind đang bật; hãy mở TCP port sau khi daemon bind thành công.")
			return lines
		}
	}
	lines = append(lines,
		fmt.Sprintf("Remote access wildcard bind đang dùng TCP port %s.", port),
		fmt.Sprintf("UFW: sudo ufw allow %s/tcp", port),
		fmt.Sprintf("firewalld: sudo firewall-cmd --permanent --add-port=%s/tcp && sudo firewall-cmd --reload", port),
	)
	return lines
}
func printRemoteFirewallGuidance(w *os.File, cfg config.Config, address string) {
	for _, line := range remoteFirewallGuidance(cfg, address) {
		fmt.Fprintln(w, line)
	}
}

func printTLSStatus(ws string, cfg config.Config) {
	if !cfg.TLS() {
		fmt.Fprintln(os.Stdout, "Protocol: HTTP (TLS disabled by config)")
		return
	}
	certPath := cfg.TLSCert
	auto := false
	if !cfg.CustomTLS() {
		var err error
		certPath, _, err = tlscert.AutoPaths(ws)
		if err != nil {
			fmt.Fprintf(os.Stdout, "HTTPS: auto self-signed certificate path unavailable: %v\n", err)
			return
		}
		auto = true
	}
	if auto {
		fmt.Fprintf(os.Stdout, "HTTPS: auto self-signed certificate: %s\n", certPath)
	} else {
		fmt.Fprintf(os.Stdout, "HTTPS: configured certificate: %s\n", certPath)
	}
	if fingerprint, err := tlscert.FingerprintSHA256(certPath); err == nil {
		fmt.Fprintf(os.Stdout, "Certificate SHA-256: %s\n", fingerprint)
	}
	if auto {
		fmt.Fprintln(os.Stdout, "NOTE: self-signed HTTPS encrypts traffic but is not trusted by browsers by default; verify the fingerprint before trusting the certificate.")
	}
}

func printRemoteSecurityWarning(cfg config.Config) {
	if warning := server.RemoteWarning(cfg); warning != "" {
		fmt.Fprintln(os.Stdout, warning)
	}
}

func printDaemonStatus(ws string, cfg config.Config) {
	st, err := state.Load(ws)
	if err != nil || !state.Healthy(st) {
		if err == nil {
			state.Remove(ws)
		}
		fmt.Println("stopped")
		return
	}
	fmt.Printf("running pid=%d url=%s started=%s\n", st.PID, st.URL, st.StartedAt)
	printRemoteFirewallGuidance(os.Stdout, cfg, st.Address)
	printRemoteSecurityWarning(cfg)
	printTLSStatus(ws, cfg)
}

func stopExistingDaemon(ws string) error {
	st, err := state.Load(ws)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Daemon chưa chạy.")
			return nil
		}
		return err
	}
	if !state.Healthy(st) {
		state.Remove(ws)
		fmt.Println("Daemon không còn hoạt động; đã dọn state cũ.")
		return nil
	}
	process, err := os.FindProcess(st.PID)
	if err != nil {
		return fmt.Errorf("find daemon pid %d: %w", st.PID, err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("dừng daemon pid %d: %w", st.PID, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !state.Healthy(st) {
			state.Remove(ws)
			fmt.Printf("Đã dừng daemon pid=%d.\n", st.PID)
			return nil
		}
		time.Sleep(80 * time.Millisecond)
	}
	return fmt.Errorf("daemon pid %d không dừng sau 5 giây", st.PID)
}

func effectiveInstalledRevision(embedded, marker string) string {
	embedded = strings.TrimSpace(embedded)
	if embedded != "" && embedded != "dev" {
		return embedded
	}
	return strings.TrimSpace(marker)
}

func shortRevision(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func resolveWorkspaceForStateCheck() string {
	return activeWorkspaceForUpdateCheck
}

func daemonNeedsGlobalMigration(st state.State, global string) bool {
	if st.PID <= 0 || strings.TrimSpace(global) == "" {
		return false
	}
	if runtime.GOOS != "linux" {
		return false
	}
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", st.PID))
	if err != nil {
		return false
	}
	return !selfupdate.SameExecutablePath(exe, global)
}

func checkSelfUpdate(ctx context.Context) (server.SelfUpdateCheckResult, error) {
	remote, err := selfupdate.RemoteRevision(ctx)
	if err != nil {
		return server.SelfUpdateCheckResult{}, fmt.Errorf("check latest revision: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return server.SelfUpdateCheckResult{}, err
	}
	global, err := selfupdate.GlobalBinaryPath()
	if err != nil {
		return server.SelfUpdateCheckResult{}, err
	}
	markerRevision := selfupdate.InstalledRevision(global)
	installed := effectiveInstalledRevision(buildRevision, markerRevision)
	migrationPending := !selfupdate.GlobalReleaseReady(remote) || !selfupdate.SameExecutablePath(exe, global)
	if st, stateErr := state.Load(resolveWorkspaceForStateCheck()); stateErr == nil && state.Healthy(st) {
		migrationPending = migrationPending || daemonNeedsGlobalMigration(st, global)
	}
	return server.SelfUpdateCheckResult{
		Available:          migrationPending || installed != remote,
		InstalledRevision: installed,
		RemoteRevision:    remote,
	}, nil
}
func preferredTaskdeckExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return selfupdate.PreferredBinary(exe), nil
}

func startAutoSelfUpdate(ws string) error {
	exe, err := preferredTaskdeckExecutable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(state.LogPath(ws), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--workspace", ws, "--self-update-auto")
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Stdin = nil
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start automatic self-update: %w", err)
	}
	_ = cmd.Process.Release()
	return logFile.Close()
}

func runSelfUpdate(ws string, cfg config.Config, autoConfirm bool) (err error) {
	lock, err := state.AcquireStartLock(ws)
	if err != nil {
		return err
	}
	defer lock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	remote, err := selfupdate.RemoteRevision(ctx)
	if err != nil {
		return fmt.Errorf("check latest revision: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	global, err := selfupdate.GlobalBinaryPath()
	if err != nil {
		return err
	}
	activationBefore, err := selfupdate.CaptureGlobalActivation()
	if err != nil {
		return fmt.Errorf("snapshot current TaskDeck activation: %w", err)
	}

	oldState, stateErr := state.Load(ws)
	daemonRunning := stateErr == nil && state.Healthy(oldState)
	oldDaemonExecutable := exe
	if daemonRunning && runtime.GOOS == "linux" {
		if runningExe, readErr := os.Readlink(fmt.Sprintf("/proc/%d/exe", oldState.PID)); readErr == nil && selfupdate.ExecutableExists(runningExe) {
			oldDaemonExecutable = runningExe
		}
	}
	migratingToGlobal := !selfupdate.SameExecutablePath(exe, global)
	if daemonRunning && daemonNeedsGlobalMigration(oldState, global) {
		migratingToGlobal = true
	}

	markerRevision := selfupdate.InstalledRevision(global)
	installedRevision := effectiveInstalledRevision(buildRevision, markerRevision)
	releaseReady := selfupdate.GlobalReleaseReady(remote)
	if !migratingToGlobal && releaseReady && installedRevision == remote {
		fmt.Printf("Đã là bản mới nhất: %s\n", remote[:12])
		return nil
	}
	if markerRevision == remote && installedRevision != remote {
		fmt.Fprintf(os.Stderr, "WARNING: revision marker=%s nhưng binary revision=%s; sẽ xác minh/cài lại release.\n", shortRevision(markerRevision), shortRevision(installedRevision))
	}

	currentURL := ""
	if daemonRunning {
		currentURL = oldState.URL
	}
	req, err := selfupdate.CreateRequest(ws, remote, currentURL, !daemonRunning || autoConfirm)
	if err != nil {
		return err
	}
	fail := func(cause error) error {
		_, _ = selfupdate.Update(ws, req.ID, "failed", "Cập nhật thất bại.", "", cause.Error())
		return cause
	}

	if daemonRunning && !autoConfirm {
		switch {
		case !releaseReady:
			fmt.Printf("TaskDeck sẽ cài release %s gồm binary + Patch add-on. Chờ xác nhận tại %s ...\n", remote[:12], oldState.URL)
		case migratingToGlobal:
			fmt.Printf("TaskDeck sẽ chuyển daemon sang global release tại %s. Chờ xác nhận tại %s ...\n", global, oldState.URL)
		default:
			fmt.Printf("Có bản mới %s. Chờ xác nhận tại %s ...\n", remote[:12], oldState.URL)
		}
		decision, decisionErr := selfupdate.WaitForDecision(ws, req.ID, 30*time.Minute)
		if decisionErr != nil {
			if decision.Status == "cancelled" {
				fmt.Println("Đã hủy self-update.")
				return nil
			}
			return fail(decisionErr)
		}
	} else if daemonRunning {
		if !releaseReady {
			fmt.Printf("Tự động cài TaskDeck + Patch add-on release %s từ Settings.\n", remote[:12])
		} else if migratingToGlobal {
			fmt.Printf("Tự động chuyển daemon sang global TaskDeck: %s\n", global)
		}
	}

	progress := func(status, message string) {
		fmt.Println(message)
		_, _ = selfupdate.Update(ws, req.ID, status, message, "", "")
	}

	if !releaseReady {
		prepared, prepareErr := selfupdate.PrepareGlobalRelease(ctx, remote, progress)
		if prepareErr != nil {
			return fail(prepareErr)
		}
		finalRelease, finalErr := selfupdate.GlobalReleaseDir(remote)
		if finalErr != nil {
			return fail(finalErr)
		}
		if filepath.Clean(prepared) != filepath.Clean(finalRelease) {
			defer os.RemoveAll(prepared)
		}
		progress("installing", "Đang cài TaskDeck + Patch add-on vào versioned release…")
		if installErr := selfupdate.InstallGlobalRelease(prepared, remote); installErr != nil {
			if rollbackErr := selfupdate.RestoreGlobalActivation(activationBefore); rollbackErr != nil {
				return fail(fmt.Errorf("%v; rollback previous release failed: %w", installErr, rollbackErr))
			}
			return fail(installErr)
		}
		releaseReady = true
	} else if migratingToGlobal {
		progress("installing", "Global versioned release đã sẵn sàng; chuyển daemon sang "+global+"…")
	}

	if !releaseReady {
		return fail(fmt.Errorf("global TaskDeck release is not ready after installation"))
	}

	if !daemonRunning {
		message := "Đã cài TaskDeck + Patch add-on global release: " + global
		_, _ = selfupdate.Update(ws, req.ID, "completed", message, "", "")
		fmt.Printf("%s\n", message)
		return nil
	}

	_, _ = selfupdate.Update(ws, req.ID, "ready_restart", "Release mới đã sẵn sàng; chuẩn bị handoff daemon…", "", "")
	if err := requestDaemonHandoff(cfg, oldState, req.ID); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: socket handoff unavailable: %v; fallback restart cùng port.\n", err)
		if err := fallbackRestartAfterUpdate(ws, cfg, oldState, req.ID); err != nil {
			if recoveryErr := recoverPreviousSelfUpdate(ws, oldState, oldDaemonExecutable, activationBefore); recoveryErr != nil {
				return fail(fmt.Errorf("%v; previous release recovery failed: %w", err, recoveryErr))
			}
			return fail(err)
		}
	}
	newState, err := waitForDaemon(ws, 15*time.Second, oldState.PID)
	if err != nil {
		if latest, loadErr := selfupdate.Load(ws); loadErr == nil && latest.Status == "failed" {
			if restartErr := fallbackRestartAfterUpdate(ws, cfg, oldState, req.ID); restartErr == nil {
				newState, err = waitForDaemon(ws, 15*time.Second, oldState.PID)
			}
		}
	}
	if err != nil {
		if recoveryErr := recoverPreviousSelfUpdate(ws, oldState, oldDaemonExecutable, activationBefore); recoveryErr != nil {
			return fail(fmt.Errorf("%v; previous release recovery failed: %w", err, recoveryErr))
		}
		return fail(err)
	}

	message := "Cập nhật TaskDeck + Patch add-on hoàn tất; daemon mới đã sẵn sàng."
	if migratingToGlobal {
		message = "Đã chuyển daemon sang TaskDeck global versioned release: " + global
	}
	_, _ = selfupdate.Update(ws, req.ID, "completed", message, newState.URL, "")
	fmt.Printf("%s\n%s\n", message, newState.URL)
	if newState.URL != oldState.URL && cfg.OpenBrowser {
		_ = openBrowser(newState.URL)
	}
	return nil
}
func recoverPreviousSelfUpdate(ws string, old state.State, oldExecutable string, activation selfupdate.GlobalActivationSnapshot) error {
	if err := selfupdate.RestoreGlobalActivation(activation); err != nil {
		return err
	}
	if current, err := state.Load(ws); err == nil && state.Healthy(current) {
		return nil
	}

	if old.PID > 0 {
		_ = waitForDaemonStateRelease(ws, old.PID, 5*time.Second)
	}
	state.Remove(ws)
	if !selfupdate.ExecutableExists(oldExecutable) {
		return fmt.Errorf("previous daemon executable is unavailable: %s", oldExecutable)
	}
	if err := startDaemonExecutableWithOptions(oldExecutable, ws, old.Address, ""); err != nil {
		return fmt.Errorf("restart previous daemon: %w", err)
	}
	if _, err := waitForDaemon(ws, 10*time.Second, 0); err != nil {
		return fmt.Errorf("previous daemon did not recover: %w", err)
	}
	return nil
}

func requestDaemonAction(cfg config.Config, st state.State, id, action string) error {
	base := strings.TrimRight(st.HealthURL, "/")
	if base == "" {
		base = strings.TrimRight(st.URL, "/")
	}
	url := base + "/api/state/tasks?scope=self-update&action=" + action
	body, _ := json.Marshal(map[string]string{"id": id})
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.AuthEnabled {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.HasPrefix(strings.ToLower(url), "https://") {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- loopback control request only
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s endpoint returned %s", action, resp.Status)
	}
	return nil
}

func requestDaemonHandoff(cfg config.Config, st state.State, id string) error {
	return requestDaemonAction(cfg, st, id, "handoff")
}

func requestDaemonDetach(cfg config.Config, st state.State, id string) error {
	return requestDaemonAction(cfg, st, id, "detach")
}

func waitForDaemonStateRelease(ws string, oldPID int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, err := state.Load(ws)
		if os.IsNotExist(err) {
			return nil
		}
		if err == nil && st.PID != oldPID {
			return nil
		}
		time.Sleep(40 * time.Millisecond)
	}
	return fmt.Errorf("old daemon pid %d did not release runtime state", oldPID)
}

func fallbackRestartAfterUpdate(ws string, cfg config.Config, old state.State, updateID string) error {
	preserved := false
	if state.Healthy(old) {
		if err := requestDaemonDetach(cfg, old, updateID); err == nil {
			if waitErr := waitForDaemonStateRelease(ws, old.PID, 5*time.Second); waitErr == nil {
				preserved = true
			} else {
				fmt.Fprintf(os.Stderr, "WARNING: daemon detach timed out: %v; falling back to legacy stop.\n", waitErr)
			}
		} else {
			fmt.Fprintf(os.Stderr, "WARNING: broker-preserving daemon detach unavailable: %v; falling back to legacy stop.\n", err)
		}
	} else if err := waitForDaemonStateRelease(ws, old.PID, 2*time.Second); err == nil {
		// The old listener is already gone (for example after a partial handoff).
		// Let the process finish naturally so its independent broker stays alive.
		preserved = true
	}
	if !preserved {
		if err := stopExistingDaemon(ws); err != nil {
			return err
		}
	}
	if err := startDaemonWithOptions(ws, old.Address, updateID); err == nil {
		return nil
	}
	// Last resort: config may choose another ephemeral port. Browser UI receives
	// target_url after startup; launcher also opens it when configured.
	return startDaemonWithOptions(ws, "", updateID)
}

func waitForDaemon(ws string, timeout time.Duration, differentPID int) (state.State, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, err := state.Load(ws)
		if err == nil && (differentPID == 0 || st.PID != differentPID) && state.Healthy(st) {
			return st, nil
		}
		time.Sleep(80 * time.Millisecond)
	}
	return state.State{}, fmt.Errorf("daemon không khởi động; xem log: %s", state.LogPath(ws))
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return nil
	}
	return cmd.Start()
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}
