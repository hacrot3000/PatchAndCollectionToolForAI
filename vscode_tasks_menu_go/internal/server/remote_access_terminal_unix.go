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
 if err:=s.auditRemoteTerminalAttempt(ctx,session,"open");err!=nil{return denied}
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
     _=syscall.Kill(-cmd.Process.Pid,syscall.SIGKILL)
     _=cmd.Process.Kill()
    }
   }
   cancelContext()
   _=tty.Close()
  })
 }
 session.terminal=job
 go func(){
  // io.Copy drains the PTY regardless of the native client polling speed.
  // Capture is limited to 16 KiB and never written to local audit or PHP.
  _,_=io.Copy(capture,tty)
 }()
 go func(){
  _=cmd.Wait()
  job.mu.Lock()
  job.state="exited"
  job.mu.Unlock()
  close(job.done)
  select{case capture.notify<-struct{}{}:default:}
  job.cancel()
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
 session.terminal=nil
 job.cancel()
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

