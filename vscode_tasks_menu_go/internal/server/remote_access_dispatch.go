package server

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "io"
 "os"
 "os/exec"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A compromised PHP Portal cannot erase TaskDeck-local identity audit.
// Record locally authorized action attempts using the real TaskDeck principal,
// without request IDs, plaintext credentials, Portal tickets or ciphertext.
// A failed audit is a hard refusal; do not produce an encrypted "success".
func (s *Server) auditRemoteReadOnlyPing(session *remoteTaskDeckSession) error {
 return s.auditRemoteReadOnlyAction(session,"session_ping")
}
func (s *Server) auditRemoteReadOnlyAction(session *remoteTaskDeckSession,action string) error {
 if action!="session_ping" && action!="project_file_read" && action!="git_status" && action!="project_file_replace" {
  return remotetransport.ErrRemoteActionDenied
 }
 if s==nil || session==nil || session.userID=="" || s.Identity==nil {
  return remotetransport.ErrRemoteActionDenied
 }
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second)
 defer cancel()
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil{return remotetransport.ErrRemoteActionDenied}
 eventID,err:=identity.NewID()
 if err!=nil{return remotetransport.ErrRemoteActionDenied}
 userID:=session.userID
 projectID:=project.ID
 err=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:eventID,Timestamp:time.Now().UTC(),
  UserID:&userID,ProjectID:&projectID,
  Action:"remote.action."+action,ResourceType:"session",
  Result:"authorized",
  Details:`{"transport":"e2ee","taskdeck_auth":true,"remote_control":false}`,
 })
 if err!=nil{return remotetransport.ErrRemoteActionDenied}
 return nil
}

// executeRemoteE2EEPing is the first deliberately UNMOUNTED typed remote
// application operation. It is a one-packet, read-only authenticated ping,
// NOT an HTTP/WS/terminal/file/Git/Patch dispatcher and grants no session.
//
// A forged or stolen PHP Portal admin ticket cannot manufacture session:
// only authenticateRemoteTaskDeckCredentials can create its in-memory
// TaskDeck-local password principal. The exact E2EE identity and channel,
// local per-action permission, account status and Portal live role are
// rechecked both before and after running this fixed operation.
//
// Arbitrary commands/URLs/paths/user-provided callbacks are not accepted.
// The caller MUST forward a sealed response only while its Agent Grant
// and browser connection are still live and must retire the session on
// logout, disconnect, revocation or the 5-minute local deadline.
func (s *Server) executeRemoteE2EEPing(
 ctx context.Context, session *remoteTaskDeckSession,
 channel *remotetransport.Channel, frame []byte,
) ([]byte,error) {
 if s==nil || ctx==nil || session==nil || channel==nil ||
  !channel.Established() || channel!=session.channel ||
  session.closed.Load() || session.portalGrant==nil ||
  !session.portalGrant.ActiveForSession(channel) {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Validate the encrypted schema only AFTER checking that this exact
 // TaskDeck-local password session owns this authenticated channel.
 // A badly typed but authenticated record permanently poisons the E2EE key.
 request,err:=remotetransport.OpenRemoteAction(channel,frame)
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 // Dispatch exactly the two reviewed typed read-only operations.
 // Never proxy an HTTP handler: such a handler trusts web cookies/CSRF state,
 // not this in-process E2EE principal. Execute through the same local
 // TaskDeck RBAC/revocation/cancellation boundary as session.ping.
 var content []byte
 var replacementHash string
 auditBeforeCommit:=false
 permission:=identity.PermissionTasksView
 auditAction:="session_ping"
 var run func(context.Context)error=func(c context.Context)error{return c.Err()}
 switch request.Operation {
 case remotetransport.RemoteActionSessionPing:
 case remotetransport.RemoteActionGitStatus:
  permission=identity.PermissionGitStatus
  auditAction="git_status"
  run=func(actionCtx context.Context)error{
   if actionCtx.Err()!=nil{return remotetransport.ErrRemoteActionDenied}
   root,err:=s.projectRoot()
   if err!=nil{return remotetransport.ErrRemoteActionDenied}
   // Never interpolate client text in a shell or execute Git aliases.
   // Untrusted workspace gitconfig cannot spawn a configured fsmonitor.
   cmd:=exec.CommandContext(actionCtx,"git",
    "-c","core.fsmonitor=false","-c","core.untrackedCache=false",
    "-C",root,"status","--short","--branch","--untracked-files=no")
   cmd.Env=append(os.Environ(),"GIT_CONFIG_NOSYSTEM=1",
    "GIT_CONFIG_GLOBAL="+os.DevNull,"GIT_OPTIONAL_LOCKS=0")
   cmd.Stderr=io.Discard
   var output cappedRemoteGitOutput
   cmd.Stdout=&output
   if err:=cmd.Run();err!=nil||actionCtx.Err()!=nil{
    return remotetransport.ErrRemoteActionDenied
   }
   content=append([]byte(nil),output.Bytes()...)
   return actionCtx.Err()
  }
 case remotetransport.RemoteActionProjectFileReplace:
  permission=identity.PermissionFilesWrite
  auditAction="project_file_replace"
  auditBeforeCommit=true
  run=func(actionCtx context.Context)error{
   if actionCtx.Err()!=nil{return remotetransport.ErrRemoteActionDenied}
   relative,pathErr:=cleanProjectRelativePath(request.Path,false)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   path,pathErr:=s.resolveProjectPath(relative,false,false)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   rootView,_,pathErr:=s.projectRootForVirtualPath(relative)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   pinned,pathErr:=openProjectPinnedFile(rootView.Path,path)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   defer pinned.close()
   current,info,readErr:=pinned.readCurrent(192)
   if readErr!=nil||info==nil||!info.Mode().IsRegular()||
    info.Mode().Perm()&0o222==0||!projectTextBytesValid(current){
    return remotetransport.ErrRemoteActionDenied
   }
   digest:=sha256.Sum256(current)
   if hex.EncodeToString(digest[:])!=request.ExpectedSHA256{
    return remotetransport.ErrRemoteActionDenied
   }
   next:=[]byte(request.Content)
   defer func(){for i:=range next{next[i]=0}}()
   if !projectTextBytesValid(next)||len(next)>96{
    return remotetransport.ErrRemoteActionDenied
   }
   if err:=pinned.writeTemp(next,info);err!=nil{
    return remotetransport.ErrRemoteActionDenied
   }
   latest,_,readErr:=pinned.readCurrent(192)
   if readErr!=nil{return remotetransport.ErrRemoteActionDenied}
   latestSHA:=sha256.Sum256(latest)
   if hex.EncodeToString(latestSHA[:])!=request.ExpectedSHA256{
    return remotetransport.ErrRemoteActionDenied
   }
   // A durable, local TaskDeck identity audit must succeed BEFORE
   // any file side effect. PHP cannot erase or synthesize this audit.
   if err:=s.auditRemoteReadOnlyAction(session,auditAction);err!=nil{
    return remotetransport.ErrRemoteActionDenied
   }
   // Uncached host + Portal RBAC again immediately before the atomic
   // commit. Client-side approval at login is never sufficient.
   if err:=s.authorizeRemoteTransportAction(actionCtx,session,channel,
    identity.PermissionFilesWrite);err!=nil{return remotetransport.ErrRemoteActionDenied}
   session.executionMu.Lock()
   if session.closed.Load()||session.channel!=channel||
    session.portalGrant==nil||!session.portalGrant.ActiveForSession(channel)||
    actionCtx.Err()!=nil{
    session.executionMu.Unlock()
    return remotetransport.ErrRemoteActionDenied
   }
   // commitTemp is an atomic anchored rename and is serialized with
   // session.Close to linearize revocation at the commit boundary.
   commitErr:=pinned.commitTemp()
   session.executionMu.Unlock()
   if commitErr!=nil{return remotetransport.ErrRemoteActionDenied}
   writtenHash:=sha256.Sum256([]byte(request.Content))
   replacementHash=hex.EncodeToString(writtenHash[:])
   return actionCtx.Err()
  }
 case remotetransport.RemoteActionProjectFileRead:
  permission=identity.PermissionFilesRead
  auditAction="project_file_read"
  run=func(actionCtx context.Context)error{
   if actionCtx.Err()!=nil{return remotetransport.ErrRemoteActionDenied}
   relative,pathErr:=cleanProjectRelativePath(request.Path,false)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   path,pathErr:=s.resolveProjectPath(relative,false,false)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   rootView,_,pathErr:=s.projectRootForVirtualPath(relative)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   pinned,pathErr:=openProjectPinnedFile(rootView.Path,path)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   defer pinned.close()
   data,info,readErr:=pinned.readCurrent(192)
   if readErr!=nil || info==nil || !info.Mode().IsRegular() ||
    !projectTextBytesValid(data) || actionCtx.Err()!=nil {
    return remotetransport.ErrRemoteActionDenied
   }
   content=append([]byte(nil),data...)
   return actionCtx.Err()
  }
 default:
  return nil,remotetransport.ErrRemoteActionDenied
 }
 defer func(){for i:=range content{content[i]=0}}()
 err=s.executeRemoteAuthenticatedAction(ctx,session,channel,permission,run)
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 // Local audit must succeed BEFORE any confidential file data leaves host.
 // No content/path is recorded in Portal or the TaskDeck audit details.
 if !auditBeforeCommit {
  if err:=s.auditRemoteReadOnlyAction(session,auditAction);err!=nil {
   return nil,remotetransport.ErrRemoteActionDenied
  }
 }

 // Serialize final sealing with local session Close, which cancels active
 // work and destroys the Channel. The outer Agent must still check grant
 // liveness immediately before forwarding the encrypted response.
 session.executionMu.Lock()
 defer session.executionMu.Unlock()
 if session.closed.Load() || !session.portalGrant.ActiveForSession(channel) ||
  ctx.Err()!=nil {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 var sealed []byte
 if request.Operation==remotetransport.RemoteActionProjectFileRead{
  sealed,err=remotetransport.SealRemoteFileReadResult(channel,request.RequestID,content)
 } else if request.Operation==remotetransport.RemoteActionGitStatus{
  sealed,err=remotetransport.SealRemoteGitStatusResult(channel,request.RequestID,content)
 } else if request.Operation==remotetransport.RemoteActionProjectFileReplace{
  sealed,err=remotetransport.SealRemoteFileReplaceResult(channel,request.RequestID,replacementHash)
 } else {
  sealed,err=remotetransport.SealRemoteActionPingResult(channel,request.RequestID)
 }
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 return sealed,nil
}


// Never buffer unbounded Git output in TaskDeck memory. Output longer than
// 192 bytes fails closed rather than returning a misleading truncated status.
type cappedRemoteGitOutput struct{ bytes []byte }
func (w *cappedRemoteGitOutput) Write(p []byte)(int,error){
 if len(w.bytes)+len(p)>192{return 0,errors.New("git status exceeds AEAD limit")}
 w.bytes=append(w.bytes,p...)
 return len(p),nil
}
func (w *cappedRemoteGitOutput) Bytes()[]byte{return w.bytes}
