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

// Host-owned authorization pins the ENTIRE task definition (SHA-256), not
// just the 31-bit TaskDeck menu ID. The former numeric-only variable is
// intentionally ignored: it is insufficient against crafted ID collisions.
func remoteHostApprovedTask(task tasks.Task) bool {
	token, err := tasks.RemoteApprovalToken(task)
	if err != nil {
		return false
	}
	raw := os.Getenv("TASKDECK_REMOTE_APPROVED_TASKS")
	if raw == "" || len(raw) > 4096 {
		return false
	}
	seen := make(map[string]bool)
	matched := false
	for _, entry := range strings.Split(raw, ",") {
		idText, digest, ok := strings.Cut(entry, "@")
		if !ok || len(digest) != 64 || seen[entry] {
			return false
		}
		id, err := strconv.Atoi(idText)
		if err != nil || id < 1 || id > 0x7fffffff || strconv.Itoa(id) != idText {
			return false
		}
		for _, ch := range digest {
			if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
				return false
			}
		}
		seen[entry] = true
		if entry == token {
			matched = true
		}
	}
	return matched
}

// A remote task must not interpolate arbitrary daemon secrets into a command.
// Process tasks receive a minimal explicit environment, not os.Environ().
func remoteTaskUsesAmbientExpansion(task tasks.Task) bool {
	command, ok := task.Command.(string)
	if !ok || strings.Contains(command, "${env:") {
		return true
	}
	if task.Args == nil {
		return false
	}
	args, ok := task.Args.([]any)
	if !ok {
		return true
	}
	for _, arg := range args {
		value, ok := arg.(string)
		if !ok || strings.Contains(value, "${env:") {
			return true
		}
	}
	return false
}

func isolatedRemoteTaskEnvironment() []string {
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/bin:/usr/bin:/bin"
	}
	return []string{
		"PATH=" + path,
		"HOME=" + os.Getenv("HOME"),
		"LANG=" + os.Getenv("LANG"),
		"TERM=dumb",
	}
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
  s.Sessions==nil||ctx.Err()!=nil{return "",denied}
 cap,ok:=s.Sessions.(session.OwnershipCapability)
 if !ok||!cap.SupportsSessionOwnership(){return "",denied}
 selected,_,root,err:=s.workspaceWorkflowByTaskID(taskID)
 if err!=nil||root.Path==""||
  filepath.Clean(root.Path)!=filepath.Clean(s.Workspace)||
  selected.ID!=taskID||selected.Raw==nil||
  selected.Type!="process"||!remoteHostApprovedTask(selected)||
  remoteTaskUsesAmbientExpansion(selected)||
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
 spec.Env=isolatedRemoteTaskEnvironment()
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
