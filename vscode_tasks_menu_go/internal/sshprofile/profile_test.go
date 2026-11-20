package sshprofile

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeAppliesSafeDefaults(t *testing.T) {
	got, err := Normalize(Profile{
		ID:         "prod",
		Name:       " Production ",
		Host:       "server.example.com",
		Username:   "deploy",
		AuthMethod: AuthAgent,
		PresetCommands: []PresetCommand{
			{ID: "logs", Name: " Logs ", Command: " tail -f app.log ", Cwd: " /srv/app "},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != DefaultPort {
		t.Fatalf("port = %d, want %d", got.Port, DefaultPort)
	}
	if got.ConnectTimeoutSeconds != DefaultConnectTimeoutSecond {
		t.Fatalf("connect timeout = %d", got.ConnectTimeoutSeconds)
	}
	if got.ServerAliveIntervalSeconds != DefaultServerAliveSecond {
		t.Fatalf("alive interval = %d", got.ServerAliveIntervalSeconds)
	}
	if got.ServerAliveCountMax != DefaultServerAliveCountMax {
		t.Fatalf("alive count = %d", got.ServerAliveCountMax)
	}
	if got.Name != "Production" {
		t.Fatalf("name = %q", got.Name)
	}
	if got.PresetCommands[0].Command != "tail -f app.log" {
		t.Fatalf("preset command = %q", got.PresetCommands[0].Command)
	}
	if got.PresetCommands[0].Cwd != "/srv/app" {
		t.Fatalf("preset cwd = %q", got.PresetCommands[0].Cwd)
	}
}

func TestNormalizeRequiresAuthSpecificFields(t *testing.T) {
	tests := []struct {
		name    string
		profile Profile
		wantErr string
	}{
		{
			name: "password requires secret reference",
			profile: Profile{
				ID: "prod", Name: "Production", Host: "server", Username: "deploy", AuthMethod: AuthPassword,
			},
			wantErr: "secret reference is required",
		},
		{
			name: "private key requires identity file",
			profile: Profile{
				ID: "prod", Name: "Production", Host: "server", Username: "deploy", AuthMethod: AuthPrivateKey,
			},
			wantErr: "identity file is required",
		},
		{
			name: "agent cannot carry secret",
			profile: Profile{
				ID: "prod", Name: "Production", Host: "server", Username: "deploy", AuthMethod: AuthAgent, SecretRef: "secret-1",
			},
			wantErr: "must not reference a secret",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Normalize(tc.profile)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestNormalizeRejectsUnsafeDestinationParts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		host     string
		username string
	}{
		{name: "host option injection", host: "-oProxyCommand=bad", username: "deploy"},
		{name: "host user separator", host: "root@example.com", username: "deploy"},
		{name: "username separator", host: "example.com", username: "root@other"},
		{name: "username whitespace", host: "example.com", username: "root admin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Normalize(Profile{
				ID: "prod", Name: "Production", Host: tc.host, Username: tc.username, AuthMethod: AuthAgent,
			})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNormalizeRejectsDuplicatePresetIDs(t *testing.T) {
	_, err := Normalize(Profile{
		ID: "prod", Name: "Production", Host: "example.com", Username: "deploy", AuthMethod: AuthAgent,
		PresetCommands: []PresetCommand{
			{ID: "logs", Name: "Logs", Command: "tail app.log"},
			{ID: "logs", Name: "Other", Command: "pwd"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("error = %v", err)
	}
}

func TestSecretRefIsNotSerialized(t *testing.T) {
	p, err := Normalize(Profile{
		ID: "prod", Name: "Production", Host: "example.com", Username: "deploy",
		AuthMethod: AuthPassword, SecretRef: "ssh-password-prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "ssh-password-prod") || strings.Contains(string(data), "secret_ref") {
		t.Fatalf("secret reference leaked into JSON: %s", data)
	}
}

func TestNormalizePortForwardings(t *testing.T) {
	got, err := Normalize(Profile{
		ID: "prod", Name: "Production", Host: "example.com", Username: "deploy", AuthMethod: AuthAgent,
		Forwardings: []PortForwarding{
			{Kind: ForwardLocal, BindPort: 8080, TargetHost: "app.internal", TargetPort: 80},
			{Kind: ForwardRemote, BindHost: "127.0.0.1", BindPort: 9000, TargetHost: "127.0.0.1", TargetPort: 9001},
			{Kind: ForwardDynamic, BindHost: "::1", BindPort: 1080},
		},
	})
	if err != nil { t.Fatal(err) }
	if got.Forwardings[0].BindHost != "127.0.0.1" {
		t.Fatalf("default bind host=%q", got.Forwardings[0].BindHost)
	}
	if got.Forwardings[2].BindHost != "::1" {
		t.Fatalf("IPv6 bind host=%q", got.Forwardings[2].BindHost)
	}
}

func TestNormalizeRejectsInvalidPortForwardings(t *testing.T) {
	base := Profile{ID: "prod", Name: "Production", Host: "example.com", Username: "deploy", AuthMethod: AuthAgent}
	tests := []PortForwarding{
		{Kind: ForwardLocal, BindPort: 0, TargetHost: "app", TargetPort: 80},
		{Kind: ForwardLocal, BindPort: 8080, TargetHost: "", TargetPort: 80},
		{Kind: ForwardRemote, BindPort: 9000, TargetHost: "bad host", TargetPort: 80},
		{Kind: ForwardDynamic, BindPort: 1080, TargetHost: "unexpected", TargetPort: 80},
		{Kind: ForwardingKind("bad"), BindPort: 1080},
	}
	for _, item := range tests {
		profile := base
		profile.Forwardings = []PortForwarding{item}
		if _, err := Normalize(profile); err == nil {
			t.Fatalf("invalid forwarding accepted: %+v", item)
		}
	}
}
