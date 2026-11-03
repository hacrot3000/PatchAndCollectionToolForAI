package main

import (
 "context"
 "io"
 "net/http"
 "net/url"
 "strings"
 "testing"
 "time"
)

func TestTrustedGUIRealLoopbackLifecycleAndAuthenticatedHTTP(t *testing.T){
 ctx,cancel:=context.WithTimeout(context.Background(),8*time.Second)
 defer cancel()
 actions:=&fakeGUIActions{status:"## main\n M main.go\n"}
 opened:=make(chan string,1)
 result:=make(chan error,1)
 go func(){
  result<-runTrustedRemoteGUIWithIdentityAndOpener(ctx,actions,io.Discard,
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
  len(parsed.Fragment)!=64 || parsed.Port()=="" {
  t.Fatalf("GUI exposed unsafe listener/URL: %#v",parsed)
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
 if code,_:=request("POST","/api/ping",base,"evil.example:4444",parsed.Fragment);code!=403 {
  t.Fatalf("DNS rebinding Host unexpectedly admitted: %d",code)
 }
 if code,_:=request("POST","/api/ping","https://portal.invalid","",parsed.Fragment);code!=403 {
  t.Fatalf("compromised PHP Origin unexpectedly admitted: %d",code)
 }
 if code,_:=request("POST","/api/ping",base,"","wrong");code!=403 {
  t.Fatalf("forged client GUI token admitted: %d",code)
 }
 if code,body:=request("POST","/api/session",base,"",parsed.Fragment);
  code!=200 || !strings.Contains(body,`"username":"alice"`) ||
  !strings.Contains(body,strings.Repeat("a",64)) ||
  !strings.Contains(body,`"remaining":4`){
  t.Fatalf("verified TaskDeck account/device not bound to local GUI metadata: %d %q",code,body)
 }
 if code,_:=request("POST","/api/ping",base,"",parsed.Fragment);code!=200 {
  t.Fatalf("pinned client fixed E2EE action denied by local handler: %d",code)
 }
 if code,body:=request("POST","/api/git-status",base,"",parsed.Fragment);
  code!=200 || !strings.Contains(body,"main.go") {
  t.Fatalf("read-only Git result missing: status=%d body=%q",code,body)
 }
 if actions.pingCount.Load()!=1 || actions.gitCount.Load()!=1 {
  t.Fatal("local UI used an unexpected application dispatcher")
 }
 if code,_:=request("POST","/api/close",base,"",parsed.Fragment);code!=200{
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
