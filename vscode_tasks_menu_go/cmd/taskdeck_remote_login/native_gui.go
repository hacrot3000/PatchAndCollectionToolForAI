package main

// This is an independently distributed, executable-owned GUI: NO HTML, JS,
// credentials, commands or app session are loaded from a potentially hacked
// PHP Portal. Only the pinned-Agent E2EE PingClient crosses the network.
import (
 "context"
 "crypto/rand"
 "crypto/sha256"
 "crypto/subtle"
 "embed"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "os"
 "os/exec"
 "runtime"
 "strconv"
 "strings"
 "sync"
 "time"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/remoterelay"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

//go:embed native_gui.html native_gui.js native_gui.css
var trustedGUIAssets embed.FS

type guiRemoteActions interface {
 ListDirectory(context.Context,string)(string,error)
 ReadFilePreview(context.Context,string)(string,error)
 ReplaceFileText(context.Context,string,string,string)(string,error)
 EditLargeFileLine(context.Context,string,int64,string,string)(string,error)
 InsertLargeFileLine(context.Context,string,int64,string,string)(string,error)
 DeleteLargeFileLine(context.Context,string,int64,string)(string,error)
 ReadFileLines(context.Context,string,int64)(string,error)
 FindFileLines(context.Context,string,string)(string,error)
 GitStatusFull(context.Context)(string,error)
 GitLog(context.Context)(string,error)
 GitBranches(context.Context)(string,error)
 GitRemotes(context.Context)(string,error)
 GitTrackingRefs(context.Context)(string,error)
 GitPullReview(context.Context)(string,error)
 GitPullCachedReview(context.Context)(string,error)
 GitPullApplyCached(context.Context,string)(string,error)
 GitDiff(context.Context,string)(string,error)
 GitStagedDiff(context.Context,string)(string,error)
 GitHeadDiff(context.Context,string)(string,error)
 GitStage(context.Context,string,string)(string,error)
 GitUnstage(context.Context,string,string)(string,error)
 GitCommitReview(context.Context)(string,error)
 GitCommit(context.Context,string,string)(string,error)
 GitBranchReview(context.Context)(string,error)
 GitBranchCreate(context.Context,string,string)(string,error)
 GitSwitchReview(context.Context,string)(string,error)
 GitSwitchSameHead(context.Context,string,string)(string,error)
 GitBranchDeleteReview(context.Context,string)(string,error)
 GitBranchDelete(context.Context,string,string)(string,error)
 ListApprovedTasks(context.Context)(string,error)
 RunTaskReport(context.Context,int)(string,error)
 StartManagedTask(context.Context,int)(string,error)
 StatusManagedTask(context.Context,string)(string,error)
 CancelManagedTask(context.Context,string)(string,error)
 ReadManagedOutput(context.Context,string,int64)(string,error)
}
type guiRequest struct{
 Operation string `json:"operation"`
 Path string `json:"path"`
 TaskID int `json:"task_id"`
 Mode string `json:"mode"`
 Literal string `json:"literal"`
 Message string `json:"message"`
 Content string `json:"content"`
 Confirmation string `json:"confirmation"`
 Handle string `json:"handle"`
 Offset int64 `json:"offset"`
}
type guiResponse struct{
 OK bool `json:"ok"`
 Text string `json:"text,omitempty"`
 Error string `json:"error,omitempty"`
 Used int `json:"used"`
 Max int `json:"max"`
 Confirm string `json:"confirm,omitempty"`
 Line string `json:"line,omitempty"`
}
type guiPendingTask struct{
 id int
 mode string
 label string
 expiry time.Time
}
type guiPendingGit struct{ mode,path,digest,confirm string; expiry time.Time }
type guiPendingFileEdit struct{
 path,original,digest,confirm string
 expiry time.Time
}
type guiPendingLine struct {
 path,mode,sha,original,confirm string
 line int64
 expiry time.Time
}
type trustedGUI struct {
 remote guiRemoteActions
 parent context.Context
 origin string
 key string
 mu sync.Mutex
 used int
 pending guiPendingTask
 pendingGit guiPendingGit
 pendingFile guiPendingFileEdit
 pendingLine guiPendingLine
 closed bool
 done chan struct{}
 closeOnce sync.Once
}
func newTrustedGUI(parent context.Context,remote guiRemoteActions,origin,key string)*trustedGUI{
 return &trustedGUI{remote:remote,parent:parent,origin:origin,key:key,done:make(chan struct{})}
}
func (g *trustedGUI) close(){
 g.closeOnce.Do(func(){
  g.mu.Lock()
  g.closed=true
  g.pending=guiPendingTask{}
  g.pendingGit=guiPendingGit{}
  g.pendingFile=guiPendingFileEdit{}
  g.pendingLine=guiPendingLine{}
  g.mu.Unlock()
  close(g.done)
 })
}
func (g *trustedGUI) response(w http.ResponseWriter,status int,value guiResponse){
 w.Header().Set("Content-Type","application/json; charset=utf-8")
 w.WriteHeader(status)
 _=json.NewEncoder(w).Encode(value)
}
func guiSecurityHeaders(w http.ResponseWriter){
 h:=w.Header()
 h.Set("Cache-Control","no-store, max-age=0")
 h.Set("Pragma","no-cache")
 h.Set("X-Content-Type-Options","nosniff")
 h.Set("X-Frame-Options","DENY")
 h.Set("Referrer-Policy","no-referrer")
 h.Set("Cross-Origin-Resource-Policy","same-origin")
 h.Set("Cross-Origin-Opener-Policy","same-origin")
 h.Set("Content-Security-Policy","default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'; object-src 'none'")
}
func (g *trustedGUI) handler()http.Handler{
 mux:=http.NewServeMux()
 mux.HandleFunc("/",func(w http.ResponseWriter,r *http.Request){
  guiSecurityHeaders(w)
  if r.Method!="GET"||r.URL.Path!="/"{http.NotFound(w,r);return}
  serveGUIAsset(w,r,"native_gui.html","text/html; charset=utf-8")
 })
 mux.HandleFunc("/native_gui.js",func(w http.ResponseWriter,r *http.Request){
  guiSecurityHeaders(w)
  if r.Method!="GET"{http.Error(w,"method not allowed",405);return}
  serveGUIAsset(w,r,"native_gui.js","text/javascript; charset=utf-8")
 })
 mux.HandleFunc("/native_gui.css",func(w http.ResponseWriter,r *http.Request){
  guiSecurityHeaders(w)
  if r.Method!="GET"{http.Error(w,"method not allowed",405);return}
  serveGUIAsset(w,r,"native_gui.css","text/css; charset=utf-8")
 })
 mux.HandleFunc("/api/action",func(w http.ResponseWriter,r *http.Request){
  guiSecurityHeaders(w)
  if r.Method!="POST"||r.Host!=strings.TrimPrefix(g.origin,"http://")||
   r.Header.Get("Origin")!=g.origin||
   r.Header.Get("Content-Type")!="application/json"||
   subtle.ConstantTimeCompare([]byte(r.Header.Get("X-TaskDeck-Local-Key")),[]byte(g.key))!=1{
   g.response(w,403,guiResponse{Error:"Local GUI authorization denied"})
   return
  }
  // No unbounded JSON payload, unknown JSON fields, shell strings, URLs,
  // file upload, generic TaskDeck HTTP proxy or remotely loaded JavaScript.
  // A text-only file edit carries up to 4096 UTF-8 bytes plus a fixed JSON
   // envelope. Still bounded, no upload/URL/command or dynamic script.
   r.Body=http.MaxBytesReader(w,r.Body,8192)
  dec:=json.NewDecoder(r.Body)
  dec.DisallowUnknownFields()
  var input guiRequest
  if dec.Decode(&input)!=nil||dec.Decode(new(any))!=io.EOF{
   g.response(w,400,guiResponse{Error:"Invalid bounded GUI request"})
   return
  }
  reply:=g.dispatch(r.Context(),input)
  code:=200
  if !reply.OK{code=400}
  g.response(w,code,reply)
 })
 mux.HandleFunc("/api/close",func(w http.ResponseWriter,r *http.Request){
  guiSecurityHeaders(w)
  if r.Method!="POST"||r.Host!=strings.TrimPrefix(g.origin,"http://")||
   r.Header.Get("Origin")!=g.origin||
   subtle.ConstantTimeCompare([]byte(r.Header.Get("X-TaskDeck-Local-Key")),[]byte(g.key))!=1{
   http.Error(w,"unauthorized",403);return
  }
  g.close()
  g.response(w,200,guiResponse{OK:true})
 })
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  guiSecurityHeaders(w)
  if r.Host!=strings.TrimPrefix(g.origin,"http://"){
   http.Error(w,"invalid local GUI host",403);return
  }
  mux.ServeHTTP(w,r)
 })
}
func serveGUIAsset(w http.ResponseWriter,r *http.Request,name,mime string){
 content,err:=trustedGUIAssets.ReadFile(name)
 if err!=nil{http.Error(w,"trusted embedded UI unavailable",500);return}
 w.Header().Set("Content-Type",mime)
 _,_=w.Write(content)
}
func (g *trustedGUI) dispatch(ctx context.Context,req guiRequest)guiResponse{
 g.mu.Lock()
 defer g.mu.Unlock()
 fail:=func(msg string)guiResponse{return guiResponse{Error:msg,Used:g.used,Max:remoterelay.MaxAppPingRounds}}
 if g.closed||g.parent==nil||g.parent.Err()!=nil||ctx==nil||ctx.Err()!=nil{
  return fail("Authenticated GUI session closed; sign in again")
 }
 if g.remote==nil{return fail("Native E2EE client unavailable")}
 if g.used>=remoterelay.MaxAppPingRounds{
  return fail("E2EE request limit reached; close and sign in again")
 }
 // Literal search is a separate, read-only capability. No other GUI
 // operation may smuggle a query field into a task/Git/file write.
 if req.Operation!="file_find_lines"&&req.Literal!=""{
  return fail("Search literal only allowed for file_find_lines")
 }
 if req.Operation!="git_write_execute"&&req.Message!=""{
  return fail("Commit message only allowed in reviewed Git write")
 }
 if req.Operation!="file_edit_execute"&&req.Operation!="line_write_execute"&&req.Content!=""{
  return fail("File edit text only permitted in reviewed file write")
 }
 // A reviewed write applies to the very next matching GUI action only.
 // Any unrelated operation (including a read-only one) invalidates all
 // other pending approvals. A stale confirmation cannot survive viewing
 // another file, switching Git context, or checking a managed task.
 if req.Operation!="git_write_review"&&req.Operation!="git_write_execute"{
  g.pendingGit=guiPendingGit{}
 }
 if req.Operation!="file_edit_review"&&req.Operation!="file_edit_execute"{
  g.pendingFile=guiPendingFileEdit{}
 }
 if req.Operation!="line_write_review"&&req.Operation!="line_write_execute"{
  g.pendingLine=guiPendingLine{}
 }
 if req.Operation!="task_review"&&req.Operation!="task_execute"{
  g.pending=guiPendingTask{}
 }
 // The local GUI NEVER accepts arbitrary executable names, Git argv,
 // HTTP methods, env vars, scripts, or a pathname outside this workspace.
 var output string
 var err error
 var path string
 switch req.Operation {
 case "files_list":
  if req.TaskID!=0||req.Handle!=""||req.Offset!=0||req.Confirmation!=""{return fail("Invalid file listing request")}
  path,err=consolePath(".",req.Path,true)
  if err!=nil{return fail("Invalid workspace-relative directory")}
  output,err=g.remote.ListDirectory(ctx,path)
 case "file_view","file_lines":
  if req.TaskID!=0||req.Handle!=""||req.Confirmation!=""{return fail("Invalid file view request")}
  path,err=consolePath(".",req.Path,false)
  if err!=nil{return fail("Invalid workspace-relative file")}
  if req.Operation=="file_view"{
   if req.Offset!=0{return fail("Invalid preview cursor")}
   output,err=g.remote.ReadFilePreview(ctx,path)
  }else{
   if req.Offset<1||req.Offset>2<<20{return fail("Invalid line number")}
   output,err=g.remote.ReadFileLines(ctx,path,req.Offset)
   if err==nil{_,err=consoleLinePageSHA(output,req.Offset)}
  }
 case "file_find_lines":
  // Enforce the exact dedicated, bounded plaintext search contract BEFORE
  // sending any E2EE operation. The host rechecks FilesRead, path and term.
  if req.TaskID!=0||req.Handle!=""||req.Offset!=0||
   req.Confirmation!=""||req.Mode!=""||
   !remotetransport.ValidRemoteFindLiteral(req.Literal){
   return fail("Invalid bounded literal search")
  }
  path,err=consolePath(".",req.Path,false)
  if err!=nil{return fail("Invalid workspace-relative source")}
  for _,component:=range strings.Split(path,"/"){
   if strings.HasPrefix(component,"."){return fail("Hidden or Git metadata sources denied")}
  }
  output,err=g.remote.FindFileLines(ctx,path,req.Literal)
  if err==nil {
   // The Agent response is authenticated, but reject malformed, truncated
   // or mismatched search snapshots instead of displaying partial results.
   _,err=consoleFindLinesMetadata(output,req.Literal)
  }

 case "line_write_review":
  // Read only one complete SHA-pinned source page (<=4KiB response),
  // then accept exactly one typed line operation in a separate E2EE round.
  g.pendingLine=guiPendingLine{}
  if g.used+2>remoterelay.MaxAppPingRounds||req.TaskID!=0||
   req.Handle!=""||req.Confirmation!=""||req.Message!=""||
   req.Literal!=""||req.Content!=""||
   req.Offset<1||req.Offset>2<<20||
   (req.Mode!="edit"&&req.Mode!="insert"&&req.Mode!="delete"){
   return fail("Large file line review requires two E2EE rounds and a typed action")
  }
  path,err=consolePath(".",req.Path,false)
  if err!=nil||path!=req.Path{return fail("Invalid canonical line edit path")}
  for _,part:=range strings.Split(path,"/"){
   if strings.HasPrefix(part,".")||strings.EqualFold(part,".git"){
    return fail("Metadata/hidden file edit denied")
   }
  }
  output,err=g.remote.ReadFileLines(ctx,path,req.Offset)
  if err==nil{
   var sha string
   sha,err=consoleLinePageSHA(output,req.Offset)
   if err==nil{
    chunks:=strings.SplitN(output,"\n",6)
    lineText:=strings.SplitN(chunks[5],"\n",2)[0]
    prefix:=strconv.FormatInt(req.Offset,10)+": "
    if !strings.HasPrefix(lineText,prefix){
     err=errors.New("Reviewed line offset mismatch")
    }else{
     original:=strings.TrimSuffix(strings.TrimPrefix(lineText,prefix),"\r")
     if !remotetransport.ValidRemoteLargeLineText(original){
      err=errors.New("Reviewed line exceeds bounded text policy")
     }else{
      verb:=strings.ToUpper(req.Mode)
      if req.Mode=="edit"{verb="EDIT"}
      if req.Mode=="delete"{verb="DELETE"}
      if req.Mode=="insert"{verb="INSERT"}
      g.pendingLine=guiPendingLine{
       path:path,mode:req.Mode,line:req.Offset,sha:sha,original:original,
       confirm:verb+" LINE "+strconv.FormatInt(req.Offset,10)+" "+sha[:12],
       expiry:time.Now().Add(45*time.Second),
      }
     }
    }
   }
  }
 case "line_write_execute":
  pending:=g.pendingLine
  g.pendingLine=guiPendingLine{} // consume even on invalid confirmation
  if pending.path==""||time.Now().After(pending.expiry)||
   req.Path!=pending.path||req.Mode!=pending.mode||req.Offset!=pending.line||
   req.Confirmation!=pending.confirm||
   req.TaskID!=0||req.Handle!=""||req.Message!=""||req.Literal!=""||
   !remotetransport.ValidRemoteLargeLineText(req.Content)||
   (pending.mode=="delete"&&req.Content!="")||
   (pending.mode=="edit"&&req.Content==pending.original){
   return fail("Reviewed line mutation expired or mismatched")
  }
  switch pending.mode{
  case "edit":
   output,err=g.remote.EditLargeFileLine(ctx,pending.path,pending.line,req.Content,pending.sha)
  case "insert":
   output,err=g.remote.InsertLargeFileLine(ctx,pending.path,pending.line,req.Content,pending.sha)
  case "delete":
   output,err=g.remote.DeleteLargeFileLine(ctx,pending.path,pending.line,pending.sha)
  default:return fail("Line edit operation not allowed")
  }
 case "file_edit_review":
  g.pendingFile=guiPendingFileEdit{}
  if g.used+2>remoterelay.MaxAppPingRounds||
   req.TaskID!=0||req.Handle!=""||req.Offset!=0||
   req.Confirmation!=""||req.Mode!=""||req.Message!=""{
   return fail("File edit requires two E2EE rounds and no extra fields")
  }
  path,err=consolePath(".",req.Path,false)
  if err!=nil||req.Path!=path{return fail("Invalid canonical file-edit path")}
  // No .git metadata edits from remote GUI, even where a host policy
  // happens to allow reading. The host independently enforces path/RBAC.
  for _,component:=range strings.Split(path,"/"){
   if strings.EqualFold(component,".git")||strings.HasPrefix(component,"."){
    return fail("Hidden and Git metadata editing denied")
   }
  }
  output,err=g.remote.ReadFilePreview(ctx,path)
  if err==nil{
   if len(output)>4096||!utf8.ValidString(output)||strings.ContainsRune(output,'\x00'){
    err=errors.New("Untrusted or oversized complete file preview")
   }else{
    h:=sha256.Sum256([]byte(output))
    digest:=hex.EncodeToString(h[:])
    g.pendingFile=guiPendingFileEdit{
     path:path,original:output,digest:digest,
     confirm:"EDIT "+path+" "+digest[:12],
     expiry:time.Now().Add(45*time.Second),
    }
   }
  }
 case "file_edit_execute":
  pending:=g.pendingFile
  g.pendingFile=guiPendingFileEdit{} // one-time, also on failed confirmation
  if pending.path==""||time.Now().After(pending.expiry)||
   req.Path!=pending.path||req.Confirmation!=pending.confirm||
   req.TaskID!=0||req.Handle!=""||req.Offset!=0||
   req.Mode!=""||req.Message!=""||
   len(req.Content)>4096||!utf8.ValidString(req.Content)||
   strings.ContainsRune(req.Content,'\x00')||
   req.Content==pending.original{
   return fail("File review expired, content invalid or confirmation mismatched")
  }
  output,err=g.remote.ReplaceFileText(ctx,pending.path,req.Content,pending.digest)
 case "git_status","git_log","git_branches","git_remotes","git_tracking_refs":
  if req.Path!=""||req.Mode!=""||req.Content!=""||req.Message!=""||req.TaskID!=0||req.Handle!=""||req.Offset!=0||req.Confirmation!=""{
   return fail("Invalid Git metadata request")
  }
  switch req.Operation{
  case "git_status":output,err=g.remote.GitStatusFull(ctx)
  case "git_log":output,err=g.remote.GitLog(ctx)
  case "git_branches":output,err=g.remote.GitBranches(ctx)
  case "git_tracking_refs":
   output,err=g.remote.GitTrackingRefs(ctx)
   if err==nil&&!remotetransport.ValidRemoteGitTrackingRefsReport(output){
    return fail("Authenticated Git tracking refs invalid")
   }
  case "git_remotes":
   output,err=g.remote.GitRemotes(ctx)
   if err==nil&&!remotetransport.ValidRemoteGitRemotesReport(output){
    return fail("Authenticated Git remotes metadata invalid")
   }
  }
 case "git_review":
  // The read-only two-way review must consume TWO separately authenticated
  // E2EE actions and MUST NOT display a partially obtained review. Recheck
  // host authorization and live Portal grant separately for each direction.
  if g.used+2>remoterelay.MaxAppPingRounds||req.TaskID!=0||
   req.Handle!=""||req.Offset!=0||req.Confirmation!=""||
   req.Mode!=""||req.Message!=""||req.Content!=""{
   return fail("Git two-way review requires two E2EE rounds")
  }
  path,err=consolePath(".",req.Path,false)
  if err!=nil||path!=req.Path||strings.HasPrefix(path,"-")||
   strings.ContainsAny(path,"*?[]")||
   strings.HasPrefix(strings.ToLower(path),".git/")||
   strings.Contains(strings.ToLower(path),"/.git/"){
   return fail("Invalid canonical Git review file")
  }
  var unstaged,staged string
  unstaged,err=g.remote.GitDiff(ctx,path)
  if err==nil&&len(unstaged)>remotetransport.RemoteGitDiffMaxBytes{
   err=errors.New("Oversized unstaged diff")
  }
  if err==nil{
   g.used++ // first authenticated E2EE read is consumed
   staged,err=g.remote.GitStagedDiff(ctx,path)
   if err==nil&&len(staged)>remotetransport.RemoteGitDiffMaxBytes{
    err=errors.New("Oversized staged diff")
   }
   if err==nil{
    output="-- WORKTREE / UNSTAGED --\n"+unstaged+
     "\n-- INDEX / STAGED --\n"+staged
   }
  }
 case "git_pull_review":
  // Read-only and OFFLINE: never interpret PHP URLs or Git argv.
  if req.Path!=""||req.TaskID!=0||req.Mode!=""||req.Handle!=""||
   req.Offset!=0||req.Confirmation!=""{return fail("Invalid Pull review request")}
  output,err=g.remote.GitPullReview(ctx)
  if err==nil&&!remotetransport.ValidRemoteGitPullReviewReport(output){
   err=errors.New("Invalid E2EE Pull preflight report")
  }
 case "git_diff","git_staged","git_head":
  if req.TaskID!=0||req.Handle!=""||req.Offset!=0||req.Confirmation!=""{
   return fail("Invalid Git diff request")
  }
  path,err=consolePath(".",req.Path,false)
  if err!=nil{return fail("Invalid workspace-relative Git file")}
  switch req.Operation{
  case "git_diff":output,err=g.remote.GitDiff(ctx,path)
  case "git_staged":output,err=g.remote.GitStagedDiff(ctx,path)
  case "git_head":output,err=g.remote.GitHeadDiff(ctx,path)
  }

 case "git_write_review":
  g.pendingGit=guiPendingGit{}
  if g.used+2>remoterelay.MaxAppPingRounds||req.TaskID!=0||
   req.Handle!=""||req.Offset!=0||req.Confirmation!=""||req.Message!=""{
   return fail("Git write requires two available E2EE rounds and no extra fields")
  }
  review:=guiPendingGit{mode:req.Mode,expiry:time.Now().Add(45*time.Second)}
  switch req.Mode{
  case "stage","unstage":
   path,err=consolePath(".",req.Path,false)
   if err!=nil{return fail("Invalid Git source path")}
   if req.Path!=path||strings.HasPrefix(path,"-")||strings.ContainsAny(path,"*?[]")||
    path==".git"||strings.HasPrefix(path,".git/")||strings.Contains(path,"/.git/"){
    return fail("Git path must be canonical without metadata, patterns or options")
   }
   review.path=path
   if req.Mode=="stage"{
    output,err=g.remote.ReadFilePreview(ctx,path)
    review.confirm="STAGE "+path
   }else{
    output,err=g.remote.GitStagedDiff(ctx,path)
    review.confirm="UNSTAGE "+path
   }
   if err==nil&&req.Mode=="unstage"&&output==""{err=errors.New("nothing staged")}
  case "commit":
   if req.Path!=""{return fail("Git commit review does not take a path")}
   output,err=g.remote.GitCommitReview(ctx)
   if err==nil{
    lines:=strings.SplitN(output,"\n",2)
    if len(lines)!=2||!strings.HasPrefix(lines[0],"HEAD ")||
     !strings.Contains(output,"\nBRANCH refs/heads/")||
     !strings.Contains(output,"\nINDEX_SHA256 ")||
     !strings.Contains(output,"\n-- COMPLETE STAGED DIFF --\n"){
     err=errors.New("incomplete Git commit review")
    }else{
     head:=strings.TrimPrefix(lines[0],"HEAD ")
     if !validGUIHexOID(head){err=errors.New("invalid Git HEAD")}
     if err==nil{review.confirm="COMMIT "+head[:12]}
    }
   }
  case "pull-cached":
   if req.Path!=""{return fail("Cached Pull review never accepts ref or URL")}
   output,err=g.remote.GitPullCachedReview(ctx)
   if err==nil{
    if !remotetransport.ValidRemoteGitPullCachedReviewReport(output){
     err=errors.New("invalid cached Pull review")
    }else{
     lines:=strings.Split(output,"\n")
     head:=strings.TrimPrefix(lines[2],"HEAD ")
     target:=strings.TrimPrefix(lines[4],"UPSTREAM_HEAD ")
     review.confirm="APPLY CACHED FF "+head[:12]+" "+target[:12]
    }
   }
  case "branch-create","branch-switch","branch-delete":
   slug:=req.Path
   if !remotetransport.ValidRemoteTaskDeckBranchSlug(slug){
    return fail("Branch must be a valid TaskDeck ASCII slug")
   }
   review.path=slug
   switch req.Mode{
   case "branch-create":
    output,err=g.remote.GitBranchReview(ctx)
    if err==nil{
     lines:=strings.Split(output,"\n")
     if len(lines)!=3||lines[2]!=""||!strings.HasPrefix(lines[0],"HEAD ")||
      !strings.HasPrefix(lines[1],"BRANCH refs/heads/"){
      err=errors.New("invalid branch-create review")
     }else{
      head:=strings.TrimPrefix(lines[0],"HEAD ")
      if !validGUIHexOID(head){err=errors.New("invalid branch parent")}
      if err==nil{review.confirm="CREATE BRANCH taskdeck/"+slug+" "+head[:12]}
     }
    }
   case "branch-switch":
    output,err=g.remote.GitSwitchReview(ctx,slug)
    if err==nil{
     lines:=strings.Split(output,"\n")
     if len(lines)!=7||lines[6]!=""||
      !strings.HasPrefix(lines[0],"HEAD ")||
      !strings.HasPrefix(lines[1],"BRANCH refs/heads/")||
      lines[2]!="TARGET refs/heads/taskdeck/"+slug||
      !strings.HasPrefix(lines[3],"TARGET_HEAD ")||
      !strings.HasPrefix(lines[4],"INDEX_SHA256 ")||lines[5]!="CLEAN yes"{
      err=errors.New("invalid same-HEAD switch review")
     }else{
      head:=strings.TrimPrefix(lines[0],"HEAD ")
      target:=strings.TrimPrefix(lines[3],"TARGET_HEAD ")
      index:=strings.TrimPrefix(lines[4],"INDEX_SHA256 ")
      if !validGUIHexOID(head)||head!=target||len(index)!=64||
       !validGUIHex64(index)||lines[1]=="BRANCH refs/heads/taskdeck/"+slug{
       err=errors.New("branch switch requires different branch, equal HEAD and clean index")
      }
      if err==nil{review.confirm="SWITCH taskdeck/"+slug+" "+head[:12]}
     }
    }
   case "branch-delete":
    output,err=g.remote.GitBranchDeleteReview(ctx,slug)
    if err==nil{
     var oid string
     oid,err=consoleSafeBranchDeletePreview(output,slug)
     if err==nil{
      if !validGUIHexOID(oid){err=errors.New("invalid reviewed branch oid")}
      if err==nil{review.confirm="DELETE BRANCH taskdeck/"+slug+" "+oid[:12]}
     }
    }
   }
  default:return fail("Unsupported reviewed Git operation")
  }
  // Count malformed authenticated outputs as a consumed E2EE round too.
  if err==nil&&len(output)>remotetransport.RemoteGitDiffMaxBytes{
   err=errors.New("oversized Git review; no partial preview")
  }
  if err==nil{
   digest:=sha256.Sum256([]byte(output))
   review.digest=hex.EncodeToString(digest[:])
   g.pendingGit=review
  }
 case "git_write_execute":
  // A review is one-time even when a bad confirmation is supplied.
  review:=g.pendingGit
  g.pendingGit=guiPendingGit{}
  if req.TaskID!=0||req.Handle!=""||req.Offset!=0||
   review.mode==""||time.Now().After(review.expiry)||
   req.Mode!=review.mode||req.Path!=review.path||
   req.Confirmation!=review.confirm{
   return fail("Git write review missing, expired or confirmation mismatched")
  }
  if req.Mode=="commit"{
   if !remotetransport.ValidRemoteCommitMessage(req.Message){
    return fail("Invalid ASCII commit message")
   }
   output,err=g.remote.GitCommit(ctx,req.Message,review.digest)
  }else{
   if req.Message!=""{return fail("Stage/unstage cannot include commit message")}
   switch req.Mode{
   case "stage":output,err=g.remote.GitStage(ctx,review.path,review.digest)
   case "unstage":output,err=g.remote.GitUnstage(ctx,review.path,review.digest)
   case "branch-create":output,err=g.remote.GitBranchCreate(ctx,review.path,review.digest)
   case "branch-switch":output,err=g.remote.GitSwitchSameHead(ctx,review.path,review.digest)
   case "branch-delete":output,err=g.remote.GitBranchDelete(ctx,review.path,review.digest)
   case "pull-cached":output,err=g.remote.GitPullApplyCached(ctx,review.digest)
   default:return fail("Unsupported Git write mode")
   }
  }
 case "tasks":
  if req.Path!=""||req.TaskID!=0||req.Handle!=""||req.Offset!=0||req.Confirmation!=""{
   return fail("Invalid task list request")
  }
  output,err=g.remote.ListApprovedTasks(ctx)
 case "task_review":
  if req.Path!=""||req.Handle!=""||req.Offset!=0||req.Confirmation!=""||
   req.TaskID<1||req.TaskID>0x7fffffff||
   (req.Mode!="run"&&req.Mode!="start"){
   return fail("Invalid approved task review")
  }
  var catalogue string
  catalogue,err=g.remote.ListApprovedTasks(ctx)
  if err==nil{
   var label string
   label,err=consoleApprovedTaskLabel(catalogue,req.TaskID)
   if err==nil{
    g.pending=guiPendingTask{id:req.TaskID,mode:req.Mode,label:label,expiry:time.Now().Add(45*time.Second)}
    output=fmt.Sprintf("TaskDeck host đã phê duyệt task #%d:\n%s",req.TaskID,label)
   }
  }
 case "task_execute":
  review:=g.pending
  g.pending=guiPendingTask{} // consume before execution, even if denied
  confirmation:="RUN TASK "+strconv.Itoa(req.TaskID)
  if req.Mode=="start"{confirmation="START TASK "+strconv.Itoa(req.TaskID)}
  if req.Path!=""||req.Handle!=""||req.Offset!=0||
   req.TaskID<1||req.TaskID!=review.id||req.Mode!=review.mode||
   time.Now().After(review.expiry)||req.Confirmation!=confirmation{
   return fail("Task review expired or confirmation mismatched")
  }
  if req.Mode=="start"{output,err=g.remote.StartManagedTask(ctx,req.TaskID)
  }else{output,err=g.remote.RunTaskReport(ctx,req.TaskID)}
 case "task_status","task_output","task_cancel":
  if req.Path!=""||req.TaskID!=0||!remotetransport.ValidManagedTaskHandle(req.Handle){
   return fail("Invalid managed task handle")
  }
  if req.Operation=="task_cancel"{
   expected:="CANCEL TASK "+req.Handle[:12]
   if req.Confirmation!=expected||req.Offset!=0{return fail("Cancel confirmation required")}
   output,err=g.remote.CancelManagedTask(ctx,req.Handle)
  }else if req.Operation=="task_status"{
   if req.Confirmation!=""||req.Offset!=0{return fail("Invalid task status request")}
   output,err=g.remote.StatusManagedTask(ctx,req.Handle)
  }else{
   if req.Confirmation!=""||!remotetransport.ValidManagedOutputOffset(req.Offset){
    return fail("Invalid task output cursor")
   }
   output,err=g.remote.ReadManagedOutput(ctx,req.Handle,req.Offset)
  }
 default:return fail("Operation not supported; no shell or generic Git execution")
 }
// Count any attempted remote operation, even errors, so compromised local
 // browser code cannot exceed the exact finite E2EE mailbox allowance.
 g.used++
 if err!=nil{return fail("Host rejected authenticated E2EE action; session may require a new login")}
 resp:=guiResponse{OK:true,Text:output,Used:g.used,Max:remoterelay.MaxAppPingRounds}
 if req.Operation=="file_edit_review"{resp.Confirm=g.pendingFile.confirm}
 if req.Operation=="line_write_review"{
  resp.Confirm=g.pendingLine.confirm
  resp.Line=g.pendingLine.original
 }
 if req.Operation=="git_write_review"{resp.Confirm=g.pendingGit.confirm}
 if req.Operation=="task_review"{
  confirm:="RUN TASK "+strconv.Itoa(req.TaskID)
  if req.Mode=="start"{confirm="START TASK "+strconv.Itoa(req.TaskID)}
  resp.Confirm=confirm
 }
 return resp
}
func validGUIHexOID(value string)bool{
 if len(value)!=40&&len(value)!=64{return false}
 for _,c:=range value{
  if (c<'0'||c>'9')&&(c<'a'||c>'f'){return false}
 }
 return true
}
func validGUIHex64(value string)bool{
 if len(value)!=64{return false}
 for _,c:=range value{
  if (c<'0'||c>'9')&&(c<'a'||c>'f'){return false}
 }
 return true
}
func randomGUIKey()(string,error){
 var key [32]byte
 if _,err:=rand.Read(key[:]);err!=nil{return "",err}
 return hex.EncodeToString(key[:]),nil
}
func openTrustedBrowser(url string)error{
 switch runtime.GOOS{
 case "darwin":return exec.Command("open",url).Start()
 case "linux":return exec.Command("xdg-open",url).Start()
 default:return errors.New("native GUI browser launcher unsupported")
 }
}
func runNativeGUI(ctx context.Context,remote guiRemoteActions,tty *os.File)error{
 if ctx==nil||remote==nil||tty==nil{return errors.New("trusted native E2EE GUI requires local TTY")}
 listen,err:=net.Listen("tcp4","127.0.0.1:0")
 if err!=nil{return err}
 defer listen.Close()
 key,err:=randomGUIKey()
 if err!=nil{return err}
 origin:="http://"+listen.Addr().String()
 gui:=newTrustedGUI(ctx,remote,origin,key)
 srv:=&http.Server{Handler:gui.handler(),ReadHeaderTimeout:3*time.Second,
  IdleTimeout:10*time.Second,MaxHeaderBytes:4096}
 defer func(){
  gui.close()
  shutdown,cancel:=context.WithTimeout(context.Background(),3*time.Second)
  defer cancel()
  _=srv.Shutdown(shutdown)
 }()
 go func(){_ =srv.Serve(listen)}()
 // Only a fragment carries the local UI session secret; it is NOT sent to
 // the local HTTP server with GET, never to PHP or browser referrers. The
 // locally embedded JS immediately removes it from the browser address bar.
 launchURL:=origin+"/#"+key
 fmt.Fprintln(tty,"TaskDeck trusted native GUI (không phải trang Portal):",origin)
 if err:=openTrustedBrowser(launchURL);err!=nil{
  fmt.Fprintln(tty,"Không thể tự mở trình duyệt. Mở URL bên dưới trên chính máy này:")
  fmt.Fprintln(tty,launchURL)
 }
 fmt.Fprintln(tty,"Chọn Đóng phiên trong GUI hoặc Ctrl+C để chấm dứt. Không nhập mật khẩu TaskDeck vào Portal.")
 select{
 case <-ctx.Done():return ctx.Err()
 case <-gui.done:return nil
 }
}
