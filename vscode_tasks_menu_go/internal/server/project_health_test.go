package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectHealthDiskUsageIsBoundedAndSkipsGit(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil { t.Fatal(err) }
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "src", "a.txt"), []byte("abc"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, ".git", "objects"), []byte("ignored"), 0o644); err != nil { t.Fatal(err) }

	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		DiskUsageBytes int64 `json:"disk_usage_bytes"`
		FileCount int `json:"file_count"`
		DirectoryCount int `json:"directory_count"`
		EntryCount int `json:"entry_count"`
		Truncated bool `json:"truncated"`
		MaxEntries int `json:"max_entries"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	if got.DiskUsageBytes != 3 || got.FileCount != 1 {
		t.Fatalf("health=%+v want only src/a.txt counted", got)
	}
	if got.DirectoryCount != 1 || got.Truncated || got.MaxEntries != projectHealthMaxEntries {
		t.Fatalf("health metadata=%+v", got)
	}
}

func TestProjectHealthRejectsNonGET(t *testing.T) {
	s := &Server{Workspace: t.TempDir()}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/project/health", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want 405", rr.Code)
	}
}
