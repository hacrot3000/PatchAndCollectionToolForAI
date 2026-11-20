package server

import (
	"bytes"
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

func TestFileTransferSFTPTestAndListUseReferencedSSHProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	s, store, _ := newFileTransferProfileAPITestServer(t)
	profile, err := store.Create(filetransferprofile.Profile{
		ID: "sftp-files", Name: "SFTP Files", Protocol: filetransferprofile.ProtocolSFTP,
		SSHProfileID: "ssh-prod", InitialPath: "/srv/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-sftp")
	scriptBody := "#!/bin/sh\ninput=$(cat)\ncase \"$input\" in\n  *\"ls -lan\"*) printf '%s\\n' '-rw-r--r-- 1 1000 1000 7 Sep 30 12:00 app.txt' 'drwxr-xr-x 2 1000 1000 4096 Sep 29 2026 uploads' ;;\n  *) printf '%s\\n' 'Remote working directory: /srv/app' ;;\nesac\n"
	if err := os.WriteFile(script, []byte(scriptBody), 0o700); err != nil {
		t.Fatal(err)
	}
	s.SFTPExecutable = script
	h := s.Handler()

	testReq := httptest.NewRequest(http.MethodPost, "/api/file-transfer/test", fileTransferJSONBody(t, map[string]any{"profile_id": profile.ID}))
	testReq.Header.Set("Content-Type", "application/json")
	testRR := httptest.NewRecorder()
	h.ServeHTTP(testRR, testReq)
	if testRR.Code != http.StatusOK || !strings.Contains(testRR.Body.String(), "\"ok\":true") {
		t.Fatalf("test status=%d body=%s", testRR.Code, testRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodPost, "/api/file-transfer/list", fileTransferJSONBody(t, map[string]any{
		"profile_id": profile.ID, "path": "/srv/app",
	}))
	listReq.Header.Set("Content-Type", "application/json")
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	var result struct {
		Path    string              `json:"path"`
		Entries []fileTransferEntry `json:"entries"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Path != "/srv/app" || len(result.Entries) != 2 {
		t.Fatalf("result=%+v", result)
	}
	if result.Entries[0].Name != "app.txt" || result.Entries[1].Type != "directory" {
		t.Fatalf("entries=%+v", result.Entries)
	}
}

func TestFileTransferDraftSFTPTestRejectsSecret(t *testing.T) {
	s, _, _ := newFileTransferProfileAPITestServer(t)
	h := s.Handler()
	body, err := json.Marshal(map[string]any{
		"profile": map[string]any{
			"name": "SFTP", "protocol": "sftp", "ssh_profile_id": "ssh-prod",
			"secret": "must-not-be-accepted",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/file-transfer/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}


func TestFileTransferSFTPMutationsUseStructuredCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	s, store, _ := newFileTransferProfileAPITestServer(t)
	profile, err := store.Create(filetransferprofile.Profile{
		ID: "sftp-mutate", Name: "SFTP Mutate", Protocol: filetransferprofile.ProtocolSFTP,
		SSHProfileID: "ssh-prod", InitialPath: "/srv/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "stdin.txt")
	script := filepath.Join(dir, "fake-sftp")
	scriptBody := "#!/bin/sh\ncat >" + inputPath + "\n"
	if err := os.WriteFile(script, []byte(scriptBody), 0o700); err != nil {
		t.Fatal(err)
	}
	s.SFTPExecutable = script
	h := s.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/file-transfer/mutate", fileTransferJSONBody(t, map[string]any{
		"profile_id": profile.ID,
		"action": "rename",
		"path": "/srv/app/old file.txt",
		"new_path": "/srv/app/new [file].txt",
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "rename \"/srv/app/old file.txt\" \"/srv/app/new \\[file\\].txt\"") {
		t.Fatalf("unexpected sftp stdin %q", got)
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("sftp command input leaked secret: %q", got)
	}
}

func TestFileTransferMutationRejectsRawAction(t *testing.T) {
	s, _, _ := newFileTransferProfileAPITestServer(t)
	h := s.Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/file-transfer/mutate", fileTransferJSONBody(t, map[string]any{
		"profile_id": "missing",
		"action": "chmod",
		"path": "/srv/app",
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
