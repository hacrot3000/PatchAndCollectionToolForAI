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
 if action!="session_ping" && action!="project_file_read" && action!="project_file_preview" && action!="project_file_window" && action!="project_file_chunk" && action!="project_file_block" && action!="project_dir_list" && action!="git_status" && action!="git_diff" && action!="project_file_replace" && action!="task_run" {
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

// executeRemoteE2EEPing is a deliberately restricted, already-mounted
// encrypted action dispatcher. Its only operations are session.ping, a small
// workspace text preview/write, read-only Git status, and an independently
// host-allowlisted, short task.run. It is NOT an arbitrary HTTP/WS/PTY,
// Terminal, Git command, Patch/Collection or browser session proxy.
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
 var window remotetransport.FileWindow
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
   // Never resolve Git using daemon PATH: an environment-controlled or
   // workspace-local git binary could execute arbitrary code with TaskDeck
   // credentials before the caller's GitStatus RBAC is rechecked.
   executable,err:=remoteTrustedGitExecutable(root)
   if err!=nil{return remotetransport.ErrRemoteActionDenied}
   // Fixed read-only Git invocation. No aliases, hooks, shell or client
   // supplied arguments; prevent repository fsmonitor helpers.
   cmd:=exec.CommandContext(actionCtx,executable,
    "-c","core.fsmonitor=false","-c","core.untrackedCache=false",
    "-C",root,"status","--short","--branch","--untracked-files=no")
   cmd.Env=isolatedRemoteGitEnvironment()
   cmd.Stderr=io.Discard
   var output cappedRemoteGitOutput
   cmd.Stdout=&output
   if err:=cmd.Run();err!=nil||actionCtx.Err()!=nil{
    return remotetransport.ErrRemoteActionDenied
   }
   content=append([]byte(nil),output.Bytes()...)
   return actionCtx.Err()
  }
 case remotetransport.RemoteActionGitDiff:
  // Git diff can disclose file CONTENT. Require BOTH GitStatus and
  // FilesRead rights; a Git metadata-only role never receives a diff.
  // Fixed Git worktree diff: no arbitrary Git flags, shell, pager,
  // external diff helpers, textconv, plugins or scripts from the Portal.
  permission=identity.PermissionGitStatus
  auditAction="git_diff"
  run=func(actionCtx context.Context)error{
   if actionCtx.Err()!=nil{return remotetransport.ErrRemoteActionDenied}
   if err:=s.authorizeRemoteTransportAction(actionCtx,session,channel,identity.PermissionFilesRead);err!=nil{
    return remotetransport.ErrRemoteActionDenied
   }
   _,path,root,err:=s.restrictedSourcePath(request.Path)
   if err!=nil{return remotetransport.ErrRemoteActionDenied}
   pinned,err:=openProjectPinnedFile(root,path)
   if err!=nil{return remotetransport.ErrRemoteActionDenied}
   defer pinned.close()
   current,statErr:=pinned.openCurrent()
   if statErr!=nil{return remotetransport.ErrRemoteActionDenied}
   info,statErr:=current.Stat()
   _=current.Close()
   if statErr!=nil||!info.Mode().IsRegular(){
    return remotetransport.ErrRemoteActionDenied
   }
   executable,err:=remoteTrustedGitExecutable(root)
   if err!=nil{return remotetransport.ErrRemoteActionDenied}
   cmd:=exec.CommandContext(actionCtx,executable,
    "-c","core.fsmonitor=false","-c","core.untrackedCache=false",
    "-c","diff.external=","-C",root,
    "--no-pager","diff","--no-ext-diff","--no-textconv",
    "--no-color","--",request.Path)
   cmd.Env=isolatedRemoteGitEnvironment()
   cmd.Stderr=io.Discard
   var output cappedRemoteGitDiffOutput
   cmd.Stdout=&output
   if err:=cmd.Run();err!=nil||actionCtx.Err()!=nil{
    return remotetransport.ErrRemoteActionDenied
   }
   // The secondary FilesRead right must still be valid after the Git
   // subprocess exits; the generic wrapper independently rechecks GitStatus.
   if err:=s.authorizeRemoteTransportAction(actionCtx,session,channel,identity.PermissionFilesRead);err!=nil{
    return remotetransport.ErrRemoteActionDenied
   }
   content=append([]byte(nil),output.Bytes()...)
   return actionCtx.Err()
  }
 case remotetransport.RemoteActionTaskRun:
  permission=identity.PermissionTasksRun
  auditAction="task_run"
  auditBeforeCommit=true
  run=func(actionCtx context.Context)error{
   output,err:=s.runRemoteApprovedTask(actionCtx,session,channel,request.TaskID)
   if err!=nil{return remotetransport.ErrRemoteActionDenied}
   content=[]byte(output)
   return actionCtx.Err()
  }
 case remotetransport.RemoteActionProjectFileReplace:
  // A rollout of the existing read-only app transport must NEVER silently
  // enable writes. Only a TaskDeck host operator can explicitly opt in to
  // this narrow, experimental mutation capability.
  if os.Getenv("TASKDECK_REMOTE_FILE_REPLACE")!="1"{
   return nil,remotetransport.ErrRemoteActionDenied
  }
  permission=identity.PermissionFilesWrite
  auditAction="project_file_replace"
  auditBeforeCommit=true
  run=func(actionCtx context.Context)error{
   if actionCtx.Err()!=nil{return remotetransport.ErrRemoteActionDenied}
   relative,pathErr:=cleanProjectRelativePath(request.Path,false)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   // Reuse the EXACT mutation lock also used by local editor/Patch/other
   // TaskDeck API writes: two concurrent browser/remote writers cannot
   // both pass the same stale optimistic hash and race to commit.
   if s.Config.SharedServerEnabled{
    s.refreshSharedMutationLock()
    lease,conflict,lockErr:=s.sharedMutation.acquire(identity.Principal{
     UserID:session.userID,Username:session.clientUsername,
    },"file.write",relative,time.Now().UTC())
    if lockErr!=nil||conflict!=nil{return remotetransport.ErrRemoteActionDenied}
    defer s.releaseSharedMutation(lease)
   }
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
 case remotetransport.RemoteActionProjectDirectoryList:
  permission=identity.PermissionFilesRead
  auditAction="project_dir_list"
  run=func(actionCtx context.Context)error{
   data,err:=s.readRemoteDirectory(actionCtx,request.Path)
   if err!=nil{return remotetransport.ErrRemoteActionDenied}
   content=data
   return actionCtx.Err()
  }
 case remotetransport.RemoteActionProjectFileBlock:
  permission=identity.PermissionFilesRead
  auditAction="project_file_block"
  run=func(actionCtx context.Context)error{
   result,err:=s.readRemoteFileBlock(actionCtx,request.Path,request.Offset)
   if err!=nil{return remotetransport.ErrRemoteActionDenied}
   window=result
   return actionCtx.Err()
  }
 case remotetransport.RemoteActionProjectFileChunk:
  permission=identity.PermissionFilesRead
  auditAction="project_file_chunk"
  run=func(actionCtx context.Context)error{
   result,err:=s.readRemoteFileChunk(actionCtx,request.Path,request.Offset)
   if err!=nil{return remotetransport.ErrRemoteActionDenied}
   window=result
   return actionCtx.Err()
  }
 case remotetransport.RemoteActionProjectFileWindow:
  permission=identity.PermissionFilesRead
  auditAction="project_file_window"
  run=func(actionCtx context.Context)error{
   result,err:=s.readRemoteFileWindow(actionCtx,request.Path,request.Offset)
   if err!=nil{return remotetransport.ErrRemoteActionDenied}
   window=result
   return actionCtx.Err()
  }
 case remotetransport.RemoteActionProjectFilePreview:
  permission=identity.PermissionFilesRead
  auditAction="project_file_preview"
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
   // Unlike project.file.chunk, preview must contain the entire short
   // UTF-8 file and never activates scoped binary transfer/download.
   data,info,readErr:=pinned.readCurrent(remotetransport.RemoteFilePreviewMaxBytes)
   if readErr!=nil||info==nil||!info.Mode().IsRegular()||
    !projectTextBytesValid(data)||actionCtx.Err()!=nil{
    return remotetransport.ErrRemoteActionDenied
   }
   content=append([]byte(nil),data...)
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
 // Switch to file-only mode INSIDE the already serialized host execution
 // callback. This ordering is critical: a concurrently authorized task.run
 // or file write cannot slip between a successful chunk and mode locking.
 originalRun:=run
 run=func(actionCtx context.Context)error{
  if err:=session.checkScopedFileTransfer(request.Operation,request.Path,request.Offset);err!=nil{
   return remotetransport.ErrRemoteActionDenied
  }
  if err:=originalRun(actionCtx);err!=nil{return err}
  if actionCtx.Err()!=nil{return remotetransport.ErrRemoteActionDenied}
  if err:=session.commitScopedFileTransfer(
   request.Operation,request.Path,request.Offset,window);err!=nil{
   return remotetransport.ErrRemoteActionDenied
  }
  return actionCtx.Err()
 }
 defer func(){
  for i:=range content{content[i]=0}
  for i:=range window.Data{window.Data[i]=0}
 }()
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
 } else if request.Operation==remotetransport.RemoteActionProjectFilePreview{
  sealed,err=remotetransport.SealRemoteFilePreviewResult(channel,request.RequestID,content)
 } else if request.Operation==remotetransport.RemoteActionProjectFileWindow{
  sealed,err=remotetransport.SealRemoteFileWindowResult(channel,request.RequestID,window)
 } else if request.Operation==remotetransport.RemoteActionProjectFileChunk{
  sealed,err=remotetransport.SealRemoteFileChunkResult(channel,request.RequestID,window)
 } else if request.Operation==remotetransport.RemoteActionProjectFileBlock{
  sealed,err=remotetransport.SealRemoteFileBlockResult(channel,request.RequestID,window)
 } else if request.Operation==remotetransport.RemoteActionProjectDirectoryList{
  sealed,err=remotetransport.SealRemoteDirectoryListResult(channel,request.RequestID,content)
  } else if request.Operation==remotetransport.RemoteActionGitStatus{
  sealed,err=remotetransport.SealRemoteGitStatusResult(channel,request.RequestID,content)
 } else if request.Operation==remotetransport.RemoteActionGitDiff{
  sealed,err=remotetransport.SealRemoteGitDiffResult(channel,request.RequestID,content)
 } else if request.Operation==remotetransport.RemoteActionProjectFileReplace{
  sealed,err=remotetransport.SealRemoteFileReplaceResult(channel,request.RequestID,replacementHash)
 } else if request.Operation==remotetransport.RemoteActionTaskRun{
  sealed,err=remotetransport.SealRemoteTaskRunResult(channel,request.RequestID,string(content))
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

// No truncation: an operator must not mistake a partial Git diff for the
// complete change when deciding whether to edit. Refuse >4096 bytes.
type cappedRemoteGitDiffOutput struct{ bytes []byte }
func (w *cappedRemoteGitDiffOutput) Write(p []byte)(int,error){
 if len(w.bytes)+len(p)>remotetransport.RemoteGitDiffMaxBytes{
  return 0,errors.New("remote Git diff exceeds E2EE read limit")
 }
 w.bytes=append(w.bytes,p...)
 return len(p),nil
}
func (w *cappedRemoteGitDiffOutput) Bytes()[]byte{return w.bytes}
