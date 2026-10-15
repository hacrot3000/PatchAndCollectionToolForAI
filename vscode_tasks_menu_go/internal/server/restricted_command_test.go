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

func restrictedRequest(t *testing.T,root,addr string) (*Server,http.Handler) {
 t.Helper()
 p:=restrictedpolicy.Default()
 s:=&Server{Workspace:root,Config:config.Config{Restricted:&p}}
 return s,s.restrictedGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  t.Fatalf("legacy HTTP route unexpectedly used: %s",r.URL.Path)
 }))
}
func callRestrictedCommand(t *testing.T,h http.Handler,remote string,payload any) *httptest.ResponseRecorder {
 t.Helper()
 body,err:=json.Marshal(payload)
 if err!=nil{t.Fatal(err)}
 req:=httptest.NewRequest(http.MethodPost,"/api/restricted/command",bytes.NewReader(body))
 req.RemoteAddr=remote;req.Header.Set("Content-Type","application/json")
 rr:=httptest.NewRecorder();h.ServeHTTP(rr,req)
 return rr
}
func TestRestrictedCommandIsMetadataOnly(t *testing.T){
 root:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(root,"secret.go"),[]byte("hidden source body"),0600);err!=nil{t.Fatal(err)}
 _,handler:=restrictedRequest(t,root,"")
 response:=callRestrictedCommand(t,handler,"127.0.0.1:4567",map[string]any{"command":"ls","args":[]string{}})
 if response.Code!=http.StatusOK||!strings.Contains(response.Body.String(),"secret.go"){
  t.Fatalf("ls: status=%d body=%s",response.Code,response.Body.String())
 }
 if strings.Contains(response.Body.String(),"hidden source body"){t.Fatal("leaked source content")}
 for _,args:=range [][]string{{"show","HEAD:secret.go"},{"diff"},{"push","origin","HEAD"}}{
  response=callRestrictedCommand(t,handler,"127.0.0.1:4567",map[string]any{"command":"git","args":args})
  if response.Code!=http.StatusForbidden {t.Fatalf("git %v was allowed: %d",args,response.Code)}
 }
}
func TestRestrictedCommandRejectsRemoteAnonymousAndLegacyRoutes(t *testing.T){
 root:=t.TempDir();_,handler:=restrictedRequest(t,root,"")
 rr:=callRestrictedCommand(t,handler,"203.0.113.41:8888",map[string]any{"command":"ls"})
 if rr.Code!=http.StatusForbidden {t.Fatalf("remote anonymous metadata access permitted: %d",rr.Code)}
 for _,path:=range []string{"/api/git/status","/api/project/file","/api/sessions/abc/ws"}{
  request:=httptest.NewRequest(http.MethodGet,path,nil)
  request.RemoteAddr="127.0.0.1:4567"
  rr=httptest.NewRecorder();handler.ServeHTTP(rr,request)
  if rr.Code!=http.StatusForbidden {t.Fatalf("legacy path %s permitted: %d",path,rr.Code)}
 }
}
func TestRestrictedCommandRejectsMalformedInput(t *testing.T) {
 _,handler:=restrictedRequest(t,t.TempDir(),"")
 for _,payload:=range []string{
  `{"command":"ls","unknown":true}`,
  `{"command":"ls"} {"command":"ls"}`,
  `{"command":"pwd","args":["cat secret"]}`,
 } {
  req:=httptest.NewRequest(http.MethodPost,"/api/restricted/command",strings.NewReader(payload))
  req.RemoteAddr="127.0.0.1:3213";req.Header.Set("Content-Type","application/json")
  rr:=httptest.NewRecorder();handler.ServeHTTP(rr,req)
  if rr.Code==http.StatusOK {t.Fatalf("accepted request %s",payload)}
 }
}

func TestSharedGatewayFailsClosedWithoutProjectRootConfinement(t *testing.T) {
 root:=t.TempDir()
 srv,handler:=restrictedRequest(t,root,"")
 srv.Config.SharedServerEnabled=true
 response:=callRestrictedCommand(t,handler,"127.0.0.1:4567",map[string]any{"command":"ls"})
 if response.Code!=http.StatusForbidden {
  t.Fatalf("shared user gained workspace-wide listing: %d",response.Code)
 }
}
