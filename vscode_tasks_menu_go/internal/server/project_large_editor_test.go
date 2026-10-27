package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"strings"
	"testing"
)

func projectLargeEditorCall(s *Server, method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}
	record := httptest.NewRecorder()
	s.Handler().ServeHTTP(record, req)
	return record
}

func TestWindowedEditorStreamsLargeFileWithoutDownloadingWholeSource(t *testing.T) {
	root := t.TempDir()
	// 4.4 MiB: larger than both the historical 2 MiB write cap and a single window.
	original := []byte(strings.Repeat("alpha line\n", 400000))
	path := filepath.Join(root, "huge.txt")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	open := projectLargeEditorCall(s, http.MethodGet, "/api/project/file?windowed=1&path=huge.txt", nil)
	if open.Code != http.StatusOK {
		t.Fatalf("large editor open: %d %s", open.Code, open.Body.String())
	}
	var meta projectFileResponse
	if err := json.Unmarshal(open.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if !meta.Streamed || !meta.LargeFile || meta.ReadOnly || meta.Content != "" || meta.Size != int64(len(original)) {
		t.Fatalf("large file should be editable without full content: %+v", meta)
	}
	if open.Body.Len() > 2048 {
		t.Fatalf("initial large-file response too large: %d bytes", open.Body.Len())
	}

	url := "/api/project/file-window?path=huge.txt&start=100000&count=150"
	read := projectLargeEditorCall(s, http.MethodGet, url, nil)
	if read.Code != http.StatusOK {
		t.Fatalf("window load: %d %s", read.Code, read.Body.String())
	}
	var window largeEditorWindowResponse
	if err := json.Unmarshal(read.Body.Bytes(), &window); err != nil {
		t.Fatal(err)
	}
	if window.StartLine != 100000 || window.TotalLines != 400001 || window.EndLine != 100149 ||
		window.StartByte <= 0 || window.EndByte <= window.StartByte || len(window.Content) > largeEditorMaxWindow {
		t.Fatalf("wrong bounded line window: %+v", window)
	}
	want := []byte(window.Content)
	if !bytes.Equal(original[window.StartByte:window.EndByte], want) {
		t.Fatal("window bytes do not match original source")
	}
	edited := strings.Replace(window.Content, "alpha line\n", "beta edited line\n", 1)
	if edited == window.Content {
		t.Fatal("test replacement did not apply")
	}
	payload, err := json.Marshal(largeEditorSaveInput{
		Path: "huge.txt", StartByte: window.StartByte, EndByte: window.EndByte,
		Size: window.Size, MtimeNS: window.MtimeNS, SHA256: window.SHA256,
		WindowSHA: window.WindowSHA, Content: edited,
	})
	if err != nil {
		t.Fatal(err)
	}
	save := projectLargeEditorCall(s, http.MethodPut, "/api/project/file-window/save", payload)
	if save.Code != http.StatusOK {
		t.Fatalf("large editor save: %d %s", save.Code, save.Body.String())
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := append([]byte(nil), original[:window.StartByte]...)
	expected = append(expected, []byte(edited)...)
	expected = append(expected, original[window.EndByte:]...)
	if !bytes.Equal(expected, actual) {
		t.Fatal("large editor save modified bytes outside the selected window")
	}
	reloaded := projectLargeEditorCall(s, http.MethodGet, url, nil)
	if reloaded.Code != http.StatusOK || !strings.Contains(reloaded.Body.String(), "beta edited line") {
		t.Fatalf("large editor index did not refresh after save: %d", reloaded.Code)
	}
}

func TestWindowedEditorRejectsStaleSourceAndInvalidRange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "huge.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("line\n", 450000)), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	read := projectLargeEditorCall(s, http.MethodGet, "/api/project/file-window?path=huge.txt&start=800&count=10", nil)
	if read.Code != http.StatusOK {
		t.Fatalf("read status=%d: %s", read.Code, read.Body.String())
	}
	var window largeEditorWindowResponse
	if err := json.Unmarshal(read.Body.Bytes(), &window); err != nil {
		t.Fatal(err)
	}
	request := largeEditorSaveInput{
		Path: "huge.txt", StartByte: window.StartByte, EndByte: window.EndByte,
		Size: window.Size, MtimeNS: window.MtimeNS, SHA256: window.SHA256,
		WindowSHA: window.WindowSHA, Content: "replacement\n",
	}
	request.EndByte = window.Size + 1
	raw, _ := json.Marshal(request)
	bad := projectLargeEditorCall(s, http.MethodPut, "/api/project/file-window/save", raw)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid byte range status=%d: %s", bad.Code, bad.Body.String())
	}
	request.EndByte = window.EndByte
	if err := os.WriteFile(path, []byte(strings.Repeat("changed\n", 350000)), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(request)
	stale := projectLargeEditorCall(s, http.MethodPut, "/api/project/file-window/save", raw)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale source status=%d: %s", stale.Code, stale.Body.String())
	}
}

func TestWindowedEditorRejectsBinaryAndNonexistentPath(t *testing.T) {
	root := t.TempDir()
	binary := append([]byte(strings.Repeat("ok\n", 800000)), 0)
	if err := os.WriteFile(filepath.Join(root, "binary.txt"), binary, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	invalid := projectLargeEditorCall(s, http.MethodGet, "/api/project/file-window?path=binary.txt&start=1", nil)
	if invalid.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("binary must be rejected; status=%d: %s", invalid.Code, invalid.Body.String())
	}
	missing := projectLargeEditorCall(s, http.MethodGet, fmt.Sprintf("/api/project/file-window?path=%s", "missing.txt"), nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing file status=%d", missing.Code)
	}
}

func TestWindowedEditorSaveAcceptsExactTimestampAndRejectsWrongRevision(t *testing.T) {
 root:=t.TempDir()
 path:=filepath.Join(root,"huge.txt")
 original:=[]byte(strings.Repeat("alpha line\n",210000))
 if err:=os.WriteFile(path,original,0o644);err!=nil{t.Fatal(err)}
 const exactNS int64=1761588600123456789
 ts:=time.Unix(0,exactNS)
 if err:=os.Chtimes(path,ts,ts);err!=nil{t.Fatal(err)}
 s:=&Server{Workspace:root}
 read:=projectLargeEditorCall(s,http.MethodGet,"/api/project/file-window?path=huge.txt&start=1000&count=100",nil)
 if read.Code!=http.StatusOK{t.Fatalf("window: %d %s",read.Code,read.Body.String())}
 var window largeEditorWindowResponse
 if err:=json.Unmarshal(read.Body.Bytes(),&window);err!=nil{t.Fatal(err)}
 if window.MtimeNSExact!=strconv.FormatInt(exactNS,10) {
  t.Fatalf("lost exact mtime in API: %q",window.MtimeNSExact)
 }
 edited:=strings.Replace(window.Content,"alpha","beta",1)
 req:=largeEditorSaveInput{
  Path:"huge.txt",StartByte:window.StartByte,EndByte:window.EndByte,
  Size:window.Size,MtimeNS:int64(float64(window.MtimeNS)),
  MtimeNSExact:window.MtimeNSExact,SHA256:window.SHA256,
  WindowSHA:window.WindowSHA,Content:edited,
 }
 send:=func(req largeEditorSaveInput)*httptest.ResponseRecorder{
  payload,err:=json.Marshal(req);if err!=nil{t.Fatal(err)}
  return projectLargeEditorCall(s,http.MethodPut,"/api/project/file-window/save",payload)
 }
 wrong:=req
 wrong.MtimeNSExact=strconv.FormatInt(exactNS+1,10)
 bad:=send(wrong)
 if bad.Code!=http.StatusConflict {t.Fatalf("wrong exact timestamp accepted: %d %s",bad.Code,bad.Body.String())}
 result:=send(req)
 if result.Code!=http.StatusOK {t.Fatalf("Save with exact mtime failed: %d %s",result.Code,result.Body.String())}
 actual,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
 expected:=append([]byte(nil),original[:window.StartByte]...)
 expected=append(expected,[]byte(edited)...)
 expected=append(expected,original[window.EndByte:]...)
 if !bytes.Equal(actual,expected){t.Fatal("Save with exact mtime altered unrelated file bytes")}
}
