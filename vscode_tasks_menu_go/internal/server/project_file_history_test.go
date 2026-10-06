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
	"strings"
	"testing"
)

func saveEditorVersion(t *testing.T, s *Server, rel, content string, current []byte) projectFileResponse {
	t.Helper()
	sum := sha256.Sum256(current)
	body, err := json.Marshal(projectFileSaveRequest{
		Path: rel, Content: content, ExpectedSHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil { t.Fatal(err) }
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("save %s status=%d body=%s", rel, rr.Code, rr.Body.String())
	}
	var got projectFileResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	return got
}

func listLocalFileHistory(t *testing.T, s *Server, rel string) []projectFileHistoryEntry {
	t.Helper()
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/file-history?path="+urlQueryEscape(rel), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("history list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Path    string                    `json:"path"`
		Entries []projectFileHistoryEntry `json:"entries"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	if got.Path != rel { t.Fatalf("history path=%q want=%q", got.Path, rel) }
	return got.Entries
}

func TestLocalFileHistoryCapturesPreviousVersionWithExactFormat(t *testing.T) {
	root := t.TempDir()
	rel := "versioned.txt"
	original := append([]byte{0xEF, 0xBB, 0xBF}, []byte("one\r\ntwo\r\n")...)
	if err := os.WriteFile(filepath.Join(root, rel), original, 0o644); err != nil { t.Fatal(err) }
	s := &Server{Workspace: root}

	saved := saveEditorVersion(t, s, rel, "alpha\nbeta\n", original)
	if saved.HistoryWarning != "" {
		t.Fatalf("unexpected history warning: %q", saved.HistoryWarning)
	}
	entries := listLocalFileHistory(t, s, rel)
	if len(entries) != 1 {
		t.Fatalf("history entries=%+v", entries)
	}
	sum := sha256.Sum256(original)
	if entries[0].SHA256 != hex.EncodeToString(sum[:]) || entries[0].Size != int64(len(original)) || entries[0].SavedAt == "" {
		t.Fatalf("history entry=%+v", entries[0])
	}

	detail := httptest.NewRecorder()
	target := "/api/project/file-history?path="+urlQueryEscape(rel)+"&id="+entries[0].ID
	s.Handler().ServeHTTP(detail, httptest.NewRequest(http.MethodGet, target, nil))
	if detail.Code != http.StatusOK {
		t.Fatalf("history detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	var got projectFileHistoryContent
	if err := json.Unmarshal(detail.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	if got.Content != "one\r\ntwo\r\n" || !got.BOM || got.LineEnding != "crlf" || got.Encoding != "utf-8" {
		t.Fatalf("history detail=%+v", got)
	}

	dir, err := projectFileHistoryDir(root, rel, false)
	if err != nil { t.Fatal(err) }
	for _, path := range []string{filepath.Join(dir, projectFileHistoryIndexName), filepath.Join(dir, entries[0].ID+".bin")} {
		info, err := os.Stat(path)
		if err != nil { t.Fatal(err) }
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode=%o want 600", path, info.Mode().Perm())
		}
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("history dir mode=%v err=%v", info.Mode().Perm(), err)
	}
}

func TestLocalFileHistoryOrdersVersionsAndPrunesRetention(t *testing.T) {
	root := t.TempDir()
	rel := "rolling.txt"
	current := []byte("v0\n")
	if err := os.WriteFile(filepath.Join(root, rel), current, 0o644); err != nil { t.Fatal(err) }
	s := &Server{Workspace: root}

	for i := 1; i <= projectFileHistoryMaxEntries+3; i++ {
		next := "v"+strconv.Itoa(i)+"\n"
		saveEditorVersion(t, s, rel, next, current)
		current = []byte(next)
	}
	entries := listLocalFileHistory(t, s, rel)
	if len(entries) != projectFileHistoryMaxEntries {
		t.Fatalf("history len=%d want=%d entries=%+v", len(entries), projectFileHistoryMaxEntries, entries)
	}
	previous := []byte("v"+strconv.Itoa(projectFileHistoryMaxEntries+2)+"\n")
	sum := sha256.Sum256(previous)
	if entries[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("latest history sha=%s want=%s", entries[0].SHA256, hex.EncodeToString(sum[:]))
	}
	dir, err := projectFileHistoryDir(root, rel, false)
	if err != nil { t.Fatal(err) }
	files, err := os.ReadDir(dir)
	if err != nil { t.Fatal(err) }
	binCount := 0
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".bin") { binCount++ }
	}
	if binCount != projectFileHistoryMaxEntries {
		t.Fatalf("history bin count=%d want=%d files=%+v", binCount, projectFileHistoryMaxEntries, files)
	}
}

func TestLocalFileHistoryRejectsTraversalUnknownAndTamperedEntry(t *testing.T) {
	root := t.TempDir()
	rel := "safe.txt"
	original := []byte("safe\n")
	if err := os.WriteFile(filepath.Join(root, rel), original, 0o644); err != nil { t.Fatal(err) }
	s := &Server{Workspace: root}
	saveEditorVersion(t, s, rel, "next\n", original)
	entries := listLocalFileHistory(t, s, rel)
	if len(entries) != 1 { t.Fatal("missing history fixture") }

	bad := httptest.NewRecorder()
	s.Handler().ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/api/project/file-history?path=..%2Fescape.txt", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("traversal status=%d body=%s", bad.Code, bad.Body.String())
	}
	missing := httptest.NewRecorder()
	s.Handler().ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/project/file-history?path=safe.txt&id=1234567890-aaaaaaaaaaaa", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing history status=%d body=%s", missing.Code, missing.Body.String())
	}

	dir, err := projectFileHistoryDir(root, rel, false)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(dir, entries[0].ID+".bin"), []byte("tampered\n"), 0o600); err != nil { t.Fatal(err) }
	tampered := httptest.NewRecorder()
	target := "/api/project/file-history?path=safe.txt&id="+entries[0].ID
	s.Handler().ServeHTTP(tampered, httptest.NewRequest(http.MethodGet, target, nil))
	if tampered.Code != http.StatusConflict || !strings.Contains(tampered.Body.String(), "unsafe") && !strings.Contains(tampered.Body.String(), "checksum") {
		t.Fatalf("tampered history status=%d body=%s", tampered.Code, tampered.Body.String())
	}
}
