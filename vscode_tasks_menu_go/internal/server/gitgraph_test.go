package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitGraphRepo(t *testing.T) (string, *Server, string, string, string) {
	t.Helper()
	workspace, s, mainBranch := setupGitQuickRepo(t)
	gitQuickRun(t, workspace, "switch", "-c", "feature/graph")
	if err := os.WriteFile(filepath.Join(workspace, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "feature.txt")
	gitQuickRun(t, workspace, "commit", "-m", "feature graph commit")
	featureSHA := gitQuickRun(t, workspace, "rev-parse", "HEAD")

	gitQuickRun(t, workspace, "switch", mainBranch)
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "main.txt")
	gitQuickRun(t, workspace, "commit", "-m", "main graph commit")
	mainBeforeMerge := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	gitQuickRun(t, workspace, "merge", "--no-ff", "-m", "merge graph feature", "feature/graph")
	mergeSHA := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	gitQuickRun(t, workspace, "tag", "graph-v1", featureSHA)
	gitQuickRun(t, workspace, "update-ref", "refs/remotes/origin/"+mainBranch, mergeSHA)
	return workspace, s, mainBranch, featureSHA, mainBeforeMerge
}

func TestGitGraphReturnsDAGRefsAndFilters(t *testing.T) {
	_, s, mainBranch, featureSHA, _ := setupGitGraphRepo(t)

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=graph&limit=50", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("graph status=%d body=%s", rr.Code, rr.Body.String())
	}
	var graph struct {
		Commits []gitGraphCommit `json:"commits"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Commits) < 4 {
		t.Fatalf("graph commits=%d want at least 4: %#v", len(graph.Commits), graph.Commits)
	}
	var merge *gitGraphCommit
	var feature *gitGraphCommit
	for i := range graph.Commits {
		row := &graph.Commits[i]
		if row.Subject == "merge graph feature" {
			merge = row
		}
		if row.SHA == featureSHA {
			feature = row
		}
	}
	if merge == nil || len(merge.Parents) != 2 {
		t.Fatalf("merge row=%+v", merge)
	}
	hasHead, hasLocal, hasRemote := false, false, false
	for _, ref := range merge.Refs {
		switch {
		case ref.Kind == "head" && ref.Name == "HEAD":
			hasHead = true
		case ref.Kind == "local" && ref.Name == mainBranch:
			hasLocal = true
		case ref.Kind == "remote" && ref.Name == "origin/"+mainBranch:
			hasRemote = true
		}
	}
	if !hasHead || !hasLocal || !hasRemote {
		t.Fatalf("merge refs=%+v", merge.Refs)
	}
	if feature == nil {
		t.Fatal("feature commit missing")
	}
	hasTag := false
	for _, ref := range feature.Refs {
		if ref.Kind == "tag" && ref.Name == "graph-v1" {
			hasTag = true
		}
	}
	if !hasTag {
		t.Fatalf("feature refs=%+v", feature.Refs)
	}

	for _, tc := range []struct {
		target string
		wantSubject string
	}{
		{"/api/git/status?view=graph&message=feature+graph&limit=20", "feature graph commit"},
		{"/api/git/status?view=graph&path=feature.txt&limit=20", "feature graph commit"},
		{"/api/git/status?view=graph&search=graph-v1&limit=20", "feature graph commit"},
	} {
		filtered := callGitStatusHandler(t, s, http.MethodGet, tc.target, "")
		if filtered.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", tc.target, filtered.Code, filtered.Body.String())
		}
		var got struct{ Commits []gitGraphCommit `json:"commits"` }
		if err := json.Unmarshal(filtered.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range got.Commits {
			if row.Subject == tc.wantSubject {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s commits=%+v missing %q", tc.target, got.Commits, tc.wantSubject)
		}
	}

	future := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=graph&since=2999-01-01", "")
	if future.Code != http.StatusOK {
		t.Fatalf("future filter status=%d body=%s", future.Code, future.Body.String())
	}
	var none struct{ Commits []gitGraphCommit `json:"commits"` }
	if err := json.Unmarshal(future.Body.Bytes(), &none); err != nil {
		t.Fatal(err)
	}
	if len(none.Commits) != 0 {
		t.Fatalf("future filter returned commits: %+v", none.Commits)
	}

	badDate := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=graph&since=05-10-2026", "")
	if badDate.Code != http.StatusBadRequest {
		t.Fatalf("bad date status=%d body=%s", badDate.Code, badDate.Body.String())
	}
}

func TestGitCommitFilesReportsAddedAndRenamedFiles(t *testing.T) {
	workspace, s, _, featureSHA, _ := setupGitGraphRepo(t)

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=commit-files&ref="+featureSHA, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("commit-files status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Commit string         `json:"commit"`
		Files  []gitGraphFile `json:"files"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Commit != featureSHA || len(response.Files) != 1 || response.Files[0].Status != "A" || response.Files[0].Path != "feature.txt" {
		t.Fatalf("response=%+v", response)
	}

	gitQuickRun(t, workspace, "switch", "feature/graph")
	if err := os.Rename(filepath.Join(workspace, "feature.txt"), filepath.Join(workspace, "renamed feature.txt")); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "-A")
	gitQuickRun(t, workspace, "commit", "-m", "rename feature file")
	renameSHA := gitQuickRun(t, workspace, "rev-parse", "HEAD")

	rr = callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=commit-files&ref="+renameSHA, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("rename commit-files status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	foundRename := false
	for _, file := range response.Files {
		if strings.HasPrefix(file.Status, "R") && file.OldPath == "feature.txt" && file.Path == "renamed feature.txt" {
			foundRename = true
		}
	}
	if !foundRename {
		t.Fatalf("rename files=%+v", response.Files)
	}
}

func TestGitCommitFilesRejectsSymbolicRef(t *testing.T) {
	_, s, _ := setupGitQuickRepo(t)
	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=commit-files&ref=HEAD", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
