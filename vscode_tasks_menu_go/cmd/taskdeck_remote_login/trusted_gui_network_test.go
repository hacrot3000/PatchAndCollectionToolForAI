package main

import (
 "bytes"
 "context"
 "io"
 "net/http"
 "net/url"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestTrustedGUIRealLoopbackLifecycleAndAuthenticatedHTTP(t *testing.T){
 ctx,cancel:=context.WithTimeout(context.Background(),8*time.Second)
 defer cancel()
 actions:=&fakeGUIActions{status:"## main\n M main.go\n",
  projectInfo:remotetransport.RemoteProjectInfo{
   ProjectName:"Demo Project",WorkspaceName:"demo-repo",HostOS:"linux",
  },
 }
 opened:=make(chan string,1)
 result:=make(chan error,1)
 var nativeOutput bytes.Buffer
 go func(){
  result<-runTrustedRemoteGUIWithIdentityAndOpener(ctx,actions,&nativeOutput,
   trustedGUITarget{DeviceID:strings.Repeat("a",64),Username:"alice"},
   func(target string){opened<-target})
 }()
 var target string
 select{
 case target=<-opened:
 case <-ctx.Done():t.Fatal("GUI loopback listener never started")
 }
 parsed,err:=url.Parse(target)
 if err!=nil{t.Fatal(err)}
 if parsed.Scheme!="http" || parsed.Hostname()!="127.0.0.1" ||
  parsed.Fragment!="" || parsed.Port()=="" {
  t.Fatalf("GUI exposed unsafe listener/secret in process argv: %#v",parsed)
 }
 // The capability is printed only by the trusted native process to
 // the user's TTY, not passed as xdg-open/open command-line argument.
 const marker="Mã kích hoạt GUI (chỉ nhập tại địa chỉ localhost ở trên): "
 printed:=nativeOutput.String()
 at:=strings.Index(printed,marker)
 if at<0 {t.Fatal("native CLI did not print the local activation secret")}
 token:=strings.TrimSpace(strings.SplitN(printed[at+len(marker):],"\n",2)[0])
 if len(token)!=64 || strings.Contains(target,token){
  t.Fatal("GUI secret missing or leaked into OS browser process argv")
 }
 base:=parsed.Scheme+"://"+parsed.Host
 client:=&http.Client{Timeout:3*time.Second}
 request:=func(method,path,origin,host,token string)(int,string){
  t.Helper()
  req,err:=http.NewRequestWithContext(ctx,method,base+path,nil)
  if err!=nil{t.Fatal(err)}
  if origin!=""{req.Header.Set("Origin",origin)}
  if token!=""{req.Header.Set("X-TaskDeck-Local-Capability",token)}
  if host!=""{req.Host=host}
  resp,err:=client.Do(req)
  if err!=nil{t.Fatal(err)}
  raw,err:=io.ReadAll(io.LimitReader(resp.Body,4096))
  _=resp.Body.Close()
  if err!=nil{t.Fatal(err)}
  return resp.StatusCode,string(raw)
 }
 if code,html:=request("GET","/","","","");code!=200 ||
  !strings.Contains(html,"TaskDeck") {
  t.Fatalf("trusted GUI not served through REAL loopback HTTP: %d",code)
 }
 if code,_:=request("POST","/api/ping",base,"evil.example:4444",token);code!=403 {
  t.Fatalf("DNS rebinding Host unexpectedly admitted: %d",code)
 }
 if code,_:=request("POST","/api/ping","https://portal.invalid","",token);code!=403 {
  t.Fatalf("compromised PHP Origin unexpectedly admitted: %d",code)
 }
 if code,_:=request("POST","/api/ping",base,"","wrong");code!=403 {
  t.Fatalf("forged client GUI token admitted: %d",code)
 }
 if code,body:=request("POST","/api/session",base,"",token);
  code!=200 || !strings.Contains(body,`"username":"alice"`) ||
  !strings.Contains(body,strings.Repeat("a",64)) ||
  !strings.Contains(body,`"remaining":4`){
  t.Fatalf("verified TaskDeck account/device not bound to local GUI metadata: %d %q",code,body)
 }
 if code,_:=request("POST","/api/ping",base,"",token);code!=200 {
  t.Fatalf("pinned client fixed E2EE action denied by local handler: %d",code)
 }
 if code,body:=request("POST","/api/git-status",base,"",token);
  code!=200 || !strings.Contains(body,"main.go") {
  t.Fatalf("read-only Git result missing: status=%d body=%q",code,body)
 }
 if code,body:=request("POST","/api/project-info",base,"",token);
  code!=200 || !strings.Contains(body,`"project_name":"Demo Project"`) ||
  !strings.Contains(body,`"workspace_name":"demo-repo"`) ||
  strings.Contains(body,`"/home/"`) {
  t.Fatalf("bounded project info not returned via real local HTTP: %d %q",code,body)
 }
 if actions.pingCount.Load()!=1 || actions.gitCount.Load()!=1 ||
  actions.projectCount.Load()!=1 {
  t.Fatal("local UI used an unexpected application dispatcher")
 }
 if code,_:=request("POST","/api/close",base,"",token);code!=200{
  t.Fatalf("user could not disconnect GUI: %d",code)
 }
 select{
 case err:=<-result:
  if err!=nil{t.Fatalf("native GUI server did not shut down cleanly: %v",err)}
 case <-ctx.Done():
  t.Fatal("local GUI server retained authority after user disconnect")
 }
}

func TestTrustedGUITargetRejectsUnverifiedNativeIdentity(t *testing.T){
 good:=trustedGUITarget{DeviceID:strings.Repeat("c",64),Username:"alice"}
 if !good.valid(){t.Fatal("valid pinned TaskDeck identity rejected")}
 for _,bad:=range []trustedGUITarget{
  {DeviceID:"supplied-by-php",Username:"alice"},
  {DeviceID:strings.Repeat("C",64),Username:"alice"},
  {DeviceID:strings.Repeat("c",64),Username:""},
  {DeviceID:strings.Repeat("c",64),Username:"alice\nheader"},
  {DeviceID:strings.Repeat("c",64),Username:" alice"},
  {DeviceID:strings.Repeat("c",64),Username:strings.Repeat("b",129)},
 }{
  if bad.valid(){t.Fatalf("unverified identity accepted: %+v",bad)}
 }
}
