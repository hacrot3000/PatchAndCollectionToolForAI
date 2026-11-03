package main

import (
 "context"
 "errors"
 "net/http"
 "net/http/httptest"
 "strings"
 "sync"
 "sync/atomic"
 "testing"
 "time"
)

const uiTestOrigin="http://127.0.0.1:49237"
const uiTestToken="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type fakeGUIActions struct{
 pingCount atomic.Int32
 gitCount atomic.Int32
 status string
 pingErr error
 gitErr error
 pingStart chan struct{}
 pingRelease chan struct{}
 startOnce sync.Once
}
func(f *fakeGUIActions)Ping(ctx context.Context)error{
 f.pingCount.Add(1)
 if f.pingStart!=nil {
  f.startOnce.Do(func(){close(f.pingStart)})
  select {
  case <-f.pingRelease:
  case <-ctx.Done():return ctx.Err()
  }
 }
 return f.pingErr
}
func(f *fakeGUIActions)GitStatus(ctx context.Context)(string,error){
 f.gitCount.Add(1)
 return f.status,f.gitErr
}
func newUITest(t *testing.T,actions *fakeGUIActions)(*trustedGUI,*atomic.Int32){
 t.Helper()
 closed:=new(atomic.Int32)
 ui,err:=newTrustedGUI(uiTestOrigin,uiTestToken,actions,func(){closed.Add(1)})
 if err!=nil{t.Fatal(err)}
 return ui,closed
}
func localRequest(method,path,origin,token string)*http.Request{
 // httptest.NewRequest with an absolute URL sets URL.Host, unlike the
 // origin-form requests the real loopback HTTP server receives.
 req:=httptest.NewRequest(method,path,nil)
 req.Host="127.0.0.1:49237"
 if origin!=""{req.Header.Set("Origin",origin)}
 if token!=""{req.Header.Set("X-TaskDeck-Local-Capability",token)}
 return req
}
func TestTrustedGUIRejectsUntrustedHostOriginAndCapability(t *testing.T){
 actions:=&fakeGUIActions{status:"## main\n"}
 ui,_:=newUITest(t,actions)
 attempts:=[]struct{name string;request *http.Request}{
  {"missing local capability",localRequest("POST","/api/ping",uiTestOrigin,"")},
  {"wrong local capability",localRequest("POST","/api/ping",uiTestOrigin,strings.Repeat("a",64))},
  {"malicious Origin",localRequest("POST","/api/git-status","https://portal.attacker.invalid",uiTestToken)},
  {"missing Origin",localRequest("POST","/api/ping","",uiTestToken)},
  {"GET cannot execute",localRequest("GET","/api/ping",uiTestOrigin,uiTestToken)},
  {"query cannot select action",localRequest("POST","/api/ping?command=sh",uiTestOrigin,uiTestToken)},
 }
 badHost:=localRequest("POST","/api/ping",uiTestOrigin,uiTestToken)
 badHost.Host="attacker.example:49237"
 attempts=append(attempts,struct{name string;request *http.Request}{"DNS rebinding host",badHost})
 badFetch:=localRequest("POST","/api/ping",uiTestOrigin,uiTestToken)
 badFetch.Header.Set("Sec-Fetch-Site","cross-site")
 attempts=append(attempts,struct{name string;request *http.Request}{"cross-site fetch metadata",badFetch})
 badContent:=localRequest("POST","/api/ping",uiTestOrigin,uiTestToken)
 badContent.Header.Set("Content-Type","application/json")
 attempts=append(attempts,struct{name string;request *http.Request}{"JSON mode forbidden",badContent})
 for _,tc:=range attempts{
  t.Run(tc.name,func(t *testing.T){
   rr:=httptest.NewRecorder()
   ui.ServeHTTP(rr,tc.request)
   if rr.Code==200{t.Fatalf("unauthorized action accepted: %s",tc.name)}
  })
 }
 if actions.pingCount.Load()!=0 || actions.gitCount.Load()!=0 {
  t.Fatal("untrusted Portal/web origin triggered a TaskDeck operation")
 }
}

func TestTrustedGUIAllowsOnlyFourFixedE2EEActions(t *testing.T){
 actions:=&fakeGUIActions{status:"## main\n M src/main.go\n"}
 ui,closed:=newUITest(t,actions)
 for _,path:=range []string{"/api/ping","/api/git-status","/api/git-status","/api/ping"}{
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("POST",path,uiTestOrigin,uiTestToken))
  if rr.Code!=200{t.Fatalf("%s failed: %d %s",path,rr.Code,rr.Body.String())}
  if strings.Contains(rr.Body.String(),"<script") {t.Fatal("unsafe response content")}
 }
 extra:=httptest.NewRecorder()
 ui.ServeHTTP(extra,localRequest("POST","/api/git-status",uiTestOrigin,uiTestToken))
 if extra.Code!=http.StatusGone {t.Fatalf("fifth operation should be 410, got %d",extra.Code)}
 if actions.pingCount.Load()!=2 || actions.gitCount.Load()!=2{t.Fatal("unexpected extra Git or TaskDeck action")}
 rr:=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/close",uiTestOrigin,uiTestToken))
 if rr.Code!=200 || closed.Load()!=1{t.Fatal("close did not end local GUI session")}
 again:=httptest.NewRecorder()
 ui.ServeHTTP(again,localRequest("POST","/api/close",uiTestOrigin,uiTestToken))
 if closed.Load()!=1{t.Fatal("close callback executed twice")}
}

func TestTrustedGUIActiveRequestCannotRaceAnotherAction(t *testing.T){
 actions:=&fakeGUIActions{pingStart:make(chan struct{}),pingRelease:make(chan struct{})}
 ui,_:=newUITest(t,actions)
 done:=make(chan struct{})
 go func(){
  defer close(done)
  ui.ServeHTTP(httptest.NewRecorder(),localRequest("POST","/api/ping",uiTestOrigin,uiTestToken))
 }()
 select{
 case <-actions.pingStart:
 case <-time.After(2*time.Second):t.Fatal("ping not admitted")
 }
 rr:=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/git-status",uiTestOrigin,uiTestToken))
 if rr.Code!=http.StatusConflict || actions.gitCount.Load()!=0{
  t.Fatal("concurrent Git action passed the single-flight E2EE gate")
 }
 close(actions.pingRelease)
 select{
 case <-done:
 case <-time.After(2*time.Second):t.Fatal("active UI request did not finish")
 }
}

func TestTrustedGUIPoisonedSessionFailClosed(t *testing.T){
 actions:=&fakeGUIActions{gitErr:errors.New("revoked")}
 ui,stopped:=newUITest(t,actions)
 rr:=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/git-status",uiTestOrigin,uiTestToken))
 if rr.Code!=http.StatusForbidden{t.Fatalf("revoked Git should deny, got %d",rr.Code)}
 rr=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/ping",uiTestOrigin,uiTestToken))
 if rr.Code!=http.StatusGone || actions.pingCount.Load()!=0{
  t.Fatal("GUI continued after revoked E2EE session")
 }
 // A failed/revoked operation must NOT leave the native CLI's local
 // server stranded: Disconnect remains possible and closes the UI.
 rr=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/close",uiTestOrigin,uiTestToken))
 if rr.Code!=200 || stopped.Load()!=1 {
  t.Fatalf("revoked GUI cannot disconnect cleanly: status=%d stopped=%d",rr.Code,stopped.Load())
 }
}

func TestTrustedGUIAssetsAreLocalAndCSPIsRestrictive(t *testing.T){
 actions:=&fakeGUIActions{}
 ui,_:=newUITest(t,actions)
 for _,path:=range []string{"/","/app.js","/style.css"}{
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("GET",path,"",""))
  if rr.Code!=200 || rr.Header().Get("Cache-Control")!="no-store, max-age=0" ||
   !strings.Contains(rr.Header().Get("Content-Security-Policy"),"default-src 'none'") ||
   !strings.Contains(rr.Header().Get("Content-Security-Policy"),"connect-src 'self'") ||
   rr.Header().Get("X-Frame-Options")!="DENY"{
   t.Fatalf("static local GUI security headers incorrect for %s",path)
  }
 }
 if strings.Contains(trustedGUIJavaScript,"innerHTML") ||
  strings.Contains(trustedGUIJavaScript,"localStorage") ||
  strings.Contains(trustedGUIJavaScript,"https://") ||
  !strings.Contains(trustedGUIJavaScript,"history.replaceState") ||
  !strings.Contains(trustedGUIJavaScript,"textContent") {
  t.Fatal("native GUI loads untrusted code or renders unescaped data")
 }
 for _,unsafeOrigin:=range []string{
  "http://attacker.test:49237","http://0.0.0.0:49237",
  "http://127.0.0.1:49237/path","https://127.0.0.1:49237",
 }{
  if _,err:=newTrustedGUI(unsafeOrigin,uiTestToken,actions,func(){});err==nil{
   t.Fatalf("untrusted native GUI origin accepted: %s",unsafeOrigin)
  }
 }
}
