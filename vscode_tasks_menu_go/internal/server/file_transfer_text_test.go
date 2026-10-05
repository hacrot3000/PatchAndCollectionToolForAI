package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

func TestBoundedCompareBufferRejectsOversize(t *testing.T) {
	buffer := &boundedCompareBuffer{remaining: 4}
	n, err := buffer.Write([]byte("abcdef"))
	if err == nil || !buffer.exceeded {
		t.Fatalf("oversized write err=%v exceeded=%v", err, buffer.exceeded)
	}
	if n != 4 || buffer.buf.String() != "abcd" {
		t.Fatalf("n=%d body=%q", n, buffer.buf.String())
	}
}

func TestFileTransferSFTPTextCompareReadWriteCAS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	s, store, _ := newFileTransferProfileAPITestServer(t)
	profile, err := store.Create(filetransferprofile.Profile{
		ID: "sftp-compare", Name: "SFTP Compare", Protocol: filetransferprofile.ProtocolSFTP,
		SSHProfileID: "ssh-prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.txt")
	if err := os.WriteFile(remote, []byte("alpha\nbeta\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "fake-sftp")
	safeRemote := strings.ReplaceAll(remote, "\"", "\\\"")
	body := "#!/bin/sh\n" +
		"input=$(cat)\n" +
		"get_local=$(printf '%s\\n' \"$input\" | sed -n 's/^get \"[^\"]*\" \"\\([^\"]*\\)\"$/\\1/p')\n" +
		"put_local=$(printf '%s\\n' \"$input\" | sed -n 's/^put \"\\([^\"]*\\)\" \"[^\"]*\"$/\\1/p')\n" +
		"if [ -n \"$get_local\" ]; then cp \"" + safeRemote + "\" \"$get_local\"; fi\n" +
		"if [ -n \"$put_local\" ]; then cp \"$put_local\" \"" + safeRemote + "\"; fi\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	s.SFTPExecutable = script
	h := s.Handler()

	readTarget := "/api/file-transfer/text?profile_id=" + profile.ID + "&path=/srv/app/remote.txt"
	readRR := httptest.NewRecorder()
	h.ServeHTTP(readRR, httptest.NewRequest(http.MethodGet, readTarget, nil))
	if readRR.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", readRR.Code, readRR.Body.String())
	}
	var read struct {
		Content string `json:"content"`
		SHA256  string `json:"sha256"`
	}
	if err := json.Unmarshal(readRR.Body.Bytes(), &read); err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256([]byte("alpha\nbeta\n"))
	if read.Content != "alpha\nbeta\n" || read.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("read=%+v", read)
	}

	payload, _ := json.Marshal(map[string]any{
		"profile_id": profile.ID, "path": "/srv/app/remote.txt",
		"content": "alpha\nBETA\n", "expected_sha256": read.SHA256,
	})
	writeReq := httptest.NewRequest(http.MethodPut, "/api/file-transfer/text", bytes.NewReader(payload))
	writeReq.Header.Set("Content-Type", "application/json")
	writeRR := httptest.NewRecorder()
	h.ServeHTTP(writeRR, writeReq)
	if writeRR.Code != http.StatusOK {
		t.Fatalf("write status=%d body=%s", writeRR.Code, writeRR.Body.String())
	}
	if data, err := os.ReadFile(remote); err != nil || string(data) != "alpha\nBETA\n" {
		t.Fatalf("remote data=%q err=%v", data, err)
	}

	staleReq := httptest.NewRequest(http.MethodPut, "/api/file-transfer/text", bytes.NewReader(payload))
	staleReq.Header.Set("Content-Type", "application/json")
	staleRR := httptest.NewRecorder()
	h.ServeHTTP(staleRR, staleReq)
	if staleRR.Code != http.StatusConflict {
		t.Fatalf("stale write status=%d body=%s", staleRR.Code, staleRR.Body.String())
	}
	if data, err := os.ReadFile(remote); err != nil || string(data) != "alpha\nBETA\n" {
		t.Fatalf("stale write changed remote data=%q err=%v", data, err)
	}
}

func TestFileTransferTextRejectsBinaryAndInvalidCAS(t *testing.T) {
	if validCompareSHA256("not-a-sha") {
		t.Fatal("invalid SHA accepted")
	}
	if !validCompareSHA256(strings.Repeat("a", 64)) {
		t.Fatal("valid SHA rejected")
	}
	if projectTextBytesValid([]byte{'a', 0, 'b'}) {
		t.Fatal("binary fixture unexpectedly considered text")
	}
}
