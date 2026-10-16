package server

import (
 "context"
 "encoding/json"
 "net"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

func setupTestRequest(s *Server,method,body string,principal *identity.Principal)*httptest.ResponseRecorder{
 req:=httptest.NewRequest(method,"/api/semantic-browser/setup",strings.NewReader(body))
 if principal!=nil {
  req=req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},*principal))
 }
 out:=httptest.NewRecorder()
 s.semanticBrowserSetupAPI(out,req)
 return out
}
func TestSemanticBrowserSetupGeneratesPrivateSecretWithoutEcho(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("Unix socket worker is configured inside WSL")}
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 t.Setenv("TASKDECK_BROWSER_WORKER_SOCKET","")
 t.Setenv("TASKDECK_BROWSER_WORKER_TOKEN","")
 s:=&Server{}
 result:=setupTestRequest(s,"POST",`{"allowed_hosts":"example.com, *.example.org"}`,nil)
 if result.Code!=200{t.Fatalf("save status=%d body=%s",result.Code,result.Body)}
 var public map[string]any
 if err:=json.Unmarshal(result.Body.Bytes(),&public);err!=nil{t.Fatal(err)}
 if public["source"]!="managed"||public["configured"]!=true {t.Fatalf("unexpected setup state: %v",public)}
 saved,err:=readSemanticSettings();if err!=nil{t.Fatal(err)}
 if len(saved.Token)<48||saved.Socket==""||len(saved.AllowedHosts)!=2{t.Fatal("missing managed values")}
 if strings.Contains(result.Body.String(),saved.Token){t.Fatal("secret echoed to browser")}
 path,err:=semanticSetupFile();if err!=nil{t.Fatal(err)}
 file,err:=os.Stat(path);if err!=nil{t.Fatal(err)}
 if file.Mode().Perm()!=0600{t.Fatalf("config permissions=%o",file.Mode().Perm())}
 parent,err:=os.Stat(filepath.Dir(path));if err!=nil{t.Fatal(err)}
 if parent.Mode().Perm()&0077!=0{t.Fatalf("config dir permissions=%o",parent.Mode().Perm())}
 sock,token:=semanticWorkerConnection()
 if sock!=saved.Socket||token!=saved.Token{t.Fatal("Go worker transport did not use managed settings")}
 listener,err:=net.Listen("unix",sock);if err!=nil{t.Fatal(err)}
 defer listener.Close()
 if err:=os.Chmod(sock,0600);err!=nil{t.Fatal(err)}
 status:=setupTestRequest(s,"GET","",nil)
 if status.Code!=200||!strings.Contains(status.Body.String(),`"worker_connected":true`){t.Fatalf("worker socket was not detected: %s",status.Body)}
 if strings.Contains(status.Body.String(),saved.Token){t.Fatal("secret leaked in diagnostics")}
}
func TestSemanticBrowserSetupRejectsUnsafeHostnamesAndMethods(t *testing.T){
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 s:=&Server{}
 for _,host:=range []string{"localhost","127.0.0.1","example.com:8443","https://example.com","*.local","*.","a..example.com","*.localhost","public.com,internal.local",""}{
  result:=setupTestRequest(s,"POST",`{"allowed_hosts":`+string(mustJSONHost(t,host))+`}`,nil)
  if result.Code==200{t.Fatalf("unsafe host %q accepted",host)}
 }
 r:=setupTestRequest(s,"DELETE","",nil)
 if r.Code!=http.StatusMethodNotAllowed{t.Fatalf("unexpected method %d",r.Code)}
}
func mustJSONHost(t *testing.T,host string)[]byte{t.Helper();raw,err:=json.Marshal(host);if err!=nil{t.Fatal(err)};return raw}
func TestSemanticBrowserSetupRequiresBothSharedPermissions(t *testing.T){
 s:=&Server{Config:config.Config{SharedServerEnabled:true}}
 good:=&identity.Principal{Permissions:map[string]bool{identity.PermissionSettingsWrite:true,identity.PermissionBrowserUse:true}}
 writer:=&identity.Principal{Permissions:map[string]bool{identity.PermissionSettingsWrite:true}}
 browser:=&identity.Principal{Permissions:map[string]bool{identity.PermissionBrowserUse:true}}
 for _,tc:=range []struct{name string;principal *identity.Principal;code int}{
  {"anonymous",nil,401},{"browser-only",browser,403},{"settings-only",writer,403},{"both",good,200},
 }{
  t.Run(tc.name,func(t *testing.T){
   out:=setupTestRequest(s,"GET","",tc.principal)
   if out.Code!=tc.code{t.Fatalf("status=%d want=%d",out.Code,tc.code)}
  })
 }
 for _,method:=range []string{http.MethodGet,http.MethodPost}{
  route:=httptest.NewRequest(method,"/api/semantic-browser/setup",nil)
  got:=sharedRoutePermissions(route)
  if len(got)!=2||got[0]!=identity.PermissionSettingsWrite||got[1]!=identity.PermissionBrowserUse{
   t.Fatalf("route grants (%s) = %v",method,got)
  }
 }
}
func TestSemanticBrowserManagedSettingsRejectSymlink(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("private permissions are POSIX")}
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 path,err:=semanticSetupFile();if err!=nil{t.Fatal(err)}
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{t.Fatal(err)}
 target:=filepath.Join(t.TempDir(),"unsafe.json")
 if err:=os.WriteFile(target,[]byte("{}"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(target,path);err!=nil{t.Fatal(err)}
 _,err=readSemanticSettings()
 if err==nil{t.Fatal("config symlink was accepted")}
 err=writeSemanticSettings(semanticWorkerSettings{Socket:filepath.Join(filepath.Dir(path),"browser.sock"),Token:strings.Repeat("a",64),AllowedHosts:[]string{"example.com"}})
 if err==nil{t.Fatal("symlink replacement was accepted")}
}
