package server

import (
 "context"
 "errors"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "runtime"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

var remoteSafeTrackedFilename=regexp.MustCompile("^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$")

// Reject unsafe names, renames and conflicts; status rows are host-issued,
// never free-form path strings from untrusted PHP or a browser.
func remoteGitSelectedTrackedFile(status string,index int,operation string)(string,bool){
 if !remotetransport.ValidRemoteGitFileSelection(index,status){return "",false}
 lines:=strings.Split(status,"\n")
 if index>=len(lines){return "",false}
 row:=lines[index]
 if len(row)<4||row[2]!=' '{return "",false}
 code,path:=row[:2],row[3:]
 if len(path)>120||!remoteSafeTrackedFilename.MatchString(path)||
  strings.HasPrefix(path,".git/"){return "",false}
 for _,part:=range strings.Split(path,"/"){
  if part=="."||part==".."||part==""{return "",false}
 }
 switch operation{
 case remotetransport.RemoteActionGitStageFile:
  if code[1]!='M'&&code[1]!='D'{return "",false}
 case remotetransport.RemoteActionGitUnstageFile:
  if code[0]!='M'&&code[0]!='A'&&code[0]!='D'{return "",false}
 default:return "",false
 }
 return path,true
}

// The host re-reads the exact authenticated Git Status snapshot and only
// then derives a restricted tracked path from the requested one-based row.
func (s *Server) executeRemoteGitFileIndex(ctx context.Context,
 session *remoteTaskDeckSession,op string,index int,snapshot string,
)(remotetransport.RemoteGitIndexResult,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteGitIndexResult
 optIn:=""
 switch op{
 case remotetransport.RemoteActionGitStageFile:optIn="TASKDECK_REMOTE_GIT_STAGE_FILE"
 case remotetransport.RemoteActionGitUnstageFile:optIn="TASKDECK_REMOTE_GIT_UNSTAGE_FILE"
 default:return empty,denied
 }
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  runtime.GOOS=="windows"||s.Workspace==""||
  os.Getenv(optIn)!="1"||os.Getenv("TASKDECK_REMOTE_GIT_STATUS")!="1"||
  !remotetransport.ValidRemoteGitFileSelection(index,snapshot)||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){return empty,denied}
 principal,err:=s.verifyRemoteSessionLocalPrincipal(ctx,session)
 if err!=nil||remotetransport.AuthorizeAction(true,session.role,
  &principal,session.projectKey,identity.PermissionGitStatus)!=nil{return empty,denied}
 // Recheck and reserve the Git write slot through the complete fixed
 // stage/unstage command, not only at the beginning of this request.
 release,err:=session.reserveRemoteGitMutation(ctx)
 if err!=nil{return empty,denied}
 defer release()
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return empty,denied}
 fresh,err:=s.readRemoteGitStatus(ctx)
 if err!=nil||fresh!=snapshot{return empty,denied}
 path,ok:=remoteGitSelectedTrackedFile(fresh,index,op)
 if !ok{return empty,denied}
 env:=[]string{
  "PATH="+os.Getenv("PATH"),"HOME="+os.Getenv("HOME"),
  "LC_ALL=C","GIT_OPTIONAL_LOCKS=0","GIT_CONFIG_NOSYSTEM=1",
  "GIT_CONFIG_GLOBAL="+os.DevNull,"GIT_CONFIG_SYSTEM="+os.DevNull,
  "GIT_PROTOCOL_FROM_USER=0",
 }
 base:=[]string{"-c","core.fsmonitor=false","-c","core.hooksPath="+os.DevNull,
  "-c","protocol.ext.allow=never","-C",root}
 run:=func(callCtx context.Context,limit int,args ...string)(string,error){
  if callCtx.Err()!=nil||session.closed.Load(){return "",denied}
  command:=exec.CommandContext(callCtx,"git",append(append([]string(nil),base...),args...)...)
  command.Dir=root
  command.Env=env
  output:=&boundedCommandOutput{remaining:limit}
  if limit<=0{command.Stdout=io.Discard}else{command.Stdout=output}
  command.Stderr=io.Discard
  prepareGitJobProcess(command)
  command.Cancel=func()error{killGitJobProcess(command);return nil}
  command.WaitDelay=time.Second
  err:=command.Run()
  killGitJobProcess(command)
  if err!=nil||callCtx.Err()!=nil||session.closed.Load()||output.truncated{return "",denied}
  if limit<=0{return "",nil}
  return strings.TrimSpace(output.String()),nil
 }
 // Exact equality rejects Git pathspecs that match whole directories.
 tracked,err:=run(ctx,256,"ls-files","--cached","--full-name","--",path)
 if err!=nil||tracked!=path{return empty,denied}
 head,err:=run(ctx,256,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(head){return empty,denied}
 diff:=[]string{"diff","--quiet","--no-ext-diff","--no-textconv","--",path}
 if op==remotetransport.RemoteActionGitUnstageFile{
  diff=[]string{"diff","--cached","--quiet","--no-ext-diff","--no-textconv","--",path}
 }
 checkChanged:=func()bool{
  if ctx.Err()!=nil||session.closed.Load(){return false}
  command:=exec.CommandContext(ctx,"git",append(append([]string(nil),base...),diff...)...)
  command.Dir=root
  command.Env=env
  command.Stdout=io.Discard
  command.Stderr=io.Discard
  prepareGitJobProcess(command)
  command.Cancel=func()error{killGitJobProcess(command);return nil}
  command.WaitDelay=time.Second
  err:=command.Run()
  killGitJobProcess(command)
  var exit *exec.ExitError
  return errors.As(err,&exit)&&exit.ExitCode()==1&&ctx.Err()==nil&&!session.closed.Load()
 }
 if !checkChanged(){return empty,denied}
 if op==remotetransport.RemoteActionGitStageFile{
  // Any configured external clean/process filter is a hard denial.
  for _,key:=range []string{"^filter\\..*\\.clean$","^filter\\..*\\.process$"}{
   command:=exec.CommandContext(ctx,"git",append(append([]string(nil),base...),
    "config","--includes","--get-regexp",key)...)
   command.Dir=root
   command.Env=env
   command.Stdout=io.Discard
   command.Stderr=io.Discard
   prepareGitJobProcess(command)
   command.Cancel=func()error{killGitJobProcess(command);return nil}
   command.WaitDelay=time.Second
   configErr:=command.Run()
   killGitJobProcess(command)
   var exit *exec.ExitError
   if !errors.As(configErr,&exit)||exit.ExitCode()!=1||
    ctx.Err()!=nil||session.closed.Load(){return empty,denied}
  }
 }
 fresh,err=s.readRemoteGitStatus(ctx)
 if err!=nil||fresh!=snapshot{return empty,denied}
 if err:=s.auditRemoteGitIndexAttempt(ctx,session,op);err!=nil{return empty,denied}
 mutateCtx,cancel:=context.WithTimeout(ctx,remoteGitIndexTimeout)
 defer cancel()
 if mutateCtx.Err()!=nil||session.closed.Load()||
  !session.portalGrant.ActiveForSession(session.channel){return empty,denied}
 if op==remotetransport.RemoteActionGitStageFile{
  _,err=run(mutateCtx,0,"add","-u","--",path)
 }else{
  _,err=run(mutateCtx,0,"restore","--staged","--",path)
 }
 if err!=nil||mutateCtx.Err()!=nil||session.closed.Load(){return empty,denied}
 afterHead,err:=run(ctx,256,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||afterHead!=head{return empty,denied}
 return remotetransport.RemoteGitIndexResult{Operation:op,Head:head},nil
}
