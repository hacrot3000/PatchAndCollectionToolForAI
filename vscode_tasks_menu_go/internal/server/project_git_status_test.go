package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectGitStatusCode(t *testing.T) {
	tests := []struct {
		name   string
		change gitChange
		want   string
	}{
		{name: "conflict", change: gitChange{Conflicted: true, IndexStatus: "U", WorktreeStatus: "U"}, want: "U"},
		{name: "untracked", change: gitChange{Untracked: true, IndexStatus: "?", WorktreeStatus: "?"}, want: "?"},
		{name: "deleted", change: gitChange{IndexStatus: " ", WorktreeStatus: "D"}, want: "D"},
		{name: "added", change: gitChange{IndexStatus: "A", WorktreeStatus: " "}, want: "A"},
		{name: "renamed", change: gitChange{IndexStatus: "R", WorktreeStatus: " "}, want: "R"},
		{name: "copied", change: gitChange{IndexStatus: "C", WorktreeStatus: " "}, want: "C"},
		{name: "type", change: gitChange{IndexStatus: "T", WorktreeStatus: " "}, want: "T"},
		{name: "modified", change: gitChange{IndexStatus: " ", WorktreeStatus: "M"}, want: "M"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := projectGitStatusCode(test.change); got != test.want {
				t.Fatalf("status=%q want=%q", got, test.want)
			}
		})
	}
}

func TestProjectGitStatusReportsWorkspaceRelativeChanges(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "staged.txt")

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/git-status", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Available bool                    `json:"available"`
		Changes   []projectGitStatusEntry `json:"changes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Available {
		t.Fatal("Git repository should be available")
	}
	got := map[string]string{}
	for _, change := range response.Changes {
		got[change.Path] = change.Status
		if change.Repository != "." {
			t.Fatalf("root repository id=%q", change.Repository)
		}
	}
	if got["tracked.txt"] != "M" {
		t.Fatalf("tracked status=%q changes=%+v", got["tracked.txt"], response.Changes)
	}
	if got["new.txt"] != "?" {
		t.Fatalf("untracked status=%q changes=%+v", got["new.txt"], response.Changes)
	}
	if got["staged.txt"] != "A" {
		t.Fatalf("staged status=%q changes=%+v", got["staged.txt"], response.Changes)
	}
}

func TestProjectGitStatusIsOptionalOutsideGitRepository(t *testing.T) {
	s := &Server{Workspace: t.TempDir()}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/git-status", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Available bool                    `json:"available"`
		Changes   []projectGitStatusEntry `json:"changes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Available || len(response.Changes) != 0 {
		t.Fatalf("unexpected non-Git response: %+v", response)
	}
}
