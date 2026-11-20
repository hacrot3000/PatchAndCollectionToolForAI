package server

import (
 "context"
 "crypto/sha256"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const remoteGitPatchMaxBytes=4096
// Host-only, default-OFF bigger review. E2EE pages and time remain bounded.
const remoteGitPatchLargeMaxBytes=16*1024
const remoteGitPatchDeadline=7*time.Second

func remoteGitPatchSnapshotMaxBytes()int{
 if os.Getenv("TASKDECK_REMOTE_GIT_PATCH_LARGE")=="1"{
  return remoteGitPatchLargeMaxBytes
 }
 return remoteGitPatchMaxBytes
}

// Each page is computed from the same host-owned, fixed HEAD diff. The
// client never supplies a file path, ref, offset, content hash or Git argv.
// Between pages, the Agent recomputes and fingerprints the complete diff:
// edits or workspace changes fail closed instead of mixing versions.
func (s *Server) readRemoteGitPatchPage(
 ctx context.Context,session *remoteTaskDeckSession,first bool,requestID string,
)(remotetransport.RemoteGitPatchPage,error){
 return s.readRemoteGitPatchPageMode(ctx,session,first,requestID,false)
}
func (s *Server) readRemoteGitPatchPageMode(ctx context.Context,session *remoteTaskDeckSession,first bool,requestID string,cached bool)(remotetransport.RemoteGitPatchPage,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteGitPatchPage
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  runtime.GOOS=="windows"||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_PATCH")!="1"||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){
  return empty,denied
 }
 session.gitPatchMu.Lock()
 defer session.gitPatchMu.Unlock()
 if !first&&!session.gitPatchReady{return empty,denied}
 // Read and cap before returning even the first byte. The host defaults
 // to 4 KiB; a separate opt-in enables up to 16 KiB. Never clip silently.
 patch,root,err:=s.readRemoteGitPatchSnapshotMode(ctx,cached)
 if err!=nil||ctx.Err()!=nil||session.closed.Load(){
  session.gitPatchReady=false
  return empty,denied
 }
 h:=sha256.New()
 _,_=h.Write([]byte(root))
 if cached{_,_=h.Write([]byte{0,1})}
 _,_=h.Write([]byte{0})
 _,_=h.Write([]byte(patch))
 var digest [32]byte
 copy(digest[:],h.Sum(nil))
 start,pageNo:=0,1
 if !first {
  if digest!=session.gitPatchDigest||session.gitPatchCursor>=len(patch){
   session.gitPatchReady=false
   return empty,denied
  }
  start=session.gitPatchCursor
  pageNo=session.gitPatchPage+1
 }
 page,end,err:=remotetransport.BoundRemoteGitPatchPage(requestID,patch,start,pageNo)
 if err!=nil||ctx.Err()!=nil||session.closed.Load(){
  session.gitPatchReady=false
  return empty,denied
 }
 if page.More{
  session.gitPatchDigest=digest
  session.gitPatchCursor=end
  session.gitPatchPage=pageNo
  session.gitPatchReady=true
 }else{
  session.gitPatchReady=false
  session.gitPatchCursor=0
  session.gitPatchPage=0
  session.gitPatchDigest=[32]byte{}
 }
 return page,nil
}

func (s *Server) readRemoteGitPatchSnapshot(ctx context.Context)(string,string,error){
 return s.readRemoteGitPatchSnapshotMode(ctx,false)
}
func (s *Server) readRemoteGitPatchSnapshotMode(ctx context.Context,cached bool)(string,string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""{
  return "","",denied
 }
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return "","",denied}
 root,err=filepath.Abs(root)
 if err!=nil{return "","",denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return "","",denied}
 env:=[]string{
  "PATH="+os.Getenv("PATH"),"HOME="+os.Getenv("HOME"),"LC_ALL=C",
  "GIT_OPTIONAL_LOCKS=0","GIT_CONFIG_NOSYSTEM=1",
  "GIT_CONFIG_GLOBAL="+os.DevNull,"GIT_CONFIG_SYSTEM="+os.DevNull,
  "GIT_PROTOCOL_FROM_USER=0",
 }
 base:=[]string{
  "-c","core.fsmonitor=false",
  "-c","diff.external=",
  "-c","protocol.ext.allow=never",
  "-C",root,
 }
 deadlineCtx,cancel:=context.WithTimeout(ctx,remoteGitPatchDeadline)
 defer cancel()
 run:=func(limit int,args ...string)(string,error){
  if deadlineCtx.Err()!=nil{return "",denied}
  command:=exec.CommandContext(deadlineCtx,"git",
   append(append([]string(nil),base...),args...)...)
  command.Dir=root
  command.Env=env
  output:=&boundedCommandOutput{remaining:limit}
  command.Stdout=output
  command.Stderr=io.Discard
  prepareGitJobProcess(command)
  command.Cancel=func()error{killGitJobProcess(command);return nil}
  command.WaitDelay=time.Second
  err:=command.Run()
  killGitJobProcess(command)
  if err!=nil||deadlineCtx.Err()!=nil||output.truncated{return "",denied}
  return output.String(),nil
 }
 top,err:=run(4096,"rev-parse","--show-toplevel")
 if err!=nil{return "","",denied}
 canonical,err:=filepath.EvalSymlinks(strings.TrimSpace(top))
 if err!=nil{return "","",denied}
 canonical,err=filepath.Abs(canonical)
 if err!=nil||filepath.Clean(canonical)!=filepath.Clean(root){return "","",denied}
 head,err:=run(256,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(strings.TrimSpace(head)){
  return "","",denied
 }
 // A fixed HEAD diff still reads the worktree through local clean
 // filters; deny executable filter drivers before any diff page.
 if !remoteGitNoExecutableFilters(deadlineCtx,root,env){
  return "","",denied
 }
 args:=[]string{"--no-pager","diff","--no-ext-diff","--no-textconv","--no-color","--no-renames","--ignore-submodules=all","--unified=1"}
 if cached{args=append(args,"--cached")}else{args=append(args,"HEAD")}
 args=append(args,"--")
 data,err:=run(remoteGitPatchSnapshotMaxBytes(),args...)
 if err!=nil{return "","",denied}
 if data==""{
  if cached{data="(no staged changes)\n"}else{data="(no tracked changes)\n"}
 }
 return data,root,nil
}
