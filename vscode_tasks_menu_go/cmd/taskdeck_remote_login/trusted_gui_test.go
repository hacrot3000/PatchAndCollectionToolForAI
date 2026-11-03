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

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const uiTestOrigin="http://127.0.0.1:49237"
const uiTestToken="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type fakeGUIActions struct{
 pingCount atomic.Int32
 featuresCount atomic.Int32
 features remotetransport.RemoteFeatures
 featuresErr error
 gitCount atomic.Int32
 projectCount atomic.Int32
 tasksPreviewCount atomic.Int32
 tasksPreview remotetransport.RemoteTasksPreview
 tasksPreviewErr error
 projectInfo remotetransport.RemoteProjectInfo
 projectErr error
 closeCount atomic.Int32
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
func(f *fakeGUIActions)Features(ctx context.Context)(remotetransport.RemoteFeatures,error){
 f.featuresCount.Add(1)
 return f.features,f.featuresErr
}
func(f *fakeGUIActions)GitStatus(ctx context.Context)(string,error){
 f.gitCount.Add(1)
 return f.status,f.gitErr
}
func(f *fakeGUIActions)ProjectInfo(ctx context.Context)(remotetransport.RemoteProjectInfo,error){
 f.projectCount.Add(1)
 return f.projectInfo,f.projectErr
}
func(f *fakeGUIActions)TasksPreview(ctx context.Context)(remotetransport.RemoteTasksPreview,error){
 f.tasksPreviewCount.Add(1)
 return f.tasksPreview,f.tasksPreviewErr
}
func(f *fakeGUIActions)Close(){f.closeCount.Add(1)}
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
 // CORS preflight from a compromised PHP page must not gain a local
 // capability or an Access-Control-Allow-Origin escape hatch.
 preflight:=localRequest("OPTIONS","/api/ping","https://portal.attacker.invalid",uiTestToken)
 preflight.Header.Set("Access-Control-Request-Method","POST")
 preflight.Header.Set("Access-Control-Request-Headers","X-TaskDeck-Local-Capability")
 preflightRR:=httptest.NewRecorder()
 ui.ServeHTTP(preflightRR,preflight)
 if preflightRR.Code==200 || preflightRR.Header().Get("Access-Control-Allow-Origin")!=""{
  t.Fatal("compromised PHP successfully preflighted native client GUI")
 }
 // Even a legitimate local Origin may NEVER supply remote commands or
 // arbitrary Git paths in request bodies.
 command:=httptest.NewRequest("POST","/api/git-status",
  strings.NewReader(`{"command":"git pull","path":"../../other-project"}`))
 command.Host="127.0.0.1:49237"
 command.Header.Set("Origin",uiTestOrigin)
 command.Header.Set("X-TaskDeck-Local-Capability",uiTestToken)
 badBody:=httptest.NewRecorder()
 ui.ServeHTTP(badBody,command)
 if badBody.Code!=http.StatusBadRequest{
  t.Fatalf("native GUI accepted arbitrary Git command arguments: %d",badBody.Code)
 }
 if actions.pingCount.Load()!=0 || actions.gitCount.Load()!=0 {
  t.Fatal("untrusted Portal/web origin or payload triggered TaskDeck action")
 }
}

func TestTrustedGUIAllowsOnlyFourFixedE2EEActions(t *testing.T){
 actions:=&fakeGUIActions{
  status:"## main\n M src/main.go\n",
  features:remotetransport.RemoteFeatures{SessionPing:true,ProjectInfo:true,TasksPreview:true,GitStatus:true},
  projectInfo:remotetransport.RemoteProjectInfo{
   ProjectName:"Development",WorkspaceName:"test-workspace",HostOS:"linux",
  },
  tasksPreview:remotetransport.RemoteTasksPreview{
   Total:4,Labels:[]string{"Build","Unit test","Deploy"},
  },
 }
 ui,closed:=newUITest(t,actions)
 for _,path:=range []string{"/api/features","/api/project-info","/api/tasks-preview","/api/git-status"}{
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("POST",path,uiTestOrigin,uiTestToken))
  if rr.Code!=200{t.Fatalf("%s failed: %d %s",path,rr.Code,rr.Body.String())}
  if strings.Contains(rr.Body.String(),"<script") {t.Fatal("unsafe response content")}
 }
 extra:=httptest.NewRecorder()
 ui.ServeHTTP(extra,localRequest("POST","/api/git-status",uiTestOrigin,uiTestToken))
 if extra.Code!=http.StatusGone {t.Fatalf("fifth operation should be 410, got %d",extra.Code)}
 if actions.featuresCount.Load()!=1 || actions.pingCount.Load()!=0 ||
  actions.gitCount.Load()!=1 || actions.projectCount.Load()!=1 ||
  actions.tasksPreviewCount.Load()!=1 {t.Fatal("unexpected extra Git/project or TaskDeck action")}
 rr:=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/close",uiTestOrigin,uiTestToken))
 if rr.Code!=200 || closed.Load()!=1 || actions.closeCount.Load()!=1{
  t.Fatal("close did not revoke native E2EE session")
 }
 again:=httptest.NewRecorder()
 ui.ServeHTTP(again,localRequest("POST","/api/close",uiTestOrigin,uiTestToken))
 if closed.Load()!=1 || actions.closeCount.Load()!=1{
  t.Fatal("session/channel Close callback executed more than once")
 }
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
 if rr.Code!=http.StatusForbidden || actions.closeCount.Load()!=1 {
  t.Fatalf("revoked Git did not poison E2EE channel: http=%d closes=%d",rr.Code,actions.closeCount.Load())
 }
 rr=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/ping",uiTestOrigin,uiTestToken))
 if rr.Code!=http.StatusGone || actions.pingCount.Load()!=0{
  t.Fatal("GUI continued after revoked E2EE session")
 }
 // A failed/revoked operation must NOT leave the native CLI's local
 // server stranded: Disconnect remains possible and closes the UI.
 rr=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/close",uiTestOrigin,uiTestToken))
 if rr.Code!=200 || stopped.Load()!=1 || actions.closeCount.Load()!=1 {
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
  "http://127.0.0.1:0","http://127.0.0.1:70000",
  "http://127.0.0.1:abc",
 }{
  if _,err:=newTrustedGUI(unsafeOrigin,uiTestToken,actions,func(){});err==nil{
   t.Fatalf("untrusted native GUI origin accepted: %s",unsafeOrigin)
  }
 }
}

func TestTrustedGUICloseCancelsInFlightE2EE(t *testing.T){
 actions:=&fakeGUIActions{pingStart:make(chan struct{}),pingRelease:make(chan struct{})}
 ui,stopped:=newUITest(t,actions)
 done:=make(chan int,1)
 go func(){
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("POST","/api/ping",uiTestOrigin,uiTestToken))
  done<-rr.Code
 }()
 select{
 case <-actions.pingStart:
 case <-time.After(2*time.Second):t.Fatal("E2EE request never entered")
 }
 closed:=httptest.NewRecorder()
 ui.ServeHTTP(closed,localRequest("POST","/api/close",uiTestOrigin,uiTestToken))
 if closed.Code!=http.StatusOK || stopped.Load()!=1 || actions.closeCount.Load()!=1 {
  t.Fatal("local Disconnect failed to destroy native AEAD session immediately")
 }
 select{
 case result:=<-done:
  if result==http.StatusOK {t.Fatal("late E2EE result treated as success after logout")}
 case <-time.After(2*time.Second):t.Fatal("inflight E2EE request survived GUI disconnect")
 }
 if actions.closeCount.Load()!=1{t.Fatal("channel closed more than once")}
}

func TestTrustedGUISessionReadDoesNotSpendRemoteQuotaAndExpires(t *testing.T){
 actions:=&fakeGUIActions{status:"## main"}
 ui,_:=newUITest(t,actions)
 for i:=0;i<8;i++{
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("POST","/api/session",uiTestOrigin,uiTestToken))
  if rr.Code!=http.StatusOK ||
   !strings.Contains(rr.Body.String(),`"remaining":4`) ||
   !strings.Contains(rr.Body.String(),`"expires_in_seconds":`) {
   t.Fatalf("local-only session metadata incorrectly spent E2EE operations: %d %s",rr.Code,rr.Body.String())
  }
 }
 if actions.pingCount.Load()!=0 || actions.gitCount.Load()!=0 {
  t.Fatal("local session metadata reached Portal or TaskDeck")
 }
 // A stale session cannot be brought back with any valid local capability.
 ui.mu.Lock()
 ui.startedAt=time.Now().Add(-trustedGUIMaxLifetime-time.Second)
 ui.mu.Unlock()
 rr:=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/ping",uiTestOrigin,uiTestToken))
 if rr.Code!=http.StatusGone || actions.pingCount.Load()!=0 || actions.closeCount.Load()!=1{
  t.Fatal("expired GUI unexpectedly retained native authenticated operation authority")
 }
 rr=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/session",uiTestOrigin,uiTestToken))
 if rr.Code!=http.StatusGone{t.Fatal("local status revived expired session")}
}

func TestTrustedGUIStaticAssetsNeverEmbedBearerOrLocalIdentity(t *testing.T){
 actions:=&fakeGUIActions{}
 ui,_:=newUITest(t,actions)
 ui.target=trustedGUITarget{DeviceID:strings.Repeat("b",64),Username:"private-trusted-user"}
 for _,path:=range []string{"/","/app.js","/style.css"}{
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("GET",path,"",""))
  if rr.Code!=http.StatusOK || strings.Contains(rr.Body.String(),ui.token) ||
   strings.Contains(rr.Body.String(),ui.target.DeviceID) ||
   strings.Contains(rr.Body.String(),ui.target.Username) {
   t.Fatalf("native public GUI asset leaked private local session data on %s",path)
  }
 }
 rr:=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/session",uiTestOrigin,""))
 if rr.Code==http.StatusOK || strings.Contains(rr.Body.String(),ui.target.DeviceID) ||
  strings.Contains(rr.Body.String(),ui.target.Username){
  t.Fatal("local GUI identity leaked to unauthenticated PHP-driven request")
 }
}

func TestTrustedGUIFeaturesOnlyForLocalCapabilityAndFourRounds(t *testing.T){
 actions:=&fakeGUIActions{features:remotetransport.RemoteFeatures{
  SessionPing:true,ProjectInfo:true,TasksPreview:false,GitStatus:true,
 }}
 ui,_:=newUITest(t,actions)
 bad:=httptest.NewRecorder()
 ui.ServeHTTP(bad,localRequest("POST","/api/features",
  "https://portal.attacker.invalid",uiTestToken))
 if bad.Code!=http.StatusForbidden||actions.featuresCount.Load()!=0{
  t.Fatal("PHP Portal bypassed native GUI Origin gate for capabilities")
 }
 good:=httptest.NewRecorder()
 ui.ServeHTTP(good,localRequest("POST","/api/features",uiTestOrigin,uiTestToken))
 if good.Code!=http.StatusOK ||
  !strings.Contains(good.Body.String(),`"project_info":true`) ||
  !strings.Contains(good.Body.String(),`"tasks_preview":false`) ||
  !strings.Contains(good.Body.String(),`"git_status":true`) ||
  !strings.Contains(good.Body.String(),`"remaining":3`) ||
  !strings.Contains(good.Body.String(),`"remote_control_available":false`) ||
  actions.featuresCount.Load()!=1{
  t.Fatalf("native feature list was unverified or not charged to E2EE quota: %d %s",good.Code,good.Body.String())
 }
}
