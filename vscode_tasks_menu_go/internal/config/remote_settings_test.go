package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetRemoteSettingsPreservesConfiguration(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, ".vscode", "vscode_tasks_menu.ini")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	initial := "[server]\nprotocol = https\nbind = 127.0.0.1\n[auth]\nenabled = true\nusername = admin\npassword = strong-secret\n[self_update]\nbranch = main\n"
	if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}
	public,_,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	pin:=base64.StdEncoding.EncodeToString(public)
	v := RemoteSettings{Enabled: true, PortalURL: "https://portal.example.com", RelayURL: "https://relay.example.com",PortalPublicKey:pin}
	if err := SetRemoteSettings(ws, v); err != nil {
		t.Fatalf("enable: %v", err)
	}
	saved, err := ReadRemoteSettings(ws)
	if err != nil || saved != v {
		t.Fatalf("saved remote settings: %#v, %v", saved, err)
	}
	if err := SetRemoteSettings(ws, RemoteSettings{Enabled: false, PortalURL: v.PortalURL, RelayURL: v.RelayURL,PortalPublicKey:v.PortalPublicKey}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	data, _ := os.ReadFile(path)
	for _, part := range []string{"[self_update]", "branch = main", "password = strong-secret", "enabled = false", "portal_url = https://portal.example.com", "relay_url = https://relay.example.com", "portal_public_key = "+pin} {
		if !strings.Contains(string(data), part) {
			t.Fatalf("config missing %q after updating remote settings", part)
		}
	}
	if err := SetRemoteSettings(ws, RemoteSettings{Enabled: true, PortalURL: v.PortalURL, RelayURL: "http://insecure-relay.example.com"}); err == nil {
		t.Fatal("insecure Relay URL accepted")
	}
	if err := SetRemoteSettings(ws, RemoteSettings{Enabled: true, PortalURL: "http://insecure.example.com"}); err == nil {
		t.Fatal("insecure Portal URL accepted")
	}
	if err := SetRemoteSettings(ws, RemoteSettings{Enabled:true,PortalURL:v.PortalURL,
		RelayURL:v.RelayURL,PortalPublicKey:"invalid-signing-key"});err==nil{
		t.Fatal("malformed Portal Ed25519 signing key accepted")
	}
	after,err:=ReadRemoteSettings(ws)
	if err!=nil{t.Fatal(err)}
	if after.PortalPublicKey!=pin {
		t.Fatal("invalid signing key overwrote previously pinned public key")
	}
}

func TestRemoteRequiresAuthenticationToBeEnabled(t *testing.T) {
	ws := t.TempDir()
	_, _, err := Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetRemoteSettings(ws, RemoteSettings{Enabled: true, PortalURL: "https://portal.example.com"}); err == nil {
		t.Fatal("Remote was enabled without local authentication")
	}
}
