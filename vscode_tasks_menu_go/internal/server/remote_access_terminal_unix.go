//go:build linux || darwin

package server

import (
 "context"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "sync"
 "syscall"
 "time"

 "github.com/creack/pty"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const remoteTerminalMaxLifetime=90*time.Second

// An already-open PTY is side-effecting even without another E2EE Write:
// it can keep running a command after the owner's Terminal permission is
// downgraded. This host-local check never relies on stale browser/Portal
// role data; the independent session watcher still covers key/account loss.
func(s *Server)remoteTerminalHostConsent(ctx context.Context,
 session *remoteTaskDeckSession)bool{
 if s==nil||ctx==nil||ctx.Err()!=nil||session==nil||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel)||
  os.Getenv("TASKDECK_REMOTE_TERMINAL")!="1"{return false}
 checkCtx,cancel:=context.WithTimeout(ctx,remoteLocalRevokeCheckDeadline)
 defer cancel()
 principal,err:=s.verifyRemoteSessionLocalPrincipal(checkCtx,session)
 return err==nil&&checkCtx.Err()==nil&&
  remotetransport.AuthorizeAction(true,session.role,&principal,
   session.projectKey,identity.PermissionTerminalCreate)==nil&&
  remotetransport.AuthorizeAction(true,session.role,&principal,
   session.projectKey,identity.PermissionTerminalControlOwn)==nil&&
  os.Getenv("TASKDECK_REMOTE_TERMINAL")=="1"&&
  !session.closed.Load()
}

// A single short-lived Unix PTY is always started from the host's fixed
// /bin/sh binary. E2EE inputs are ONLY keystrokes to this already-authorized
// terminal, never an executable/path/shell selector or environment override.
// The PHP Portal/Relay cannot observe the keystrokes or terminal output.
func(s *Server) startRemoteTerminal(ctx context.Context,session *remoteTaskDeckSession)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  os.Getenv("TASKDECK_REMOTE_TERMINAL")!="1"||
  os.Geteuid()==0||s.Workspace==""||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){
  return denied
 }
 // A terminal requires BOTH terminal.create and terminal.control_own. The
 // dispatcher enforces terminal.create; this independent live RBAC check
 // denies a role which could only start sessions but not control them.
 if err:=s.authorizeRemoteTransportAction(ctx,session,
  session.channel,identity.PermissionTerminalControlOwn);err!=nil{return denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return denied}
 root,err=filepath.Abs(root)
 if err!=nil{return denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return denied}
 session.terminalMu.Lock()
 defer session.terminalMu.Unlock()
 if session.terminal!=nil{return denied}
 session.longTaskMu.Lock()
 hasTask:=session.longTask!=nil
 session.longTaskMu.Unlock()
 if hasTask{return denied}
 if ctx.Err()!=nil||session.closed.Load(){return denied}
 // Another E2EE session must not be running Git, Task or PTY in the
 // same canonical workspace while the shell can modify its files.
 releaseWorkspace,err:=reserveRemoteWorkspaceExecution(ctx,root)
 if err!=nil{return denied}
 launched:=false
 defer func(){if !launched{releaseWorkspace()}}()
 if err:=s.auditRemoteTerminalAttempt(ctx,session,"open");err!=nil{return denied}
 // Host can revoke TerminalCreate/ControlOwn while SQLite stores the
 // attempted row. No PTY process may be launched on stale authorization.
 if !s.remoteTerminalHostConsent(ctx,session){return denied}
 cmdCtx,cancelContext:=context.WithTimeout(context.Background(),remoteTerminalMaxLifetime)
 cmd:=exec.CommandContext(cmdCtx,"/bin/sh","-i")
 cmd.Dir=root
 cmd.Env=[]string{
  "PATH=/usr/bin:/bin","HOME="+root,"TERM=dumb",
  "ENV=/dev/null","PS1=TaskDeck> ","LC_ALL=C.UTF-8",
 }
 // creack/pty starts a fresh Unix session and controlling TTY. Kill that
 // whole process group, including children, when the client disconnects.
 var tty *os.File
 cmd.Cancel=func()error{
  if cmd.Process!=nil&&cmd.Process.Pid>0{
   // Background commands can acquire their own process group while still
   // belonging to this PTY session: kill session members FIRST.
   remoteKillPTYSession(cmd.Process.Pid)
   _=syscall.Kill(-cmd.Process.Pid,syscall.SIGKILL)
   _=cmd.Process.Kill()
  }
  return nil
 }
 cmd.WaitDelay=time.Second
 if ctx.Err()!=nil||session.closed.Load()||
  !session.portalGrant.ActiveForSession(session.channel){
  cancelContext();return denied
 }
 tty,err=pty.StartWithSize(cmd,&pty.Winsize{Rows:24,Cols:80})
 if err!=nil{cancelContext();return denied}
 capture:=newRemoteLongTaskCapture()
 job:=&remoteTerminalJob{
  ptyFile:tty,process:cmd,done:make(chan struct{}),
  capture:capture,started:time.Now(),state:"running",
  workspaceRelease:releaseWorkspace,workspaceReleased:make(chan struct{}),
 }
 var stopOnce sync.Once
 job.cancel=func(){
  stopOnce.Do(func(){
   // Do not signal a PID after Wait has reaped the shell; the kernel may
   // already have recycled it for an unrelated process/session.
   select{
   case <-job.done:
   default:
    if cmd.Process!=nil&&cmd.Process.Pid>0{
     remoteKillPTYSession(cmd.Process.Pid)
     _=syscall.Kill(-cmd.Process.Pid,syscall.SIGKILL)
     _=cmd.Process.Kill()
    }
   }
   cancelContext()
   _=tty.Close()
  })
 }
 session.terminal=job
 launched=true
 // Periodically re-evaluate host permission for an ALREADY running PTY.
 // A role downgrade alone need not close a read-only E2EE session, but it
 // must terminate an interactive shell and any surviving child processes.
 go func(){
  ticker:=time.NewTicker(remoteLocalRevokePoll)
  defer ticker.Stop()
  for{
   select{
   case <-job.done:return
   case <-cmdCtx.Done():return
   case <-ticker.C:
    if !s.remoteTerminalHostConsent(cmdCtx,session){
     job.cancel()
     return
    }
   }
  }
 }()
 go func(){
  // io.Copy drains the PTY regardless of the native client polling speed.
  // Capture is limited to 16 KiB and never written to local audit or PHP.
  _,_=io.Copy(capture,tty)
 }()
 go func(){
  _=cmd.Wait()
  // A user can type "nohup command &; exit": the interactive shell
  // exits while children in DIFFERENT process groups still belong to
  // its PTY session. Closing the master TTY alone does not stop a
  // child which deliberately ignores SIGHUP. Retire session members
  // before making the workspace available to other E2EE sessions.
  // IMPORTANT: Do not signal the already-reaped shell PID or its
  // process group here: it may have been recycled. The session helper
  // inspects the live session ID of each process before signalling it.
  if cmd.Process!=nil&&cmd.Process.Pid>0{
   remoteKillPTYSession(cmd.Process.Pid)
  }
  job.mu.Lock()
  job.state="exited"
  job.mu.Unlock()
  close(job.done)
  select{case capture.notify<-struct{}{}:default:}
  job.cancel()
  // Release only after the shell has exited and its controlling PTY
  // has been closed, including when logout asynchronously kills it.
  job.workspaceRelease()
  close(job.workspaceReleased)
 }()
 go func(){
  select{
  case <-session.lifetimeDone:job.cancel()
  case <-session.portalGrant.Done():job.cancel()
  case <-session.channel.Done():job.cancel()
  case <-job.done:
  }
 }()
 return nil
}

func (session *remoteTaskDeckSession) pollRemoteTerminal(
 ctx context.Context)(remotetransport.RemoteTerminalProgress,error){
 if session==nil||ctx==nil||ctx.Err()!=nil||session.closed.Load(){
  return remotetransport.RemoteTerminalProgress{},remotetransport.ErrRemoteActionDenied
 }
 return session.remoteTerminalSnapshot()
}

func (session *remoteTaskDeckSession) pollRemoteTerminalForSeal(
 ctx context.Context)(remotetransport.RemoteTerminalProgress,func(),error){
 if session==nil||ctx==nil||ctx.Err()!=nil||session.closed.Load(){
  return remotetransport.RemoteTerminalProgress{},nil,remotetransport.ErrRemoteActionDenied
 }
 return session.remoteTerminalSnapshotWithMode(true)
}

func (s *Server) writeRemoteTerminal(ctx context.Context,
 session *remoteTaskDeckSession,input string)(remotetransport.RemoteTerminalProgress,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  os.Getenv("TASKDECK_REMOTE_TERMINAL")!="1"||
  !remotetransport.ValidRemoteTerminalInput(input)||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){
  return remotetransport.RemoteTerminalProgress{},denied
 }
 session.terminalMu.Lock()
 defer session.terminalMu.Unlock()
 job:=session.terminal
 if job==nil{return remotetransport.RemoteTerminalProgress{},denied}
 job.mu.Lock()
 state:=job.state
 job.mu.Unlock()
 if state!="running"{return remotetransport.RemoteTerminalProgress{},denied}
 if err:=s.auditRemoteTerminalAttempt(ctx,session,"write");err!=nil{
  return remotetransport.RemoteTerminalProgress{},denied
 }
 // An audit-time role downgrade cannot leave the PTY accepting keystrokes.
 if !s.remoteTerminalHostConsent(ctx,session){
  if job.cancel!=nil{job.cancel()}
  return remotetransport.RemoteTerminalProgress{},denied
 }
 if ctx.Err()!=nil||session.closed.Load(){return remotetransport.RemoteTerminalProgress{},denied}
 n,err:=job.ptyFile.Write([]byte(input))
 if err!=nil||n!=len(input)||ctx.Err()!=nil||session.closed.Load(){
  return remotetransport.RemoteTerminalProgress{},denied
 }
 return remotetransport.RemoteTerminalProgress{State:"running"},nil
}

func (s *Server) resizeRemoteTerminal(ctx context.Context,
 session *remoteTaskDeckSession,cols,rows int)(remotetransport.RemoteTerminalProgress,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  os.Getenv("TASKDECK_REMOTE_TERMINAL")!="1"||
  !remotetransport.ValidRemoteTerminalSize(cols,rows)||session.closed.Load()||
  session.channel==nil||!session.channel.Established()||
  session.portalGrant==nil||!session.portalGrant.ActiveForSession(session.channel){
  return remotetransport.RemoteTerminalProgress{},denied
 }
 session.terminalMu.Lock()
 defer session.terminalMu.Unlock()
 job:=session.terminal
 if job==nil{return remotetransport.RemoteTerminalProgress{},denied}
 job.mu.Lock()
 state:=job.state
 job.mu.Unlock()
 if state!="running"{return remotetransport.RemoteTerminalProgress{},denied}
 if err:=s.auditRemoteTerminalAttempt(ctx,session,"resize");err!=nil{
  return remotetransport.RemoteTerminalProgress{},denied
 }
 if !s.remoteTerminalHostConsent(ctx,session){
  if job.cancel!=nil{job.cancel()}
  return remotetransport.RemoteTerminalProgress{},denied
 }
 if ctx.Err()!=nil||session.closed.Load()||
  pty.Setsize(job.ptyFile,&pty.Winsize{Rows:uint16(rows),Cols:uint16(cols)})!=nil{
  return remotetransport.RemoteTerminalProgress{},denied
 }
 return remotetransport.RemoteTerminalProgress{State:"running"},nil
}

func (s *Server) closeRemoteTerminal(ctx context.Context,
 session *remoteTaskDeckSession)(remotetransport.RemoteTerminalProgress,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  os.Getenv("TASKDECK_REMOTE_TERMINAL")!="1"||session.closed.Load()||
  session.channel==nil||!session.channel.Established()||
  session.portalGrant==nil||!session.portalGrant.ActiveForSession(session.channel){
  return remotetransport.RemoteTerminalProgress{},denied
 }
 session.terminalMu.Lock()
 defer session.terminalMu.Unlock()
 job:=session.terminal
 if job==nil{return remotetransport.RemoteTerminalProgress{},denied}
 if err:=s.auditRemoteTerminalAttempt(ctx,session,"close");err!=nil{
  return remotetransport.RemoteTerminalProgress{},denied
 }
 if ctx.Err()!=nil||session.closed.Load(){return remotetransport.RemoteTerminalProgress{},denied}
 // Keep the job attached until its workspace lease is actually
 // released. If ctx expires while Wait is still retiring the PTY,
 // a later authenticated Close can retry instead of orphaning a hidden
 // terminal and wrongly allowing another PTY to be started.
 job.cancel()
 // An explicit successful Close must mean the workspace has actually
 // been released. Otherwise an immediate Task/Git action can race the
 // asynchronous Wait goroutine and be spuriously denied as busy.
 // Session-wide revocation remains nonblocking in stopRemoteTerminal.
 if job.workspaceReleased!=nil{
  select{
  case <-job.workspaceReleased:
  case <-ctx.Done():return remotetransport.RemoteTerminalProgress{},denied
  case <-time.After(2*time.Second):return remotetransport.RemoteTerminalProgress{},denied
  }
 }
 // Only successful, completed shutdown detaches the session-owned job.
 // Session revocation may still detach it in stopRemoteTerminal, but the
 // async PTY waiter independently retains and eventually frees its lease.
 session.terminal=nil
 return remotetransport.RemoteTerminalProgress{State:"closed"},nil
}

func (session *remoteTaskDeckSession) stopRemoteTerminal(){
 if session==nil{return}
 session.terminalMu.Lock()
 job:=session.terminal
 session.terminal=nil
 session.terminalMu.Unlock()
 if job!=nil&&job.cancel!=nil{job.cancel()}
}

func(s *Server) auditRemoteTerminalAttempt(
 ctx context.Context,session *remoteTaskDeckSession,op string)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if op!="open"&&op!="write"&&op!="resize"&&op!="close"{return denied}
 if s==nil||s.Identity==nil||session==nil||session.userID==""||
  ctx==nil||ctx.Err()!=nil{return denied}
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled{return denied}
 eventID,err:=identity.NewID()
 if err!=nil{return denied}
 userID,projectID:=session.userID,project.ID
 if err:=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:eventID,Timestamp:time.Now().UTC(),
  UserID:&userID,ProjectID:&projectID,
  Action:"remote.action.terminal_"+op,ResourceType:"terminal",
  Result:"attempted",
  Details:`{"transport":"e2ee","terminal":"unix","bounded":true}`,
 });err!=nil||ctx.Err()!=nil{return denied}
 return nil
}

