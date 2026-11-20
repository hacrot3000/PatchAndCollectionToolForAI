package server

import (
	"strings"
	"testing"
)

func TestSharedAccountUIProvidesPasswordChange(t *testing.T) {
	for _, want := range []string{
		`id="password-form"`,
		`id="current-password"`,
		`id="new-password"`,
		`id="confirm-password"`,
		"/api/auth/password",
		"signs out all of your TaskDeck login sessions across projects",
		"Password changed. All login sessions were signed out.",
	} {
		if !strings.Contains(sharedLoginHTML+sharedLoginJS, want) {
			t.Fatalf("shared account password UI missing %q", want)
		}
	}
	for _, want := range []string{
		`id="account" href="/login" hidden`,
		"document.querySelector('#account').hidden=false",
	} {
		if !strings.Contains(indexHTML+appJS, want) {
			t.Fatalf("main account link missing %q", want)
		}
	}
}
