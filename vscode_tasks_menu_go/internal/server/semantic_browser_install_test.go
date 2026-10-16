package server

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

func semanticInstallerTestAPI(s *Server,method,body string,principal *identity.Principal)*httptest.ResponseRecorder{
 request:=httptest.NewRequest(method,"/api/semantic-browser/setup/run",strings.NewReader(body))
 if principal!=nil{
  request=request.WithContext(context.WithValue(request.Context(),sharedPrincipalContextKey{},*principal))
 }
 result:=httptest.NewRecorder()
 s.semanticBrowserInstallAPI(result,request)
 return result
}
func TestSemanticInstallerRequiresPermissionsAndExplicitWorkerIsolation(t *testing.T){
 s:=&Server{Config:config.Config{SharedServerEnabled:true}}
 both:=&identity.Principal{Permissions:map[string]bool{
  identity.PermissionSettingsWrite:true,identity.PermissionBrowserUse:true,
 }}
 settingsOnly:=&identity.Principal{Permissions:map[string]bool{identity.PermissionSettingsWrite:true}}
 browserOnly:=&identity.Principal{Permissions:map[string]bool{identity.PermissionBrowserUse:true}}
 for _,tc:=range []struct{user *identity.Principal;expected int}{
  {nil,http.StatusUnauthorized},
  {settingsOnly,http.StatusForbidden},
  {browserOnly,http.StatusForbidden},
  {both,http.StatusOK},
 }{
  got:=semanticInstallerTestAPI(s,http.MethodGet,"",tc.user)
  if got.Code!=tc.expected{t.Fatalf("GET: status %d want %d",got.Code,tc.expected)}
 }
 for _,method:=range []string{http.MethodGet,http.MethodPost}{
  perms:=sharedRoutePermissions(httptest.NewRequest(method,"/api/semantic-browser/setup/run",nil))
  if len(perms)!=2||perms[0]!=identity.PermissionSettingsWrite||perms[1]!=identity.PermissionBrowserUse{
   t.Fatalf("%s installer permission route=%v",method,perms)
  }
 }
 got:=semanticInstallerTestAPI(s,http.MethodPost,
  `{"action":"start","allowed_hosts":"example.com","start_worker":true}`,both)
 if got.Code!=http.StatusBadRequest{t.Fatalf("worker launch without explicit isolation acknowledgement: %d",got.Code)}
 got=semanticInstallerTestAPI(s,http.MethodDelete,"",both)
 if got.Code!=http.StatusMethodNotAllowed{t.Fatalf("unsafe HTTP method: %d",got.Code)}
}
func TestSemanticInstallerCancelRetainsManualCommandAndStopsContext(t *testing.T){
 var controller semanticInstallController
 ctx,cancel:=context.WithCancel(context.Background())
 controller.cancel=cancel
 controller.state="running"
 controller.step("install Chromium","running","cd /private/worker && node node_modules/playwright/cli.js install chromium")
 controller.append("Downloading Chromium")
 controller.stop()
 if ctx.Err()!=context.Canceled{t.Fatal("cancellation did not stop installation context")}
 got:=controller.snapshot()
 if got["state"]!="manual"||got["stage"]!="cancelled"{
  t.Fatalf("cancel must switch to manual: %v",got)
 }
 if !strings.Contains(got["manual_command"].(string),"install chromium"){
  t.Fatal("manual continuation command was lost")
 }
 if logs:=got["logs"].([]string);len(logs)!=1{t.Fatalf("missing step progress log: %v",logs)}
}
func TestSemanticInstallerMissingConfigIsInitializedBeforeOtherSteps(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("worker requires WSL2 Unix sockets")}
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 t.Setenv("TASKDECK_BROWSER_WORKER_SOCKET","")
 t.Setenv("TASKDECK_BROWSER_WORKER_TOKEN","")
 // A canceled job must never execute git/npm/node, but still repairs the
 // user's original ENOENT by creating the managed config FIRST.
 s:=&Server{}
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 s.executeSemanticInstall(ctx,"example.com",false)
 saved,err:=readSemanticSettings()
 if err!=nil{t.Fatalf("missing worker.json after config phase: %v",err)}
 t.Cleanup(func(){_ =os.RemoveAll(filepath.Dir(saved.Socket))})
 if len(saved.Token)<48||saved.Socket==""{t.Fatal("incomplete automatically generated worker.json")}
}
func TestSemanticInstallOutputRedactsWorkerToken(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("worker requires WSL2 Unix sockets")}
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 path,err:=semanticSetupFile();if err!=nil{t.Fatal(err)}
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{t.Fatal(err)}
 secret:=strings.Repeat("s",64)
 if err:=writeSemanticSettings(semanticWorkerSettings{
  Socket:filepath.Join(t.TempDir(),"worker.sock"),Token:secret,
  AllowedHosts:[]string{"example.com"},
 });err!=nil{t.Fatal(err)}
 ctrl:=&semanticInstallController{}
 writer:=&semanticInstallOutput{controller:ctrl}
 if _,err:=writer.Write([]byte("progress token="+secret+"\n"));err!=nil{t.Fatal(err)}
 logs:=strings.Join(ctrl.snapshot()["logs"].([]string)," ")
 if strings.Contains(logs,secret)||!strings.Contains(logs,"[REDACTED]"){
  t.Fatalf("worker token exposed in installation output: %q",logs)
 }
}
func TestSemanticInstallerRejectsUnknownFieldsAndCancelIsSafe(t *testing.T){
 s:=&Server{}
 for _,body:=range []string{
  `{"action":"start","allowed_hosts":"example.com","command":"sh -c rm"}`,
  `{"action":"execute","allowed_hosts":"example.com"}`,
  `{}`,
 }{
  got:=semanticInstallerTestAPI(s,http.MethodPost,body,nil)
  if got.Code!=http.StatusBadRequest{t.Fatalf("invalid action accepted %s: %d",body,got.Code)}
 }
 got:=semanticInstallerTestAPI(s,http.MethodPost,`{"action":"cancel"}`,nil)
 if got.Code!=http.StatusOK{t.Fatalf("cancellation without active job: %d",got.Code)}
 var output map[string]any
 if err:=json.Unmarshal(got.Body.Bytes(),&output);err!=nil{t.Fatal(err)}
 if _,ok:=output["manual_command"];!ok{t.Fatalf("missing manual fallback field: %v",output)}
}
func TestSemanticAutoInstallStopsRunningCommandOnCancel(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("installer runs inside WSL2, not native Windows")}
 if _,err:=exec.LookPath("sh");err!=nil{t.Skip("sh unavailable")}
 controller:=&semanticInstallController{}
 ctx,cancel:=context.WithCancel(context.Background())
 controller.cancel=cancel
 result:=make(chan error,1)
 go func(){
  result<-controller.run(ctx,t.TempDir(),"sample slow step","sh -c 'sleep 30'",
   "sh",15*time.Second,"-c","sleep 30")
 }()
 time.Sleep(150*time.Millisecond)
 controller.stop()
 select {
 case err:=<-result:
  if err==nil{t.Fatal("cancelled command unexpectedly succeeded")}
 case <-time.After(4*time.Second):t.Fatal("cancel did not terminate active step promptly")
 }
}
