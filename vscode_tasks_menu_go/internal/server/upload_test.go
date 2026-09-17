package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func uploadRequest(t *testing.T, handler http.Handler, dir, name, content string, overwrite bool) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("dir", dir); err != nil { t.Fatal(err) }
	part, err := mw.CreateFormFile("file", name)
	if err != nil { t.Fatal(err) }
	if _, err := io.WriteString(part, content); err != nil { t.Fatal(err) }
	if err := mw.Close(); err != nil { t.Fatal(err) }
	url := "/api/files/upload"
	if overwrite { url += "?overwrite=1" }
	req := httptest.NewRequest(http.MethodPost, url, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder();handler.ServeHTTP(rr, req);return rr
}

func TestWorkspaceUploadAndOverwritePolicy(t *testing.T) {
	root := t.TempDir();sub := filepath.Join(root, "artifacts");if err := os.Mkdir(sub, 0o755); err != nil { t.Fatal(err) }
	s := &Server{Workspace: root}
	h := s.Handler()
	rr := uploadRequest(t, h, "artifacts", "result.txt", "first", false)
	if rr.Code != http.StatusCreated { t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String()) }
	data, err := os.ReadFile(filepath.Join(sub, "result.txt"));if err != nil || string(data)!="first" { t.Fatalf("uploaded content=%q err=%v", data, err) }
	rr = uploadRequest(t, h, "artifacts", "result.txt", "second", false)
	if rr.Code != http.StatusConflict { t.Fatalf("duplicate status=%d want 409", rr.Code) }
	rr = uploadRequest(t, h, "artifacts", "result.txt", "second", true)
	if rr.Code != http.StatusCreated { t.Fatalf("overwrite status=%d body=%s", rr.Code, rr.Body.String()) }
	data, err = os.ReadFile(filepath.Join(sub, "result.txt"));if err != nil || string(data)!="second" { t.Fatalf("overwritten content=%q err=%v", data, err) }
}

func TestWorkspaceUploadRejectsTraversalAndSymlinkDirectory(t *testing.T) {
	root := t.TempDir();outside := t.TempDir();s := &Server{Workspace: root};h := s.Handler()
	rr := uploadRequest(t, h, "../", "bad.txt", "x", false)
	if rr.Code != http.StatusBadRequest { t.Fatalf("traversal status=%d want 400", rr.Code) }
	link := filepath.Join(root, "outside");if err := os.Symlink(outside, link); err != nil { t.Fatal(err) }
	rr = uploadRequest(t, h, "outside", "bad.txt", "x", false)
	if rr.Code != http.StatusBadRequest { t.Fatalf("symlink escape status=%d want 400", rr.Code) }
}

func TestUploadFeatureModule(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/upload.js");if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{"Upload", "dragenter", "FormData", "/api/files/upload", "overwrite=1", "Ghi đè?"} {
		if !strings.Contains(js, want) { t.Fatalf("upload module missing %q", want) }
	}
}
