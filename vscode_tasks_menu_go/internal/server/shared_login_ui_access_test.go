package server

import (
	"strings"
	"testing"
)

func TestSharedLoginPageOffersWorkspaceAfterAuthentication(t *testing.T) {
	for _, want := range []string{
		"Workspace access is governed by your project permissions.",
		`id="open-workspace" href="/"`,
		"Refresh access",
	} {
		if !strings.Contains(sharedLoginHTML, want) {
			t.Fatalf("shared login page missing %q", want)
		}
	}
	if strings.Contains(sharedLoginHTML, "Workspace access is not available yet.") {
		t.Fatal("shared login page still reports the pre-authorization gate")
	}
}
