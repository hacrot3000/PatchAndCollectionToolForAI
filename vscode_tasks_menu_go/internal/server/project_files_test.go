package server

import (
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
