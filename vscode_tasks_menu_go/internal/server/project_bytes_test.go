package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectBytesReadsBoundedChunks(t *testing.T) {
	root := t.TempDir()
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 251)
	}
	path := filepath.Join(root, "sample.bin")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	req := httptest.NewRequest(http.MethodGet, "/api/project/bytes?path=sample.bin&offset=100&limit=64", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got projectBytesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != "sample.bin" || got.Offset != 100 || got.Length != 64 || got.Size != int64(len(data)) || got.NextOffset != 164 || got.EOF {
		t.Fatalf("response=%+v", got)
	}
	raw, err := base64.StdEncoding.DecodeString(got.Base64)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(data[100:164]) {
		t.Fatalf("chunk mismatch")
	}
}

func TestProjectBytesEOFAndRangeValidation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "short.bin"), []byte{1, 2, 3, 4}, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/bytes?path=short.bin&offset=3&limit=32", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got projectBytesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.EOF || got.Length != 1 || got.NextOffset != 4 {
		t.Fatalf("response=%+v", got)
	}

	for _, rawURL := range []string{
		"/api/project/bytes?path=short.bin&offset=-1",
		"/api/project/bytes?path=short.bin&limit=0",
		"/api/project/bytes?path=short.bin&limit=65537",
		"/api/project/bytes?path=short.bin&offset=bad",
	} {
		rr = httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, rawURL, nil))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", rawURL, rr.Code, rr.Body.String())
		}
	}
}

func TestProjectBytesRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.bin")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "secret.bin")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/bytes?path=secret.bin", nil))
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret") {
		t.Fatalf("response leaks outside file content: %s", rr.Body.String())
	}
}
