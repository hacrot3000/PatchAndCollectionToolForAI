package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectTreeLazyListingAndOrdering(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "z.txt"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/tree?path=", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("tree status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got []projectTreeEntry
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("entries=%#v", got)
	}
	if got[0].Name != "src" || got[0].Type != "dir" {
		t.Fatalf("first entry=%#v, want src dir", got[0])
	}
	if got[1].Name != "a.txt" || got[1].Type != "file" || got[1].Size != 3 {
		t.Fatalf("second entry=%#v, want a.txt file", got[1])
	}
	if got[2].Name != "z.txt" {
		t.Fatalf("third entry=%#v, want z.txt", got[2])
	}
}

func TestProjectTreeRejectsTraversalAbsoluteAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	s := &Server{Workspace: root}

	for _, path := range []string{"../", "/etc", "C:\\Windows"} {
		req := httptest.NewRequest(http.MethodGet, "/api/project/tree?path="+urlQueryEscape(path), nil)
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("path %q status=%d want 400 body=%s", path, rr.Code, rr.Body.String())
		}
	}

	link := filepath.Join(root, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/tree?path=outside", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("symlink escape status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
}

func TestProjectTreeSkipsEscapingSymlinkEntry(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "secret-link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "visible.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/tree", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got []projectTreeEntry
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "visible.txt" {
		t.Fatalf("entries=%#v, want only visible.txt", got)
	}
}

func urlQueryEscape(value string) string {
	replacer := map[rune]string{
		' ': "%20",
		'/': "%2F",
		'\\': "%5C",
		':': "%3A",
	}
	out := ""
	for _, r := range value {
		if escaped, ok := replacer[r]; ok {
			out += escaped
		} else {
			out += string(r)
		}
	}
	return out
}


func TestProjectFileReadMetadataAndEncoding(t *testing.T) {
	root := t.TempDir()
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("one\r\ntwo\r\n")...)
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/file?path=sample.txt", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got projectFileResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got.Path != "sample.txt" || got.Content != "one\r\ntwo\r\n" {
		t.Fatalf("response=%#v", got)
	}
	if got.SHA256 != hex.EncodeToString(sum[:]) || got.Size != int64(len(data)) {
		t.Fatalf("hash/size response=%#v", got)
	}
	if got.Encoding != "utf-8" || got.LineEnding != "crlf" || !got.BOM || got.ReadOnly {
		t.Fatalf("metadata response=%#v", got)
	}
}

func TestProjectFileReadRejectsBinaryInvalidUTF8AndOversize(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name   string
		data   []byte
		status int
	}{
		{name: "binary.bin", data: []byte{'a', 0, 'b'}, status: http.StatusUnsupportedMediaType},
		{name: "invalid.txt", data: []byte{0xff, 0xfe, 0xfd}, status: http.StatusUnsupportedMediaType},
	}
	s := &Server{Workspace: root}
	for _, tc := range cases {
		if err := os.WriteFile(filepath.Join(root, tc.name), tc.data, 0o644); err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/file?path="+tc.name, nil))
		if rr.Code != tc.status {
			t.Fatalf("%s status=%d want=%d body=%s", tc.name, rr.Code, tc.status, rr.Body.String())
		}
	}
	huge := filepath.Join(root, "huge.txt")
	f, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(projectReadableLimit + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/file?path=huge.txt", nil))
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status=%d want=413 body=%s", rr.Code, rr.Body.String())
	}
}

func TestProjectFileReadMarksMediumAndPermissionFilesReadOnly(t *testing.T) {
	root := t.TempDir()
	medium := filepath.Join(root, "medium.txt")
	f, err := os.Create(medium)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(projectEditableLimit + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/file?path=medium.txt", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("medium status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got projectFileResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.ReadOnly || got.Warning == "" {
		t.Fatalf("medium response=%#v", got)
	}

	locked := filepath.Join(root, "locked.txt")
	if err := os.WriteFile(locked, []byte("locked"), 0o444); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/file?path=locked.txt", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("locked status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.ReadOnly {
		t.Fatalf("locked response=%#v", got)
	}
}
