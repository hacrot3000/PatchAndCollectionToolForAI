package main

import (
 "context"
 "errors"
 "fmt"
 "net"
 "net/http"
 "net/http/httptest"
 "strings"
 "sync"
 "sync/atomic"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remoterelay"
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
 tasksNextCount atomic.Int32
 inspectCount atomic.Int32
 runCount atomic.Int32
 runChoice int
 runText string
 runErr error
 longCount atomic.Int32
 longTask int
 longChoice int
 longText string
 longOperation string
 inspectedTask int
 inspectedValue remotetransport.RemoteTaskInspect
 inspectErr error
 tasksNext remotetransport.RemoteTasksPreview
 tasksNextErr error
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
func(f *fakeGUIActions)GitPull(ctx context.Context)(remotetransport.RemoteGitPullResult,error){
 f.gitCount.Add(1)
 return remotetransport.RemoteGitPullResult{
  Before:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  After:"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",Updated:true,
 },nil
}

func(f *fakeGUIActions)ProjectInfo(ctx context.Context)(remotetransport.RemoteProjectInfo,error){
 f.projectCount.Add(1)
 return f.projectInfo,f.projectErr
}
func(f *fakeGUIActions)TasksPreview(ctx context.Context)(remotetransport.RemoteTasksPreview,error){
 f.tasksPreviewCount.Add(1)
 return f.tasksPreview,f.tasksPreviewErr
}
func(f *fakeGUIActions)TasksPreviewNext(ctx context.Context)(remotetransport.RemoteTasksPreview,error){
 f.tasksNextCount.Add(1)
 return f.tasksNext,f.tasksNextErr
}
func(f *fakeGUIActions)StartLongTask(ctx context.Context,id,choice int,text string)(remotetransport.RemoteLongTaskProgress,error){
 f.longCount.Add(1)
 f.longTask=id
 f.longChoice=choice
 f.longText=text
 f.longOperation="start"
 return remotetransport.RemoteLongTaskProgress{TaskID:id,Status:"started",ExitCode:-1},nil
}
func(f *fakeGUIActions)PollLongTask(ctx context.Context,id int)(remotetransport.RemoteLongTaskProgress,error){
 f.longCount.Add(1);f.longOperation="poll";f.longTask=id
 return remotetransport.RemoteLongTaskProgress{TaskID:id,Status:"running",ExitCode:-1,Output:"chunk"},nil
}
func(f *fakeGUIActions)CancelLongTask(ctx context.Context,id int)(remotetransport.RemoteLongTaskProgress,error){
 f.longCount.Add(1);f.longOperation="cancel";f.longTask=id
 return remotetransport.RemoteLongTaskProgress{TaskID:id,Status:"cancel_requested",ExitCode:-1},nil
}
func(f *fakeGUIActions)ClearLongTask(ctx context.Context,id int)(remotetransport.RemoteLongTaskProgress,error){
 f.longCount.Add(1);f.longOperation="clear";f.longTask=id
 return remotetransport.RemoteLongTaskProgress{TaskID:id,Status:"cleared",ExitCode:-1},nil
}
func(f *fakeGUIActions)RunTaskText(ctx context.Context,id int,text string)(remotetransport.RemoteTaskRun,error){
 f.runText=text
 return f.RunTaskChoice(ctx,id,0)
}
func(f *fakeGUIActions)RunTask(ctx context.Context,id int)(remotetransport.RemoteTaskRun,error){
 return f.RunTaskChoice(ctx,id,0)
}
func(f *fakeGUIActions)RunTaskChoice(ctx context.Context,id,choice int)(remotetransport.RemoteTaskRun,error){
 f.runCount.Add(1)
 f.runChoice=choice
 if f.runErr!=nil{return remotetransport.RemoteTaskRun{},f.runErr}
 return remotetransport.RemoteTaskRun{TaskID:id,ExitCode:0,Output:"native task complete"},nil
}
func(f *fakeGUIActions)InspectTask(ctx context.Context,id int)(remotetransport.RemoteTaskInspect,error){
 f.inspectCount.Add(1)
 f.inspectedTask=id
 if f.inspectErr!=nil{return remotetransport.RemoteTaskInspect{},f.inspectErr}
 if f.inspectedValue.TaskID==id{return f.inspectedValue,nil}
 return remotetransport.RemoteTaskInspect{TaskID:id,Eligible:true},nil
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

func TestTrustedGUIAllowsOnlyBoundedFixedE2EEActions(t *testing.T){
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
 // Consume the remaining independently authorized, read-only rounds.
 // The 17th attempt must NEVER be forwarded to TaskDeck or Relay.
 for n:=4;n<trustedGUIMaxActions;n++{
  ok:=httptest.NewRecorder()
  ui.ServeHTTP(ok,localRequest("POST","/api/ping",uiTestOrigin,uiTestToken))
  if ok.Code!=http.StatusOK ||
   !strings.Contains(ok.Body.String(),fmt.Sprintf(`"remaining":%d`,trustedGUIMaxActions-n-1)){
   t.Fatalf("authorized round %d failed: %d %s",n+1,ok.Code,ok.Body.String())
  }
 }
 extra:=httptest.NewRecorder()
 ui.ServeHTTP(extra,localRequest("POST","/api/git-status",uiTestOrigin,uiTestToken))
 if extra.Code!=http.StatusGone {
  t.Fatalf("operation %d must be refused: HTTP %d",trustedGUIMaxActions+1,extra.Code)
 }
 if actions.featuresCount.Load()!=1 ||
  actions.pingCount.Load()!=int32(trustedGUIMaxActions-4) ||
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
   !strings.Contains(rr.Body.String(),`"remaining":16`) ||
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

func TestTrustedGUIFeaturesOnlyForLocalCapabilityAndSharedRoundQuota(t *testing.T){
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
  !strings.Contains(good.Body.String(),`"remaining":15`) ||
  !strings.Contains(good.Body.String(),`"remote_control_available":false`) ||
  actions.featuresCount.Load()!=1{
  t.Fatalf("native feature list was unverified or not charged to E2EE quota: %d %s",good.Code,good.Body.String())
 }
}

func TestTrustedGUIHostOwnedTaskNextPageUsesOneRound(t *testing.T){
 actions:=&fakeGUIActions{tasksNext:remotetransport.RemoteTasksPreview{
  Total:11,Labels:[]string{"E","F","G"},
 }}
 ui,_:=newUITest(t,actions)
 unauth:=httptest.NewRecorder()
 ui.ServeHTTP(unauth,localRequest("POST","/api/tasks-preview-next",
  "https://php.attacker.invalid",uiTestToken))
 if unauth.Code!=http.StatusForbidden||actions.tasksNextCount.Load()!=0{
  t.Fatal("PHP Portal triggered trusted GUI task next-page")
 }
 rr:=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/tasks-preview-next",uiTestOrigin,uiTestToken))
 if rr.Code!=http.StatusOK||!strings.Contains(rr.Body.String(),`"total":11`)||
  !strings.Contains(rr.Body.String(),`"remaining":15`)||
  actions.tasksNextCount.Load()!=1{
  t.Fatalf("native load next did not use one E2EE round: %d %s",rr.Code,rr.Body.String())
 }
}

func TestTrustedGUIReadOnlyQuotaMatchesRelay(t *testing.T){
 if trustedGUIMaxActions!=remoterelay.MaxAppPingRounds{
  t.Fatalf("native GUI quota=%d disagrees with opaque Relay quota=%d",
   trustedGUIMaxActions,remoterelay.MaxAppPingRounds)
 }
}


// No browser request or JS interval is required for the native E2EE
// session to be retired. The host's five-minute safety deadline is
// tested here with a short client-only injection, not a long sleep.
func TestTrustedGUIIdleNativeDeadlineRevokesChannel(t *testing.T){
 actions:=&fakeGUIActions{}
 var opened string
 var out strings.Builder
 err:=runTrustedRemoteGUIWithIdentityAndOpenerForLifetime(
  context.Background(),actions,&out,trustedGUITarget{},
  func(url string){opened=url},75*time.Millisecond,
 )
 if err!=nil{t.Fatalf("native lifetime failed: %v",err)}
 if actions.closeCount.Load()!=1{
  t.Fatalf("idle GUI did not revoke E2EE once: %d",actions.closeCount.Load())
 }
 if opened=="" || !strings.HasPrefix(opened,"http://127.0.0.1:"){
  t.Fatal("native test did not start localhost-only GUI")
 }
 if strings.Contains(opened,"#"){
  t.Fatal("native GUI URL exposed the local activation capability")
 }
 host:=strings.TrimSuffix(strings.TrimPrefix(opened,"http://"),"/")
 conn,dialErr:=net.DialTimeout("tcp4",host,250*time.Millisecond)
 if dialErr==nil {
  _=conn.Close()
  t.Fatal("expired native GUI still accepts local connections")
 }
 if actions.pingCount.Load()!=0||actions.gitCount.Load()!=0{
  t.Fatal("idle timer accidentally sent E2EE/PHP traffic")
 }
}

func TestTrustedGUIParentCancelRevokesEvenBeforeDeadline(t *testing.T){
 actions:=&fakeGUIActions{}
 ctx,cancel:=context.WithCancel(context.Background())
 defer cancel()
 var out strings.Builder
 err:=runTrustedRemoteGUIWithIdentityAndOpenerForLifetime(
  ctx,actions,&out,trustedGUITarget{},
  func(string){cancel()},time.Second,
 )
 if err!=nil||actions.closeCount.Load()!=1{
  t.Fatalf("parent cancel left GUI E2EE active: %v, closes %d",err,actions.closeCount.Load())
 }
}

func TestTrustedGUIRejectsInvalidDeadlineBeforeListening(t *testing.T){
 actions:=&fakeGUIActions{}
 for _,lifetime:=range []time.Duration{0,-time.Second,trustedGUIMaxLifetime+time.Second}{
  var out strings.Builder
  err:=runTrustedRemoteGUIWithIdentityAndOpenerForLifetime(
   context.Background(),actions,&out,trustedGUITarget{},
   func(string){t.Fatal("invalid deadline reached browser opener")},lifetime,
  )
  if err==nil||out.Len()!=0{t.Fatalf("invalid native lifetime accepted: %v",lifetime)}
 }
}

func TestTrustedGUITaskSelectionOnlyCanonicalIssuedID(t *testing.T){
 actions:=&fakeGUIActions{}
 ui,_:=newUITest(t,actions)
 for _,path:=range []string{
  "/api/tasks-inspect/0","/api/tasks-inspect/-1",
  "/api/tasks-inspect/01","/api/tasks-inspect/1?command=sh",
  "/api/tasks-inspect/9999999999999","/api/tasks-inspect/1/../../etc",
  "/api/tasks-inspect/%2e%2e","/api/tasks-inspect/+1",
 }{
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("POST",path,uiTestOrigin,uiTestToken))
  if rr.Code==200{t.Fatalf("unsafe task selection accepted: %q",path)}
 }
 for _,id:=range []int{1,123,2147483647}{
  path:=fmt.Sprintf("/api/tasks-inspect/%d",id)
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("POST",path,uiTestOrigin,uiTestToken))
  if rr.Code!=http.StatusOK ||
   !strings.Contains(rr.Body.String(),fmt.Sprintf(`"task_id":%d`,id)) ||
   !strings.Contains(rr.Body.String(),`"remote_control_available":false`){
   t.Fatalf("canonical selected ID %d failed: %d %s",id,rr.Code,rr.Body.String())
  }
 }
 if actions.inspectCount.Load()!=3{t.Fatal("noncanonical IDs reached native E2EE channel")}
 rejected:=httptest.NewRecorder()
 ui.ServeHTTP(rejected,localRequest("POST","/api/tasks-inspect/1",
  "https://compromised-portal.invalid",uiTestToken))
 if rejected.Code!=http.StatusForbidden || actions.inspectCount.Load()!=3 {
  t.Fatal("Portal forged an authenticated localhost task selection")
 }
}
func TestTrustedGUIInspectDenialPoisonsE2EEAndRejectsArguments(t *testing.T){
 actions:=&fakeGUIActions{inspectErr:errors.New("stale snapshot")}
 ui,_:=newUITest(t,actions)
 request:=httptest.NewRequest("POST","/api/tasks-inspect/123",
  strings.NewReader(`{"command":"sh","args":["-c","rm -rf /"]}`))
 request.Host="127.0.0.1:49237"
 request.Header.Set("Origin",uiTestOrigin)
 request.Header.Set("X-TaskDeck-Local-Capability",uiTestToken)
 rejected:=httptest.NewRecorder()
 ui.ServeHTTP(rejected,request)
 if rejected.Code!=http.StatusBadRequest||actions.inspectCount.Load()!=0{
  t.Fatal("native task inspect accepted a script body")
 }
 rr:=httptest.NewRecorder()
 ui.ServeHTTP(rr,localRequest("POST","/api/tasks-inspect/123",uiTestOrigin,uiTestToken))
 if rr.Code!=http.StatusForbidden||actions.inspectCount.Load()!=1||
  actions.closeCount.Load()!=1{t.Fatal("expired snapshot did not revoke E2EE")}
 after:=httptest.NewRecorder()
 ui.ServeHTTP(after,localRequest("POST","/api/tasks-inspect/123",uiTestOrigin,uiTestToken))
 if after.Code!=http.StatusGone||actions.inspectCount.Load()!=1{
  t.Fatal("inspected stale task continued after revocation")
 }
}

func TestTrustedGUIBoundedTaskRunByIDOnly(t *testing.T){
 actions:=&fakeGUIActions{}
 ui,_:=newUITest(t,actions)
 for _,route:=range []string{
  "/api/tasks-run/0","/api/tasks-run/01","/api/tasks-run/-1",
  "/api/tasks-run/1?command=sh","/api/tasks-run/2147483648",
  "/api/tasks-run/123/extra",
 }{
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("POST",route,uiTestOrigin,uiTestToken))
  if rr.Code==200{t.Fatalf("noncanonical task run allowed: %q",route)}
 }
 r:=httptest.NewRecorder()
 ui.ServeHTTP(r,localRequest("POST","/api/tasks-run/123",uiTestOrigin,uiTestToken))
 if r.Code!=http.StatusOK||actions.runCount.Load()!=1||
  !strings.Contains(r.Body.String(),`"exit_code":0`)||
  !strings.Contains(r.Body.String(),`"remote_control_available":false`){
  t.Fatalf("task.run not bound to local GUI capability: %d %s",r.Code,r.Body.String())
 }
 unauthorized:=httptest.NewRecorder()
 ui.ServeHTTP(unauthorized,localRequest("POST","/api/tasks-run/123",
  "https://compromised-portal.invalid",uiTestToken))
 if unauthorized.Code!=http.StatusForbidden||actions.runCount.Load()!=1{
  t.Fatal("untrusted PHP origin launched a task through local GUI")
 }
}
func TestTrustedGUITaskRunRejectsRemoteCommands(t *testing.T){
 actions:=&fakeGUIActions{}
 ui,_:=newUITest(t,actions)
 request:=httptest.NewRequest("POST","/api/tasks-run/123",
  strings.NewReader(`{"command":"sh","args":["-c","touch /tmp/pwned"]}`))
 request.Host="127.0.0.1:49237"
 request.Header.Set("Origin",uiTestOrigin)
 request.Header.Set("X-TaskDeck-Local-Capability",uiTestToken)
 rejected:=httptest.NewRecorder()
 ui.ServeHTTP(rejected,request)
 if rejected.Code!=http.StatusBadRequest||actions.runCount.Load()!=0{
  t.Fatal("command or argument injection reached native task runner")
 }
}

func TestTrustedGUIPickChoiceRouteNeverAcceptsRawString(t *testing.T){
 actions:=&fakeGUIActions{}
 ui,_:=newUITest(t,actions)
 for _,route:=range []string{
  "/api/tasks-run/9/0","/api/tasks-run/9/07",
  "/api/tasks-run/9/-1","/api/tasks-run/9/37",
  "/api/tasks-run/9/main","/api/tasks-run/9/1/extra",
  "/api/tasks-run/9/1?command=sh",
 }{
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("POST",route,uiTestOrigin,uiTestToken))
  if rr.Code==http.StatusOK{
   t.Fatalf("raw or noncanonical choice reached native runner: %q",route)
  }
 }
 r:=httptest.NewRecorder()
 ui.ServeHTTP(r,localRequest("POST","/api/tasks-run/9/2",uiTestOrigin,uiTestToken))
 if r.Code!=http.StatusOK || actions.runCount.Load()!=1 || actions.runChoice!=2{
  t.Fatalf("a bounded numeric host choice was denied: %d %s",r.Code,r.Body.String())
 }
}

func TestTrustedGUIExposesBoundedTaskRunFeatureOnlyAsAdvisoryFlag(t *testing.T){
 for _,enabled:=range []bool{false,true}{
  actions:=&fakeGUIActions{features:remotetransport.RemoteFeatures{
   SessionPing:true,TasksPreview:true,TaskRun:enabled,
  }}
  ui,_:=newUITest(t,actions)
  response:=httptest.NewRecorder()
  ui.ServeHTTP(response,localRequest("POST","/api/features",uiTestOrigin,uiTestToken))
  expected:=fmt.Sprintf(`"task_run":%t`,enabled)
  if response.Code!=http.StatusOK||
   !strings.Contains(response.Body.String(),expected)||
   !strings.Contains(response.Body.String(),`"remote_control_available":false`){
   t.Fatalf("native GUI did not faithfully forward verified E2EE task-run flag: %d %s",response.Code,response.Body.String())
  }
  if actions.runCount.Load()!=0{
   t.Fatal("advisory capability query executed a task")
  }
 }
}

func TestTrustedGUIPromptStringNativeInputIsBoundedAndNotInURL(t *testing.T){
 actions:=&fakeGUIActions{}
 ui,_:=newUITest(t,actions)
 target:="/api/tasks-run-input/123"
 makeRequest:=func(body,contentType,origin string)*http.Request{
  req:=httptest.NewRequest("POST",target,strings.NewReader(body))
  req.Host="127.0.0.1:49237"
  req.Header.Set("Origin",origin)
  req.Header.Set("X-TaskDeck-Local-Capability",uiTestToken)
  if contentType!=""{req.Header.Set("Content-Type",contentType)}
  return req
 }
 for _,item:=range []struct{body,contentType,origin string}{
  {`{"value":"bad;sh"}`,"application/json",uiTestOrigin},
  {`{"value":"ok","extra":2}`,"application/json",uiTestOrigin},
  {`{"value":"one","value":"two"}`,"application/json",uiTestOrigin},
  {`{"value":"ok"}`,"text/plain",uiTestOrigin},
  {`{"value":"ok"}`,"application/json","https://php.attacker.invalid"},
 }{
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,makeRequest(item.body,item.contentType,item.origin))
  if rr.Code==http.StatusOK{t.Fatalf("untrusted GUI input was accepted: %s",item.body)}
 }
 if actions.runCount.Load()!=0{t.Fatal("invalid input reached authenticated Agent")}
 rr:=httptest.NewRecorder()
 ui.ServeHTTP(rr,makeRequest(`{"value":"release-1.2"}`,"application/json",uiTestOrigin))
 if rr.Code!=http.StatusOK||actions.runCount.Load()!=1||
  actions.runText!="release-1.2"{
  t.Fatalf("valid single token did not reach trusted native E2EE client: %d %s",rr.Code,rr.Body.String())
 }
}

func TestTrustedGUIMultiplePickIndexCanonicalBound(t *testing.T){
 actions:=&fakeGUIActions{}
 ui,_:=newUITest(t,actions)
 for _,choice:=range []int{7,36}{
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("POST",fmt.Sprintf("/api/tasks-run/19/%d",choice),
   uiTestOrigin,uiTestToken))
  if rr.Code!=http.StatusOK||actions.runChoice!=choice{
   t.Fatalf("two-input encoded choice %d never reached E2EE: %d %s",choice,rr.Code,rr.Body.String())
  }
 }
 if actions.runCount.Load()!=2{t.Fatal("multi-input GUI sent unexpected requests")}
}

func TestTrustedGUILongTaskRoutesRemainNativeAndIDOnly(t *testing.T){
 actions:=&fakeGUIActions{}
 ui,_:=newUITest(t,actions)
 for _,route:=range []string{
  "/api/tasks-start/123/5",
  "/api/tasks-poll/123",
  "/api/tasks-cancel/123",
  "/api/tasks-clear/123",
 }{
  w:=httptest.NewRecorder()
  ui.ServeHTTP(w,localRequest("POST",route,uiTestOrigin,uiTestToken))
  if w.Code!=http.StatusOK||
   !strings.Contains(w.Body.String(),`"remote_control_available":false`)||
   !strings.Contains(w.Body.String(),`"task_id":123`){
   t.Fatalf("native-only long-task action failed: %s => %d %s",route,w.Code,w.Body.String())
  }
 }
 if actions.longCount.Load()!=4||actions.longChoice!=5{
  t.Fatalf("invalid native E2EE long-task methods invoked: %d choice %d",
   actions.longCount.Load(),actions.longChoice)
 }
 for _,route:=range []string{
  "/api/tasks-start/0",
  "/api/tasks-start/123/0",
  "/api/tasks-start/123/037",
  "/api/tasks-start/123/37",
  "/api/tasks-poll/0",
  "/api/tasks-cancel/123/extra",
  "/api/tasks-clear/123/../oops",
 }{
  w:=httptest.NewRecorder()
  ui.ServeHTTP(w,localRequest("POST",route,uiTestOrigin,uiTestToken))
  if w.Code==http.StatusOK{
   t.Fatalf("noncanonical job route accepted: %s",route)
  }
 }
 if actions.longCount.Load()!=4{t.Fatal("forged routes were forwarded to E2EE client")}
}

func TestTrustedGUILongTaskPromptUsesStrictLocalJSON(t *testing.T){
 actions:=&fakeGUIActions{}
 ui,_:=newUITest(t,actions)
 req:=httptest.NewRequest("POST","/api/tasks-start-input/123",
  strings.NewReader(`{"value":"release-4.2"}`))
 req.Host="127.0.0.1:49237"
 req.Header.Set("Origin",uiTestOrigin)
 req.Header.Set("X-TaskDeck-Local-Capability",uiTestToken)
 req.Header.Set("Content-Type","application/json")
 w:=httptest.NewRecorder()
 ui.ServeHTTP(w,req)
 if w.Code!=http.StatusOK||actions.longCount.Load()!=1||
  actions.longText!="release-4.2"||actions.longTask!=123{
  t.Fatalf("native prompt never bound to long task: %d %s",w.Code,w.Body.String())
 }
 bad:=httptest.NewRequest("POST","/api/tasks-start-input/123",
  strings.NewReader(`{"value":"abc; rm -rf"}`))
 bad.Host="127.0.0.1:49237"
 bad.Header.Set("Origin",uiTestOrigin)
 bad.Header.Set("X-TaskDeck-Local-Capability",uiTestToken)
 bad.Header.Set("Content-Type","application/json")
 denied:=httptest.NewRecorder()
 ui.ServeHTTP(denied,bad)
 if denied.Code==http.StatusOK||actions.longCount.Load()!=1{
  t.Fatal("malicious long-task input was forwarded")
 }
}

func TestTrustedGUIForwardsAllAuthenticatedTaskInputMetadata(t *testing.T){
 for _,value:=range []remotetransport.RemoteTaskInspect{
  {TaskID:23,Eligible:true,InputID:"tag",InputType:"promptString"},
  {TaskID:23,Eligible:true,InputID:"region",Choices:[]string{"us","eu"},
   InputID2:"mode",Choices2:[]string{"debug","release"}},
 }{
  actions:=&fakeGUIActions{inspectedValue:value}
  ui,_:=newUITest(t,actions)
  response:=httptest.NewRecorder()
  ui.ServeHTTP(response,localRequest("POST",
   "/api/tasks-inspect/23",uiTestOrigin,uiTestToken))
  body:=response.Body.String()
  if response.Code!=http.StatusOK||
   !strings.Contains(body,`"input_type":"`+value.InputType+`"`)||
   !strings.Contains(body,`"input_id_2":"`+value.InputID2+`"`)||
   !strings.Contains(body,`"remote_control_available":false`){
   t.Fatalf("native GUI dropped E2EE input type or second pick: %d %s",response.Code,body)
  }
  if actions.runCount.Load()!=0||actions.longCount.Load()!=0{
   t.Fatal("read-only task inspection started a process")
  }
 }
}

func TestTrustedGUIGitPullIsFixedAuthenticatedAction(t *testing.T){
 fake:=&fakeGUIActions{}
 ui,_:=newUITest(t,fake)
 request:=localRequest("POST","/api/git-pull",uiTestOrigin,uiTestToken)
 rr:=httptest.NewRecorder()
 ui.ServeHTTP(rr,request)
 if rr.Code!=200||!strings.Contains(rr.Body.String(),`"updated":true`)||
  !strings.Contains(rr.Body.String(),`"remote_control_available":false`)||
  fake.gitCount.Load()!=1 {
  t.Fatalf("native GitPull result=%d %s called=%d",rr.Code,rr.Body.String(),fake.gitCount.Load())
 }
 for _,path:=range []string{"/api/git-pull/other","/api/git-pull?branch=main","/api/git-commit"}{
  rr=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest("POST",path,uiTestOrigin,uiTestToken))
  if rr.Code==200||fake.gitCount.Load()!=1{
   t.Fatalf("unexpected Git mutation route %q admitted (%d)",path,rr.Code)
  }
 }
 denied:=httptest.NewRecorder()
 ui.ServeHTTP(denied,localRequest("POST","/api/git-pull","https://portal.invalid",uiTestToken))
 if denied.Code!=403||fake.gitCount.Load()!=1{
  t.Fatal("portal origin reached trusted GitPull action")
 }
}
