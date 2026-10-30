package config

import (
	"strings"
	"testing"
)

func TestRemoteAccessFailsClosed(t *testing.T) {
	base := Default()
	base.RemoteAccessEnabled = true
	base.RemotePortalURL = "https://portal.example.com"
	base.RemoteRelayURL = "https://relay.example.com"
	if err := base.Validate(); err == nil || !strings.Contains(err.Error(), "authenticated") {
		t.Fatalf("unauthenticated remote should be denied: %v", err)
	}
	base.AuthEnabled = true
	base.Password = "a-new-nondefault-secret"
	if err := base.Validate(); err != nil {
		t.Fatalf("authenticated HTTPS remote rejected: %v", err)
	}
	for _, portal := range []string{"", "http://portal.example.com", "https://user:pass@portal.example.com",
		"https://portal.example.com/path", "https://portal.example.com?key=123"} {
		bad := base
		bad.RemotePortalURL = portal
		if err := bad.Validate(); err == nil {
			t.Fatalf("invalid portal URL accepted: %q", portal)
		}
	}
	insecure := base
	insecure.Protocol = "http"
	if err := insecure.Validate(); err == nil {
		t.Fatal("plaintext TaskDeck should not support Remote")
	}
	invalidRelay := base
	invalidRelay.RemoteRelayURL = "http://relay.example.com"
	if err := invalidRelay.Validate(); err == nil {
		t.Fatal("HTTP Relay was accepted")
	}
	disabled := Default()
	disabled.RemotePortalURL = "http://bad.example.com"
	if err := disabled.Validate(); err != nil {
		t.Fatalf("disabled remote should not change local validation: %v", err)
	}
}
