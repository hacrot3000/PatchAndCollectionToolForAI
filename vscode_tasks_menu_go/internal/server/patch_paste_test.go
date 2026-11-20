package server

import (
 "archive/zip"
 "bytes"
 "encoding/json"
 "fmt"
 "io"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func postPatchPaste(t *testing.T, s *Server, kind, name, source string) *httptest.ResponseRecorder {
 t.Helper()
 body, err := json.Marshal(patchPasteRequest{Kind:kind, Name:name, Text:source})
 if err != nil { t.Fatal(err) }
 req := httptest.NewRequest(http.MethodPost,"/api/patch/paste",bytes.NewReader(body))
 req.Header.Set("Content-Type","application/json")
 res:=httptest.NewRecorder()
 s.patchPaste(res,req)
 return res
}

func openQueuedPaste(t *testing.T, root string, res *httptest.ResponseRecorder) map[string][]byte {
 t.Helper()
 if res.Code != http.StatusCreated { t.Fatalf("paste status=%d body=%s",res.Code,res.Body.String()) }
 var result patchUploadResult
 if err:=json.Unmarshal(res.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if !strings.HasPrefix(result.QueuePath,"patchs/") || !strings.HasSuffix(result.Name,".zip") {
  t.Fatalf("unsafe queue result %+v",result)
 }
 z,err:=zip.OpenReader(filepath.Join(root,filepath.FromSlash(result.QueuePath)))
 if err!=nil{t.Fatal(err)}
 defer z.Close()
 files:=map[string][]byte{}
 for _,f:=range z.File{
  reader,err:=f.Open();if err!=nil{t.Fatal(err)}
  raw,err:=io.ReadAll(reader);reader.Close()
  if err!=nil{t.Fatal(err)}
  files[f.Name]=raw
 }
 return files
}

func TestPatchPastePythonPackagesManifestBoundToActiveProject(t *testing.T) {
 root:=t.TempDir()
 s:=&Server{Workspace:root}
 identity,err:=patchtool.EnsureProjectIdentity(root)
 if err!=nil{t.Fatal(err)}
 python:=fmt.Sprintf("# TASKDECK_PROJECT_KEY: %s\n# TASKDECK_PROJECT_ID: %s\nprint('safe')\n",identity.Key,identity.ID)
 files:=openQueuedPaste(t,root,postPatchPaste(t,s,"patch-python","demo python",python))
 if string(files["entrypoint.py"])!=python {t.Fatalf("Python source mismatch: %q",files["entrypoint.py"])}
 var manifest struct {SchemaVersion int `json:"schema_version"`;Project patchtool.ProjectIdentity `json:"project"`;Patch struct{ID string `json:"id"`} `json:"patch"`}
 if err:=json.Unmarshal(files["PATCH_TOOL_MANIFEST.json"],&manifest);err!=nil{t.Fatal(err)}
 if manifest.SchemaVersion!=1||manifest.Project!=identity||!strings.HasPrefix(manifest.Patch.ID,"cleartext-demo-python-"){
  t.Fatalf("invalid manifest: %+v",manifest)
 }
}

func TestPatchPasteCollectLiteralBecomesReadOnlyRequestZIP(t *testing.T) {
 root:=t.TempDir()
 s:=&Server{Workspace:root}
 identity,err:=patchtool.EnsureProjectIdentity(root)
 if err!=nil{t.Fatal(err)}
 request:=fmt.Sprintf(`{"id":"collect-demo","project":{"key":%q,"id":%q},"actions":[{"type":"pack","paths":["src/example.py"]}]}`,identity.Key,identity.ID)
 text:="COLLECT_REQUEST_JSON = r'''"+request+"'''\nprint('THIS IS NEVER EXECUTED')"
 files:=openQueuedPaste(t,root,postPatchPaste(t,s,"collect","demo",text))
 if len(files)!=1{t.Fatalf("COLLECT ZIP should have only the request JSON: %v",files)}
 for name,content:=range files{
  if !strings.HasPrefix(name,"CODE_COLLECTION_REQUEST_")||!strings.HasSuffix(name,".json")||string(content)!=request{
   t.Fatalf("unexpected collection member %q (%q)",name,content)
  }
 }
}

func TestPatchPasteRejectsWrongIdentityAndMalformedInput(t *testing.T) {
 root:=t.TempDir()
 s:=&Server{Workspace:root}
 identity,err:=patchtool.EnsureProjectIdentity(root)
 if err!=nil{t.Fatal(err)}
 wrong:=fmt.Sprintf(`{"project":{"key":%q,"id":"11111111-1111-4111-8111-111111111111"},"actions":[{"type":"pack","paths":["a"]}]}`,identity.Key)
 cases:=[]struct{kind, source string}{
  {"collect",wrong},
  {"collect",`{"actions":[{"type":"pack","paths":["a"]}]}`},
  {"collect","print('arbitrary script')"},
  {"collect",`{"project":{"key":"x","id":"y"},"actions":[]}`},
  {"patch-python",fmt.Sprintf("# TASKDECK_PROJECT_KEY: wrong\n# TASKDECK_PROJECT_ID: %s\nprint('bad')",identity.ID)},
  {"patch-python",""},
  {"patch-python","print('missing identity')"},
  {"unknown","hello"},
 }
 for _,tt:=range cases{
  res:=postPatchPaste(t,s,tt.kind,"bad",tt.source)
  if res.Code!=http.StatusBadRequest{t.Errorf("kind=%s source=%q status=%d body=%s",tt.kind,tt.source,res.Code,res.Body.String())}
 }
 entries,err:=os.ReadDir(filepath.Join(root,"patchs"))
 if err!=nil&&!os.IsNotExist(err){t.Fatal(err)}
 if len(entries)!=0{t.Fatalf("invalid inputs unexpectedly reached queue: %+v",entries)}
}

func TestPatchPasteIsPostOnly(t *testing.T) {
 s:=&Server{Workspace:t.TempDir()}
 res:=httptest.NewRecorder()
 s.patchPaste(res,httptest.NewRequest(http.MethodGet,"/api/patch/paste",nil))
 if res.Code!=http.StatusMethodNotAllowed{t.Fatalf("status=%d",res.Code)}
}
