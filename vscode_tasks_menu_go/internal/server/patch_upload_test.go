package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func patchUploadRequest(t *testing.T, name string, data []byte, overwrite bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	target := "/api/patch/upload"
	if overwrite {
		target += "?overwrite=1"
	}
	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestPatchUploadPublishesPackageIntoPatchQueue(t *testing.T) {
	root := t.TempDir()
	s := &Server{Workspace: root}
	payload := []byte("PK\x03\x04test-patch-package")
	req := patchUploadRequest(t, "PATCH_demo.zip", payload, false)
	res := httptest.NewRecorder()

	s.patchUpload(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	stored, err := os.ReadFile(filepath.Join(root, "patchs", "PATCH_demo.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, payload) {
		t.Fatalf("stored bytes=%q want=%q", stored, payload)
	}

	var result patchUploadResult
	if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if result.Name != "PATCH_demo.zip" || result.QueuePath != "patchs/PATCH_demo.zip" || result.Size != int64(len(payload)) || result.SHA256 != hex.EncodeToString(sum[:]) || result.Overwrite {
		t.Fatalf("unexpected upload result: %+v", result)
	}
}

func TestPatchUploadConflictRequiresExplicitOverwrite(t *testing.T) {
	root := t.TempDir()
	queue := filepath.Join(root, "patchs")
	if err := os.Mkdir(queue, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(queue, "collect.zip")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}

	res := httptest.NewRecorder()
	s.patchUpload(res, patchUploadRequest(t, "collect.zip", []byte("new"), false))
	if res.Code != http.StatusConflict || !strings.Contains(res.Body.String(), "already exists") {
		t.Fatalf("status=%d body=%q", res.Code, res.Body.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("conflict changed existing package: %q", got)
	}

	res = httptest.NewRecorder()
	s.patchUpload(res, patchUploadRequest(t, "collect.zip", []byte("new"), true))
	if res.Code != http.StatusCreated {
		t.Fatalf("overwrite status=%d body=%q", res.Code, res.Body.String())
	}
	got, err = os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("overwrite bytes=%q want new", got)
	}
}

func TestPatchUploadRejectsUnsupportedInputAndUnsafeQueueDir(t *testing.T) {
	root := t.TempDir()
	s := &Server{Workspace: root}

	res := httptest.NewRecorder()
	s.patchUpload(res, patchUploadRequest(t, "notes.txt", []byte("not a package"), false))
	if res.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("unsupported extension status=%d body=%q", res.Code, res.Body.String())
	}

	if runtime.GOOS == "windows" {
		return
	}
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(root, "patchs")); err != nil {
		t.Fatal(err)
	}
	res = httptest.NewRecorder()
	s.patchUpload(res, patchUploadRequest(t, "safe.zip", []byte("zip"), false))
	if res.Code != http.StatusConflict || !strings.Contains(res.Body.String(), "unsafe") {
		t.Fatalf("unsafe patchs status=%d body=%q", res.Code, res.Body.String())
	}
}

func TestSupportedPatchUploadName(t *testing.T) {
	for _, name := range []string{"a.zip", "legacy.tar", "legacy.tgz", "legacy.tar.gz", "UPPER.ZIP"} {
		if !supportedPatchUploadName(name) {
			t.Fatalf("supported package rejected: %s", name)
		}
	}
	for _, name := range []string{"", "a.txt", "archive.gz", "patch.zip.exe"} {
		if supportedPatchUploadName(name) {
			t.Fatalf("unsupported package accepted: %s", name)
		}
	}
}
