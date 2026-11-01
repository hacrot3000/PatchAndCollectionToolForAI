package main

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

type fakeDesktopGUIRemote struct{
 calls []string
 lastGitSHA string
 emptyStaged bool
}
func (f *fakeDesktopGUIRemote) record(name string)(string,error){
 f.calls=append(f.calls,name)
 return "HOST-E2EE: "+name,nil
}
func (f *fakeDesktopGUIRemote) ListDirectory(_ context.Context,p string)(string,error){return f.record("list:"+p)}
func (f *fakeDesktopGUIRemote) ReadFilePreview(_ context.Context,p string)(string,error){return f.record("view:"+p)}
func (f *fakeDesktopGUIRemote) ReadFileLines(_ context.Context,p string,start int64)(string,error){return f.record("lines:"+p)}
func (f *fakeDesktopGUIRemote) GitStatusFull(context.Context)(string,error){return f.record("git:status")}
func (f *fakeDesktopGUIRemote) GitLog(context.Context)(string,error){return f.record("git:log")}
func (f *fakeDesktopGUIRemote) GitBranches(context.Context)(string,error){return f.record("git:branches")}
func (f *fakeDesktopGUIRemote) GitDiff(_ context.Context,p string)(string,error){return f.record("git:diff:"+p)}
func (f *fakeDesktopGUIRemote) GitStagedDiff(_ context.Context,p string)(string,error){
 if f.emptyStaged{
  f.calls=append(f.calls,"git:staged:"+p)
  return "",nil
 }
 return f.record("git:staged:"+p)
}
func (f *fakeDesktopGUIRemote) GitHeadDiff(_ context.Context,p string)(string,error){return f.record("git:head:"+p)}
func (f *fakeDesktopGUIRemote) GitStage(_ context.Context,p,sha string)(string,error){
 f.lastGitSHA=sha
 return f.record("git:stage:"+p)
}
func (f *fakeDesktopGUIRemote) GitUnstage(_ context.Context,p,sha string)(string,error){
 f.lastGitSHA=sha
 return f.record("git:unstage:"+p)
}
func (f *fakeDesktopGUIRemote) ListApprovedTasks(context.Context)(string,error){
 f.calls=append(f.calls,"tasks:list")
 return "1\tHost approved smoke test\n2\tHost approved backup\n",nil
}
func (f *fakeDesktopGUIRemote) RunTaskReport(_ context.Context,n int)(string,error){return f.record("tasks:run:1")}
func (f *fakeDesktopGUIRemote) StartManagedTask(_ context.Context,n int)(string,error){return f.record("tasks:start:1")}
func (f *fakeDesktopGUIRemote) StatusManagedTask(_ context.Context,h string)(string,error){return f.record("tasks:status:"+h)}
func (f *fakeDesktopGUIRemote) CancelManagedTask(_ context.Context,h string)(string,error){return f.record("tasks:cancel:"+h)}
func (f *fakeDesktopGUIRemote) ReadManagedOutput(_ context.Context,h string,n int64)(string,error){return f.record("tasks:output:"+h)}

func guiRequestTest(t *testing.T,g *trustedGUI,request guiRequest,origin,token string)(int,guiResponse){
 t.Helper()
 payload,err:=json.Marshal(request)
 if err!=nil{t.Fatal(err)}
 r:=httptest.NewRequest(http.MethodPost,g.origin+"/api/action",bytes.NewReader(payload))
 r.Host=strings.TrimPrefix(g.origin,"http://")
 r.Header.Set("Content-Type","application/json")
 if origin!=""{r.Header.Set("Origin",origin)}
 if token!=""{r.Header.Set("X-TaskDeck-Local-Key",token)}
 rr:=httptest.NewRecorder()
 g.handler().ServeHTTP(rr,r)
 var response guiResponse
 if err:=json.Unmarshal(rr.Body.Bytes(),&response);err!=nil{t.Fatal(err,rr.Body.String())}
 return rr.Code,response
}
func newTestNativeGUI()(*trustedGUI,*fakeDesktopGUIRemote){
 f:=&fakeDesktopGUIRemote{}
 g:=newTrustedGUI(context.Background(),f,"http://127.0.0.1:42119",strings.Repeat("a",64))
 return g,f
}

func TestNativeGUIBlocksCompromisedPHPOriginAndMissingLocalSessionKey(t *testing.T){
 g,f:=newTestNativeGUI()
 for _,tc:=range []struct{name,origin,key string}{
  {"PHP-origin","https://evil.example",g.key},
  {"no-origin","",g.key},
  {"no-key",g.origin,""},
  {"wrong-key",g.origin,strings.Repeat("b",64)},
 }{
  t.Run(tc.name,func(t *testing.T){
   status,res:=guiRequestTest(t,g,guiRequest{Operation:"git_status"},tc.origin,tc.key)
   if status!=403||res.OK||len(f.calls)!=0{
    t.Fatalf("cross-site PHP requested TaskDeck E2EE data: %d %+v %+v",status,res,f.calls)
   }
  })
 }
 status,response:=guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 if status!=200||!response.OK||response.Text!="HOST-E2EE: git:status"||response.Used!=1{
  t.Fatalf("trusted loopback GUI refused E2EE Git status: %d %+v",status,response)
 }
}

func TestNativeGUIRejectsShellCommandsAndPathTraversalBeforeNetwork(t *testing.T){
 g,f:=newTestNativeGUI()
 invalid:=[]guiRequest{
  {Operation:"terminal.shell",Path:"rm -rf /"},
  {Operation:"git_push",Path:"origin"},
  {Operation:"git_diff",Path:"../.ssh/id_rsa"},
  {Operation:"git_diff",Path:"../../etc/passwd"},
  {Operation:"file_view",Path:"/etc/passwd"},
  {Operation:"files_list",Path:"../"},
  {Operation:"file_lines",Path:"src/file.go",Offset:0},
  {Operation:"task_cancel",Handle:"1234",Confirmation:"CANCEL TASK 1234"},
  {Operation:"task_execute",TaskID:1,Mode:"run",Confirmation:"RUN TASK 1"},
 }
 for _,req:=range invalid{
  status,res:=guiRequestTest(t,g,req,g.origin,g.key)
  if status!=400||res.OK||res.Used!=0{t.Fatalf("invalid operation accepted: %+v %+v",req,res)}
 }
 if len(f.calls)!=0{t.Fatalf("untrusted browser command crossed relay: %+v",f.calls)}
}

func TestNativeGUITaskReviewAndExactOneTimeConfirmation(t *testing.T){
 g,f:=newTestNativeGUI()
 s,rev:=guiRequestTest(t,g,guiRequest{Operation:"task_review",TaskID:1,Mode:"run"},g.origin,g.key)
 if s!=200||rev.Confirm!="RUN TASK 1"||rev.Used!=1{t.Fatalf("bad host task review %+v",rev)}
 status,denied:=guiRequestTest(t,g,guiRequest{Operation:"task_execute",TaskID:1,Mode:"run",Confirmation:"RUN TASK 2"},g.origin,g.key)
 if status!=400||denied.OK||len(f.calls)!=1{t.Fatal("wrong confirmation executed task")}
 status,denied=guiRequestTest(t,g,guiRequest{Operation:"task_execute",TaskID:1,Mode:"run",Confirmation:"RUN TASK 1"},g.origin,g.key)
 if status!=400||denied.OK||len(f.calls)!=1{t.Fatal("replayed review bypassed one-time confirmation")}
 _,rev=guiRequestTest(t,g,guiRequest{Operation:"task_review",TaskID:1,Mode:"start"},g.origin,g.key)
 if rev.Confirm!="START TASK 1"{t.Fatal("wrong managed task confirmation")}
 status,exec:=guiRequestTest(t,g,guiRequest{Operation:"task_execute",TaskID:1,Mode:"start",Confirmation:"START TASK 1"},g.origin,g.key)
 if status!=200||!exec.OK||exec.Text!="HOST-E2EE: tasks:start:1"{
  t.Fatalf("approved managed task could not start: %+v",exec)
 }
 if len(f.calls)!=3{t.Fatalf("unexpected remote action count: %+v",f.calls)}
}

func TestNativeGUIFiniteRoundQuotaAndLocalHTMLNoRemoteJS(t *testing.T){
 g,f:=newTestNativeGUI()
 for i:=1;i<=6;i++{
  status,res:=guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
  if status!=200||res.Used!=i{t.Fatalf("wrong E2EE quota: %d %+v",status,res)}
 }
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=6{t.Fatal("GUI exceeded authenticated mailbox quota")}
 req:=httptest.NewRequest("GET",g.origin+"/",nil)
 rr:=httptest.NewRecorder()
 g.handler().ServeHTTP(rr,req)
 if rr.Code!=200||strings.Contains(rr.Body.String(),"<script src=\"https://")||
  !strings.Contains(rr.Header().Get("Content-Security-Policy"),"default-src 'none'")||
  strings.Contains(rr.Body.String(),g.key){
  t.Fatal("native GUI leaked session bearer or loaded PHP/CDN script")
 }
}

func TestNativeGUISessionCloseStopsAllNewRequests(t *testing.T){
 g,f:=newTestNativeGUI()
 g.close()
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)>0{t.Fatal("closed GUI retained a TaskDeck principal")}
}

func TestNativeGUIGitWriteNeedsAuthenticatedReviewAndExactOneShotCAS(t *testing.T){
 g,f:=newTestNativeGUI()
 denied:=guiRequest{Operation:"git_write_execute",Path:"src/main.go",
  Mode:"stage",Confirmation:"STAGE src/main.go"}
 status,res:=guiRequestTest(t,g,denied,g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=0{t.Fatal("Git index write bypassed review")}
 review:=guiRequest{Operation:"git_write_review",Path:"src/main.go",Mode:"stage"}
 status,result:=guiRequestTest(t,g,review,g.origin,g.key)
 if status!=200||result.Confirm!="STAGE src/main.go"||len(f.calls)!=1{
  t.Fatalf("Git stage preview not required: %+v %+v",result,f.calls)
 }
 expected:=sha256.Sum256([]byte("HOST-E2EE: view:src/main.go"))
 status,result=guiRequestTest(t,g,denied,g.origin,g.key)
 if status!=200||!result.OK||f.lastGitSHA!=hex.EncodeToString(expected[:])||
  len(f.calls)!=2||f.calls[1]!="git:stage:src/main.go"{
  t.Fatalf("Git stage used unreviewed source data: %+v %s",result,f.lastGitSHA)
 }
 status,result=guiRequestTest(t,g,denied,g.origin,g.key)
 if status!=400||result.OK||len(f.calls)!=2{t.Fatal("replay of Git stage review executed")}
}

func TestNativeGUIGitUnstageUsesStagedDiffDigestAndRejectsBadConfirmation(t *testing.T){
 g,f:=newTestNativeGUI()
 review:=guiRequest{Operation:"git_write_review",Mode:"unstage",Path:"README.md"}
 status,reply:=guiRequestTest(t,g,review,g.origin,g.key)
 if status!=200||reply.Confirm!="UNSTAGE README.md"{t.Fatalf("wrong Git unstage review %+v",reply)}
 wrong:=guiRequest{Operation:"git_write_execute",Mode:"unstage",Path:"README.md",
  Confirmation:"UNSTAGE src/main.go"}
 status,reply=guiRequestTest(t,g,wrong,g.origin,g.key)
 if status!=400||reply.OK||len(f.calls)!=1{t.Fatal("wrong Git confirmation wrote index")}
 _,_ = guiRequestTest(t,g,review,g.origin,g.key)
 right:=guiRequest{Operation:"git_write_execute",Mode:"unstage",Path:"README.md",
  Confirmation:"UNSTAGE README.md"}
 status,reply=guiRequestTest(t,g,right,g.origin,g.key)
 digest:=sha256.Sum256([]byte("HOST-E2EE: git:staged:README.md"))
 if status!=200||!reply.OK||f.lastGitSHA!=hex.EncodeToString(digest[:])||
  f.calls[len(f.calls)-1]!="git:unstage:README.md"{
  t.Fatal("host-staged diff SHA was not pinned by native client")
 }
}

func TestNativeGUIRefusesGitIndexPathspecAndDotGitBeforeE2EE(t *testing.T){
 g,f:=newTestNativeGUI()
 for _,path:=range []string{"../secrets","/etc/shadow","--upload-pack",".git/config",
  "nested/.git/config","src/[ab].go","*.go","-Xdanger"}{
  status,res:=guiRequestTest(t,g,guiRequest{Operation:"git_write_review",Path:path,Mode:"stage"},g.origin,g.key)
  if status!=400||res.OK||res.Used!=0||len(f.calls)!=0{
   t.Fatalf("unsafe Git index path reached E2EE: %q %+v",path,res)
  }
 }
}

func TestNativeGUIRejectsDNSRebindingAndDisconnectedSession(t *testing.T){
 g,f:=newTestNativeGUI()
 body:=[]byte(`{"operation":"git_status"}`)
 attack:=httptest.NewRequest("POST",g.origin+"/api/action",bytes.NewReader(body))
 attack.Host="attacker-controlled.example"
 attack.Header.Set("Origin",g.origin)
 attack.Header.Set("Content-Type","application/json")
 attack.Header.Set("X-TaskDeck-Local-Key",g.key)
 w:=httptest.NewRecorder()
 g.handler().ServeHTTP(w,attack)
 if w.Code!=403||len(f.calls)!=0{
  t.Fatal("DNS-rebound localhost Host reached authenticated TaskDeck GUI")
 }
 ctx,cancel:=context.WithCancel(context.Background())
 g.parent=ctx
 cancel() // simulates lost native E2EE session or forced logout
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=0{
  t.Fatalf("disconnected native E2EE principal continued executing: %+v",res)
 }
}

func TestNativeGUIRejectsOversizedAndUnknownJSONFields(t *testing.T){
 g,f:=newTestNativeGUI()
 for _,input:=range []string{
  `{"operation":"git_status","shell":"rm -rf /"}`,
  `{"operation":"git_status","path":"x","task_id":1,"arguments":"--danger"}`,
  `{"operation":"git_status","path":"`+strings.Repeat("x",2100)+`"}`,
  `{"operation":"git_status"}{"operation":"task_execute"}`,
 }{
  req:=httptest.NewRequest("POST",g.origin+"/api/action",strings.NewReader(input))
  req.Host=strings.TrimPrefix(g.origin,"http://")
  req.Header.Set("Origin",g.origin)
  req.Header.Set("Content-Type","application/json")
  req.Header.Set("X-TaskDeck-Local-Key",g.key)
  w:=httptest.NewRecorder()
  g.handler().ServeHTTP(w,req)
  if w.Code!=400||len(f.calls)!=0{
   t.Fatalf("untrusted local browser command was accepted: code=%d",w.Code)
  }
 }
}

func TestNativeGUIEmptyStagedPreviewStillConsumesRealE2EERound(t *testing.T){
 g,f:=newTestNativeGUI()
 f.emptyStaged=true
 status,reply:=guiRequestTest(t,g,guiRequest{
  Operation:"git_write_review",Mode:"unstage",Path:"README.md",
 },g.origin,g.key)
 if status!=400||reply.OK||reply.Used!=1||len(f.calls)!=1{
  t.Fatalf("empty Git review escaped E2EE quota: %+v %+v",reply,f.calls)
 }
 // No stale CAS transaction may be executed after an empty diff preview.
 status,reply=guiRequestTest(t,g,guiRequest{
  Operation:"git_write_execute",Mode:"unstage",Path:"README.md",
  Confirmation:"UNSTAGE README.md",
 },g.origin,g.key)
 if status!=400||reply.OK||reply.Used!=1||len(f.calls)!=1{
  t.Fatal("empty staged diff created a Git index write opportunity")
 }
}
