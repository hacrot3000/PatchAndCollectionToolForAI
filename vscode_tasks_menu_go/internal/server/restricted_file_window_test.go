package server

import (
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/restrictedpolicy"
 "bletonfc/vscode_tasks_menu/internal/config"
)

func protectedFixture(t *testing.T)*Server{
 t.Helper()
 t.Setenv("XDG_CONFIG_HOME",filepath.Join(t.TempDir(),"private-config"))
 root:=t.TempDir()
 source:=strings.Join([]string{"line01\n","line02\n","line03\n","line04\n","line05\n","line06\n","line07\n","line08\n","line09\n","line10\n"},"")
 if err:=os.WriteFile(filepath.Join(root,"main.go"),[]byte(source),0600);err!=nil{t.Fatal(err)}
 if err:=os.MkdirAll(filepath.Join(root,".git"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,".git","config"),[]byte("PRIVATE_GIT_CONFIG"),0600);err!=nil{t.Fatal(err)}
 p:=restrictedpolicy.Default()
 p.MaxVisibleLines=3;p.MaxCopyLines=2;p.MaxUniqueLinesPerFile=3
 p.MaxUniqueLinesPerDay=10;p.MaxRevealPercent=80
 return &Server{Workspace:root,Config:config.Config{Restricted:&p}}
}
func protectedCall(s *Server,body string)*httptest.ResponseRecorder{
 w:=httptest.NewRecorder()
 req:=httptest.NewRequest(http.MethodPost,"/api/restricted/file-window",strings.NewReader(body))
 s.restrictedGate(http.HandlerFunc(s.restrictedFileWindowAPI)).ServeHTTP(w,req)
 return w
}
func TestProtectedWindowsOnlyReturnChargedSlices(t *testing.T){
 s:=protectedFixture(t)
 one:=protectedCall(s,`{"path":"main.go","start":1,"count":2}`)
 if one.Code!=http.StatusOK{t.Fatalf("first window: %d %s",one.Code,one.Body.String())}
 var response struct{Content string `json:"content"`}
 if err:=json.Unmarshal(one.Body.Bytes(),&response);err!=nil{t.Fatal(err)}
 if response.Content!="line01\nline02\n"||strings.Contains(one.Body.String(),"line08"){
  t.Fatalf("window leaked unrelated file lines: %s",one.Body.String())
 }
 again:=protectedCall(s,`{"path":"main.go","start":1,"count":2}`)
 if again.Code!=http.StatusOK{t.Fatalf("repeat read must not burn quota: %d",again.Code)}
 three:=protectedCall(s,`{"path":"main.go","start":3,"count":1}`)
 if three.Code!=http.StatusOK{t.Fatalf("third distinct line: %d %s",three.Code,three.Body)}
 denied:=protectedCall(s,`{"path":"main.go","start":4,"count":1}`)
 if denied.Code!=http.StatusTooManyRequests{t.Fatalf("exhausted quota: %d %s",denied.Code,denied.Body)}
 // Restarting the service must not allow replaying the unseen fourth line.
 newServer:=&Server{Workspace:s.Workspace,Config:s.Config}
 afterRestart:=protectedCall(newServer,`{"path":"main.go","start":4,"count":1}`)
 if afterRestart.Code!=http.StatusTooManyRequests{t.Fatalf("restart reset view quota: %d",afterRestart.Code)}
}
func TestProtectedWindowsRejectHiddenAndLegacyPaths(t *testing.T){
 s:=protectedFixture(t)
 for _,body:=range []string{
  `{"path":".git/config","start":1,"count":1}`,
  `{"path":"../private.go","start":1,"count":1}`,
  `{"path":"main.go","start":1,"count":999}`,
  `{"path":"main.go","start":0,"count":1}`,
  `{"path":"main.go","start":1,"count":1,"extra":true}`,
 }{
  r:=protectedCall(s,body)
  if r.Code==http.StatusOK{t.Fatalf("unsafe source request accepted: %s",body)}
 }
 rr:=httptest.NewRecorder()
 gate:=s.restrictedGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){t.Fatal("legacy API reached")}))
 gate.ServeHTTP(rr,httptest.NewRequest(http.MethodGet,"/api/project/file?path=main.go",nil))
 if rr.Code!=http.StatusForbidden{t.Fatalf("legacy unrestricted editor reopened: %d",rr.Code)}
}
func TestProtectedWindowsAreNotAnonymousInSharedMode(t *testing.T){
 s:=protectedFixture(t)
 s.Config.SharedServerEnabled=true
 r:=protectedCall(s,`{"path":"main.go","start":1,"count":1}`)
 if r.Code!=http.StatusUnauthorized{t.Fatalf("anonymous shared file access: %d",r.Code)}
}


func TestProtectedWindowDuplicateTextAtSeparateLinesStillConsumesQuota(t *testing.T){
 s:=protectedFixture(t)
 // Replace the fixture source with ten identical physical lines.
 if err:=os.WriteFile(filepath.Join(s.Workspace,"main.go"),
  []byte(strings.Repeat("duplicate\n",10)),0600);err!=nil{t.Fatal(err)}
 for i:=1;i<=3;i++ {
  payload:=fmt.Sprintf(`{"path":"main.go","start":%d,"count":1}`,i)
  r:=protectedCall(s,payload)
  if r.Code!=http.StatusOK {
   t.Fatalf("line %d unexpectedly rejected: %d %s",i,r.Code,r.Body)
  }
 }
 fourth:=protectedCall(s,`{"path":"main.go","start":4,"count":1}`)
 if fourth.Code!=http.StatusTooManyRequests{
  t.Fatalf("duplicated source content evaded per-file quota: %d %s",fourth.Code,fourth.Body)
 }
 // A previously viewed position remains available without charging again.
 again:=protectedCall(s,`{"path":"main.go","start":2,"count":1}`)
 if again.Code!=http.StatusOK{t.Fatalf("same line reread rejected: %d %s",again.Code,again.Body)}
}


func TestProtectedWindowCannotReadHiddenSourceViaSymlinkAlias(t *testing.T){
 s:=protectedFixture(t)
 alias:=filepath.Join(s.Workspace,"innocent.go")
 if err:=os.Symlink(filepath.Join(".git","config"),alias);err!=nil{t.Fatal(err)}
 result:=protectedCall(s,`{"path":"innocent.go","start":1,"count":1}`)
 if result.Code!=http.StatusForbidden{
  t.Fatalf("hidden .git/config accessed using an allowed-looking symlink: %d %s",result.Code,result.Body)
 }
 if strings.Contains(result.Body.String(),"PRIVATE_GIT_CONFIG"){t.Fatal("hidden source returned")}
}
