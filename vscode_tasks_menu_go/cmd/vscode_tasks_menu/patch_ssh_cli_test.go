package main

import (
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func TestParsePatchSSHCommandRequest(t *testing.T) {
	request, command, err := parsePatchSSHRequest(strings.NewReader(
		`{"profile_name":"Production 1","argv":["find",".","-maxdepth","2","-type","f"],"timeout_seconds":30,"dry_run":true}`))
	if err != nil || request.ProfileName != "Production 1" || command != "find . -maxdepth 2 -type f" {
		t.Fatalf("unexpected parsed command: %+v, %q, %v", request, command, err)
	}
}

func TestParsePatchSSHCommandRejectsShellInterpreterAndUnknownJSON(t *testing.T) {
	for _, input := range []string{
		`{"profile_name":"Prod","argv":["bash","script.sh"],"timeout_seconds":30}`,
		`{"profile_name":"Prod","argv":["python3","-c","print(1)"],"timeout_seconds":30}`,
		`{"profile_name":"Prod","argv":["ls",";","rm"],"timeout_seconds":30}`,
		`{"profile_name":"Prod","argv":["pwd"],"timeout_seconds":150}`,
		`{"profile_name":"Prod","argv":["pwd"],"timeout_seconds":30,"host":"evil"}`,
		`{"profile_name":"","argv":["pwd"],"timeout_seconds":30}`,
		`{"profile_name":"Prod","argv":["pwd"],"timeout_seconds":30}{"profile_name":"Other"}`,
	} {
		if _, _, err := parsePatchSSHRequest(strings.NewReader(input)); err == nil {
			t.Fatalf("unsafe PATCH SSH request accepted: %s", input)
		}
	}
}

func TestPatchSSHProfileMustHaveUniqueExactName(t *testing.T) {
	profiles := []sshprofile.Profile{
		{ID: "a", Name: "Prod"},
		{ID: "b", Name: "Prod"},
		{ID: "c", Name: "Test"},
	}
	if _, err := patchSSHProfileByName(profiles, "Prod"); err == nil {
		t.Fatal("ambiguous profile name must fail closed")
	}
	if _, err := patchSSHProfileByName(profiles, "test"); err == nil {
		t.Fatal("profile lookup must be exact, not case insensitive")
	}
	if got, err := patchSSHProfileByName(profiles, "Test"); err != nil || got.ID != "c" {
		t.Fatalf("unique profile not found: %+v %v", got, err)
	}
}
