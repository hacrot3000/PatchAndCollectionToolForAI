package dbprofile

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeDirectProfile(t *testing.T) {
	profile, err := Normalize(Profile{
		ID:        "mysql-prod",
		Name:      " MySQL Production ",
		AdapterID: "mysql-cli",
		Host:      "db.example.com",
		Port:      3306,
		Username:  "app",
		Database:  "main",
		ReadOnly:  true,
		SecretRef: "db/mysql-prod/password",
		Options: map[string]string{
			"charset": " utf8mb4 ",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Transport != TransportDirect {
		t.Fatalf("transport=%q want direct", profile.Transport)
	}
	if profile.Name != "MySQL Production" || profile.Options["charset"] != "utf8mb4" {
		t.Fatalf("normalized profile=%+v", profile)
	}
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), profile.SecretRef) || strings.Contains(string(data), "secret_ref") {
		t.Fatalf("secret reference leaked into public profile JSON: %s", data)
	}
}

func TestNormalizeSSHTunnelRequiresSSHProfileAndRemoteEndpoint(t *testing.T) {
	base := Profile{
		ID:           "mysql-prod",
		Name:         "MySQL Production",
		AdapterID:    "mysql-cli",
		Transport:    TransportSSHTunnel,
		Host:         "127.0.0.1",
		Port:         3306,
		SSHProfileID: "prod-ssh",
	}
	if _, err := Normalize(base); err != nil {
		t.Fatal(err)
	}

	missingSSH := base
	missingSSH.SSHProfileID = ""
	if _, err := Normalize(missingSSH); err == nil {
		t.Fatal("expected missing SSH profile to fail")
	}
	missingHost := base
	missingHost.Host = ""
	if _, err := Normalize(missingHost); err == nil {
		t.Fatal("expected missing remote host to fail")
	}
	missingPort := base
	missingPort.Port = 0
	if _, err := Normalize(missingPort); err == nil {
		t.Fatal("expected missing remote port to fail")
	}
}

func TestNormalizeDirectRejectsSSHProfileReference(t *testing.T) {
	_, err := Normalize(Profile{
		ID:           "mysql-prod",
		Name:         "MySQL Production",
		AdapterID:    "mysql-cli",
		Transport:    TransportDirect,
		SSHProfileID: "prod-ssh",
	})
	if err == nil || !strings.Contains(err.Error(), "must not reference") {
		t.Fatalf("error=%v", err)
	}
}

func TestNormalizeRejectsUnsafeOptionsAndHost(t *testing.T) {
	for _, profile := range []Profile{
		{
			ID: "db", Name: "DB", AdapterID: "mysql-cli",
			Host: "db host",
		},
		{
			ID: "db", Name: "DB", AdapterID: "mysql-cli",
			Options: map[string]string{"bad/key": "value"},
		},
		{
			ID: "db", Name: "DB", AdapterID: "mysql-cli",
			Options: map[string]string{"charset": "bad\nvalue"},
		},
	} {
		if _, err := Normalize(profile); err == nil {
			t.Fatalf("unsafe profile unexpectedly accepted: %+v", profile)
		}
	}
}

func TestNewIDProducesDistinctValidIDs(t *testing.T) {
	first, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("duplicate generated id %q", first)
	}
	for _, id := range []string{first, second} {
		if err := validateToken("database profile id", id, maxIDBytes, true); err != nil {
			t.Fatalf("generated id %q invalid: %v", id, err)
		}
	}
}
