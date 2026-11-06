package server

import (
 "bytes"
 "context"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "sort"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// Inventory ONLY already-fetched, local fast-forward objects. The existing
// Pull preflight validates clean workspace, ancestry, branch and upstream.
// No client-supplied path/ref/URL/command can reach Git.
func (s *Server)readRemoteGitPullChanges(ctx context.Context)(
 remotetransport.RemoteGitPullChanges,error,
){
 var empty remotetransport.RemoteGitPullChanges
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""||
  runtime.GOOS=="windows"||
  os.Getenv("TASKDECK_REMOTE_GIT_PULL_CHANGES")!="1"||
  os.Getenv("TASKDECK_REMOTE_GIT_PULL_REVIEW")!="1"{return empty,denied}
 deadline,cancel:=context.WithTimeout(ctx,12*time.Second)
 defer cancel()
 before,err:=s.readRemoteGitPullReview(deadline)
 if err!=nil{return empty,denied}
 result:=remotetransport.RemoteGitPullChanges{
  Head:before.Head,UpstreamHead:before.UpstreamHead,Files:[]string{},
 }
 if before.Status=="up-to-date"{return result,nil}
 if before.Status!="fast-forward"{return empty,denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 executable:=""
 for _,path:=range []string{"/usr/bin/git","/bin/git","/opt/homebrew/bin/git","/usr/local/bin/git"}{
  if tasks.RemoteExecutableAllowed(root,path){executable=path;break}
 }
 if executable==""{return empty,denied}
 args:=[]string{
  "-c","core.fsmonitor=false","-c","core.untrackedCache=false",
  "-c","core.hooksPath="+os.DevNull,"-c","diff.external=",
  "-c","protocol.allow=never","-C",root,
  "diff-tree","--no-commit-id","-r","--no-renames",
  "--no-ext-diff","--no-textconv","--ignore-submodules=all",
  "--name-status","-z",before.Head,before.UpstreamHead,"--",
 }
 command:=exec.CommandContext(deadline,executable,args...)
 command.Dir=root
 command.Env=[]string{
  "PATH=/usr/bin:/bin","HOME=/nonexistent","LC_ALL=C","LANG=C",
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_CONFIG_SYSTEM="+os.DevNull,"GIT_OPTIONAL_LOCKS=0",
  "GIT_TERMINAL_PROMPT=0","GIT_NO_LAZY_FETCH=1",
  "GIT_NO_REPLACE_OBJECTS=1","GIT_PROTOCOL_FROM_USER=0",
 }
 buffer:=&boundedCommandOutput{remaining:1024}
 command.Stdout=buffer;command.Stderr=io.Discard
 prepareGitJobProcess(command)
 command.Cancel=func()error{killGitJobProcess(command);return nil}
 command.WaitDelay=time.Second
 runErr:=command.Run()
 killGitJobProcess(command)
 if runErr!=nil||deadline.Err()!=nil||buffer.truncated{return empty,denied}
 raw:=buffer.buf.Bytes()
 if len(raw)!=0{
  if raw[len(raw)-1]!=0{return empty,denied}
  fields:=bytes.Split(raw[:len(raw)-1],[]byte{0})
  if len(fields)%2!=0||len(fields)/2>remotetransport.MaxRemoteGitPullChangedFiles{
   return empty,denied
  }
  for i:=0;i<len(fields);i+=2{
   value:=string(fields[i])+":"+string(fields[i+1])
   if !remotetransport.ValidRemoteGitPullChangedFile(value){return empty,denied}
   result.Files=append(result.Files,value)
  }
 }
 sort.Slice(result.Files,func(i,j int)bool{
  return strings.Compare(result.Files[i][2:],result.Files[j][2:])<0
 })
 if !remotetransport.ValidRemoteGitPullChanges(result){return empty,denied}
 // Refresh the complete preflight to reject external ref and worktree drift.
 after,err:=s.readRemoteGitPullReview(deadline)
 if err!=nil||after!=before||deadline.Err()!=nil{return empty,denied}
 return result,nil
}
