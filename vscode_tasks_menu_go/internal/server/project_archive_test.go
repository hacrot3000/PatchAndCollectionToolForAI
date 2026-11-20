package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func archiveJSONRequest(t *testing.T, s *Server, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	return rr
}

func TestProjectArchiveCreatePreviewExtractAndDownload(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "release", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "release", "app.bin"), []byte("firmware"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "release", "nested", "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}

	create := archiveJSONRequest(t, s, "/api/project/archive/create", `{"paths":["release"],"output":"release.zip","format":"zip"}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "release.zip")); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/project/archive/preview?path=release.zip", nil)
	preview := httptest.NewRecorder()
	s.Handler().ServeHTTP(preview, req)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	var result projectArchivePreviewResponse
	if err := json.Unmarshal(preview.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Format != "zip" || result.Files != 2 || result.Dirs < 1 {
		t.Fatalf("unexpected preview: %+v", result)
	}

	extract := archiveJSONRequest(t, s, "/api/project/archive/extract", `{"path":"release.zip","destination":"unpacked"}`)
	if extract.Code != http.StatusCreated {
		t.Fatalf("extract status=%d body=%s", extract.Code, extract.Body.String())
	}
	got, err := os.ReadFile(filepath.Join(root, "unpacked", "release", "app.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "firmware" {
		t.Fatalf("extracted app=%q", got)
	}

	download := archiveJSONRequest(t, s, "/api/project/archive/download", `{"paths":["release/app.bin","release/nested/readme.txt"],"name":"handoff.zip"}`)
	if download.Code != http.StatusOK {
		t.Fatalf("download status=%d body=%s", download.Code, download.Body.String())
	}
	if got := download.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("content type=%q", got)
	}
	reader, err := zip.NewReader(bytes.NewReader(download.Body.Bytes()), int64(download.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, file := range reader.File {
		names[file.Name] = true
	}
	if !names["app.bin"] || !names["readme.txt"] {
		t.Fatalf("download archive entries=%v", names)
	}
}

func TestProjectArchivePreviewAndExtractRejectTraversal(t *testing.T) {
	root := t.TempDir()
	var data bytes.Buffer
	zw := zip.NewWriter(&data)
	entry, err := zw.Create("../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(entry, "escape"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unsafe.zip"), data.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}

	req := httptest.NewRequest(http.MethodGet, "/api/project/archive/preview?path=unsafe.zip", nil)
	preview := httptest.NewRecorder()
	s.Handler().ServeHTTP(preview, req)
	if preview.Code != http.StatusBadRequest {
		t.Fatalf("unsafe preview status=%d body=%s", preview.Code, preview.Body.String())
	}

	extract := archiveJSONRequest(t, s, "/api/project/archive/extract", `{"path":"unsafe.zip","destination":"unsafe-out"}`)
	if extract.Code != http.StatusBadRequest {
		t.Fatalf("unsafe extract status=%d body=%s", extract.Code, extract.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("traversal wrote outside destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "unsafe-out")); !os.IsNotExist(err) {
		t.Fatalf("failed extract destination was not rolled back: %v", err)
	}
}

func TestProjectArchiveCreateDoesNotOverwriteOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "existing.zip"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	create := archiveJSONRequest(t, s, "/api/project/archive/create", `{"paths":["a.txt"],"output":"existing.zip","format":"zip"}`)
	if create.Code != http.StatusConflict {
		t.Fatalf("overwrite create status=%d body=%s", create.Code, create.Body.String())
	}
	got, err := os.ReadFile(filepath.Join(root, "existing.zip"))
	if err != nil || string(got) != "keep" {
		t.Fatalf("existing output changed: %q err=%v", got, err)
	}
}

func TestProjectArchiveTarGzCreatePreviewAndExtract(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bundle"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bundle", "firmware.bin"), []byte("fw"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	create := archiveJSONRequest(t, s, "/api/project/archive/create", `{"paths":["bundle"],"output":"bundle.tar.gz","format":"tar.gz"}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("tar create status=%d body=%s", create.Code, create.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/project/archive/preview?path=bundle.tar.gz", nil)
	preview := httptest.NewRecorder()
	s.Handler().ServeHTTP(preview, req)
	if preview.Code != http.StatusOK {
		t.Fatalf("tar preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	var result projectArchivePreviewResponse
	if err := json.Unmarshal(preview.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Format != "tar.gz" || result.Files != 1 {
		t.Fatalf("unexpected tar preview: %+v", result)
	}
	extract := archiveJSONRequest(t, s, "/api/project/archive/extract", `{"path":"bundle.tar.gz","destination":"tar-out"}`)
	if extract.Code != http.StatusCreated {
		t.Fatalf("tar extract status=%d body=%s", extract.Code, extract.Body.String())
	}
	if got, err := os.ReadFile(filepath.Join(root, "tar-out", "bundle", "firmware.bin")); err != nil || string(got) != "fw" {
		t.Fatalf("tar extracted payload=%q err=%v", got, err)
	}
}

func TestRemoteArchiveExtractCommandQuotesPathsAndRequiresKnownFormat(t *testing.T) {
	command, err := remoteArchiveExtractCommand("zip", "/srv/releases/a b's.zip", "/srv/app/new release")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"test ! -e '/srv/app/new release'",
		"unzip -q -- '/srv/releases/a b'\"'\"'s.zip'",
		"-d '/srv/app/new release'",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("remote zip command missing %q: %s", want, command)
		}
	}
	if _, err := remoteArchiveExtractCommand("rar", "x.rar", "out"); err == nil {
		t.Fatal("unsupported remote archive format unexpectedly accepted")
	}
}
