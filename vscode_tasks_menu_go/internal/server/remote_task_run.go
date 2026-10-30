package server

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "strconv"
 "time"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// Host-only environment allowlist is independent from workspace tasks.json,
// which can be modified by a user with FilesWrite. Both approvals are mandatory.
func remoteHostApprovedTaskID(taskID int) bool {
 if taskID<=0{return false}
 raw:=os.Getenv("TASKDECK_REMOTE_APPROVED_TASK_IDS")
 if raw==""||len(raw)>1024{return false}
 matched:=false
 for _,part:=range strings.Split(raw,","){
  if part==""||strings.TrimSpace(part)!=part{return false}
  id,err:=strconv.Atoi(part)
  if err!=nil||id<=0||id>0x7fffffff{return false}
  if id==taskID{matched=true}
 }
 return matched
}

// runRemoteApprovedTask is deliberately NOT a shell/PTY bridge. The caller
// can only choose an integer ID from HOST-owned tasks.json. Each permitted
// task must have taskdeckRemoteAllowed: true (bool) and be a single, local,
// non-secret, input-free task, approved independently by the host operator.
//
// A PHP admin / signed ticket has no authority to enable this operation:
// the TaskDeck process must explicitly set TASKDECK_REMOTE_TASK_RUN=1 and
// pass its own TasksRun RBAC, client-private-key proof and password checks.
// The process is bounded to 7 seconds, always stopped on disconnect/return,
// and only an escaped, at-most 192-byte output preview leaves the host.
func (s *Server) runRemoteApprovedTask(ctx context.Context,
 owner *remoteTaskDeckSession,channel *remotetransport.Channel,
 taskID int)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||owner==nil||channel==nil||taskID<=0||
  taskID>0x7fffffff||os.Getenv("TASKDECK_REMOTE_TASK_RUN")!="1"||
  !remoteHostApprovedTaskID(taskID)||
  s.Sessions==nil||ctx.Err()!=nil{return "",denied}
 cap,ok:=s.Sessions.(session.OwnershipCapability)
 if !ok||!cap.SupportsSessionOwnership(){return "",denied}
 selected,_,root,err:=s.workspaceWorkflowByTaskID(taskID)
 if err!=nil||root.Path==""||
  filepath.Clean(root.Path)!=filepath.Clean(s.Workspace)||
  selected.ID!=taskID||selected.Raw==nil||
  selected.Raw["taskdeckRemoteAllowed"]!=true||
  selected.Raw["dependsOn"]!=nil||
  selected.Raw["taskdeckApproval"]!=nil||
  len(selected.Inputs)!=0||len(selected.SecretEnv)!=0||
  selected.Options["env"]!=nil||selected.Options["shell"]!=nil{
  return "",denied
 }
 // Local host task definitions may be edited concurrently. Resolve from
 // the approved static Task ID, never accept commands/args/env from Relay.
 spec,err:=tasks.ResolveExecution(selected,s.Workspace)
 if err!=nil||spec.TaskID!=taskID||
  filepath.Clean(spec.Cwd)!=filepath.Clean(s.Workspace)||
  spec.TargetType!=""||spec.TargetProfileID!=""||
  spec.ProtocolEvents||spec.ProtocolCommands{
  return "",denied
 }
 project,err:=s.Identity.ProjectByKey(ctx,owner.projectKey)
 if err!=nil{return "",denied}
 spec.OwnerUserID=string(owner.userID)
 spec.ProjectID=string(project.ID)
 spec.SessionKind=tasks.SessionKindTask
 // A durable audit and fresh host+Portal authorization must PRECEDE any
 // process launch; a subsequent revoked session must interrupt this task.
 if err:=s.authorizeRemoteTransportAction(ctx,owner,channel,
  identity.PermissionTasksRun);err!=nil{return "",denied}
 if err:=s.auditRemoteReadOnlyAction(owner,"task_run");err!=nil{return "",denied}

 // Linearize the process launch with local session.Close / revocation.
 owner.executionMu.Lock()
 if owner.closed.Load()||owner.channel!=channel||owner.portalGrant==nil||
  !owner.portalGrant.ActiveForSession(channel)||ctx.Err()!=nil{
  owner.executionMu.Unlock();return "",denied
 }
 meta,err:=s.Sessions.Start(spec)
 owner.executionMu.Unlock()
 if err!=nil{return "",denied}
 defer func(){
  // Stop a still-running child even if the caller vanishes or PHP refuses
  // to route its response. The TaskDeck service owns the PTY/process tree.
  if current,ok:=s.Sessions.Metadata(meta.ID);ok&&current.Status=="running"{
   _=s.Sessions.Stop(meta.ID)
   if after,ok:=s.Sessions.Metadata(meta.ID);ok&&after.Status=="running"{
    _=s.Sessions.Kill(meta.ID)
   }
  }
 }()
 deadline,cancel:=context.WithTimeout(ctx,7*time.Second)
 defer cancel()
 ticker:=time.NewTicker(60*time.Millisecond)
 defer ticker.Stop()
 for{
  if deadline.Err()!=nil{return "",denied}
  current,ok:=s.Sessions.Metadata(meta.ID)
  if !ok||current.OwnerUserID!=string(owner.userID)||
   current.ProjectID!=string(project.ID)||current.Kind!=tasks.SessionKindTask{
   return "",denied
  }
  if current.Status!="running"{break}
  select {
  case <-deadline.Done():return "",denied
  case <-owner.lifetimeDone:return "",denied
  case <-channel.Done():return "",denied
  case <-ticker.C:
  }
 }
 if ctx.Err()!=nil||owner.closed.Load(){return "",denied}
 backlog,_,unsubscribe,err:=s.Sessions.Subscribe(meta.ID)
 if err!=nil{return "",denied}
 unsubscribe()
 preview:=strings.ToValidUTF8(string(backlog),"�")
 if len(preview)>192{
  preview=preview[len(preview)-192:]
  for len(preview)>0&&!utf8.ValidString(preview){preview=preview[1:]}
 }
 return preview,nil
}
