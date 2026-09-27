package dbredis

import (
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func TestConfigFromConnectPayloadDefaultsAndOptions(t *testing.T) {
	config, err := ConfigFromConnectPayload(dbadapter.ConnectPayload{
		Username: " app ",
		Secret:   "secret",
		Database: "2",
		ReadOnly: true,
		Options: map[string]string{
			"connect_timeout_seconds": "3",
			"command_timeout_seconds": "7",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != defaultHost || config.Port != defaultPort || config.Database != 2 {
		t.Fatalf("config=%+v", config)
	}
	if config.Username != "app" || config.Password != "secret" || !config.ReadOnly {
		t.Fatalf("config=%+v", config)
	}
	if config.ConnectTimeout != 3*time.Second || config.CommandTimeout != 7*time.Second {
		t.Fatalf("timeouts=%+v", config)
	}
}

func TestConfigFromConnectPayloadRejectsUnsafeValues(t *testing.T) {
	tests := []dbadapter.ConnectPayload{
		{Host: "redis host"},
		{Port: 70000},
		{Username: "user\nname"},
		{Secret: "secret\nnext"},
		{Database: "-1"},
		{Database: "70000"},
		{File: "/tmp/redis.db"},
		{Username: "user"},
		{Options: map[string]string{"connect_timeout_seconds": "0"}},
		{Options: map[string]string{"command_timeout_seconds": "9999"}},
		{Options: map[string]string{"unknown": "1"}},
	}
	for i, payload := range tests {
		if _, err := ConfigFromConnectPayload(payload); err == nil {
			t.Fatalf("unsafe payload %d unexpectedly accepted: %+v", i, payload)
		}
	}
}
