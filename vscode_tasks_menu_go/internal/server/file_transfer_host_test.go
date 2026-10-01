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

func TestFileTransferHostMutationsStayInsideWorkspace(t *testing.T) {
	s, _, _ := newFileTransferProfileAPITestServer(t)
	workspace := t.TempDir()
	s.Workspace = workspace
	if err := os.WriteFile(filepath.Join(workspace, "old.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := s.Handler()

	call := func(payload map[string]any) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/file-transfer/host-mutate", fileTransferJSONBody(t, payload))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	if rr := call(map[string]any{"action": "mkdir", "path": "created"}); rr.Code != http.StatusNoContent {
		t.Fatalf("mkdir status=%d body=%s", rr.Code, rr.Body.String())
	}
	if info, err := os.Stat(filepath.Join(workspace, "created")); err != nil || !info.IsDir() {
		t.Fatalf("created folder info=%v err=%v", info, err)
	}

	if rr := call(map[string]any{"action": "rename", "path": "old.txt", "new_path": "new.txt"}); rr.Code != http.StatusNoContent {
		t.Fatalf("rename status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, "new.txt")); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}

	if rr := call(map[string]any{"action": "delete", "path": "new.txt"}); rr.Code != http.StatusNoContent {
		t.Fatalf("delete file status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted file still exists: %v", err)
	}

	if rr := call(map[string]any{"action": "delete", "path": "empty"}); rr.Code != http.StatusNoContent {
		t.Fatalf("delete directory status=%d body=%s", rr.Code, rr.Body.String())
	}

	if rr := call(map[string]any{"action": "mkdir", "path": "../escape"}); rr.Code == http.StatusNoContent {
		t.Fatal("workspace traversal unexpectedly succeeded")
	}
}

func TestFileTransferHostMutationsRejectSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	s, _, _ := newFileTransferProfileAPITestServer(t)
	workspace := t.TempDir()
	s.Workspace = workspace
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "link.txt")); err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/file-transfer/host-mutate", fileTransferJSONBody(t, map[string]any{
		"action": "delete", "path": "link.txt",
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code == http.StatusNoContent {
		t.Fatal("symlink mutation unexpectedly succeeded")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside target changed: %v", err)
	}
}
