package server

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/tlscert"
)

func TestServeHTTPSWithAutoSelfSignedCertificate(t *testing.T) {
	t.Setenv("VSCODE_TASKS_MENU_CONFIG_DIR", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	result, err := tlscert.Resolve(workspace, cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.TLSCert = result.CertPath
	cfg.TLSKey = result.KeyPath

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- (&Server{Workspace: workspace, Config: cfg}).Serve(ln)
	}()
	defer func() {
		_ = ln.Close()
		select {
		case <-errCh:
		case <-time.After(time.Second):
		}
	}()

	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // self-signed test certificate
			MinVersion:         tls.VersionTLS12,
		}},
	}
	url := "https://" + ln.Addr().String() + "/api/health"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("HTTPS health status=%d body=%s", resp.StatusCode, body)
	}
	if resp.TLS == nil || resp.TLS.Version < tls.VersionTLS12 {
		t.Fatalf("unexpected TLS state: %#v", resp.TLS)
	}
}

func TestServeHTTPWhenProtocolExplicitlyDisabledTLS(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Protocol = config.ProtocolHTTP
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- (&Server{Workspace: workspace, Config: cfg}).Serve(ln)
	}()
	defer func() {
		_ = ln.Close()
		select {
		case <-errCh:
		case <-time.After(time.Second):
		}
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://" + ln.Addr().String() + "/api/health"
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP health status=%d", resp.StatusCode)
	}
	if resp.TLS != nil {
		t.Fatal("HTTP response unexpectedly negotiated TLS")
	}
}
