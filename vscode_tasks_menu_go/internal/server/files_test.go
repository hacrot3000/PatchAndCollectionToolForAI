package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadableFilesFromSelectedCollectorOutput(t *testing.T) {
	workspace := t.TempDir()
	outDir := filepath.Join(workspace, "artifacts", "ptv_to_ai")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(outDir, "CR_57a48bda.zip")
	txtPath := filepath.Join(outDir, "CR_57a48bda.txt")
	if err := os.WriteFile(zipPath, []byte("zip-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txtPath, []byte("txt-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{Workspace: workspace}
	selection := "========================================================================\n" +
		"!!! [PRIMARY - UPLOAD THIS FILE] !!!\n" +
		">>> ACTION REQUIRED: UPLOAD TO CHATGPT / AI SERVER <<<\n" +
		"ZIP (preferred) — copy path below:\n" + zipPath + "\n" +
		"Clear-text TXT — copy path below:\n" + txtPath + "\n" +
		"========================================================================"
	files := s.downloadableFilesFromText(selection)
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2: %#v", len(files), files)
	}
	if files[0].Path != zipPath || files[0].Name != filepath.Base(zipPath) {
		t.Fatalf("first file = %#v, want ZIP first", files[0])
	}
	if files[1].Path != txtPath {
		t.Fatalf("second file path = %q, want %q", files[1].Path, txtPath)
	}
	if !strings.Contains(files[0].URL, "/api/files/download?path=") {
		t.Fatalf("download URL = %q", files[0].URL)
	}
}

func TestDownloadableFilesFiltersPTVNoiseAndKeepsPrimaryArtifacts(t *testing.T) {
	workspace := t.TempDir()
	write := func(rel string) string {
		t.Helper()
		path := filepath.Join(workspace, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	runner := write("tools/run_python_patches.sh")
	collect := write("patchs/collect_ota_protect_step4_local_source.py")
	toolZip := write("patchs/python_patch_tool_v6.17.14.zip")
	archived := write("patchs/patched/CODE_COLLECTION_REQUEST_step8_phase4b4b_base_profile_lifecycle_commands_20260916_v1.zip")
	preferredZip := write("artifacts/ptv_to_ai/CR_acaa0d27.zip")
	preferredTxt := write("artifacts/ptv_to_ai/CR_acaa0d27.txt")

	console := strings.Join([]string{
		"Task       : Patchs: Run Python Patch",
		"Thư mục    : " + workspace,
		"Lệnh       : bash " + runner,
		"Mô tả      : Mở hàng đợi Patch Tool hiện tại.",
		"[PTV v6.20.2 WARNING] SKIPPED non-patch candidate: " + collect + " (no_patch_signature)",
		"[PTV v6.20.2 WARNING] SKIPPED non-patch candidate: " + toolZip + " (tool_distribution)",
		"!!! [PRIMARY - UPLOAD THIS FILE] !!!",
		"ZIP (preferred) — copy path below:",
		preferredZip,
		"Clear-text TXT — copy path below:",
		preferredTxt,
		"[INFO] REQUEST ARCHIVED: " + archived,
	}, "\n")

	files := (&Server{Workspace: workspace}).downloadableFilesFromText(console)
	if len(files) != 2 {
		t.Fatalf("files = %#v, want only primary ZIP/TXT", files)
	}
	if files[0].Path != preferredZip || files[1].Path != preferredTxt {
		t.Fatalf("files = %#v, want ZIP then TXT", files)
	}
}

func TestDownloadableFilesAcceptRelativeWorkspacePath(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "artifacts", "result.zip")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	files := s.downloadableFilesFromText("result: artifacts/result.zip")
	if len(files) != 1 || files[0].Path != path {
		t.Fatalf("files = %#v, want %q", files, path)
	}
}

func TestResolveDownloadPathRejectsOutsideWorkspaceAndSymlink(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	if _, err := s.resolveDownloadPath(outside); err == nil {
		t.Fatal("absolute path outside workspace was accepted")
	}
	if _, err := s.resolveDownloadPath(filepath.Join("..", filepath.Base(outsideDir), "secret.txt")); err == nil {
		t.Fatal("relative traversal outside workspace was accepted")
	}
	link := filepath.Join(workspace, "outside-link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := s.resolveDownloadPath(link); err == nil {
		t.Fatal("symlink escaping workspace was accepted")
	}
}

func TestFileSelectionAPIAndDownload(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "artifacts", "CR_test.zip")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("download-payload")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}

	req := httptest.NewRequest(http.MethodPost, "/api/files/selection", strings.NewReader(`{"text":"`+filepath.ToSlash(path)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.filesSelection(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("selection status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var selected struct {
		Files []downloadableFile `json:"files"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &selected); err != nil {
		t.Fatal(err)
	}
	if len(selected.Files) != 1 {
		t.Fatalf("selection files = %#v", selected.Files)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/api/files/download?path="+url.QueryEscape(path), nil)
	downloadRR := httptest.NewRecorder()
	s.fileDownload(downloadRR, downloadReq)
	res := downloadRR.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d, body=%s", res.StatusCode, downloadRR.Body.String())
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(payload) {
		t.Fatalf("download body = %q, want %q", body, payload)
	}
	if got := res.Header.Get("Content-Disposition"); !strings.Contains(got, "attachment") || !strings.Contains(got, "CR_test.zip") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}
