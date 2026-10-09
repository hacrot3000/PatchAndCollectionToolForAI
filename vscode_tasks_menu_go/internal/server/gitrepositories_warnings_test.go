package server

import (
 "encoding/json"
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
