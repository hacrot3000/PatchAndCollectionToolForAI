package server

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/restrictedpolicy"
)

func restrictedFixture(t *testing.T)*Server{
 t.Helper()
 root:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(root,"src_secret.go"),[]byte("SENSITIVE_SOURCE_NEVER_RETURN\n"),0600);err!=nil{t.Fatal(err)}
 p:=restrictedpolicy.Default()
 return &Server{Workspace:root,Config:config.Config{Restricted:&p}}
}
func restrictedRequest(s *Server,method,url,body string)*httptest.ResponseRecorder{
 w:=httptest.NewRecorder()
 h:=s.restrictedGate(http.HandlerFunc(s.restrictedCommandAPI))
 h.ServeHTTP(w,httptest.NewRequest(method,url,strings.NewReader(body)))
 return w
}
func TestRestrictedCommandGatewayNoShellOrFullSource(t *testing.T){
 s:=restrictedFixture(t)
 for _,cmd:=range []string{"pwd","ls"}{
  data,_:=json.Marshal(map[string]string{"command":cmd})
  w:=restrictedRequest(s,"POST","/api/restricted/command",string(data))
  if w.Code!=http.StatusOK{t.Fatalf("%s => %d %s",cmd,w.Code,w.Body)}
  if strings.Contains(w.Body.String(),"SENSITIVE_SOURCE_NEVER_RETURN"){t.Fatalf("%s leaked file contents",cmd)}
 }
 for _,cmd:=range []string{
  "cat src_secret.go","git show HEAD:src_secret.go","git diff",
  "git -c alias.r=!sh status","git archive HEAD","git push --force",
  "ls ../../","ls | sh","git status; cat src_secret.go",
 }{
  data,_:=json.Marshal(map[string]string{"command":cmd})
  w:=restrictedRequest(s,"POST","/api/restricted/command",string(data))
  if w.Code!=http.StatusForbidden{t.Errorf("%q status=%d body=%s",cmd,w.Code,w.Body.String())}
 }
}
func TestRestrictedCommandGatewayRejectsMethodsAndMalformedJSON(t *testing.T){
 s:=restrictedFixture(t)
 for _,item:=range []struct{method,body string;want int}{
  {"GET","",http.StatusMethodNotAllowed},
  {"POST","",http.StatusBadRequest},
  {"POST",`{"command":"git status","extra":"unsafe"}`,http.StatusBadRequest},
  {"POST",`{"command":"pwd"}{"command":"ls"}`,http.StatusBadRequest},
  {"POST",strings.Repeat("X",8192),http.StatusBadRequest},
 }{
  w:=restrictedRequest(s,item.method,"/api/restricted/command",item.body)
  if w.Code!=item.want{t.Errorf("%s %q => %d want %d",item.method,item.body[:min(len(item.body),120)],w.Code,item.want)}
 }
 w:=restrictedRequest(s,"POST","/api/project/file",`{"path":"src_secret.go"}`)
 if w.Code!=http.StatusForbidden{t.Fatalf("legacy file API reopened: %d",w.Code)}
}
func TestRestrictedCommandGatewayNotAvailableWhenPolicyDisabled(t *testing.T){
 s:=restrictedFixture(t)
 s.Config.Restricted=nil
 w:=httptest.NewRecorder()
 s.restrictedCommandAPI(w,httptest.NewRequest("POST","/api/restricted/command",bytes.NewBufferString(`{"command":"pwd"}`)))
 if w.Code!=http.StatusNotFound{t.Fatalf("unrestricted endpoint unintentionally active: %d",w.Code)}
}
