package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
)

func TestSecurityHeadersHardenTLSResponses(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	securityHeaders(next, true).ServeHTTP(rr, req)

	if got := rr.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("HSTS=%q", got)
	}
	csp := rr.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'self'",
		"object-src 'none'",
		"base-uri 'none'",
		"frame-ancestors 'none'",
		"form-action 'self'",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP missing %q: %s", want, csp)
		}
	}
}

func TestSecurityHeadersDoNotSendHSTSOnPlainHTTP(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	securityHeaders(next, false).ServeHTTP(rr, req)
	if got := rr.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("plain HTTP unexpectedly sent HSTS=%q", got)
	}
}

func TestRemoteWarningRequiresTLSForUntrustedNetwork(t *testing.T) {
	cfg := config.Default()
	cfg.Bind = "0.0.0.0"
	cfg.AuthEnabled = true
	cfg.Username = "admin"
	cfg.Password = "strong-password"
	warning := RemoteWarning(cfg)
	for _, want := range []string{"SECURITY WARNING", "Basic Auth", "tls_cert/tls_key", "auth.enabled=true"} {
		if !strings.Contains(warning, want) {
			t.Fatalf("remote HTTP warning missing %q: %s", want, warning)
		}
	}

	cfg.TLSCert = "/tmp/server.crt"
	cfg.TLSKey = "/tmp/server.key"
	if warning := RemoteWarning(cfg); warning != "" {
		t.Fatalf("TLS remote config unexpectedly warned: %s", warning)
	}
}

func TestSessionWebSocketPinsReadLimit(t *testing.T) {
	data, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "conn.SetReadLimit(32 << 10)") {
		t.Fatal("session WebSocket must pin an application read limit")
	}
}


func TestServeRejectsUnauthenticatedEffectiveRemoteListener(t *testing.T) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skipf("cannot create wildcard listener: %v", err)
	}
	s := &Server{Config: config.Default()}
	err = s.Serve(ln)
	if err == nil {
		t.Fatal("Serve accepted a non-loopback listener while auth was disabled")
	}
	if !strings.Contains(err.Error(), "listener security validation") {
		t.Fatalf("unexpected Serve error: %v", err)
	}
}
