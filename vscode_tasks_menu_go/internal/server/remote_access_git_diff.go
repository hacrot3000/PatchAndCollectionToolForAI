package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// This fixed stat-only Git diff never reads untracked files and never
// executes external diff helpers, hooks, shell or remote network helpers.
func(s *Server)readRemoteGitDiff(ctx context.Context)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_DIFF")!="1"{return "",denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return "",denied}
 root,err=filepath.Abs(root)
 if err!=nil{return "",denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return "",denied}
 env:=[]string{
  "PATH="+os.Getenv("PATH"),"HOME="+os.Getenv("HOME"),
  "LC_ALL=C","GIT_OPTIONAL_LOCKS=0",
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_CONFIG_SYSTEM="+os.DevNull,
 }
 run:=func(limit int,args ...string)(string,error){
  if ctx.Err()!=nil{return "",denied}
  cmd:=exec.CommandContext(ctx,"git",args...)
  cmd.Dir=root
  cmd.Env=env
  output:=&boundedCommandOutput{remaining:limit}
  cmd.Stdout=output
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
 // Diffing worktree blobs can invoke filter.clean/process even with
 // --no-ext-diff. Read permission is never permission to launch helpers.
 if !remoteGitNoExecutableFilters(ctx,root,env){return "",denied}
 summary,err:=run(remotetransport.MaxRemoteGitLogText,
  "-c","core.fsmonitor=false","-c","diff.external=",
  "-c","diff.trustExitCode=false","-C",root,
  "--no-pager","diff","--no-ext-diff","--no-textconv",
  "--no-color","--stat","HEAD","--")
 if err!=nil{return "",denied}
 if summary==""{summary="clean"}
 if !remotetransport.ValidRemoteGitLogText(summary){return "",denied}
 return summary,nil
}
