package server

import (
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "sync"
 "time"
 "unicode"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// No arbitrary commands, paths, arguments, scheduling, stream or PTY.
// Short tasks only until a cancellable long-running task lifecycle exists.
const remoteTaskRunTimeout = 6 * time.Second
const remoteTaskCaptureBytes = 2048

type remoteTaskCapture struct{
 mu sync.Mutex
 data []byte
 truncated bool
}
func (w *remoteTaskCapture)Write(p []byte)(int,error){
 n:=len(p)
 w.mu.Lock()
 defer w.mu.Unlock()
 space:=remoteTaskCaptureBytes-len(w.data)
 if space>0{
  if len(p)<space{space=len(p)}
  w.data=append(w.data,p[:space]...)
 }
 if space<n{w.truncated=true}
 // Always drain both stdout and stderr even beyond the capture limit.
 return n,nil
}
func (w *remoteTaskCapture)snapshot()(string,bool){
 w.mu.Lock()
 defer w.mu.Unlock()
 raw:=strings.ToValidUTF8(string(w.data),"")
 output:=strings.Map(func(r rune)rune{
  if r=='\n'||r=='\t'{return r}
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){return -1}
  return r
 },raw)
 clipped:=w.truncated
 if len(output)>remotetransport.MaxRemoteTaskOutputBytes{
  end:=0
  for i,r:=range output{
   width:=utf8.RuneLen(r)
   if width<0||i+width>remotetransport.MaxRemoteTaskOutputBytes{break}
   end=i+width
  }
  output=output[:end]
  clipped=true
 }
 return output,clipped
}

func (s *Server) auditRemoteTaskRunAttempt(ctx context.Context,session *remoteTaskDeckSession,taskID int)error{
 if ctx==nil||ctx.Err()!=nil||s==nil||s.Identity==nil||session==nil||
  !session.taskPageReady||!session.taskIssuedIDs[taskID]{
  return remotetransport.ErrRemoteActionDenied
 }
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled{return remotetransport.ErrRemoteActionDenied}
 eventID,err:=identity.NewID()
 if err!=nil{return remotetransport.ErrRemoteActionDenied}
 user:=session.userID
 projectID:=project.ID
 err=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:eventID,Timestamp:time.Now().UTC(),UserID:&user,ProjectID:&projectID,
  Action:"remote.action.tasks_run",ResourceType:"tasks",Result:"attempted",
  Details:fmt.Sprintf("{\"transport\":\"e2ee\",\"taskdeck_auth\":true,\"task_id\":%d}",taskID),
 })
 if err!=nil||ctx.Err()!=nil{return remotetransport.ErrRemoteActionDenied}
 return nil
}

// auditRemoteTaskRunFinish is a best-effort terminal outcome record. The
// mandatory attempted audit has already been durably appended before exec.
// A cancelled session cannot use its revoked request context for cleanup
// audit; the host uses a separate short local context. Never include output,
// argument values, commands, passwords or secrets in these records.
func (s *Server) auditRemoteTaskRunFinish(session *remoteTaskDeckSession,
 taskID int,status string,exitCode int,duration time.Duration){
 if s==nil||s.Identity==nil||session==nil{return}
 switch status{
 case "completed","failed","cancelled","timed_out":
 default:return
 }
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Second)
 defer cancel()
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil{return}
 eventID,err:=identity.NewID()
 if err!=nil{return}
 user,projectID:=session.userID,project.ID
 elapsed:=duration.Milliseconds()
 if elapsed<0{elapsed=0}
 _=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:eventID,Timestamp:time.Now().UTC(),UserID:&user,ProjectID:&projectID,
  Action:"remote.action.tasks_run_finished",ResourceType:"tasks",Result:status,
  Details:fmt.Sprintf(
   "{\"transport\":\"e2ee\",\"task_id\":%d,\"exit_code\":%d,\"duration_ms\":%d}",
   taskID,exitCode,elapsed),
 })
}

// runRemoteTaskBounded must only be called from the locally authenticated
// tasks.run permission gate (after BOTH approved client-key proof and
// uncached Portal authorization). The host task itself, not the client,
// supplies command, args, environment and working directory.
func (s *Server)runRemoteTaskBounded(ctx context.Context,session *remoteTaskDeckSession,taskID,choice int)(remotetransport.RemoteTaskRun,error){
 return s.runRemoteTaskBoundedWithInput(ctx,session,taskID,choice,"")
}
func (s *Server)runRemoteTaskBoundedWithInput(ctx context.Context,session *remoteTaskDeckSession,taskID,choice int,textInput string)(remotetransport.RemoteTaskRun,error){
 var denied remotetransport.RemoteTaskRun
 if s==nil||ctx==nil||ctx.Err()!=nil||session==nil||
  // Windows currently cannot reliably terminate the entire subprocess
  // tree using the existing TaskDeck group helper. Fail closed until a
  // Job Object-based process-tree supervisor is reviewed.
  runtime.GOOS=="windows"||
  os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_RUN")!="1"||
  !session.taskPageReady||!session.taskIssuedIDs[taskID]{
  return denied,remotetransport.ErrRemoteActionDenied
 }
 // A short task occupies the host's execution slot until its bounded
 // process group exits. Serialize against TerminalOpen; do not allow a
 // PTY and a task to run concurrently in the same E2EE session.
 session.terminalMu.Lock()
 defer session.terminalMu.Unlock()
 if session.terminal!=nil{return denied,remotetransport.ErrRemoteActionDenied}
 // Long tasks outlive their initial E2EE action. Respect the same
 // terminalMu -> longTaskMu lock order as TerminalOpen/TasksStart,
 // and keep both locks until the short process has exited. Otherwise
 // a pending long job and a second short task can mutate the workspace
 // concurrently even though the browser exposes one job at a time.
 session.longTaskMu.Lock()
 defer session.longTaskMu.Unlock()
 if session.longTask!=nil{return denied,remotetransport.ErrRemoteActionDenied}
 // Tasks and Git from OTHER E2EE sessions must not mutate this same
 // workspace at the same time. Hold the lease until cmd.Run has fully
 // exited and its entire process group has been retired.
 releaseWorkspace,err:=reserveRemoteWorkspaceExecution(ctx,s.Workspace)
 if err!=nil{return denied,remotetransport.ErrRemoteActionDenied}
 defer releaseWorkspace()
 spec,_,err:=prepareRemoteTaskExecutionInput(ctx,s.Workspace,session.taskPageHash,taskID,choice,false,textInput)
 if err!=nil||spec.TaskID!=taskID||ctx.Err()!=nil{
  return denied,remotetransport.ErrRemoteActionDenied
 }
 // Keep the initial implementation inside the authorized TaskDeck
 // workspace. The trusted local task may still run its own subcommands.
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return denied,remotetransport.ErrRemoteActionDenied}
 root,err=filepath.Abs(root)
 if err!=nil{return denied,remotetransport.ErrRemoteActionDenied}
 cwd,err:=filepath.EvalSymlinks(spec.Cwd)
 if err!=nil{return denied,remotetransport.ErrRemoteActionDenied}
 rel,err:=filepath.Rel(root,cwd)
 if err!=nil||filepath.IsAbs(rel)||rel==".."||
  strings.HasPrefix(rel,".."+string(filepath.Separator)){
  return denied,remotetransport.ErrRemoteActionDenied
 }
 // Persistent audit BEFORE the first externally observable side effect.
 if err:=s.auditRemoteTaskRunAttempt(ctx,session,taskID);err!=nil{
  return denied,remotetransport.ErrRemoteActionDenied
 }
 taskCtx,cancel:=context.WithTimeout(ctx,remoteTaskRunTimeout)
 defer cancel()
 if taskCtx.Err()!=nil||session.closed.Load(){
  return denied,remotetransport.ErrRemoteActionDenied
 }
 cmd:=exec.CommandContext(taskCtx,spec.Command,spec.Args...)
 cmd.Dir=cwd
 cmd.Env=spec.Env
 capture:=&remoteTaskCapture{}
 cmd.Stdout=capture
 cmd.Stderr=capture
 // Reuse the established TaskDeck process-group cleanup for shell children,
 // rather than killing only bash and leaving its descendants behind.
 prepareGitJobProcess(cmd)
 cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
 cmd.WaitDelay=time.Second
 if taskCtx.Err()!=nil||session.closed.Load(){
  return denied,remotetransport.ErrRemoteActionDenied
 }
 startedAt:=time.Now()
 err=cmd.Run()
 // Even when the short parent shell exits successfully, it may leave
 // children in its dedicated process group. This API never authorizes
 // detached background jobs; clean them up before returning the reply.
 killGitJobProcess(cmd)
 exitCode:=0
 if err!=nil{
  var exit *exec.ExitError
  if errors.As(err,&exit)&&exit.ExitCode()>=0&&exit.ExitCode()<=255{
   exitCode=exit.ExitCode()
  }else{exitCode=-1}
 }
 outcome:="completed"
 if errors.Is(taskCtx.Err(),context.DeadlineExceeded){
  outcome="timed_out"
 }else if taskCtx.Err()!=nil||session.closed.Load(){
  outcome="cancelled"
 }else if err!=nil{outcome="failed"}
 s.auditRemoteTaskRunFinish(session,taskID,outcome,exitCode,time.Since(startedAt))
 if taskCtx.Err()!=nil||session.closed.Load()||exitCode<0{
  return denied,remotetransport.ErrRemoteActionDenied
 }
 output,truncated:=capture.snapshot()
 return remotetransport.RemoteTaskRun{
  TaskID:taskID,ExitCode:exitCode,Output:output,Truncated:truncated,
 },nil
}
