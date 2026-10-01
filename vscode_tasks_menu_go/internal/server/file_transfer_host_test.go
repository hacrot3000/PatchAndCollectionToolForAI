package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
)

func TestFileTransferSFTPHostToRemoteStaysInsideWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	s, store, _ := newFileTransferProfileAPITestServer(t)
	workspace := t.TempDir()
	s.Workspace = workspace
	hostFile := filepath.Join(workspace, "build.bin")
	if err := os.WriteFile(hostFile, []byte("host-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	profile, err := store.Create(filetransferprofile.Profile{
		ID: "sftp-host", Name: "SFTP Host", Protocol: filetransferprofile.ProtocolSFTP,
		SSHProfileID: "ssh-prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	capture := filepath.Join(dir, "capture.txt")
	script := filepath.Join(dir, "fake-sftp")
	body := "#!/bin/sh\ncat > \"" + capture + "\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	s.SFTPExecutable = script
	h := s.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/file-transfer/host-to-remote", fileTransferJSONBody(t, map[string]any{
		"profile_id": profile.ID,
		"host_path": "build.bin",
		"remote_path": "/srv/releases/build.bin",
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "put \""+hostFile+"\" \"/srv/releases/build.bin\"") {
		t.Fatalf("unexpected sftp command %q", data)
	}

	bad := httptest.NewRequest(http.MethodPost, "/api/file-transfer/host-to-remote", fileTransferJSONBody(t, map[string]any{
		"profile_id": profile.ID,
		"host_path": "../outside.bin",
		"remote_path": "/srv/outside.bin",
	}))
	bad.Header.Set("Content-Type", "application/json")
	badRR := httptest.NewRecorder()
	h.ServeHTTP(badRR, bad)
	if badRR.Code != http.StatusNotFound {
		t.Fatalf("traversal status=%d body=%s", badRR.Code, badRR.Body.String())
	}
}

func TestFileTransferSFTPRemoteToHostUsesWorkspaceDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	s, store, _ := newFileTransferProfileAPITestServer(t)
	workspace := t.TempDir()
	s.Workspace = workspace
	if err := os.Mkdir(filepath.Join(workspace, "downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	profile, err := store.Create(filetransferprofile.Profile{
		ID: "sftp-host-download", Name: "SFTP Host Download", Protocol: filetransferprofile.ProtocolSFTP,
		SSHProfileID: "ssh-prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-sftp")
	body := "#!/bin/sh\ninput=$(cat)\nlocal_path=$(printf '%s\\n' \"$input\" | sed -n 's/^get \"[^\"]*\" \"\\([^\"]*\\)\"$/\\1/p')\nprintf 'remote-data' > \"$local_path\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	s.SFTPExecutable = script
	h := s.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/file-transfer/remote-to-host", fileTransferJSONBody(t, map[string]any{
		"profile_id": profile.ID,
		"remote_path": "/srv/report.txt",
		"host_dir": "downloads",
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var result struct {
		Path string `json:"path"`
		Size int64 `json:"size"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Path != "downloads/report.txt" || result.Size != int64(len("remote-data")) {
		t.Fatalf("result=%+v", result)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "downloads", "report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "remote-data" {
		t.Fatalf("content=%q", data)
	}

	conflict := httptest.NewRequest(http.MethodPost, "/api/file-transfer/remote-to-host", fileTransferJSONBody(t, map[string]any{
		"profile_id": profile.ID,
		"remote_path": "/srv/report.txt",
		"host_dir": "downloads",
	}))
	conflict.Header.Set("Content-Type", "application/json")
	conflictRR := httptest.NewRecorder()
	h.ServeHTTP(conflictRR, conflict)
	if conflictRR.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflictRR.Code, conflictRR.Body.String())
	}
}
