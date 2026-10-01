package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
)

func TestRemoteDownloadName(t *testing.T) {
	for input, want := range map[string]string{
		"/srv/app/file.txt": "file.txt",
		"C:\\drop\\file.zip": "file.zip",
		"/": "download",
	} {
		if got := remoteDownloadName(input); got != want {
			t.Fatalf("remoteDownloadName(%q)=%q want %q", input, got, want)
		}
	}
}

func TestFileTransferSFTPDownloadUsesPrivateTempAndStreamsResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	s, store, _ := newFileTransferProfileAPITestServer(t)
	profile, err := store.Create(filetransferprofile.Profile{
		ID: "sftp-download", Name: "SFTP Download", Protocol: filetransferprofile.ProtocolSFTP,
		SSHProfileID: "ssh-prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-sftp")
	body := "#!/bin/sh\ninput=$(cat)\nlocal_path=$(printf '%s\\n' \"$input\" | sed -n 's/^get \"[^\"]*\" \"\\([^\"]*\\)\"$/\\1/p')\nprintf 'hello-sftp' > \"$local_path\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	s.SFTPExecutable = script
	h := s.Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/file-transfer/download", fileTransferJSONBody(t, map[string]any{
		"profile_id": profile.ID, "path": "/srv/app/hello.txt",
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "hello-sftp" {
		t.Fatalf("body=%q", rr.Body.String())
	}
	if disposition := rr.Header().Get("Content-Disposition"); !strings.Contains(disposition, "hello.txt") {
		t.Fatalf("Content-Disposition=%q", disposition)
	}
}

func TestFileTransferSFTPUploadUsesPutCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	s, store, _ := newFileTransferProfileAPITestServer(t)
	profile, err := store.Create(filetransferprofile.Profile{
		ID: "sftp-upload", Name: "SFTP Upload", Protocol: filetransferprofile.ProtocolSFTP,
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

	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	if err := writer.WriteField("profile_id", profile.ID); err != nil { t.Fatal(err) }
	if err := writer.WriteField("path", "/srv/app/new file.txt"); err != nil { t.Fatal(err) }
	part, err := writer.CreateFormFile("file", "new file.txt")
	if err != nil { t.Fatal(err) }
	if _, err := io.WriteString(part, "upload-data"); err != nil { t.Fatal(err) }
	if err := writer.Close(); err != nil { t.Fatal(err) }

	req := httptest.NewRequest(http.MethodPost, "/api/file-transfer/upload", &payload)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(capture)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(data), "put \"") || !strings.Contains(string(data), "\"/srv/app/new file.txt\"") {
		t.Fatalf("unexpected sftp command %q", data)
	}
}


func TestFileTransferDownloadAcceptsBrowserFormPost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/file-transfer/download", strings.NewReader("profile_id=abc&path=%2Fsrv%2Ffile.txt"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	got, err := decodeFileTransferDownloadRequest(rr, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProfileID != "abc" || got.Path != "/srv/file.txt" {
		t.Fatalf("decoded=%+v", got)
	}
}
