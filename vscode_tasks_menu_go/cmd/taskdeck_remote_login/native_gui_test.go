package main

import (
 "bytes"
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

type fakeDesktopGUIRemote struct{
 calls []string
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
func (f *fakeDesktopGUIRemote) GitStagedDiff(_ context.Context,p string)(string,error){return f.record("git:staged:"+p)}
func (f *fakeDesktopGUIRemote) GitHeadDiff(_ context.Context,p string)(string,error){return f.record("git:head:"+p)}
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
