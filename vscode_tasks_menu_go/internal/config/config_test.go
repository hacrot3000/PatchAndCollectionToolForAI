package config

import "testing"

func TestRemoteRequiresAuth(t *testing.T) {
	cfg := Default()
	cfg.Bind = "0.0.0.0"
	if cfg.Validate() == nil {
		t.Fatal("expected remote auth validation error")
	}
	cfg.AuthEnabled = true
	cfg.Username = "u"
	cfg.Password = "p"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
