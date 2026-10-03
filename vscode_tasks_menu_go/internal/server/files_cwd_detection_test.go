package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadableFilesUsesWorkingDirectoryForBareFilename(t *testing.T) {
	workspace := t.TempDir()
	nested := filepath.Join(workspace, "esp32_mainpcb_uart_tester")
	if err := os.MkdirAll(nested, 0o755); err != nil { t.Fatal(err) }
	rootFile := filepath.Join(workspace, "README.md")
	nestedFile := filepath.Join(nested, "README.md")
	if err := os.WriteFile(rootFile, []byte("root"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(nestedFile, []byte("nested"), 0o644); err != nil { t.Fatal(err) }

	files := (&Server{Workspace: workspace}).downloadableFilesFromSelectionAt(
		"README.md",
		"-rw-r--r--. 1 user user 3311 Oct 3 09:21 README.md",
		nested,
	)
	if len(files) != 1 || files[0].Path != nestedFile {
		t.Fatalf("files=%#v want cwd file %q", files, nestedFile)
	}
}

func TestDownloadableFilesUsesWorkingDirectoryForRelativePath(t *testing.T) {
	workspace := t.TempDir()
	cwd := filepath.Join(workspace, "module")
	expected := filepath.Join(cwd, "logs", "result.txt")
	if err := os.MkdirAll(filepath.Dir(expected), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(expected, []byte("ok"), 0o644); err != nil { t.Fatal(err) }

	files := (&Server{Workspace: workspace}).downloadableFilesFromSelectionAt("logs/result.txt", "", cwd)
	if len(files) != 1 || files[0].Path != expected {
		t.Fatalf("files=%#v want %q", files, expected)
	}
}

func TestDownloadableFilesRejectsWorkingDirectoryOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil { t.Fatal(err) }

	files := (&Server{Workspace: workspace}).downloadableFilesFromSelectionAt("secret.txt", "", outside)
	if len(files) != 0 {
		t.Fatalf("outside cwd files=%#v want none", files)
	}
}

func TestFileSelectionAPIUsesProvidedWorkingDirectory(t *testing.T) {
	workspace := t.TempDir()
	cwd := filepath.Join(workspace, "esp32_mainpcb_uart_tester")
	if err := os.MkdirAll(cwd, 0o755); err != nil { t.Fatal(err) }
	expected := filepath.Join(cwd, "so_do_chan_jack_6_day.md")
	if err := os.WriteFile(expected, []byte("# pin map\n"), 0o644); err != nil { t.Fatal(err) }

	body, err := json.Marshal(map[string]string{
		"text": "so_do_chan_jack_6_day.md",
		"context": "-rw-r--r--. 1 user user 8088 Oct 3 17:51 so_do_chan_jack_6_day.md",
		"cwd": cwd,
	})
	if err != nil { t.Fatal(err) }
	req := httptest.NewRequest(http.MethodPost, "/api/files/selection", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	(&Server{Workspace: workspace}).filesSelection(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct{ Files []downloadableFile `json:"files"` }
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil { t.Fatal(err) }
	if len(response.Files) != 1 || response.Files[0].Path != expected {
		t.Fatalf("files=%#v want %q", response.Files, expected)
	}
}
