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
