package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitSubmoduleRepo(t *testing.T) (string, *Server, string, string, string) {
	t.Helper()
	child := t.TempDir()
	gitQuickRun(t, child, "init")
	gitQuickRun(t, child, "config", "user.name", "Submodule Test")
	gitQuickRun(t, child, "config", "user.email", "submodule@example.invalid")
	if err := os.WriteFile(filepath.Join(child, "child.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, child, "add", "child.txt")
	gitQuickRun(t, child, "commit", "-m", "child one")
	first := gitQuickRun(t, child, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(child, "child.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, child, "commit", "-am", "child two")
	second := gitQuickRun(t, child, "rev-parse", "HEAD")

	parent, s, _ := setupGitQuickRepo(t)
	gitQuickRun(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", child, "modules/child")
	gitQuickRun(t, filepath.Join(parent, "modules", "child"), "checkout", "--detach", first)
	gitQuickRun(t, parent, "add", ".gitmodules", "modules/child")
	gitQuickRun(t, parent, "commit", "-m", "add child submodule")
	expected := gitQuickRun(t, parent, "rev-parse", "HEAD:modules/child")
	if expected != first {
		t.Fatalf("expected gitlink=%q want=%q", expected, first)
	}
	return parent, s, first, second, child
}

func getSingleSubmodule(t *testing.T, s *Server) gitSubmoduleRow {
	t.Helper()
	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=submodules", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("submodules status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Submodules []gitSubmoduleRow `json:"submodules"`
		Count      int               `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || len(got.Submodules) != 1 {
		t.Fatalf("submodules=%+v count=%d", got.Submodules, got.Count)
	}
	return got.Submodules[0]
}

func TestGitSubmodulesReportsExpectedActualAndCleanState(t *testing.T) {
	parent, s, first, _, childSource := setupGitSubmoduleRepo(t)
	row := getSingleSubmodule(t, s)
	if row.Name != "modules/child" && row.Name != "child" {
		// Git chooses the path as the default submodule name; keep the assertion
		// tolerant across Git versions while still requiring a concrete name.
		if strings.TrimSpace(row.Name) == "" {
			t.Fatalf("submodule name empty: %+v", row)
		}
	}
	if row.Path != "modules/child" || row.ExpectedSHA != first || row.ActualSHA != first {
		t.Fatalf("submodule row=%+v", row)
	}
	if !row.Initialized || row.Dirty || row.Mismatch || row.Status != "clean" {
		t.Fatalf("clean state=%+v", row)
	}
	if row.URL == "" || !strings.Contains(filepath.ToSlash(row.URL), filepath.Base(childSource)) {
		t.Fatalf("url=%q child=%q", row.URL, childSource)
	}
	if row.ID == "" || strings.Contains(row.ID, "/") {
		t.Fatalf("unsafe/empty ID=%q", row.ID)
	}
	if strings.Contains(mustJSON(t, row), filepath.Clean(parent)) {
		t.Fatalf("submodule row leaked parent absolute path: %+v", row)
	}
}

func TestGitSubmodulesDetectsDirtyAndCommitMismatch(t *testing.T) {
	parent, s, first, second, _ := setupGitSubmoduleRepo(t)
	child := filepath.Join(parent, "modules", "child")
	gitQuickRun(t, child, "checkout", "--detach", second)
	if err := os.WriteFile(filepath.Join(child, "child.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	row := getSingleSubmodule(t, s)
	if !row.Initialized || !row.Dirty || !row.Mismatch || row.Status != "dirty-mismatch" {
		t.Fatalf("dirty mismatch state=%+v", row)
	}
	if row.ExpectedSHA != first || row.ActualSHA != second {
		t.Fatalf("sha state=%+v", row)
	}
}

func TestGitSubmodulesDetectsUninitialized(t *testing.T) {
	parent, s, first, _, _ := setupGitSubmoduleRepo(t)
	gitQuickRun(t, parent, "submodule", "deinit", "-f", "--", "modules/child")
	row := getSingleSubmodule(t, s)
	if row.Initialized || row.Dirty || row.Mismatch || row.Status != "uninitialized" {
		t.Fatalf("uninitialized state=%+v", row)
	}
	if row.ExpectedSHA != first || row.ActualSHA != "" {
		t.Fatalf("uninitialized SHA state=%+v", row)
	}
}

func TestValidGitSubmodulePathRejectsTraversalAndAbsolute(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "../outside", "a/../../outside", "/tmp/child"} {
		if got, err := validGitSubmodulePath(bad); err == nil {
			t.Fatalf("unsafe path %q accepted as %q", bad, got)
		}
	}
	for _, good := range []string{"modules/child", "vendor/lib-one"} {
		if got, err := validGitSubmodulePath(good); err != nil || got != good {
			t.Fatalf("valid path %q -> %q err=%v", good, got, err)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
