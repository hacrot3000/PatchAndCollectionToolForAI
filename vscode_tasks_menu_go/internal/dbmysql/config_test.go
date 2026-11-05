package dbmysql

import (
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func TestConfigFromConnectPayloadAppliesDefaultsAndOptions(t *testing.T) {
	config, err := ConfigFromConnectPayload(dbadapter.ConnectPayload{
		Username: " app ",
		Database: " main ",
		Secret:   "secret",
		ReadOnly: true,
		Options: map[string]string{
			"charset":                 " latin1 ",
			"connect_timeout_seconds": "25",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != defaultHost || config.Port != defaultPort {
		t.Fatalf("endpoint=%s:%d", config.Host, config.Port)
	}
	if config.Username != "app" || config.Database != "main" {
		t.Fatalf("identity=%q database=%q", config.Username, config.Database)
	}
	if config.Charset != "latin1" || config.ConnectTimeoutSeconds != 25 {
		t.Fatalf("options=%+v", config)
	}
	if !config.ReadOnly || config.Secret != "secret" {
		t.Fatalf("config=%+v", config)
	}
}

func TestConfigFromConnectPayloadRejectsUnsafeValues(t *testing.T) {
	tests := []dbadapter.ConnectPayload{
		{Host: "db host"},
		{Port: 70000},
		{Username: "app\nroot"},
		{Database: "main\rnext"},
		{Secret: "secret\nnext"},
		{Options: map[string]string{"charset": "utf8;drop"}},
		{Options: map[string]string{"charset": "utf8_bin"}},
		{Options: map[string]string{"charset": "utf8_general_ci"}},
		{Options: map[string]string{"charset": "utf8mb4_unicode_ci"}},
		{Options: map[string]string{"connect_timeout_seconds": "0"}},
		{Options: map[string]string{"unknown": "value"}},
	}
	for i, payload := range tests {
		if _, err := ConfigFromConnectPayload(payload); err == nil {
			t.Fatalf("unsafe payload %d unexpectedly accepted: %+v", i, payload)
		}
	}
}

func TestClientArgsNeverContainPassword(t *testing.T) {
	config, err := ConfigFromConnectPayload(dbadapter.ConnectPayload{
		Host:     "db.example.com",
		Port:     3307,
		Username: "app",
		Database: "main",
		Secret:   "top-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	credentials := &credentialFile{path: "/tmp/private/client.cnf"}
	args := clientArgs(config, credentials)
	joined := strings.Join(args, "\n")
	for _, want := range []string{
		"--defaults-extra-file=/tmp/private/client.cnf",
		"--protocol=tcp",
		"--batch",
		"--xml",
		"--host=db.example.com",
		"--port=3307",
		"--user=app",
		"--database=main",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %#v", want, args)
		}
	}
	if strings.Contains(joined, config.Secret) {
		t.Fatalf("password leaked into client argv: %#v", args)
	}

	config.ReadOnly = true
	readOnlyArgs := strings.Join(clientArgs(config, credentials), "\n")
	if !strings.Contains(readOnlyArgs, "--init-command=SET SESSION TRANSACTION READ ONLY") {
		t.Fatalf("read-only session option missing: %s", readOnlyArgs)
	}
}


func TestClientArgsForClientKeepsBlobBytesAsHexWhenSupported(t *testing.T) {
	config := Config{Host: "127.0.0.1", Port: 3306, Charset: defaultCharset, ConnectTimeoutSeconds: 10}
	tests := []struct {
		name   string
		client Client
		hex    bool
	}{
		{"mysql", Client{Flavor: "mysql", Version: "mysql Ver 8.0"}, true},
		{"mariadb", Client{Flavor: "mariadb", Version: "mariadb 10.11"}, false},
		{"mariadb mysql alias", Client{Flavor: "mysql", Version: "mysql Ver 15.1 Distrib 10.11.6-MariaDB"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := clientArgsForClient(tt.client, config, nil)
			hasHex := false
			for _, arg := range args {
				if arg == "--binary-as-hex" {
					hasHex = true
				}
			}
			if hasHex != tt.hex {
				t.Fatalf("hex=%v args=%v", hasHex, args)
			}
			if !strings.Contains(strings.Join(args, "\n"), "--xml") {
				t.Fatalf("XML transport missing: %v", args)
			}
		})
	}
}

func TestLegacyUTF8CharsetDoesNotBecomeUTF8MB4InClientArgs(t *testing.T) {
	config, err := ConfigFromConnectPayload(dbadapter.ConnectPayload{Options: map[string]string{"charset": "utf8"}})
	if err != nil {
		t.Fatal(err)
	}
	if config.Charset != "utf8" {
		t.Fatalf("configured charset unexpectedly changed: %q", config.Charset)
	}
	arguments := strings.Join(clientArgs(config, nil), " ")
	if !strings.Contains(arguments, "--default-character-set=utf8") || strings.Contains(arguments, "utf8mb4") {
		t.Fatalf("MySQL CLI args ignored profile charset: %s", arguments)
	}
	for _, bad := range []string{"utf8_bin", "utf8_general_ci"} {
		_, err := ConfigFromConnectPayload(dbadapter.ConnectPayload{Options: map[string]string{"charset": bad}})
		if err == nil || !strings.Contains(err.Error(), "is a collation") {
			t.Fatalf("expected actionable charset-vs-collation error for %q, got %v", bad, err)
		}
	}
}
