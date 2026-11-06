package server

import(
 "context"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "sort"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// Offline local names only: no fetch, URL, credential, caller-controlled
// argv/path or network. Host opt-in and GitStatus RBAC are checked by the
// existing authenticated E2EE dispatcher before this code can execute.
func(s *Server)readRemoteGitRemotes(ctx context.Context)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_REMOTES")!="1"{return "",denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return "",denied}
 root,err=filepath.Abs(root)
 if err!=nil{return "",denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return "",denied}
 // Do not resolve Git through an untrusted daemon PATH or project tree.
 gitExecutable:=""
 for _,candidate:=range []string{"/usr/bin/git","/bin/git","/opt/homebrew/bin/git","/usr/local/bin/git"}{
  if tasks.RemoteExecutableAllowed(root,candidate){gitExecutable=candidate;break}
 }
 if gitExecutable==""{return "",denied}
 // Never forward process Git configs/credentials/helpers or HOME from host.
 env:=[]string{"PATH=/usr/bin:/bin","HOME=/nonexistent","LC_ALL=C","LANG=C",
  "GIT_OPTIONAL_LOCKS=0","GIT_CONFIG_NOSYSTEM=1",
  "GIT_CONFIG_GLOBAL="+os.DevNull,"GIT_CONFIG_SYSTEM="+os.DevNull,
  "GIT_NO_LAZY_FETCH=1","GIT_TERMINAL_PROMPT=0","GIT_PROTOCOL_FROM_USER=0"}
 run:=func(limit int,args ...string)(string,error){
  if ctx.Err()!=nil{return "",denied}
  cmd:=exec.CommandContext(ctx,gitExecutable,args...)
  cmd.Dir=root;cmd.Env=env
  output:=&boundedCommandOutput{remaining:limit}
  cmd.Stdout=output;cmd.Stderr=io.Discard
  prepareGitJobProcess(cmd)
  cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
  cmd.WaitDelay=time.Second
  err:=cmd.Run()
  killGitJobProcess(cmd)
  if err!=nil||ctx.Err()!=nil||output.truncated{return "",denied}
  // boundedCommandOutput.String() trims trailing newlines for SSH display.
  // The Git metadata wire contract needs the original complete stdout.
  return output.buf.String(),nil
 }
 top,err:=run(4096,"-c","core.fsmonitor=false","-C",root,
  "rev-parse","--show-toplevel")
 if err!=nil{return "",denied}
 canonical,err:=filepath.EvalSymlinks(strings.TrimSpace(top))
 if err!=nil{return "",denied}
 canonical,err=filepath.Abs(canonical)
 if err!=nil||filepath.Clean(canonical)!=filepath.Clean(root){return "",denied}
 text,err:=run(remotetransport.MaxRemoteGitRemotesNamesBytes,
  "-c","core.fsmonitor=false","-c","core.pager=cat",
  "-C",root,"--no-pager","remote")
 if err!=nil{return "",denied}
 if text==""{return "GIT_REMOTES_V1\n",nil}
 if !strings.HasSuffix(text,"\n"){return "",denied}
 names:=strings.Split(strings.TrimSuffix(text,"\n"),"\n")
 if len(names)>16{return "",denied}
 for _,name:=range names{
  if !remotetransport.ValidRemoteGitRemoteName(name){return "",denied}
 }
 sort.Strings(names)
 report:="GIT_REMOTES_V1\n"+strings.Join(names,"\n")+"\n"
 if !remotetransport.ValidRemoteGitRemotesReport(report){return "",denied}
 return report,nil
}
