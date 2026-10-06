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
	"runtime"
	"strings"
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
	if err := os.WriteFile(medium, []byte(strings.Repeat("x", int(projectEditableLimit+1))), 0o644); err != nil {
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
	if !got.ReadOnly || !got.LargeFile || got.Warning == "" {
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


func TestProjectFileSaveAtomicPreservesModeAndUpdatesHash(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "save.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	old := sha256.Sum256([]byte("before\n"))
	body, err := json.Marshal(projectFileSaveRequest{
		Path:           "save.txt",
		Content:        "after\n",
		ExpectedSHA256: hex.EncodeToString(old[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", strings.NewReader(string(body))))
	if rr.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "after\n" {
		t.Fatalf("saved data=%q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%o want 640", info.Mode().Perm())
	}
	var got projectFileResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	nextHash := sha256.Sum256(data)
	if got.SHA256 != hex.EncodeToString(nextHash[:]) || got.ReadOnly {
		t.Fatalf("save response=%#v", got)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	foundSaved := false
	for _, entry := range entries {
		if entry.Name() == "save.txt" { foundSaved = true }
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("unexpected temp file after save: %s", entry.Name())
		}
	}
	if !foundSaved {
		t.Fatalf("saved file missing after save: %#v", entries)
	}
}

func TestProjectFileSavePreservesBOMAndCRLF(t *testing.T) {
	root := t.TempDir()
	original := append([]byte{0xEF, 0xBB, 0xBF}, []byte("one\r\ntwo\r\n")...)
	path := filepath.Join(root, "windows.txt")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	old := sha256.Sum256(original)
	body, err := json.Marshal(projectFileSaveRequest{
		Path:           "windows.txt",
		Content:        "alpha\nbeta\n",
		ExpectedSHA256: hex.EncodeToString(old[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", strings.NewReader(string(body))))
	if rr.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{0xEF, 0xBB, 0xBF}, []byte("alpha\r\nbeta\r\n")...)
	if string(data) != string(want) {
		t.Fatalf("saved bytes=%q want=%q", data, want)
	}
}

func TestProjectFileSaveDetectsExternalConflict(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "conflict.txt")
	initial := []byte("initial\n")
	if err := os.WriteFile(path, initial, 0o644); err != nil {
		t.Fatal(err)
	}
	old := sha256.Sum256(initial)
	if err := os.WriteFile(path, []byte("external\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(projectFileSaveRequest{
		Path:           "conflict.txt",
		Content:        "editor\n",
		ExpectedSHA256: hex.EncodeToString(old[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", strings.NewReader(string(body))))
	if rr.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d want=409 body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "external\n" {
		t.Fatalf("conflict overwrote disk data: %q", data)
	}
}

func TestProjectFileSaveRejectsReadOnlyAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	locked := filepath.Join(root, "locked.txt")
	if err := os.WriteFile(locked, []byte("locked"), 0o444); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("locked"))
	body, err := json.Marshal(projectFileSaveRequest{
		Path:           "locked.txt",
		Content:        "change",
		ExpectedSHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", strings.NewReader(string(body))))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("locked status=%d want=403 body=%s", rr.Code, rr.Body.String())
	}

	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "escape.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	outsideHash := sha256.Sum256([]byte("outside"))
	body, err = json.Marshal(projectFileSaveRequest{
		Path:           "escape.txt",
		Content:        "bad",
		ExpectedSHA256: hex.EncodeToString(outsideHash[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", strings.NewReader(string(body))))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("escape status=%d want=404 body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outside" {
		t.Fatalf("outside file changed: %q", data)
	}
}


func TestProjectFileSaveRejectsNULContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "text.txt")
	original := []byte("safe\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(original)
	body, err := json.Marshal(projectFileSaveRequest{
		Path: "text.txt", Content: "bad\x00text",
		ExpectedSHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", strings.NewReader(string(body))))
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d want 415 body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("file changed after rejected NUL save: %q", data)
	}
}

func TestProjectFileSaveAllowsJSONEscapingHeadroom(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "escaped.txt")
	original := []byte("old\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(original)
	content := strings.Repeat("\t", int(projectEditableLimit/2))
	body, err := json.Marshal(projectFileSaveRequest{
		Path: "escaped.txt", Content: content,
		ExpectedSHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(body)) <= projectEditableLimit {
		t.Fatalf("test payload did not exceed decoded-content limit: %d", len(body))
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}


func TestProjectFileSavePreservesMixedLineEndings(t *testing.T) {
	root := t.TempDir()
	original := []byte("one\r\ntwo\nthree\r\n")
	path := filepath.Join(root, "mixed.txt")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	old := sha256.Sum256(original)
	body, err := json.Marshal(projectFileSaveRequest{
		Path: "mixed.txt",
		Content: "ONE\nTWO\nTHREE\n",
		ExpectedSHA256: hex.EncodeToString(old[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("ONE\r\nTWO\nTHREE\r\n")
	if string(data) != string(want) {
		t.Fatalf("saved=%q want=%q", data, want)
	}
	var got projectFileResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.LineEnding != "mixed" {
		t.Fatalf("line ending=%q want mixed", got.LineEnding)
	}
}


func TestProjectTreeHidesGitDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
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
		t.Fatalf("entries=%#v", got)
	}
}

func TestProjectFileReadDecodesNonUTF8MarkdownReadOnly(t *testing.T) {
	root := t.TempDir()
	content := "# Báo cáo UART NVS\r\n\r\nPASS\r\n"
	raw := utf16LEWithBOM(content)
	path := filepath.Join(root, "BAO_CAO_MAINPCB_UART_NVS_20261003.md")
	if err := os.WriteFile(path, raw, 0o644); err != nil { t.Fatal(err) }

	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/file?path=BAO_CAO_MAINPCB_UART_NVS_20261003.md", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got projectFileResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	if got.Content != content || got.Encoding != "utf-16le" || !got.ReadOnly {
		t.Fatalf("response=%#v", got)
	}
	if !strings.Contains(got.Warning, "read-only") || !strings.Contains(got.Warning, "utf-16le") {
		t.Fatalf("warning=%q", got.Warning)
	}
}

func TestProjectFileReadDecodesWindows1252MarkdownReadOnly(t *testing.T) {
	root := t.TempDir()
	raw := []byte{'#',' ','C','a','f',0xE9,'\n'}
	if err := os.WriteFile(filepath.Join(root, "legacy.md"), raw, 0o644); err != nil { t.Fatal(err) }
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/file?path=legacy.md", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got projectFileResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	if got.Content != "# Café\n" || got.Encoding != "windows-1252" || !got.ReadOnly {
		t.Fatalf("response=%#v", got)
	}
}

func TestAttachedWorkspaceRootTreeAndEditorRoundTrip(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "main")
	attached := filepath.Join(base, "client")
	if err := os.MkdirAll(primary, 0o755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(attached, "src"), 0o755); err != nil { t.Fatal(err) }
	filePath := filepath.Join(attached, "src", "main.txt")
	if err := os.WriteFile(filePath, []byte("before\n"), 0o644); err != nil { t.Fatal(err) }
	s := &Server{Workspace: primary}

	create := callWorkspaceRoots(t, s, http.MethodPost, "/api/workspace-roots", `{"path":"../client","name":"Client"}`)
	if create.Code != http.StatusCreated { t.Fatalf("attach status=%d body=%s", create.Code, create.Body.String()) }
	var root workspaceRootView
	if err := json.Unmarshal(create.Body.Bytes(), &root); err != nil { t.Fatal(err) }
	virtualDir := workspaceVirtualPath(root.ID, "src")
	virtualFile := workspaceVirtualPath(root.ID, "src/main.txt")

	tree := httptest.NewRecorder()
	s.Handler().ServeHTTP(tree, httptest.NewRequest(http.MethodGet, "/api/project/tree?path="+urlQueryEscape(virtualDir), nil))
	if tree.Code != http.StatusOK { t.Fatalf("tree status=%d body=%s", tree.Code, tree.Body.String()) }
	var entries []projectTreeEntry
	if err := json.Unmarshal(tree.Body.Bytes(), &entries); err != nil { t.Fatal(err) }
	if len(entries) != 1 || entries[0].Name != "main.txt" { t.Fatalf("entries=%+v", entries) }

	read := httptest.NewRecorder()
	s.Handler().ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/api/project/file?path="+urlQueryEscape(virtualFile), nil))
	if read.Code != http.StatusOK { t.Fatalf("read status=%d body=%s", read.Code, read.Body.String()) }
	var file projectFileResponse
	if err := json.Unmarshal(read.Body.Bytes(), &file); err != nil { t.Fatal(err) }
	if file.Path != virtualFile || file.Content != "before\n" { t.Fatalf("file=%+v", file) }

	body, err := json.Marshal(projectFileSaveRequest{Path: virtualFile, Content: "after\n", ExpectedSHA256: file.SHA256})
	if err != nil { t.Fatal(err) }
	save := httptest.NewRecorder()
	s.Handler().ServeHTTP(save, httptest.NewRequest(http.MethodPut, "/api/project/file", bytes.NewReader(body)))
	if save.Code != http.StatusOK { t.Fatalf("save status=%d body=%s", save.Code, save.Body.String()) }
	raw, err := os.ReadFile(filePath)
	if err != nil { t.Fatal(err) }
	if string(raw) != "after\n" { t.Fatalf("attached file=%q", raw) }
}

func TestAttachedWorkspaceRootSymlinkCannotEscape(t *testing.T) {
	if runtime.GOOS == "windows" { t.Skip("symlink test") }
	base := t.TempDir()
	primary := filepath.Join(base, "main")
	attached := filepath.Join(base, "client")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{primary, attached, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil { t.Fatal(err) }
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil { t.Fatal(err) }
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(attached, "secret-link")); err != nil { t.Skipf("symlink unavailable: %v", err) }
	s := &Server{Workspace: primary}
	create := callWorkspaceRoots(t, s, http.MethodPost, "/api/workspace-roots", `{"path":"../client"}`)
	if create.Code != http.StatusCreated { t.Fatal(create.Body.String()) }
	var root workspaceRootView
	if err := json.Unmarshal(create.Body.Bytes(), &root); err != nil { t.Fatal(err) }
	virtualRoot := workspaceVirtualPath(root.ID, "")

	tree := httptest.NewRecorder()
	s.Handler().ServeHTTP(tree, httptest.NewRequest(http.MethodGet, "/api/project/tree?path="+urlQueryEscape(virtualRoot), nil))
	if tree.Code != http.StatusOK { t.Fatalf("tree status=%d body=%s", tree.Code, tree.Body.String()) }
	if strings.Contains(tree.Body.String(), "secret-link") { t.Fatalf("escaping symlink exposed: %s", tree.Body.String()) }

	read := httptest.NewRecorder()
	s.Handler().ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/api/project/file?path="+urlQueryEscape(workspaceVirtualPath(root.ID, "secret-link")), nil))
	if read.Code != http.StatusNotFound { t.Fatalf("escape read status=%d body=%s", read.Code, read.Body.String()) }
}

func TestProjectFileSaveExplicitLineEndingAndEncodingConversion(t *testing.T) {
	root := t.TempDir()
	original := append([]byte{0xEF, 0xBB, 0xBF}, []byte("one\r\ntwo\r\n")...)
	path := filepath.Join(root, "convert.txt")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	old := sha256.Sum256(original)
	body, err := json.Marshal(projectFileSaveRequest{
		Path:           "convert.txt",
		Content:        "alpha\nbeta\n",
		ExpectedSHA256: hex.EncodeToString(old[:]),
		LineEnding:     "lf",
		Encoding:       "utf-8",
	})
	if err != nil { t.Fatal(err) }
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", strings.NewReader(string(body))))
	if rr.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	if string(data) != "alpha\nbeta\n" {
		t.Fatalf("LF conversion bytes=%q", data)
	}
	var got projectFileResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	if got.LineEnding != "lf" || got.BOM {
		t.Fatalf("LF conversion response=%+v", got)
	}

	old = sha256.Sum256(data)
	body, err = json.Marshal(projectFileSaveRequest{
		Path:           "convert.txt",
		Content:        "gamma\ndelta\n",
		ExpectedSHA256: hex.EncodeToString(old[:]),
		LineEnding:     "crlf",
		Encoding:       "utf-8-bom",
	})
	if err != nil { t.Fatal(err) }
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", strings.NewReader(string(body))))
	if rr.Code != http.StatusOK {
		t.Fatalf("second save status=%d body=%s", rr.Code, rr.Body.String())
	}
	data, err = os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	want := append([]byte{0xEF, 0xBB, 0xBF}, []byte("gamma\r\ndelta\r\n")...)
	if !bytes.Equal(data, want) {
		t.Fatalf("CRLF+BOM conversion bytes=%q want=%q", data, want)
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	if got.LineEnding != "crlf" || !got.BOM || got.Encoding != "utf-8" {
		t.Fatalf("CRLF+BOM conversion response=%+v", got)
	}
}

func TestProjectFileSaveRejectsInvalidTextFormatOptions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "format.txt")
	initial := []byte("initial\n")
	if err := os.WriteFile(path, initial, 0o644); err != nil { t.Fatal(err) }
	sum := sha256.Sum256(initial)
	s := &Server{Workspace: root}
	for _, tc := range []projectFileSaveRequest{
		{Path: "format.txt", Content: "x\n", ExpectedSHA256: hex.EncodeToString(sum[:]), LineEnding: "native"},
		{Path: "format.txt", Content: "x\n", ExpectedSHA256: hex.EncodeToString(sum[:]), Encoding: "utf-16"},
	} {
		body, err := json.Marshal(tc)
		if err != nil { t.Fatal(err) }
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/project/file", strings.NewReader(string(body))))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("request=%+v status=%d body=%s", tc, rr.Code, rr.Body.String())
		}
	}
}

func TestApplyProjectLineEndingMode(t *testing.T) {
	current := []byte("one\r\ntwo\r\n")
	if label, got := applyProjectLineEndingMode("a\nb\n", current, "preserve"); label != "crlf" || got != "a\r\nb\r\n" {
		t.Fatalf("preserve label=%q got=%q", label, got)
	}
	if label, got := applyProjectLineEndingMode("a\r\nb\r\n", current, "lf"); label != "lf" || got != "a\nb\n" {
		t.Fatalf("lf label=%q got=%q", label, got)
	}
	if label, got := applyProjectLineEndingMode("a\nb\n", []byte("one\ntwo\n"), "crlf"); label != "crlf" || got != "a\r\nb\r\n" {
		t.Fatalf("crlf label=%q got=%q", label, got)
	}
}


func TestProjectFileMetadataReadAvoidsContentPayload(t *testing.T) {
	root := t.TempDir()
	content := []byte("generated content\n")
	if err := os.WriteFile(filepath.Join(root, "generated.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/file?meta=1&path=generated.txt", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("metadata status=%d body=%s", rr.Code, rr.Body.String())
	}
	var meta projectFileMetadata
	if err := json.Unmarshal(rr.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Path != "generated.txt" || meta.Size != int64(len(content)) || meta.MtimeNS <= 0 {
		t.Fatalf("metadata=%+v", meta)
	}
	if strings.Contains(rr.Body.String(), "generated content") || strings.Contains(rr.Body.String(), "\"content\"") {
		t.Fatalf("metadata-only response unexpectedly included file content: %s", rr.Body.String())
	}
}
