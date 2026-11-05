package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A fixed, read-only view of at most the three most recent commits from
// the exact TaskDeck workspace Git root; no client arguments or repo paths.
func(s *Server)readRemoteGitLog(ctx context.Context)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_LOG")!="1"{return "",denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return "",denied}
 root,err=filepath.Abs(root)
 if err!=nil{return "",denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return "",denied}
 env:=[]string{
  "PATH="+os.Getenv("PATH"),"HOME="+os.Getenv("HOME"),"LC_ALL=C",
  "GIT_OPTIONAL_LOCKS=0","GIT_CONFIG_NOSYSTEM=1",
  "GIT_CONFIG_GLOBAL="+os.DevNull,"GIT_CONFIG_SYSTEM="+os.DevNull,
 }
 run:=func(limit int,args ...string)(string,error){
  if ctx.Err()!=nil{return "",denied}
  cmd:=exec.CommandContext(ctx,"git",args...)
  cmd.Dir=root
  cmd.Env=env
  output:=&boundedCommandOutput{remaining:limit}
  cmd.Stdout=output
  cmd.Stderr=nil
  if err:=cmd.Run();err!=nil||ctx.Err()!=nil||output.truncated{
   return "",denied
  }
  return strings.TrimSpace(output.String()),nil
 }
 top,err:=run(4096,"-c","core.fsmonitor=false","-C",root,"rev-parse","--show-toplevel")
 if err!=nil{return "",denied}
 canonical,err:=filepath.EvalSymlinks(top)
 if err!=nil{return "",denied}
 canonical,err=filepath.Abs(canonical)
 if err!=nil||filepath.Clean(canonical)!=filepath.Clean(root){return "",denied}
 log,err:=run(remotetransport.MaxRemoteGitLogText,
  "-c","core.fsmonitor=false","-c","core.pager=cat",
  "-C",root,"--no-pager","log","--max-count=3",
  "--no-show-signature","--no-decorate","--pretty=format:%h %s")
 if err!=nil||!remotetransport.ValidRemoteGitLogText(log){return "",denied}
 return log,nil
}
