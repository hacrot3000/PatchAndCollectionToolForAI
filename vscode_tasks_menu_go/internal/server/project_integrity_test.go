package server

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectIntegrityHashManifestAndVerify(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "firmware", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	first := []byte("firmware-one\n")
	second := []byte{0x00, 0x01, 0x02, 0x03, 0xff}
	if err := os.WriteFile(filepath.Join(root, "firmware", "app.bin"), first, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "firmware", "nested", "data.bin"), second, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}

	req := httptest.NewRequest(http.MethodGet, "/api/project/integrity?path=firmware/app.bin", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("hash status=%d body=%s", rr.Code, rr.Body.String())
	}
	var hash projectIntegrityHashResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &hash); err != nil {
		t.Fatal(err)
	}
	wantSHA := sha256.Sum256(first)
	wantMD5 := md5.Sum(first)
	if hash.SHA256 != hex.EncodeToString(wantSHA[:]) || hash.MD5 != hex.EncodeToString(wantMD5[:]) {
		t.Fatalf("unexpected hashes: %+v", hash)
	}

	body := bytes.NewBufferString(`{"action":"manifest","path":"firmware"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/project/integrity", body)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("manifest status=%d body=%s", rr.Code, rr.Body.String())
	}
	var manifest struct {
		Files   int    `json:"files"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Files != 2 || !strings.Contains(manifest.Content, "  app.bin\n") || !strings.Contains(manifest.Content, "  nested/data.bin\n") {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	manifestPath := filepath.Join(root, "firmware", "SHA256SUMS")
	if err := os.WriteFile(manifestPath, []byte(manifest.Content), 0o644); err != nil {
		t.Fatal(err)
	}

	body = bytes.NewBufferString(`{"action":"verify_manifest","path":"firmware/SHA256SUMS"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/project/integrity", body)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify status=%d body=%s", rr.Code, rr.Body.String())
	}
	var verified projectIntegrityManifestVerifyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &verified); err != nil {
		t.Fatal(err)
	}
	if !verified.OK || verified.Total != 2 || verified.Matched != 2 {
		t.Fatalf("unexpected verification: %+v", verified)
	}

	if err := os.WriteFile(filepath.Join(root, "firmware", "app.bin"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body = bytes.NewBufferString(`{"action":"verify_manifest","path":"firmware/SHA256SUMS"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/project/integrity", body)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("mismatch verify status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &verified); err != nil {
		t.Fatal(err)
	}
	if verified.OK || verified.Mismatched != 1 {
		t.Fatalf("expected one mismatch: %+v", verified)
	}
}
