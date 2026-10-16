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
 t.Cleanup(func(){_ =os.RemoveAll(filepath.Dir(saved.Socket))})
 if len([]byte(saved.Socket))>=100{t.Fatalf("Unix socket path too long: %q",saved.Socket)}
 runtimeDir,err:=os.Lstat(filepath.Dir(saved.Socket))
 if err!=nil||!runtimeDir.IsDir()||runtimeDir.Mode().Perm()!=0700{
  t.Fatalf("runtime socket directory is not private: %v, %v",runtimeDir,err)
 }
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
 server:=&http.Server{Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("X-TaskDeck-Worker-Token")!=saved.Token{
   http.Error(w,"unauthorized",http.StatusUnauthorized);return
  }
  if r.Method!=http.MethodGet||r.URL.Path!="/health"{http.NotFound(w,r);return}
  w.Header().Set("Content-Type","application/json")
  _,_=w.Write([]byte(`{"ok":true}`))
 })}
 go func(){_ =server.Serve(listener)}()
 defer server.Close()
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

func TestSemanticBrowserSetupLegacyEnvironmentOverrideIsNotSilent(t *testing.T){
 t.Setenv("TASKDECK_BROWSER_WORKER_SOCKET","/tmp/legacy-worker.sock")
 t.Setenv("TASKDECK_BROWSER_WORKER_TOKEN",strings.Repeat("z",32))
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 s:=&Server{}
 response:=setupTestRequest(s,"POST",`{"allowed_hosts":"example.com"}`,nil)
 if response.Code!=http.StatusConflict{t.Fatalf("environment override accepted: %d",response.Code)}
}

func TestSemanticBrowserSocketAvoidsLongRuntimeDirectories(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("Unix socket is configured inside WSL")}
 base:=t.TempDir()
 longTemp:=filepath.Join(base,strings.Repeat("nested",18))
 if err:=os.MkdirAll(longTemp,0700);err!=nil{t.Fatal(err)}
 t.Setenv("XDG_RUNTIME_DIR","")
 t.Setenv("TMPDIR",longTemp)
 socket,err:=semanticSetupSocket(filepath.Join(longTemp,"an","extremely","long","worker.json"))
 if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){_ =os.RemoveAll(filepath.Dir(socket))})
 if len([]byte(socket))>=100{t.Fatalf("socket path exceeds portable limit: %q",socket)}
 if strings.HasPrefix(socket,longTemp+string(os.PathSeparator)){
  t.Fatalf("socket path unexpectedly lives in long temp tree: %q",socket)
 }
 dir,err:=os.Lstat(filepath.Dir(socket))
 if err!=nil||!dir.IsDir()||dir.Mode().Perm()!=0700{
  t.Fatalf("socket directory is not private: %v, %v",dir,err)
 }
 listener,err:=net.Listen("unix",socket)
 if err!=nil{t.Fatalf("socket must be bindable: %v",err)}
 defer listener.Close()
}

func makeTestSemanticWorker(t *testing.T,dir string){
 t.Helper()
 if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,"package.json"),
  []byte(`{"name":"taskdeck-semantic-browser-worker"}`),0600);err!=nil{t.Fatal(err)}
 for _,file:=range []string{
  "worker.mjs","start-configured.mjs","semantic_extract.mjs",
  "universal_input.mjs","popup_runtime.mjs","auth_runtime.mjs",
 }{
  if err:=os.WriteFile(filepath.Join(dir,file),[]byte("// test"),0600);err!=nil{t.Fatal(err)}
 }
}

func TestSemanticWorkerAutoDetectionHandlesGlobalReleaseAndDevCheckout(t *testing.T){
 root:=t.TempDir()
 release:=filepath.Join(root,"global","releases","revision-123")
 installed:=filepath.Join(release,"browser-worker")
 makeTestSemanticWorker(t,installed)
 if got:=semanticWorkerDirFromRoots(release);got!=installed{
  t.Fatalf("global release worker: got %q want %q",got,installed)
 }
 checkout:=filepath.Join(root,"source","vscode_tasks_menu_go")
 sourceWorker:=filepath.Join(checkout,"browser-worker")
 makeTestSemanticWorker(t,sourceWorker)
 if got:=semanticWorkerDirFromRoots(filepath.Join(checkout,".build"));got!=sourceWorker{
  t.Fatalf("dev binary worker: got %q want %q",got,sourceWorker)
 }
 other:=filepath.Join(root,"source","projects","workspace")
 if err:=os.MkdirAll(other,0700);err!=nil{t.Fatal(err)}
 if got:=semanticWorkerDirFromRoots(other);got!=sourceWorker{
  t.Fatalf("workspace ancestor worker: got %q want %q",got,sourceWorker)
 }
}

func TestSemanticWorkerAutoDetectionRejectsIncompleteOrImpostorPackages(t *testing.T){
 root:=t.TempDir()
 worker:=filepath.Join(root,"browser-worker")
 if err:=os.MkdirAll(worker,0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(worker,"package.json"),
  []byte(`{"name":"unrelated-package"}`),0600);err!=nil{t.Fatal(err)}
 if semanticWorkerDirectoryValid(worker){t.Fatal("impostor package accepted")}
 makeTestSemanticWorker(t,worker)
 if err:=os.Remove(filepath.Join(worker,"start-configured.mjs"));err!=nil{t.Fatal(err)}
 if semanticWorkerDirectoryValid(worker){t.Fatal("incomplete worker accepted")}
 if got:=semanticWorkerDirFromRoots(root);got!=""{t.Fatalf("incomplete worker returned %q",got)}
}

func TestSemanticBrowserSetupCommandsNeverGuessWorkerPath(t *testing.T){
 root:=t.TempDir()
 realDir:=filepath.Join(root,"worker's files")
 makeTestSemanticWorker(t,realDir)
 steps:=semanticSetupInstructions("/private/config/worker.json",realDir,"linux")
 found:=false
 for _,step:=range steps{
  if strings.Contains(step["command"],"/path/to/"){t.Fatal("wizard contains placeholder worker path")}
  if step["label"]=="Open detected worker directory"{
   found=true
   if step["command"]!="cd "+shellQuote(realDir){
    t.Fatalf("detected path improperly escaped: %s",step["command"])
   }
  }
 }
 if !found{t.Fatal("no auto-detected directory command")}
 missing:=semanticSetupInstructions("/private/config/worker.json","","linux")
 clone:=false
 for _,step:=range missing{
  if strings.Contains(step["command"],"/path/to/"){t.Fatal("missing package emits guessed path")}
  if strings.Contains(step["command"],"git clone --depth 1"){clone=true}
 }
 if !clone{t.Fatal("missing worker must offer a real repository checkout command")}
}
