package main

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

type fakeDesktopGUIRemote struct{
 calls []string
 findOverride string
 previewOverride string
 linePageOverride string
 stagedErr error
}
func (f *fakeDesktopGUIRemote) record(name string)(string,error){
 f.calls=append(f.calls,name)
 return "HOST-E2EE: "+name,nil
}
func (f *fakeDesktopGUIRemote) ListDirectory(_ context.Context,p string)(string,error){return f.record("list:"+p)}
func (f *fakeDesktopGUIRemote) ReadFilePreview(_ context.Context,p string)(string,error){
 if f.previewOverride!=""{
  f.calls=append(f.calls,"view:"+p)
  return f.previewOverride,nil
 }
 return f.record("view:"+p)
}
func (f *fakeDesktopGUIRemote) ReplaceFileText(_ context.Context,path,content,sha string)(string,error){
 h:=sha256.Sum256([]byte(content))
 return f.record("file:replace:"+path+":"+hex.EncodeToString(h[:])+":"+sha)
}
func (f *fakeDesktopGUIRemote) ReadFileLines(_ context.Context,p string,start int64)(string,error){
 if f.linePageOverride!=""{
  f.calls=append(f.calls,"lines:"+p)
  return f.linePageOverride,nil
 }
 return f.record("lines:"+p)
}
func (f *fakeDesktopGUIRemote) EditLargeFileLine(_ context.Context,p string,start int64,content,sha string)(string,error){
 return f.record(fmt.Sprintf("large-edit:%s:%d:%s:%s",p,start,content,sha))
}
func (f *fakeDesktopGUIRemote) InsertLargeFileLine(_ context.Context,p string,start int64,content,sha string)(string,error){
 return f.record(fmt.Sprintf("large-insert:%s:%d:%s:%s",p,start,content,sha))
}
func (f *fakeDesktopGUIRemote) DeleteLargeFileLine(_ context.Context,p string,start int64,sha string)(string,error){
 return f.record(fmt.Sprintf("large-delete:%s:%d:%s",p,start,sha))
}
func (f *fakeDesktopGUIRemote) FindFileLines(_ context.Context,p,literal string)(string,error){
 f.calls=append(f.calls,"find:"+p+":"+literal)
 if f.findOverride!=""{return f.findOverride,nil}
 digest:=sha256.Sum256([]byte(literal))
 return "FILE_SHA256 "+strings.Repeat("a",64)+
  "\nTERM_SHA256 "+hex.EncodeToString(digest[:])+
  "\nTOTAL_LINES 3\nMATCHES 1\n-- COMPLETE MATCHING LINES --\n2: matched "+literal+"\n",nil
}
func (f *fakeDesktopGUIRemote) GitStatusFull(context.Context)(string,error){return f.record("git:status")}
func (f *fakeDesktopGUIRemote) GitLog(context.Context)(string,error){return f.record("git:log")}
func (f *fakeDesktopGUIRemote) GitBranches(context.Context)(string,error){return f.record("git:branches")}
func (f *fakeDesktopGUIRemote) GitRemotes(context.Context)(string,error){return f.record("git:remotes")}
func (f *fakeDesktopGUIRemote) GitDiff(_ context.Context,p string)(string,error){return f.record("git:diff:"+p)}
func (f *fakeDesktopGUIRemote) GitStagedDiff(_ context.Context,p string)(string,error){
 if f.stagedErr!=nil{
  f.calls=append(f.calls,"git:staged:"+p)
  return "",f.stagedErr
 }
 return f.record("git:staged:"+p)
}
func (f *fakeDesktopGUIRemote) GitHeadDiff(_ context.Context,p string)(string,error){return f.record("git:head:"+p)}
func (f *fakeDesktopGUIRemote) GitStage(_ context.Context,path,sha string)(string,error){return f.record("git:stage:"+path+":"+sha)}
func (f *fakeDesktopGUIRemote) GitUnstage(_ context.Context,path,sha string)(string,error){return f.record("git:unstage:"+path+":"+sha)}
func (f *fakeDesktopGUIRemote) GitCommitReview(_ context.Context)(string,error){
 f.calls=append(f.calls,"git:commit-review")
 return "HEAD "+strings.Repeat("a",40)+"\nBRANCH refs/heads/main\nINDEX_SHA256 "+strings.Repeat("b",64)+"\n-- COMPLETE STAGED DIFF --\ndiff --git a/readme b/readme\n",nil
}
func (f *fakeDesktopGUIRemote) GitCommit(_ context.Context,message,sha string)(string,error){
 return f.record("git:commit:"+message+":"+sha)
}
func (f *fakeDesktopGUIRemote) GitBranchReview(_ context.Context)(string,error){
 f.calls=append(f.calls,"git:branch-review")
 return "HEAD "+strings.Repeat("a",40)+"\nBRANCH refs/heads/main\n",nil
}
func (f *fakeDesktopGUIRemote) GitBranchCreate(_ context.Context,slug,sha string)(string,error){
 return f.record("git:branch-create:"+slug+":"+sha)
}
func (f *fakeDesktopGUIRemote) GitSwitchReview(_ context.Context,slug string)(string,error){
 f.calls=append(f.calls,"git:branch-switch-review:"+slug)
 return f.branchTransitionReview(slug),nil
}
func (f *fakeDesktopGUIRemote) GitSwitchSameHead(_ context.Context,slug,sha string)(string,error){
 return f.record("git:branch-switch:"+slug+":"+sha)
}
func (f *fakeDesktopGUIRemote) GitBranchDeleteReview(_ context.Context,slug string)(string,error){
 f.calls=append(f.calls,"git:branch-delete-review:"+slug)
 return f.branchTransitionReview(slug),nil
}
func (f *fakeDesktopGUIRemote) GitBranchDelete(_ context.Context,slug,sha string)(string,error){
 return f.record("git:branch-delete:"+slug+":"+sha)
}
func (f *fakeDesktopGUIRemote) branchTransitionReview(slug string)string{
 return "HEAD "+strings.Repeat("a",40)+"\nBRANCH refs/heads/main\nTARGET refs/heads/taskdeck/"+slug+
  "\nTARGET_HEAD "+strings.Repeat("a",40)+"\nINDEX_SHA256 "+strings.Repeat("b",64)+"\nCLEAN yes\n"
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

func TestNativeGUIFindLinesReturnsAuthenticatedCompleteSearch(t *testing.T){
 g,f:=newTestNativeGUI()
 status,res:=guiRequestTest(t,g,guiRequest{
  Operation:"file_find_lines",Path:"src/example.go",Literal:"cần tìm",
 },g.origin,g.key)
 if status!=200||!res.OK||res.Used!=1||
  !strings.Contains(res.Text,"FILE_SHA256 "+strings.Repeat("a",64))||
  !strings.Contains(res.Text,"2: matched cần tìm")||
  len(f.calls)!=1||f.calls[0]!="find:src/example.go:cần tìm"{
  t.Fatalf("trusted GUI refused complete exact literal search: %d %+v %v",status,res,f.calls)
 }
 if !strings.Contains(res.Text,"TERM_SHA256 "){
  t.Fatal("missing host term digest binding")
 }
}

func TestNativeGUIFindLinesRejectsUnsafeInputBeforeE2EE(t *testing.T){
 g,f:=newTestNativeGUI()
 invalid:=[]guiRequest{
  {Operation:"file_find_lines",Path:"src/file.go",Literal:""},
  {Operation:"file_find_lines",Path:"src/file.go",Literal:" leading"},
  {Operation:"file_find_lines",Path:"src/file.go",Literal:"\n"},
  {Operation:"file_find_lines",Path:"src/file.go",Literal:"\u202Eoverride"},
  {Operation:"file_find_lines",Path:"src/file.go",Literal:strings.Repeat("x",97)},
  {Operation:"file_find_lines",Path:"../../private/file",Literal:"search"},
  {Operation:"file_find_lines",Path:".git/config",Literal:"search"},
  {Operation:"file_find_lines",Path:"src/.git/config",Literal:"search"},
  {Operation:"file_find_lines",Path:"src/file.go",Literal:"search",Offset:1},
  {Operation:"file_find_lines",Path:"src/file.go",Literal:"search",TaskID:1},
  {Operation:"file_find_lines",Path:"src/file.go",Literal:"search",Mode:"run"},
  {Operation:"file_find_lines",Path:"src/file.go",Literal:"search",Confirmation:"RUN TASK 1"},
  {Operation:"git_status",Literal:"search"},
  {Operation:"task_review",TaskID:1,Mode:"run",Literal:"search"},
 }
 for _,req:=range invalid{
  status,res:=guiRequestTest(t,g,req,g.origin,g.key)
  if status!=400||res.OK||res.Used!=0||len(f.calls)!=0{
   t.Fatalf("untrusted browser search bypassed local gate: %+v %d %+v %v",req,status,res,f.calls)
  }
 }
}

func TestNativeGUIFindLinesRejectsTruncatedMismatchedAndFakeResults(t *testing.T){
 for _,variant:=range []string{
  "missing-metadata","wrong-term","truncated","injected-line","fake-count",
 }{
  t.Run(variant,func(t *testing.T){
   g,f:=newTestNativeGUI()
   digest:=sha256.Sum256([]byte("alpha"))
   f.findOverride="FILE_SHA256 "+strings.Repeat("a",64)+
    "\nTERM_SHA256 "+hex.EncodeToString(digest[:])+
    "\nTOTAL_LINES 3\nMATCHES 1\n-- COMPLETE MATCHING LINES --\n2: alpha\n"
   switch variant{
   case "missing-metadata":f.findOverride="2: alpha\n"
   case "wrong-term":f.findOverride=strings.Replace(f.findOverride,
    hex.EncodeToString(digest[:]),strings.Repeat("b",64),1)
   case "truncated":f.findOverride=strings.TrimSuffix(f.findOverride,"\n")
   case "injected-line":f.findOverride=strings.Replace(f.findOverride,"2: alpha","2: no-match",1)
   case "fake-count":f.findOverride=strings.Replace(f.findOverride,"MATCHES 1","MATCHES 2",1)
   }
   status,res:=guiRequestTest(t,g,guiRequest{Operation:"file_find_lines",
    Path:"source.txt",Literal:"alpha"},g.origin,g.key)
   if status!=400||res.OK||res.Text!=""||res.Used!=1||len(f.calls)!=1{
    t.Fatalf("malformed authenticated search was displayed: %s %d %+v",variant,status,res)
   }
  })
 }
}

func TestNativeGUIFindLinesCannotBeTriggeredByPHPOrigin(t *testing.T){
 g,f:=newTestNativeGUI()
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"file_find_lines",
  Path:"notes.txt",Literal:"private"},"https://php-attacker.invalid",g.key)
 if status!=403||res.OK||len(f.calls)!=0{
  t.Fatalf("compromised PHP initiated local workspace search: %+v",res)
 }
}
