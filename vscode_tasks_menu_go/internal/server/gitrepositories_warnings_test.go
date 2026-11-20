package server

import (
 "encoding/json"
 "fmt"
 "net/http"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
)

func TestGitScanReportsChildRepoVerificationFailures(t *testing.T) {
 if runtime.GOOS == "windows" { t.Skip("POSIX shell stub") }
 workspace := t.TempDir()
 child := filepath.Join(workspace, "projects", "m3-server")
 if err := os.MkdirAll(child, 0o755); err != nil { t.Fatal(err) }
 gitQuickRun(t, child, "init")
 stubDir := t.TempDir()
 stub := "#!/bin/sh\necho 'simulated git failure' >&2\nexit 128\n"
 if err := os.WriteFile(filepath.Join(stubDir, "git"), []byte(stub), 0o755); err != nil { t.Fatal(err) }
 t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
 s := &Server{Workspace: workspace}
 rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=repositories&refresh=1", "")
 if rr.Code != http.StatusOK { t.Fatalf("status=%d: %s", rr.Code, rr.Body.String()) }
 var got struct {
  Repositories []gitRepositoryStatus `json:"repositories"`
  Warnings []string `json:"scan_warnings"`
  ScanEnabled bool `json:"scan_enabled"`
 }
 if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
 if len(got.Repositories) != 0 || !got.ScanEnabled { t.Fatalf("unexpected scan result=%+v", got) }
 if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "projects/m3-server: git rev-parse: simulated git failure") {
  t.Fatalf("missing diagnostic for nested Git marker: %v", got.Warnings)
 }
}

func TestGitScanPrioritizesProjectsOverArtifacts(t *testing.T) {
	for _, dir := range []string{"projects", "repos", "apps", "src"} {
		if gitDiscoveryDirectoryPriority(dir) >= gitDiscoveryDirectoryPriority("artifacts") {
			t.Fatalf("%s must scan before generated artifact trees", dir)
		}
	}
	workspace := t.TempDir()
	artifactCache := filepath.Join(workspace, "artifacts", "m3-server", "m2")
	if err := os.MkdirAll(artifactCache, 0o755); err != nil { t.Fatal(err) }
	// A generated tree with unrelated directories precedes projects/ in normal
	// alphabetical depth-first traversal.
	for i := 0; i < 120; i++ {
		if err := os.MkdirAll(filepath.Join(artifactCache, fmt.Sprintf("cache-%03d", i)), 0o755); err != nil { t.Fatal(err) }
	}
	child := filepath.Join(workspace, "projects", "m3-server")
	if err := os.MkdirAll(child, 0o755); err != nil { t.Fatal(err) }
	gitQuickRun(t, child, "init")
	s := &Server{Workspace: workspace}
	repos, _, err := s.discoverGitRepositories(true)
	if err != nil { t.Fatal(err) }
	if len(repos) != 1 || repos[0].ID != "projects/m3-server" {
		t.Fatalf("shallow Git repository missing among generated trees: %+v", repos)
	}
}
