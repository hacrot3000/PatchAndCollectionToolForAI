package main

import (
 "bytes"
 "context"
 "crypto/rand"
 "crypto/subtle"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/url"
 "os/exec"
 "strconv"
 "strings"
 "unicode/utf8"
 "runtime"
 "sync"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotelimits"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// The GUI is bundled in the independently verified native client, NEVER
// loaded from a potentially compromised PHP Portal. Only fixed encrypted
// host-authorized operations are available; task.run requires host opt-in.
type trustedGUIActions interface {
 Ping(context.Context) error
 Features(context.Context) (remotetransport.RemoteFeatures,error)
 GitStatus(context.Context) (string,error)
 GitLog(context.Context) (string,error)
 GitBranches(context.Context) (string,error)
 GitRemotes(context.Context) (string,error)
 GitTrackingStart(context.Context) (remotetransport.RemoteGitTrackingPage,error)
 GitTrackingNext(context.Context) (remotetransport.RemoteGitTrackingPage,error)
 GitBranchCreate(context.Context) (remotetransport.RemoteGitBranchCreateResult,error)
 GitDiff(context.Context) (string,error)
 GitPatchStart(context.Context) (remotetransport.RemoteGitPatchPage,error)
 GitPatchNext(context.Context) (remotetransport.RemoteGitPatchPage,error)
 GitPull(context.Context) (remotetransport.RemoteGitPullResult,error)
 GitPullReview(context.Context) (remotetransport.RemoteGitPullReviewResult,error)
 GitCommit(context.Context,string) (remotetransport.RemoteGitCommitResult,error)
 GitPush(context.Context) (remotetransport.RemoteGitPushResult,error)
 GitFetch(context.Context) (remotetransport.RemoteGitFetchResult,error)
 GitStageTracked(context.Context) (remotetransport.RemoteGitIndexResult,error)
 GitUnstageAll(context.Context) (remotetransport.RemoteGitIndexResult,error)
 GitStageFile(context.Context,int,string) (remotetransport.RemoteGitIndexResult,error)
 GitUnstageFile(context.Context,int,string) (remotetransport.RemoteGitIndexResult,error)
 TerminalOpen(context.Context) (remotetransport.RemoteTerminalProgress,error)
 TerminalPoll(context.Context) (remotetransport.RemoteTerminalProgress,error)
 TerminalWrite(context.Context,string) (remotetransport.RemoteTerminalProgress,error)
 TerminalResize(context.Context,int,int) (remotetransport.RemoteTerminalProgress,error)
 TerminalClose(context.Context) (remotetransport.RemoteTerminalProgress,error)
 ProjectInfo(context.Context) (remotetransport.RemoteProjectInfo,error)
 TasksPreview(context.Context) (remotetransport.RemoteTasksPreview,error)
 TasksPreviewNext(context.Context) (remotetransport.RemoteTasksPreview,error)
 InspectTask(context.Context,int) (remotetransport.RemoteTaskInspect,error)
 RunTask(context.Context,int) (remotetransport.RemoteTaskRun,error)
 RunTaskChoice(context.Context,int,int) (remotetransport.RemoteTaskRun,error)
 RunTaskText(context.Context,int,string) (remotetransport.RemoteTaskRun,error)
 StartLongTask(context.Context,int,int,string) (remotetransport.RemoteLongTaskProgress,error)
 PollLongTask(context.Context,int) (remotetransport.RemoteLongTaskProgress,error)
 CancelLongTask(context.Context,int) (remotetransport.RemoteLongTaskProgress,error)
 ClearLongTask(context.Context,int) (remotetransport.RemoteLongTaskProgress,error)
 // Close poisons the authenticated AEAD channel: closing the GUI must
 // invalidate a running E2EE request, not merely the local HTTP listener.
 Close()
}

const trustedGUIMaxActions = remotelimits.MaxReadOnlyAppRounds // Matches Relay by construction.
const trustedGUIBodyLimit = 1024
const trustedGUIMaxLifetime = 5 * time.Minute

// Native login verified these values against an independently pinned Agent
// and an approved TaskDeck account; PHP never supplies this UI identity.
type trustedGUITarget struct {
 DeviceID string
 Username string
}

func (t trustedGUITarget) valid()bool{
 if !devicePattern.MatchString(t.DeviceID) || len(t.Username)==0 ||
  len(t.Username)>128 || !utf8.ValidString(t.Username){return false}
 for _,r:=range t.Username{
  if r<32 || r==127{return false}
 }
 return strings.TrimSpace(t.Username)==t.Username
}

type trustedGUI struct {
 actions trustedGUIActions
 origin,token string
 target trustedGUITarget
 closeFn func()
 mu sync.Mutex
 remaining int
 active,closed bool
 startedAt time.Time
 lifecycleCtx context.Context
 cancelLifecycle context.CancelFunc
 closeOnce sync.Once
 closeActionOnce sync.Once
}

func newTrustedGUI(origin,token string,actions trustedGUIActions,closeFn func())(*trustedGUI,error){
 u,err:=url.Parse(origin)
 if err!=nil{return nil,errors.New("invalid local GUI URL")}
 port,portErr:=strconv.Atoi(u.Port())
 if portErr!=nil||port<1||port>65535{return nil,errors.New("invalid local GUI TCP port")}
 if u.Scheme!="http" || u.Hostname()!="127.0.0.1" ||
  u.Host!="127.0.0.1:"+u.Port() || u.Path!="" ||
  u.RawQuery!="" || u.Fragment!="" || u.User!=nil ||
  len(token)!=64 || actions==nil || closeFn==nil {
  return nil,errors.New("untrusted local GUI configuration")
 }
 key,err:=hex.DecodeString(token)
 if err!=nil || len(key)!=32 || hex.EncodeToString(key)!=token {
  return nil,errors.New("invalid local GUI capability")
 }
 lifecycleCtx,cancelLifecycle:=context.WithCancel(context.Background())
 return &trustedGUI{actions:actions,origin:origin,token:token,closeFn:closeFn,
  startedAt:time.Now(),lifecycleCtx:lifecycleCtx,cancelLifecycle:cancelLifecycle,
  remaining:trustedGUIMaxActions},nil
}

func (ui *trustedGUI) closeAuthenticatedAction(){
 ui.closeActionOnce.Do(func(){ui.actions.Close()})
}

// poison closes the trusted AEAD channel, but keeps a local-only endpoint
// for the user to press Disconnect after a denied/revoked operation.
func (ui *trustedGUI) poison(){
 ui.mu.Lock()
 ui.closed=true
 ui.mu.Unlock()
 ui.closeAuthenticatedAction()
}

func (ui *trustedGUI) stop(){
 ui.mu.Lock()
 ui.closed=true
 ui.mu.Unlock()
 ui.cancelLifecycle()
 ui.closeAuthenticatedAction()
 ui.closeOnce.Do(ui.closeFn)
}

func (ui *trustedGUI) expired()bool{
 ui.mu.Lock()
 expired:=time.Since(ui.startedAt)>=trustedGUIMaxLifetime
 ui.mu.Unlock()
 if expired{ui.poison()}
 return expired
}

// No user-supplied command, URL, path or body is accepted. This route
// carries exactly one canonical positive host-issued Task.ID in decimal.
func selectedTaskFromPath(path string)(int,bool){
 return selectedTaskIDFromRoute(path,"/api/tasks-inspect/")
}
func selectedRunTaskFromPath(path string)(int,bool){
 return selectedTaskIDFromRoute(path,"/api/tasks-run/")
}
func selectedTextTaskFromPath(path string)(int,bool){
 return selectedTaskIDFromRoute(path,"/api/tasks-run-input/")
}
func selectedStartLongTaskChoiceFromPath(path string)(int,int,bool){
 return selectedChoiceTaskRoute(path,"/api/tasks-start/")
}
func selectedStartLongTaskTextFromPath(path string)(int,bool){
 return selectedTaskIDFromRoute(path,"/api/tasks-start-input/")
}
func selectedLongPollFromPath(path string)(int,bool){
 return selectedTaskIDFromRoute(path,"/api/tasks-poll/")
}
func selectedLongCancelFromPath(path string)(int,bool){
 return selectedTaskIDFromRoute(path,"/api/tasks-cancel/")
}
func selectedLongClearFromPath(path string)(int,bool){
 return selectedTaskIDFromRoute(path,"/api/tasks-clear/")
}
// Optional one-based choice index is not a user-supplied string argument.
// Only the host-owned pickString definition can resolve it.
func selectedRunTaskChoiceFromPath(path string)(int,int,bool){
 return selectedChoiceTaskRoute(path,"/api/tasks-run/")
}
func selectedChoiceTaskRoute(path,prefix string)(int,int,bool){
 if id,ok:=selectedTaskIDFromRoute(path,prefix);ok{return id,0,true}
 if !strings.HasPrefix(path,prefix){return 0,0,false}
 tail:=strings.TrimPrefix(path,prefix)
 slash:=strings.LastIndex(tail,"/")
 if slash<=0{return 0,0,false}
 id,ok:=selectedTaskIDFromRoute(prefix+tail[:slash],prefix)
 choiceRaw:=tail[slash+1:]
 if !ok||len(choiceRaw)==0||len(choiceRaw)>2||choiceRaw[0]=='0'{
  return 0,0,false
 }
 for _,digit:=range choiceRaw {
  if digit<'0'||digit>'9'{return 0,0,false}
 }
 choice,err:=strconv.Atoi(choiceRaw)
 return id,choice,err==nil&&choice>=1&&
  choice<=remotetransport.MaxRemoteTaskCombinedChoices
}
func selectedTaskIDFromRoute(path,prefix string)(int,bool){
 if !strings.HasPrefix(path,prefix){return 0,false}
 raw:=strings.TrimPrefix(path,prefix)
 if len(raw)==0||len(raw)>10||raw[0]=='0'{return 0,false}
 for _,r:=range raw {if r<'0'||r>'9'{return 0,false}}
 id,err:=strconv.Atoi(raw)
 return id,err==nil&&id>0&&id<=0x7fffffff
}

func (ui *trustedGUI) ServeHTTP(w http.ResponseWriter,r *http.Request){
 // DNS rebinding defense: localhost binding alone does not guarantee
 // the request came from the trusted local page.
 host:="127.0.0.1:"+ui.origin[len("http://127.0.0.1:"):]
 w.Header().Set("Cache-Control","no-store, max-age=0")
 w.Header().Set("Pragma","no-cache")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("X-Frame-Options","DENY")
 w.Header().Set("Referrer-Policy","no-referrer")
 w.Header().Set("Cross-Origin-Resource-Policy","same-origin")
 w.Header().Set("Cross-Origin-Opener-Policy","same-origin")
 w.Header().Set("Content-Security-Policy",
  "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; "+
  "img-src 'none'; base-uri 'none'; form-action 'none'; object-src 'none'; frame-ancestors 'none'")
 if r.Host!=host || r.URL.Host!="" || r.URL.RawQuery!="" ||
  r.Header.Get("Sec-Fetch-Site")=="cross-site" {
  http.Error(w,"forbidden",http.StatusForbidden);return
 }
 switch r.URL.Path {
 case "/":
  if r.Method!=http.MethodGet{http.Error(w,"method not allowed",405);return}
  w.Header().Set("Content-Type","text/html; charset=utf-8")
  _,_=io.WriteString(w,trustedGUIHTML)
 case "/app.js":
  if r.Method!=http.MethodGet{http.Error(w,"method not allowed",405);return}
  w.Header().Set("Content-Type","text/javascript; charset=utf-8")
  _,_=io.WriteString(w,trustedGUIJavaScript)
 case "/style.css":
  if r.Method!=http.MethodGet{http.Error(w,"method not allowed",405);return}
  w.Header().Set("Content-Type","text/css; charset=utf-8")
  _,_=io.WriteString(w,trustedGUICSS)
 case "/api/git-switch-review","/api/git-switch-same-head","/api/git-pull-changes","/api/files-root","/api/files-root-next","/api/git-patch-cached-start","/api/git-patch-cached-next","/api/ping","/api/features","/api/git-status","/api/git-log","/api/git-branches","/api/git-remotes","/api/git-tracking-start","/api/git-tracking-next","/api/git-branch-create","/api/git-diff","/api/git-patch-start","/api/git-patch-next","/api/git-pull","/api/git-pull-review","/api/git-commit","/api/git-push","/api/git-fetch","/api/git-stage-tracked","/api/git-unstage-all","/api/git-stage-file","/api/git-unstage-file","/api/terminal-open","/api/terminal-poll","/api/terminal-write","/api/terminal-resize","/api/terminal-close","/api/project-info","/api/tasks-preview","/api/tasks-preview-next","/api/close","/api/session":
  ui.action(w,r)
 default:
  _,inspect:=selectedTaskFromPath(r.URL.Path)
  _,_,run:=selectedRunTaskChoiceFromPath(r.URL.Path)
  _,textInput:=selectedTextTaskFromPath(r.URL.Path)
  _,_,longStart:=selectedStartLongTaskChoiceFromPath(r.URL.Path)
  _,longText:=selectedStartLongTaskTextFromPath(r.URL.Path)
  _,longPoll:=selectedLongPollFromPath(r.URL.Path)
  _,longCancel:=selectedLongCancelFromPath(r.URL.Path)
  _,longClear:=selectedLongClearFromPath(r.URL.Path)
  if inspect||run||textInput||longStart||longText||
   longPoll||longCancel||longClear{
   ui.action(w,r)
  }else{http.NotFound(w,r)}
 }
}

func (ui *trustedGUI) authorizedPost(r *http.Request)bool{
 if r.Method!=http.MethodPost || r.Header.Get("Origin")!=ui.origin {
  return false
 }
 _,textInput:=selectedTextTaskFromPath(r.URL.Path)
 _,longText:=selectedStartLongTaskTextFromPath(r.URL.Path)
 expectedType:=""
 if textInput||longText||r.URL.Path=="/api/git-commit"||
  r.URL.Path=="/api/git-stage-file"||r.URL.Path=="/api/git-unstage-file"||
  r.URL.Path=="/api/terminal-write"||r.URL.Path=="/api/terminal-resize"{
  expectedType="application/json"
 }
 if r.Header.Get("Content-Type")!=expectedType{return false}
 raw:=r.Header.Get("X-TaskDeck-Local-Capability")
 want,_:=hex.DecodeString(ui.token)
 got,err:=hex.DecodeString(raw)
 return err==nil && len(got)==32 && subtle.ConstantTimeCompare(want,got)==1
}

// Only one exact non-secret argv token is allowed in a native-local body.
// Reject duplicate keys, unknown keys, null, controls, arbitrary JSON
// objects and unbounded values before any E2EE call is attempted.
func decodeTrustedGUIRemoteInput(data []byte)(string,bool){
 if len(data)==0||len(data)>trustedGUIBodyLimit{return "",false}
 decoder:=json.NewDecoder(bytes.NewReader(data))
 start,err:=decoder.Token()
 if err!=nil||start!=json.Delim('{'){return "",false}
 key,err:=decoder.Token()
 if err!=nil||key!="value"{return "",false}
 var value string
 if decoder.Decode(&value)!=nil||decoder.More(){return "",false}
 end,err:=decoder.Token()
 if err!=nil||end!=json.Delim('}'){return "",false}
 if _,err:=decoder.Token();err!=io.EOF{return "",false}
 return value,remotetransport.ValidRemoteTaskPromptValue(value)
}

// A Git commit message is the sole client-provided argument. Parse exactly
// one JSON string; duplicate or unknown fields are never authoritative.
func decodeTrustedGUIGitCommitMessage(data []byte)(string,bool){
 if len(data)==0||len(data)>trustedGUIBodyLimit{return "",false}
 decoder:=json.NewDecoder(bytes.NewReader(data))
 start,err:=decoder.Token()
 if err!=nil||start!=json.Delim('{'){return "",false}
 key,err:=decoder.Token()
 if err!=nil||key!="message"{return "",false}
 var message string
 if decoder.Decode(&message)!=nil||decoder.More(){return "",false}
 end,err:=decoder.Token()
 if err!=nil||end!=json.Delim('}'){return "",false}
 if _,err:=decoder.Token();err!=io.EOF{return "",false}
 return message,remotetransport.ValidRemoteGitCommitMessage(message)
}

// Git file input is a status-snapshot row, never a free-form filename.
// Only exactly two typed keys are accepted in this native-local request.
func decodeTrustedGUIGitFileSelection(data []byte)(int,string,bool){
 if len(data)==0||len(data)>trustedGUIBodyLimit{return 0,"",false}
 dec:=json.NewDecoder(bytes.NewReader(data))
 first,err:=dec.Token()
 if err!=nil||first!=json.Delim('{'){return 0,"",false}
 key,err:=dec.Token()
 if err!=nil||key!="index"{return 0,"",false}
 var index int
 if dec.Decode(&index)!=nil{return 0,"",false}
 key,err=dec.Token()
 if err!=nil||key!="snapshot"{return 0,"",false}
 var snapshot string
 if dec.Decode(&snapshot)!=nil||dec.More(){return 0,"",false}
 last,err:=dec.Token()
 if err!=nil||last!=json.Delim('}'){return 0,"",false}
 if _,err:=dec.Token();err!=io.EOF{return 0,"",false}
 return index,snapshot,remotetransport.ValidRemoteGitFileSelection(index,snapshot)
}

// Terminal input is a bounded, deliberately typed PTY byte stream. It is
// never passed to exec.Command as a selected program or path.
func decodeTrustedGUITerminalInput(data []byte)(string,bool){
 if len(data)==0||len(data)>trustedGUIBodyLimit{return "",false}
 dec:=json.NewDecoder(bytes.NewReader(data))
 first,err:=dec.Token()
 if err!=nil||first!=json.Delim('{'){return "",false}
 key,err:=dec.Token()
 if err!=nil||key!="input"{return "",false}
 var input string
 if dec.Decode(&input)!=nil||dec.More(){return "",false}
 last,err:=dec.Token()
 if err!=nil||last!=json.Delim('}'){return "",false}
 if _,err:=dec.Token();err!=io.EOF{return "",false}
 return input,remotetransport.ValidRemoteTerminalInput(input)
}

// Exact keys and integer bounds prevent a compromised localhost webpage
// from injecting arbitrary PTY dimensions, JSON keys or terminal commands.
func decodeTrustedGUITerminalSize(data []byte)(int,int,bool){
 if len(data)==0||len(data)>trustedGUIBodyLimit{return 0,0,false}
 dec:=json.NewDecoder(bytes.NewReader(data))
 first,err:=dec.Token()
 if err!=nil||first!=json.Delim('{'){return 0,0,false}
 k,err:=dec.Token()
 if err!=nil||k!="cols"{return 0,0,false}
 var cols int
 if dec.Decode(&cols)!=nil{return 0,0,false}
 k,err=dec.Token()
 if err!=nil||k!="rows"{return 0,0,false}
 var rows int
 if dec.Decode(&rows)!=nil||dec.More(){return 0,0,false}
 last,err:=dec.Token()
 if err!=nil||last!=json.Delim('}'){return 0,0,false}
 if _,err:=dec.Token();err!=io.EOF{return 0,0,false}
 return cols,rows,remotetransport.ValidRemoteTerminalSize(cols,rows)
}

func (ui *trustedGUI) action(w http.ResponseWriter,r *http.Request){
 if !ui.authorizedPost(r){http.Error(w,"forbidden",403);return}
 data,err:=io.ReadAll(io.LimitReader(r.Body,trustedGUIBodyLimit+1))
 _,textInput:=selectedTextTaskFromPath(r.URL.Path)
 _,longText:=selectedStartLongTaskTextFromPath(r.URL.Path)
 gitCommit:=r.URL.Path=="/api/git-commit"
 gitFile:=r.URL.Path=="/api/git-stage-file"||r.URL.Path=="/api/git-unstage-file"
 termWrite:=r.URL.Path=="/api/terminal-write"
 termResize:=r.URL.Path=="/api/terminal-resize"
 if err!=nil||len(data)>trustedGUIBodyLimit||
  (!textInput&&!longText&&!gitCommit&&!gitFile&&!termWrite&&!termResize&&len(data)!=0) {
  http.Error(w,"unexpected request body",400);return
 }
 var gitFileIndex int
 var gitFileSnapshot string
 if gitFile{
  var valid bool
  gitFileIndex,gitFileSnapshot,valid=decodeTrustedGUIGitFileSelection(data)
  if !valid{http.Error(w,"invalid status snapshot selection",400);return}
 }
 var commitMessage string
 if gitCommit{
  var valid bool
  commitMessage,valid=decodeTrustedGUIGitCommitMessage(data)
  if !valid{http.Error(w,"invalid commit message",400);return}
 }
 var terminalInput string
 if termWrite{
  var valid bool
  terminalInput,valid=decodeTrustedGUITerminalInput(data)
  if !valid{http.Error(w,"invalid bounded terminal input",400);return}
 }
 var terminalCols,terminalRows int
 if termResize{
  var valid bool
  terminalCols,terminalRows,valid=decodeTrustedGUITerminalSize(data)
  if !valid{http.Error(w,"invalid terminal size",400);return}
 }
 var selectedValue string
 if textInput||longText{
  var valid bool
  selectedValue,valid=decodeTrustedGUIRemoteInput(data)
  if !valid{http.Error(w,"invalid bounded input",400);return}
 }
 w.Header().Set("Content-Type","application/json; charset=utf-8")
 if r.URL.Path=="/api/close" {
  ui.stop()
  _=json.NewEncoder(w).Encode(map[string]any{"ok":true,"closed":true})
  return
 }
 if ui.expired(){
  http.Error(w,"native E2EE session expired",http.StatusGone);return
 }
 if r.URL.Path=="/api/session" {
  ui.mu.Lock()
  remaining:=ui.remaining
  active:=ui.active
  closed:=ui.closed
  until:=time.Until(ui.startedAt.Add(trustedGUIMaxLifetime))
  ui.mu.Unlock()
  if until<0 {until=0}
  _=json.NewEncoder(w).Encode(map[string]any{
   "ok":true,"closed":closed,"active":active,
   "remaining":remaining,"expires_in_seconds":int(until.Seconds()),
   "device_id":ui.target.DeviceID,"username":ui.target.Username,
  })
  return
 }
 ui.mu.Lock()
 if ui.closed||ui.remaining==0 {
  ui.mu.Unlock()
  http.Error(w,"session closed or read-only action quota reached",410);return
 }
 if ui.active{
  ui.mu.Unlock()
  http.Error(w,"another operation is active",409);return
 }
 ui.active=true
 ui.remaining--
 remaining:=ui.remaining
 ui.mu.Unlock()
 defer func(){ui.mu.Lock();ui.active=false;ui.mu.Unlock()}()
 ctx,cancel:=context.WithTimeout(r.Context(),16*time.Second)
 started:=time.Now()
 stopOnClose:=context.AfterFunc(ui.lifecycleCtx,cancel)
 defer func(){stopOnClose();cancel()}()
 if r.URL.Path=="/api/ping"{
  err=ui.actions.Ping(ctx)
  if err==nil && ctx.Err()==nil {
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"message":"TaskDeck Agent đã xác thực qua E2EE.","remaining":remaining,
     "round_trip_ms":time.Since(started).Milliseconds(),
    })
    return
   }
  }
 }else if r.URL.Path=="/api/features" {
  var flags remotetransport.RemoteFeatures
  flags,err=ui.actions.Features(ctx)
  if err==nil && ctx.Err()==nil {
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"session_ping":flags.SessionPing,
     "project_info":flags.ProjectInfo,"files_root":flags.FilesRoot,
     "tasks_preview":flags.TasksPreview,"task_run":flags.TaskRun,
     "task_long":flags.TaskLong,"git_status":flags.GitStatus,"git_log":flags.GitLog,"git_branches":flags.GitBranches,"git_branch_create":flags.GitBranchCreate,"git_diff":flags.GitDiff,"git_patch":flags.GitPatch,"git_pull":flags.GitPull,
     "git_commit":flags.GitCommit,"git_push":flags.GitPush,"git_fetch":flags.GitFetch,
     "git_stage_tracked":flags.GitStageTracked,"git_unstage_all":flags.GitUnstageAll,
     "git_stage_file":flags.GitStageFile,"git_unstage_file":flags.GitUnstageFile,
     "terminal":flags.Terminal,
     "remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-status" {
  var status string
  status,err=ui.actions.GitStatus(ctx)
  if err==nil && ctx.Err()==nil {
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"status":status,"remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-switch-review" {
  if actions,ok:=ui.actions.(interface{
   GitSwitchReview(context.Context)(remotetransport.RemoteGitSwitchResult,error)
  });ok{
   var plan remotetransport.RemoteGitSwitchResult
   plan,err=actions.GitSwitchReview(ctx)
   if err==nil&&ctx.Err()==nil{
    ui.mu.Lock();live:=!ui.closed;ui.mu.Unlock()
    if live{
     _=json.NewEncoder(w).Encode(map[string]any{
      "ok":true,"from":plan.From,"branch":plan.Branch,"head":plan.Head,
      "remote_control_available":false,"remaining":remaining,
     })
     return
    }
   }
  }else{err=remotetransport.ErrRemoteActionDenied}
 }else if r.URL.Path=="/api/git-switch-same-head" {
  if actions,ok:=ui.actions.(interface{
   GitSwitchSameHead(context.Context)(remotetransport.RemoteGitSwitchResult,error)
  });ok{
   var result remotetransport.RemoteGitSwitchResult
   result,err=actions.GitSwitchSameHead(ctx)
   if err==nil&&ctx.Err()==nil{
    ui.mu.Lock();live:=!ui.closed;ui.mu.Unlock()
    if live{
     _=json.NewEncoder(w).Encode(map[string]any{
      "ok":true,"from":result.From,"branch":result.Branch,"head":result.Head,
      "remote_control_available":false,"remaining":remaining,
     })
     return
    }
   }
  }else{err=remotetransport.ErrRemoteActionDenied}
 }else if r.URL.Path=="/api/git-branch-create" {
  var result remotetransport.RemoteGitBranchCreateResult
  result,err=ui.actions.GitBranchCreate(ctx)
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"branch":result.Branch,"head":result.Head,
     "remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-tracking-start" || r.URL.Path=="/api/git-tracking-next" {
  var page remotetransport.RemoteGitTrackingPage
  if r.URL.Path=="/api/git-tracking-start"{page,err=ui.actions.GitTrackingStart(ctx)}else{page,err=ui.actions.GitTrackingNext(ctx)}
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock();stillOpen:=!ui.closed;ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{"ok":true,"page":page.Page,"refs":page.Refs,"more":page.More,
     "remote_control_available":false,"remaining":remaining})
    return
   }
  }
 }else if r.URL.Path=="/api/git-remotes" {
  names,readErr:=ui.actions.GitRemotes(ctx)
  err=readErr
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock();stillOpen:=!ui.closed;ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"remotes":names,"remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-branches" {
  var branches string
  branches,err=ui.actions.GitBranches(ctx)
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"branches":branches,"remote_control_available":false,
     "remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-log" {
  var log string
  log,err=ui.actions.GitLog(ctx)
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"log":log,"remote_control_available":false,
     "remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-patch-start" ||
  r.URL.Path=="/api/git-patch-next" ||
  r.URL.Path=="/api/git-patch-cached-start" ||
  r.URL.Path=="/api/git-patch-cached-next" {
  var page remotetransport.RemoteGitPatchPage
  if r.URL.Path=="/api/git-patch-start"{
   page,err=ui.actions.GitPatchStart(ctx)
  }else if r.URL.Path=="/api/git-patch-next"{
   page,err=ui.actions.GitPatchNext(ctx)
  }else if cached,ok:=ui.actions.(interface{
   GitPatchCachedStart(context.Context)(remotetransport.RemoteGitPatchPage,error)
   GitPatchCachedNext(context.Context)(remotetransport.RemoteGitPatchPage,error)
  });ok{
   if r.URL.Path=="/api/git-patch-cached-start"{page,err=cached.GitPatchCachedStart(ctx)
   }else{page,err=cached.GitPatchCachedNext(ctx)}
  }else{err=remotetransport.ErrRemoteActionDenied}
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"page":page.Page,"patch":page.Patch,
     "more":page.More,"remote_control_available":false,
     "remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-diff" {
  var summary string
  summary,err=ui.actions.GitDiff(ctx)
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"summary":summary,"remote_control_available":false,
     "remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-pull-changes" {
  if reader,ok:=ui.actions.(interface{
   GitPullChanges(context.Context)(remotetransport.RemoteGitPullChanges,error)
  });ok{
   value,readErr:=reader.GitPullChanges(ctx)
   err=readErr
   if err==nil&&ctx.Err()==nil{
    ui.mu.Lock();stillOpen:=!ui.closed;ui.mu.Unlock()
    if stillOpen{
     _=json.NewEncoder(w).Encode(map[string]any{
      "ok":true,"head":value.Head,"upstream_head":value.UpstreamHead,
      "files":value.Files,"remote_control_available":false,"remaining":remaining,
     })
     return
    }
   }
  }else{err=remotetransport.ErrRemoteActionDenied}
 }else if r.URL.Path=="/api/git-pull-review" {
  result,reviewErr:=ui.actions.GitPullReview(ctx)
  err=reviewErr
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock();stillOpen:=!ui.closed;ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"head":result.Head,"upstream_head":result.UpstreamHead,
     "status":result.Status,"remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-pull" {
  var result remotetransport.RemoteGitPullResult
  result,err=ui.actions.GitPull(ctx)
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"before":result.Before,"after":result.After,
     "updated":result.Updated,"remote_control_available":false,
     "remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-commit" {
  var result remotetransport.RemoteGitCommitResult
  result,err=ui.actions.GitCommit(ctx,commitMessage)
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"before":result.Before,"after":result.After,
     "remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-stage-file" ||
  r.URL.Path=="/api/git-unstage-file" {
  var result remotetransport.RemoteGitIndexResult
  if r.URL.Path=="/api/git-stage-file"{
   result,err=ui.actions.GitStageFile(ctx,gitFileIndex,gitFileSnapshot)
  }else{
   result,err=ui.actions.GitUnstageFile(ctx,gitFileIndex,gitFileSnapshot)
  }
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"operation":result.Operation,"head":result.Head,
     "remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-stage-tracked" ||
  r.URL.Path=="/api/git-unstage-all" {
  var result remotetransport.RemoteGitIndexResult
  if r.URL.Path=="/api/git-stage-tracked"{
   result,err=ui.actions.GitStageTracked(ctx)
  }else{
   result,err=ui.actions.GitUnstageAll(ctx)
  }
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"operation":result.Operation,"head":result.Head,
     "remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-fetch" {
  var result remotetransport.RemoteGitFetchResult
  result,err=ui.actions.GitFetch(ctx)
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"head":result.Head,"before":result.Before,
     "after":result.After,"updated":result.Updated,
     "remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/git-push" {
  var result remotetransport.RemoteGitPushResult
  result,err=ui.actions.GitPush(ctx)
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"head":result.Head,
     "remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/terminal-open"||
  r.URL.Path=="/api/terminal-poll"||
  r.URL.Path=="/api/terminal-write"||
  r.URL.Path=="/api/terminal-resize"||
  r.URL.Path=="/api/terminal-close" {
  var result remotetransport.RemoteTerminalProgress
  switch r.URL.Path{
  case "/api/terminal-open":
   result,err=ui.actions.TerminalOpen(ctx)
  case "/api/terminal-poll":
   result,err=ui.actions.TerminalPoll(ctx)
  case "/api/terminal-write":
   result,err=ui.actions.TerminalWrite(ctx,terminalInput)
  case "/api/terminal-resize":
   result,err=ui.actions.TerminalResize(ctx,terminalCols,terminalRows)
  case "/api/terminal-close":
   result,err=ui.actions.TerminalClose(ctx)
  }
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"state":result.State,"output":result.Output,
     "truncated":result.Truncated,"more":result.More,
     "remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/files-root" || r.URL.Path=="/api/files-root-next" {
  var listing remotetransport.RemoteFilesRoot
  if r.URL.Path=="/api/files-root"{
   if scoped,ok:=ui.actions.(interface{FilesRoot(context.Context)(remotetransport.RemoteFilesRoot,error)});ok{
    listing,err=scoped.FilesRoot(ctx)
   }else{err=remotetransport.ErrRemoteActionDenied}
  }else{
   if scoped,ok:=ui.actions.(interface{FilesRootNext(context.Context)(remotetransport.RemoteFilesRoot,error)});ok{
    listing,err=scoped.FilesRootNext(ctx)
   }else{err=remotetransport.ErrRemoteActionDenied}
  }
  if err==nil&&ctx.Err()==nil{
   ui.mu.Lock();open:=!ui.closed;ui.mu.Unlock()
   if open{
     _=json.NewEncoder(w).Encode(map[string]any{
      "ok":true,"items":listing.Items,"more":listing.More,
      "remote_control_available":false,"remaining":remaining,
     })
     return
   }
  }
 }else if r.URL.Path=="/api/project-info" {
  var info remotetransport.RemoteProjectInfo
  info,err=ui.actions.ProjectInfo(ctx)
  if err==nil && ctx.Err()==nil {
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"project_name":info.ProjectName,
     "workspace_name":info.WorkspaceName,"host_os":info.HostOS,
     "remaining":remaining,
    })
    return
   }
  }
 }else if r.URL.Path=="/api/tasks-preview"||r.URL.Path=="/api/tasks-preview-next" {
  var preview remotetransport.RemoteTasksPreview
  if r.URL.Path=="/api/tasks-preview"{
   preview,err=ui.actions.TasksPreview(ctx)
  }else{
   preview,err=ui.actions.TasksPreviewNext(ctx)
  }
  if err==nil && ctx.Err()==nil {
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"total":preview.Total,"labels":preview.Labels,"ids":preview.IDs,
     "remaining":remaining,
    })
    return
   }
  }
 }else if taskID,choice,selected:=selectedStartLongTaskChoiceFromPath(r.URL.Path);selected {
  result,longErr:=ui.actions.StartLongTask(ctx,taskID,choice,"")
  if ui.writeRemoteLongTaskResponse(w,ctx,taskID,remaining,result,longErr){return}
 }else if taskID,selected:=selectedStartLongTaskTextFromPath(r.URL.Path);selected {
  result,longErr:=ui.actions.StartLongTask(ctx,taskID,0,selectedValue)
  if ui.writeRemoteLongTaskResponse(w,ctx,taskID,remaining,result,longErr){return}
 }else if taskID,selected:=selectedLongPollFromPath(r.URL.Path);selected {
  result,longErr:=ui.actions.PollLongTask(ctx,taskID)
  if ui.writeRemoteLongTaskResponse(w,ctx,taskID,remaining,result,longErr){return}
 }else if taskID,selected:=selectedLongCancelFromPath(r.URL.Path);selected {
  result,longErr:=ui.actions.CancelLongTask(ctx,taskID)
  if ui.writeRemoteLongTaskResponse(w,ctx,taskID,remaining,result,longErr){return}
 }else if taskID,selected:=selectedLongClearFromPath(r.URL.Path);selected {
  result,longErr:=ui.actions.ClearLongTask(ctx,taskID)
  if ui.writeRemoteLongTaskResponse(w,ctx,taskID,remaining,result,longErr){return}
 }else if taskID,selected:=selectedTextTaskFromPath(r.URL.Path);selected {
  var result remotetransport.RemoteTaskRun
  result,err=ui.actions.RunTaskText(ctx,taskID,selectedValue)
  if err==nil && ctx.Err()==nil && result.TaskID==taskID {
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"task_id":taskID,"exit_code":result.ExitCode,
     "output":result.Output,"truncated":result.Truncated,
     "remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if taskID,choice,selected:=selectedRunTaskChoiceFromPath(r.URL.Path);selected {
  var result remotetransport.RemoteTaskRun
  result,err=ui.actions.RunTaskChoice(ctx,taskID,choice)
  if err==nil && ctx.Err()==nil && result.TaskID==taskID {
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"task_id":taskID,"exit_code":result.ExitCode,
     "output":result.Output,"truncated":result.Truncated,
     "remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else if taskID,selected:=selectedTaskFromPath(r.URL.Path);selected {
  var result remotetransport.RemoteTaskInspect
  result,err=ui.actions.InspectTask(ctx,taskID)
  if err==nil && ctx.Err()==nil && result.TaskID==taskID && result.Eligible {
   ui.mu.Lock()
   stillOpen:=!ui.closed
   ui.mu.Unlock()
   if stillOpen{
    _=json.NewEncoder(w).Encode(map[string]any{
     "ok":true,"task_id":taskID,"eligible":true,
     "input_id":result.InputID,"choices":result.Choices,
     "input_type":result.InputType,"input_id_2":result.InputID2,
     "choices_2":result.Choices2,
     "remote_control_available":false,"remaining":remaining,
    })
    return
   }
  }
 }else{
  err=remotetransport.ErrRemoteActionDenied
 }
 // Poisoned or revoked E2EE sessions are never downgraded to local HTTP.
 ui.poison()
 http.Error(w,"TaskDeck denied action or E2EE session expired",403)
}

func (ui *trustedGUI) writeRemoteLongTaskResponse(
 w http.ResponseWriter,ctx context.Context,taskID,remaining int,
 value remotetransport.RemoteLongTaskProgress,err error,
)bool{
 if err!=nil||ctx.Err()!=nil||value.TaskID!=taskID{return false}
 ui.mu.Lock()
 open:=!ui.closed
 ui.mu.Unlock()
 if !open{return false}
 _=json.NewEncoder(w).Encode(map[string]any{
  "ok":true,"task_id":taskID,"status":value.Status,
  "exit_code":value.ExitCode,"output":value.Output,
  "truncated":value.Truncated,"more":value.More,
  "remote_control_available":false,
  "remaining":remaining,
 })
 return true
}

func runTrustedRemoteGUI(ctx context.Context,actions trustedGUIActions,output io.Writer)error{
 return runTrustedRemoteGUIWithOpener(ctx,actions,output,openTrustedGUIBrowser)
}

// Only a successfully authenticated native login can call this with the
// out-of-band pinned Agent ID and the locally verified username.
func runTrustedRemoteGUIForTarget(ctx context.Context,actions trustedGUIActions,
 output io.Writer,target trustedGUITarget)error{
 if !target.valid(){return errors.New("unverified native GUI device/account identity")}
 return runTrustedRemoteGUIWithIdentityAndOpener(ctx,actions,output,target,openTrustedGUIBrowser)
}

// The opener is supplied only by this trusted binary. A test-specific
// no-op lets integration tests exercise a REAL loopback TCP socket without
// launching a user's browser or contacting the Portal.
func runTrustedRemoteGUIWithOpener(ctx context.Context,actions trustedGUIActions,
 output io.Writer,opener func(string))error{
 return runTrustedRemoteGUIWithIdentityAndOpener(ctx,actions,output,trustedGUITarget{},opener)
}

func runTrustedRemoteGUIWithIdentityAndOpener(ctx context.Context,actions trustedGUIActions,
 output io.Writer,target trustedGUITarget,opener func(string))error{
 return runTrustedRemoteGUIWithIdentityAndOpenerForLifetime(
  ctx,actions,output,target,opener,trustedGUIMaxLifetime)
}

// A native deadline, independent of the browser's JavaScript timers or
// localhost HTTP requests, guarantees that an abandoned GUI cannot keep
// the authenticated E2EE channel alive indefinitely. The short lifetime
// override is only used by Go tests; production always uses five minutes.
func runTrustedRemoteGUIWithIdentityAndOpenerForLifetime(
 ctx context.Context,actions trustedGUIActions,
 output io.Writer,target trustedGUITarget,opener func(string),lifetime time.Duration,
)error{
 if lifetime<=0 || lifetime>trustedGUIMaxLifetime {
  return errors.New("untrusted native GUI lifetime")
 }
 if (target.DeviceID!="" || target.Username!="") && !target.valid(){
  return errors.New("unverified native GUI device/account identity")
 }
 if ctx==nil||ctx.Err()!=nil||actions==nil||output==nil{
  return errors.New("trusted GUI session unavailable")
 }
 listener,err:=net.Listen("tcp4","127.0.0.1:0")
 if err!=nil{return err}
 defer listener.Close()
 var secret [32]byte
 if _,err=rand.Read(secret[:]);err!=nil{return err}
 token:=hex.EncodeToString(secret[:])
 origin:="http://"+listener.Addr().String()
 guiCtx,stop:=context.WithTimeout(ctx,lifetime)
 defer stop()
 ui,err:=newTrustedGUI(origin,token,actions,stop)
 if err!=nil{return err}
 ui.target=target
 // Every exit path, including unexpected Serve errors, must destroy
 // the password-bound native action channel exactly once.
 defer ui.stop()
 srv:=&http.Server{
  Handler:ui,ReadHeaderTimeout:3*time.Second,ReadTimeout:10*time.Second,
  WriteTimeout:18*time.Second,IdleTimeout:20*time.Second,
 }
 done:=make(chan error,1)
 go func(){done<-srv.Serve(listener)}()
 // Never expose the 256-bit capability in a browser launch argument:
 // local process listings on shared Linux/macOS hosts may reveal argv.
 // Open a SAFE origin-only URL, then let the user paste the capability
 // from the trusted native CLI into its bundled localhost-only page.
 fmt.Fprintln(output,"TaskDeck GUI tin cậy (chỉ trên máy client):",origin+"/")
 fmt.Fprintln(output,"Mã kích hoạt GUI (chỉ nhập tại địa chỉ localhost ở trên):",token)
 fmt.Fprintln(output,"KHÔNG gửi mã này hoặc mật khẩu TaskDeck vào PHP Portal.")
 fmt.Fprintln(output,"GUI E2EE hỗ trợ đọc và chạy task khi host đã bật quyền riêng; tối đa 16 lượt mỗi phiên. Git ghi và terminal vẫn bị khóa.")
 if opener!=nil {opener(origin+"/")}
 select{
 case <-guiCtx.Done():
 case err=<-done:
  if err!=nil&&!errors.Is(err,http.ErrServerClosed){return err}
 }
 ui.stop()
 closeCtx,cancel:=context.WithTimeout(context.Background(),2*time.Second)
 defer cancel()
 _=srv.Shutdown(closeCtx)
 return nil
}

func openTrustedGUIBrowser(target string){
 var command *exec.Cmd
 switch runtime.GOOS{
 case "darwin":command=exec.Command("open",target)
 case "linux":command=exec.Command("xdg-open",target)
 default:return
 }
 // Optional convenience only; no shell evaluation or remote download.
 _=command.Start()
 if command.Process!=nil{_=command.Process.Release()}
}
