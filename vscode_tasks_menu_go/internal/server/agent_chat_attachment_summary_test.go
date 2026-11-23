package server

import (
 "bytes"
 "encoding/base64"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat/contextcompact"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

func TestAgentAttachmentSummaryProjectLargeNimAndNonCodeMetadata(t *testing.T){
 s:=newAIServerForTest(t)
 if err:=os.MkdirAll(s.Workspace,0700);err!=nil{t.Fatal(err)}
 var src strings.Builder
 src.WriteString("import std/[times, os]\n")
 for i:=0;i<800;i++{
  src.WriteString("proc handler*(n: int): int =\n  result = n + 100\n")
 }
 if src.Len()<=32<<10{t.Fatal("fixture below legacy limit")}
 path:=filepath.Join(s.Workspace,"big.nim")
 if err:=os.WriteFile(path,[]byte(src.String()),0600);err!=nil{t.Fatal(err)}
 req:=httptest.NewRequest(http.MethodGet,"/api/project/attachment-summary?path=big.nim",nil)
 rec:=httptest.NewRecorder();s.agentAttachmentSummaryAPI(rec,req)
 if rec.Code!=200{t.Fatalf("large Nim attach: %d %s",rec.Code,rec.Body.String())}
 var result contextcompact.AttachmentSummary
 if err:=json.Unmarshal(rec.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if result.Kind!="source_outline"||!strings.Contains(result.Text,"proc handler*")||
  strings.Contains(result.Text,"result = n + 100"){
  t.Fatalf("source not compacted: %+v",result)
 }
 if len(result.Text)>contextcompact.AttachmentSummaryLimit{
  t.Fatalf("summary oversized: %d",len(result.Text))
 }
 binaryPath:=filepath.Join(s.Workspace,"image.bin")
 if err:=os.WriteFile(binaryPath,[]byte{0,1,2,3,4},0600);err!=nil{t.Fatal(err)}
 rec=httptest.NewRecorder()
 s.agentAttachmentSummaryAPI(rec,httptest.NewRequest(http.MethodGet,"/api/project/attachment-summary?path=image.bin",nil))
 if rec.Code!=200{t.Fatalf("binary metadata: %d %s",rec.Code,rec.Body.String())}
 if strings.Contains(rec.Body.String(),"AAECAwQ=")||!strings.Contains(rec.Body.String(),"metadata"){
  t.Fatalf("binary data was forwarded or metadata absent: %s",rec.Body.String())
 }
 permissions:=sharedRoutePermissions(httptest.NewRequest(http.MethodGet,"/api/project/attachment-summary?path=big.nim",nil))
 if len(permissions)!=1||permissions[0]!=identity.PermissionFilesRead{
  t.Fatalf("unexpected shared permission for attachment summary: %v",permissions)
 }
}

func TestAgentAttachmentSummaryLocalBrowserImageMetadataOnly(t *testing.T){
 s:=newAIServerForTest(t)
 if err:=os.MkdirAll(s.Workspace,0700);err!=nil{t.Fatal(err)}
 sample:=[]byte{0xff,0xd8,0xff,0xd9}
 raw,_:=json.Marshal(map[string]any{
  "name":"camera.jpg","size":100000,"mime":"image/jpeg",
  "sample_base64":base64.StdEncoding.EncodeToString(sample),
 })
 request:=httptest.NewRequest(http.MethodPost,"/api/project/attachment-summary",bytes.NewReader(raw))
 rec:=httptest.NewRecorder();s.agentAttachmentSummaryAPI(rec,request)
 if rec.Code!=200{t.Fatalf("local image metadata: %d %s",rec.Code,rec.Body.String())}
 var result contextcompact.AttachmentSummary
 if err:=json.Unmarshal(rec.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if result.Kind!="image_metadata"||result.OriginalSize!=100000||
    !strings.Contains(result.Text,"FILE_PATH: not available")||
    !strings.Contains(result.Text,"FILE_FORMAT: image/jpeg"){
  t.Fatalf("local metadata lacks requested facts: %+v",result)
 }
 bad:=httptest.NewRecorder()
 s.agentAttachmentSummaryAPI(bad,httptest.NewRequest(http.MethodGet,
   "/api/project/attachment-summary?path=../outside",nil))
 if bad.Code!=http.StatusBadRequest{t.Fatalf("path traversal accepted: %d",bad.Code)}
 oversize:=httptest.NewRecorder()
 s.agentAttachmentSummaryAPI(oversize,httptest.NewRequest(http.MethodPost,
   "/api/project/attachment-summary",strings.NewReader("{\"name\":\"a.py\",\"size\":4,\"source\":\"payload-too-large\"}")))
 if oversize.Code!=http.StatusBadRequest{t.Fatalf("inaccurate source length accepted: %d",oversize.Code)}
}
