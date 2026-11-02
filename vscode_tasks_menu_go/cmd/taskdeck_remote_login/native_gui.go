package main

// This is an independently distributed, executable-owned GUI: NO HTML, JS,
// credentials, commands or app session are loaded from a potentially hacked
// PHP Portal. Only the pinned-Agent E2EE PingClient crosses the network.
import (
 "context"
 "crypto/rand"
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

 "bletonfc/vscode_tasks_menu/internal/remoterelay"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

//go:embed native_gui.html native_gui.js native_gui.css
var trustedGUIAssets embed.FS

type guiRemoteActions interface {
 ListDirectory(context.Context,string)(string,error)
 ReadFilePreview(context.Context,string)(string,error)
 ReadFileLines(context.Context,string,int64)(string,error)
 FindFileLines(context.Context,string,string)(string,error)
 GitStatusFull(context.Context)(string,error)
 GitLog(context.Context)(string,error)
 GitBranches(context.Context)(string,error)
 GitDiff(context.Context,string)(string,error)
 GitStagedDiff(context.Context,string)(string,error)
 GitHeadDiff(context.Context,string)(string,error)
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
}
type guiPendingTask struct{
 id int
 mode string
 label string
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
  r.Body=http.MaxBytesReader(w,r.Body,2048)
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
 case "git_status","git_log","git_branches":
  if req.Path!=""||req.TaskID!=0||req.Handle!=""||req.Offset!=0||req.Confirmation!=""{
   return fail("Invalid Git metadata request")
  }
  switch req.Operation{
  case "git_status":output,err=g.remote.GitStatusFull(ctx)
  case "git_log":output,err=g.remote.GitLog(ctx)
  case "git_branches":output,err=g.remote.GitBranches(ctx)
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
 if req.Operation=="task_review"{
  confirm:="RUN TASK "+strconv.Itoa(req.TaskID)
  if req.Mode=="start"{confirm="START TASK "+strconv.Itoa(req.TaskID)}
  resp.Confirm=confirm
 }
 return resp
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
