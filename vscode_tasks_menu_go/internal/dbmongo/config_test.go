package dbmongo

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func TestConfigFromConnectPayloadDefaultsAndOptions(t *testing.T) {
	config, err := ConfigFromConnectPayload(dbadapter.ConnectPayload{
		Username: " app ",
		Secret:   "p@ss:/word",
		Database: "main",
		ReadOnly: true,
		Options: map[string]string{
			"auth_source":             "admin",
			"connect_timeout_seconds": "25",
			"tls":                     "true",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != defaultHost || config.Port != defaultPort {
		t.Fatalf("endpoint=%s:%d", config.Host, config.Port)
	}
	if config.Username != "app" || config.Password != "p@ss:/word" || config.Database != "main" {
		t.Fatalf("config=%+v", config)
	}
	if config.AuthSource != "admin" || !config.TLS || config.ConnectTimeout != 25*time.Second || !config.ReadOnly {
		t.Fatalf("options=%+v", config)
	}
}

func TestConfigURIEncodesCredentialsAndOptions(t *testing.T) {
	config, err := ConfigFromConnectPayload(dbadapter.ConnectPayload{
		Host:     "db.example.com",
		Port:     27018,
		Username: "user:name",
		Secret:   "p@ss/word",
		Database: "main",
		Options: map[string]string{
			"auth_source": "admin",
			"tls":         "true",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	uri, err := config.URI()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	password, ok := parsed.User.Password()
	if !ok || parsed.User.Username() != "user:name" || password != "p@ss/word" {
		t.Fatalf("userinfo=%v password=%q ok=%v uri=%q", parsed.User, password, ok, uri)
	}
	if parsed.Host != "db.example.com:27018" || parsed.Path != "/main" {
		t.Fatalf("uri=%q", uri)
	}
	query := parsed.Query()
	if query.Get("authSource") != "admin" || query.Get("tls") != "true" {
		t.Fatalf("query=%v", query)
	}
	if query.Get("connectTimeoutMS") != "10000" || query.Get("serverSelectionTimeoutMS") != "10000" {
		t.Fatalf("timeouts=%v", query)
	}
}

func TestConfigFromConnectPayloadRejectsUnsafeValues(t *testing.T) {
	tests := []dbadapter.ConnectPayload{
		{Host: "db host"},
		{Port: 70000},
		{Username: "user\nname"},
		{Secret: "secret\nnext", Username: "user"},
		{Secret: "secret"},
		{Database: "bad/name"},
		{File: "/tmp/mongo.db"},
		{Options: map[string]string{"auth_source": "bad/name"}},
		{Options: map[string]string{"connect_timeout_seconds": "0"}},
		{Options: map[string]string{"tls": "maybe"}},
		{Options: map[string]string{"unknown": "x"}},
	}
	for i, payload := range tests {
		if _, err := ConfigFromConnectPayload(payload); err == nil {
			t.Fatalf("unsafe payload %d unexpectedly accepted: %+v", i, payload)
		}
	}
}

func TestURIStringContainsEncodedNotRawSpecialPassword(t *testing.T) {
	config := Config{
		Host: "127.0.0.1", Port: 27017,
		Username: "user", Password: "secret/with?reserved",
		ConnectTimeout: 10 * time.Second,
	}
	uri, err := config.URI()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(uri, "secret/with?reserved") {
		t.Fatalf("raw password appeared in URI: %q", uri)
	}
}
