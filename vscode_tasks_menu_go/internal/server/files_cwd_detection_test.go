package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestDownloadableFilesInfersDirectoryFromNearestLLCommand(t *testing.T) {
	workspace := t.TempDir()
	nested := filepath.Join(workspace, "esp32_mainpcb_uart_tester")
	if err := os.MkdirAll(nested, 0o755); err != nil { t.Fatal(err) }
	rootREADME := filepath.Join(workspace, "README.md")
	nestedREADME := filepath.Join(nested, "README.md")
	pinMap := filepath.Join(nested, "so_do_chan_jack_6_day.md")
	if err := os.WriteFile(rootREADME, []byte("# root\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(nestedREADME, []byte("# tester\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(pinMap, []byte("# pins\n"), 0o644); err != nil { t.Fatal(err) }

	context := strings.Join([]string{
		"duongtc@fedora:~/Documents/MyProject/Datbike/BleToNfc$ ll esp32_mainpcb_uart_tester/",
		"total 208",
		"-rw-r--r--. 1 duongtc duongtc 3311 Oct 3 09:21 README.md",
		"-rw-r--r--. 1 duongtc duongtc 8088 Oct 3 17:51 so_do_chan_jack_6_day.md",
	}, "\n")
	s := &Server{Workspace: workspace}

	files := s.downloadableFilesFromSelectionAt("README.md", context, workspace)
	if len(files) != 1 || files[0].Path != nestedREADME {
		t.Fatalf("README files=%#v want nested %q", files, nestedREADME)
	}
	if files[0].PreviewKind != "markdown" {
		t.Fatalf("README preview kind=%q want markdown", files[0].PreviewKind)
	}

	files = s.downloadableFilesFromSelectionAt("so_do_chan_jack_6_day.md", context, workspace)
	if len(files) != 1 || files[0].Path != pinMap {
		t.Fatalf("pin map files=%#v want %q", files, pinMap)
	}
	if files[0].PreviewKind != "markdown" {
		t.Fatalf("pin map preview kind=%q want markdown", files[0].PreviewKind)
	}
}

func TestDownloadableFilesNearestLLWithoutOperandUsesCurrentDirectory(t *testing.T) {
	workspace := t.TempDir()
	nested := filepath.Join(workspace, "esp32_mainpcb_uart_tester")
	if err := os.MkdirAll(nested, 0o755); err != nil { t.Fatal(err) }
	rootREADME := filepath.Join(workspace, "README.md")
	nestedREADME := filepath.Join(nested, "README.md")
	if err := os.WriteFile(rootREADME, []byte("root"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(nestedREADME, []byte("nested"), 0o644); err != nil { t.Fatal(err) }

	context := strings.Join([]string{
		"duongtc@fedora:~/Documents/MyProject/Datbike/BleToNfc$ ll",
		"total 1004",
		"-rw-r--r--. 1 duongtc duongtc 3483 Sep 27 18:28 README.md",
	}, "\n")
	files := (&Server{Workspace: workspace}).downloadableFilesFromSelectionAt("README.md", context, workspace)
	if len(files) != 1 || files[0].Path != rootREADME {
		t.Fatalf("files=%#v want root README %q", files, rootREADME)
	}
}

func TestDownloadableFilesLLQuotedDirectoryWithSpaces(t *testing.T) {
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "dir with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil { t.Fatal(err) }
	expected := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(expected, []byte("# notes"), 0o644); err != nil { t.Fatal(err) }

	context := "user@host:~/project$ ls -la \"dir with spaces\"\n-rw-r--r-- 1 user user 10 Oct 3 notes.md"
	files := (&Server{Workspace: workspace}).downloadableFilesFromSelectionAt("notes.md", context, workspace)
	if len(files) != 1 || files[0].Path != expected {
		t.Fatalf("files=%#v want %q", files, expected)
	}
}

func TestDownloadableFilesDoesNotInferAmbiguousMultipleLSOperands(t *testing.T) {
	workspace := t.TempDir()
	for _, dir := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(workspace, dir), 0o755); err != nil { t.Fatal(err) }
		if err := os.WriteFile(filepath.Join(workspace, dir, "same.md"), []byte(dir), 0o644); err != nil { t.Fatal(err) }
	}
	context := "user@host:~/project$ ls a b\n-rw-r--r-- 1 user user 1 Oct 3 same.md"
	files := (&Server{Workspace: workspace}).downloadableFilesFromSelectionAt("same.md", context, workspace)
	if len(files) != 0 {
		t.Fatalf("ambiguous ls files=%#v want none", files)
	}
}

func TestDownloadableFilesUsesWorkspaceForListingContextWhenCWDUnavailable(t *testing.T) {
	workspace := t.TempDir()
	nested := filepath.Join(workspace, "esp32_mainpcb_uart_tester")
	if err := os.MkdirAll(nested, 0o755); err != nil { t.Fatal(err) }
	rootREADME := filepath.Join(workspace, "README.md")
	nestedREADME := filepath.Join(nested, "README.md")
	pinMap := filepath.Join(nested, "so_do_chan_jack_6_day.md")
	if err := os.WriteFile(rootREADME, []byte("# root\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(nestedREADME, []byte("# tester\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(pinMap, []byte("# pins\n"), 0o644); err != nil { t.Fatal(err) }

	context := strings.Join([]string{
		"duongtc@fedora:~/Documents/MyProject/Datbike/BleToNfc$ ll esp32_mainpcb_uart_tester/",
		"total 208",
		"-rw-r--r--. 1 duongtc duongtc 3311 Oct 3 09:21 README.md",
		"-rw-r--r--. 1 duongtc duongtc 8088 Oct 3 17:51 so_do_chan_jack_6_day.md",
	}, "\n")
	s := &Server{Workspace: workspace}

	files := s.downloadableFilesFromSelectionAt("README.md", context, "")
	if len(files) != 1 || files[0].Path != nestedREADME {
		t.Fatalf("README files=%#v want nested %q with empty cwd", files, nestedREADME)
	}
	files = s.downloadableFilesFromSelectionAt("so_do_chan_jack_6_day.md", context, "")
	if len(files) != 1 || files[0].Path != pinMap {
		t.Fatalf("pin map files=%#v want %q with empty cwd", files, pinMap)
	}
	if files[0].PreviewKind != "markdown" {
		t.Fatalf("pin map preview kind=%q want markdown", files[0].PreviewKind)
	}
}

func TestFileSelectionAPIUsesListingContextWhenCWDUnavailable(t *testing.T) {
	workspace := t.TempDir()
	nested := filepath.Join(workspace, "esp32_mainpcb_uart_tester")
	if err := os.MkdirAll(nested, 0o755); err != nil { t.Fatal(err) }
	expected := filepath.Join(nested, "so_do_chan_jack_6_day.md")
	if err := os.WriteFile(expected, []byte("# pin map\n"), 0o644); err != nil { t.Fatal(err) }

	body, err := json.Marshal(map[string]string{
		"text": "so_do_chan_jack_6_day.md",
		"context": strings.Join([]string{
			"user@host:~/project$ ll esp32_mainpcb_uart_tester/",
			"-rw-r--r-- 1 user user 8088 Oct 3 so_do_chan_jack_6_day.md",
		}, "\n"),
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
	if response.Files[0].PreviewKind != "markdown" {
		t.Fatalf("preview kind=%q want markdown", response.Files[0].PreviewKind)
	}
}
