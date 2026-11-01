package server

import (
 "context"
 "crypto/rand"
 "crypto/subtle"
 "encoding/hex"
 "os"
 "path/filepath"
 "strings"
 "time"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

type remoteManagedTask struct {
 handle string // random, visible ONLY within the authenticated E2EE payload
 sessionID string // host-local TaskDeck session ID: NEVER returned to client
 userID string
 projectID string
 done chan struct{}
}

// Shared preflight for both synchronous and managed task execution. The
// complete host task definition SHA, absolute safe executable, isolated env,
// TaskDeck owner and project are pinned BEFORE any process may be launched.
func (s *Server) prepareRemoteApprovedProcessTask(ctx context.Context,
 owner *remoteTaskDeckSession,channel *remotetransport.Channel,
 taskID int)(tasks.Execution,string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||owner==nil||channel==nil||
  taskID<1||taskID>0x7fffffff||os.Getenv("TASKDECK_REMOTE_TASK_RUN")!="1"||
  s.Sessions==nil||s.Identity==nil{return tasks.Execution{},"",denied}
 cap,ok:=s.Sessions.(session.OwnershipCapability)
 if !ok||!cap.SupportsSessionOwnership(){return tasks.Execution{},"",denied}
 selected,_,root,err:=s.workspaceWorkflowByTaskID(taskID)
 if err!=nil||root.Path==""||filepath.Clean(root.Path)!=filepath.Clean(s.Workspace)||
  selected.ID!=taskID||selected.Raw==nil||selected.Type!="process"||
  !remoteHostApprovedTask(selected)||remoteTaskUsesAmbientExpansion(selected)||
  selected.Raw["taskdeckRemoteAllowed"]!=true||
  selected.Raw["dependsOn"]!=nil||selected.Raw["taskdeckApproval"]!=nil||
  len(selected.Inputs)!=0||len(selected.SecretEnv)!=0||
  selected.Options["env"]!=nil||selected.Options["shell"]!=nil{
  return tasks.Execution{},"",denied
 }
 spec,err:=tasks.ResolveExecution(selected,s.Workspace)
 if err!=nil||spec.TaskID!=taskID||
  filepath.Clean(spec.Cwd)!=filepath.Clean(s.Workspace)||
  !tasks.RemoteExecutableAllowed(s.Workspace,spec.Command)||
  spec.TargetType!=""||spec.TargetProfileID!=""||
  spec.ProtocolEvents||spec.ProtocolCommands{
  return tasks.Execution{},"",denied
 }
 project,err:=s.Identity.ProjectByKey(ctx,owner.projectKey)
 if err!=nil{return tasks.Execution{},"",denied}
 spec.Env=isolatedRemoteTaskEnvironment()
 spec.OwnerUserID=string(owner.userID)
 spec.ProjectID=string(project.ID)
 spec.SessionKind=tasks.SessionKindTask
 return spec,string(project.ID),nil
}

func remoteManagedTaskStop(s *Server,task *remoteManagedTask){
 if s==nil||s.Sessions==nil||task==nil{return}
 state,ok:=s.Sessions.Metadata(task.sessionID)
 if !ok||state.OwnerUserID!=task.userID||state.ProjectID!=task.projectID||
  state.Kind!=tasks.SessionKindTask||state.Status!="running"{return}
 _=s.Sessions.Stop(task.sessionID)
 after,ok:=s.Sessions.Metadata(task.sessionID)
 if ok&&after.OwnerUserID==task.userID&&after.ProjectID==task.projectID&&
  after.Kind==tasks.SessionKindTask&&after.Status=="running"{
  _=s.Sessions.Kill(task.sessionID)
 }
}

// A managed process is bounded even if a compromised PHP/Relay deliberately
// abandons the mailbox. No heartbeat from PHP can extend this hard deadline.
// Disconnection/revocation cancels the owned process without a client request.
func (s *Server) watchRemoteManagedTask(owner *remoteTaskDeckSession,
 channel *remotetransport.Channel,task *remoteManagedTask){
 timer:=time.NewTimer(remotetransport.RemoteManagedTaskLifetimeSeconds*time.Second)
 defer timer.Stop()
 ticker:=time.NewTicker(150*time.Millisecond)
 defer ticker.Stop()
 for {
  select{
  case <-task.done:return
  case <-owner.lifetimeDone:
   remoteManagedTaskStop(s,task);return
  case <-channel.Done():
   remoteManagedTaskStop(s,task);return
  case <-owner.portalGrant.Done():
   remoteManagedTaskStop(s,task);return
  case <-timer.C:
   remoteManagedTaskStop(s,task);return
  case <-ticker.C:
   current,ok:=s.Sessions.Metadata(task.sessionID)
   if !ok||current.Status!="running" {return}
  }
 }
}

// All three operations require the normal authenticated E2EE TaskDeck
// principal, live Portal role, approved key and TasksRun RBAC. The controller
// never accepts arbitrary process IDs, commands, file paths or task arguments.
func (s *Server) remoteManagedTaskStart(ctx context.Context,
 owner *remoteTaskDeckSession,channel *remotetransport.Channel,taskID int)(
 remotetransport.RemoteManagedTaskResult,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if os.Getenv("TASKDECK_REMOTE_TASK_MANAGE")!="1"||
  owner==nil||owner.lifetimeDone==nil||owner.portalGrant==nil{return remotetransport.RemoteManagedTaskResult{},denied}
 spec,projectID,err:=s.prepareRemoteApprovedProcessTask(ctx,owner,channel,taskID)
 if err!=nil{return remotetransport.RemoteManagedTaskResult{},denied}
 if err:=s.authorizeRemoteTransportAction(ctx,owner,channel,identity.PermissionTasksRun);err!=nil{
  return remotetransport.RemoteManagedTaskResult{},denied
 }
 if err:=s.auditRemoteReadOnlyAction(owner,"task_managed_start");err!=nil{
  return remotetransport.RemoteManagedTaskResult{},denied
 }
 // Generate unpredictable handle BEFORE launch; do not expose a TaskDeck
 // PID/session ID in an E2EE response (even though PHP cannot decrypt it).
 var nonce [16]byte
 if _,err:=rand.Read(nonce[:]);err!=nil{return remotetransport.RemoteManagedTaskResult{},denied}
 handle:=hex.EncodeToString(nonce[:])
 owner.executionMu.Lock()
 defer owner.executionMu.Unlock()
 if owner.closed.Load()||owner.channel!=channel||
  !owner.portalGrant.ActiveForSession(channel)||ctx.Err()!=nil{
  return remotetransport.RemoteManagedTaskResult{},denied
 }
 owner.managedMu.Lock()
 defer owner.managedMu.Unlock()
 if owner.managedTask!=nil{return remotetransport.RemoteManagedTaskResult{},denied}
 meta,err:=s.Sessions.Start(spec)
 if err!=nil{return remotetransport.RemoteManagedTaskResult{},denied}
 task:=&remoteManagedTask{handle:handle,sessionID:meta.ID,
  userID:string(owner.userID),projectID:projectID,done:make(chan struct{})}
 owner.managedTask=task
 go s.watchRemoteManagedTask(owner,channel,task)
 return remotetransport.RemoteManagedTaskResult{
  Handle:handle,State:"running",ExitCode:-1,
 },nil
}

func (s *Server) remoteManagedTaskLookup(ctx context.Context,
 owner *remoteTaskDeckSession,channel *remotetransport.Channel,
 handle string,cancel bool)(remotetransport.RemoteManagedTaskResult,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||owner==nil||channel==nil||
  owner.channel!=channel||owner.closed.Load()||owner.portalGrant==nil||
  !owner.portalGrant.ActiveForSession(channel)||
  !remotetransport.ValidManagedTaskHandle(handle)||
  os.Getenv("TASKDECK_REMOTE_TASK_MANAGE")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASK_RUN")!="1"{
  return remotetransport.RemoteManagedTaskResult{},denied
 }
 owner.executionMu.Lock()
 defer owner.executionMu.Unlock()
 if owner.closed.Load()||owner.channel!=channel||
  !owner.portalGrant.ActiveForSession(channel)||ctx.Err()!=nil{
  return remotetransport.RemoteManagedTaskResult{},denied
 }
 owner.managedMu.Lock()
 defer owner.managedMu.Unlock()
 task:=owner.managedTask
 if task==nil||subtle.ConstantTimeCompare([]byte(task.handle),[]byte(handle))!=1||
  task.userID!=string(owner.userID){return remotetransport.RemoteManagedTaskResult{},denied}
 project,err:=s.Identity.ProjectByKey(ctx,owner.projectKey)
 if err!=nil||string(project.ID)!=task.projectID{return remotetransport.RemoteManagedTaskResult{},denied}
 current,ok:=s.Sessions.Metadata(task.sessionID)
 if !ok||current.OwnerUserID!=task.userID||
  current.ProjectID!=task.projectID||current.Kind!=tasks.SessionKindTask{
  return remotetransport.RemoteManagedTaskResult{},denied
 }
 if cancel&&current.Status=="running"{
  remoteManagedTaskStop(s,task)
  return remotetransport.RemoteManagedTaskResult{
   Handle:handle,State:"cancel_requested",ExitCode:-1,
  },nil
 }
 if current.Status=="running"{
  return remotetransport.RemoteManagedTaskResult{
   Handle:handle,State:"running",ExitCode:-1,
  },nil
 }
 if (current.Status!="exited"&&current.Status!="stopped")||current.ExitCode==nil{
  return remotetransport.RemoteManagedTaskResult{},denied
 }
 // TaskDeck may report -1 when a child was killed by a signal. A stopped
 // process is not misrepresented as a successful exit.
 exitCode:=*current.ExitCode
 if current.Status=="exited"&&(exitCode<0||exitCode>255)||
  current.Status=="stopped"&&(exitCode< -1||exitCode>255){
  return remotetransport.RemoteManagedTaskResult{},denied
 }
 backlog,_,unsubscribe,err:=s.Sessions.Subscribe(task.sessionID)
 if err!=nil{return remotetransport.RemoteManagedTaskResult{},denied}
 unsubscribe()
 output:=strings.ToValidUTF8(string(backlog),"�")
 if len(output)>remotetransport.RemoteManagedTaskMaxOutput||!utf8.ValidString(output){
  return remotetransport.RemoteManagedTaskResult{},denied
 }
 // A final receipt releases the single-task slot. Retrying the same token
 // or retaining it across future logins never grants access to old processes.
 owner.managedTask=nil
 close(task.done)
 return remotetransport.RemoteManagedTaskResult{
  Handle:handle,State:current.Status,ExitCode:exitCode,Output:output,
 },nil
}
