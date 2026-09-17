package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/server"
	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/state"
	terminalui "bletonfc/vscode_tasks_menu/internal/terminal"
)

func main() {
	workspace := flag.String("workspace", "", "workspace chứa .vscode/tasks.json")
	serve := flag.Bool("serve", false, "chạy HTTP server foreground (internal)")
	terminal := flag.Bool("terminal", false, "mở terminal menu native Go")
	noBrowser := flag.Bool("no-browser", false, "không tự mở trình duyệt")
	statusOnly := flag.Bool("status", false, "in trạng thái daemon rồi thoát")
	stopDaemonFlag := flag.Bool("stop-daemon", false, "dừng daemon của workspace rồi thoát")
	restartDaemon := flag.Bool("restart-daemon", false, "dừng daemon cũ rồi khởi động lại")
	flag.Parse()

	ws, err := resolveWorkspace(*workspace)
	fatalIf(err)
	if *terminal {
		fatalIf(terminalui.Run(ws))
		return
	}
	cfg, cfgPath, err := config.Load(ws)
	fatalIf(err)
	if *serve {
		fatalIf(serveForeground(ws, cfg, cfgPath))
		return
	}
	startLock, err := state.AcquireStartLock(ws)
	fatalIf(err)
	defer startLock.Close()

	if *statusOnly {
		printDaemonStatus(ws)
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
		if cfg.OpenBrowser && !*noBrowser {
			_ = openBrowser(st.URL)
		}
		return
	}
	state.Remove(ws)
	fatalIf(startDaemon(ws))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st, err := state.Load(ws); err == nil && state.Healthy(st) {
			fmt.Println(st.URL)
			if cfg.OpenBrowser && !*noBrowser {
				_ = openBrowser(st.URL)
			}
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
	fatalIf(fmt.Errorf("daemon không khởi động; xem log: %s", state.LogPath(ws)))
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

func serveForeground(ws string, cfg config.Config, cfgPath string) error {
	if err := state.EnsureDir(ws); err != nil {
		return err
	}
	daemonLock, err := state.AcquireDaemonLock(ws)
	if err != nil {
		return err
	}
	defer daemonLock.Close()
	ln, err := net.Listen("tcp", cfg.Address())
	if err != nil {
		return err
	}
	url := publicURLForListener(cfg, ln)
	healthURL := healthURLForListener(cfg, ln)
	st := state.New(ws, url, healthURL, ln.Addr().String())
	if err := state.Save(st); err != nil {
		_ = ln.Close()
		return err
	}
	defer state.Remove(ws)
	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf("workspace=%s", ws)
	logger.Printf("config=%s", cfgPath)
	logger.Printf("url=%s", url)
	if warning := server.RemoteWarning(cfg); warning != "" {
		logger.Print(warning)
	}
	manager := session.NewManager(4 << 20)
	srv := &server.Server{Workspace: ws, Config: cfg, Log: logger, Sessions: manager}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	serveDone := make(chan struct{})
	go func() {
		select {
		case sig := <-sigCh:
			logger.Printf("shutdown signal=%s", sig)
			manager.Shutdown(2 * time.Second)
			_ = ln.Close()
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
	if err := state.EnsureDir(ws); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(state.LogPath(ws), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--workspace", ws, "--serve")
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

func printDaemonStatus(ws string) {
	st, err := state.Load(ws)
	if err != nil || !state.Healthy(st) {
		if err == nil {
			state.Remove(ws)
		}
		fmt.Println("stopped")
		return
	}
	fmt.Printf("running pid=%d url=%s started=%s\n", st.PID, st.URL, st.StartedAt)
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
