package selfupdate

import (
	"strings"
	"testing"
)

func TestGitHubCommitURLUsesMainByDefault(t *testing.T) {
	got := githubCommitURL("")
	want := "https://api.github.com/repos/" + Repository + "/commits/main"
	if got != want {
		t.Fatalf("githubCommitURL(\"\")=%q want %q", got, want)
	}
}

func TestGitHubCommitURLEscapesNestedBranch(t *testing.T) {
	got := githubCommitURL("feat/self-update-test")
	if !strings.HasSuffix(got, "/commits/feat%2Fself-update-test") {
		t.Fatalf("nested branch was not escaped safely: %q", got)
	}
}
