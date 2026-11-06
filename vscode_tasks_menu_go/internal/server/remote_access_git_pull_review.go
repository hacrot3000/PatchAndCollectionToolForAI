package server

import (
 "context"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

const remoteGitPullReviewTimeout=7*time.Second

// readRemoteGitPullReview adapts main's *offline*, read-only pull preflight.
// Unlike GitPull, this never contacts a remote or changes any Git refs.
// It reads only the host's current branch and its preconfigured upstream.
// No Git options, environment, URL, refspec or path come from the client.
func (s *Server)readRemoteGitPullReview(ctx context.Context)(
 remotetransport.RemoteGitPullReviewResult,error,
){
 var empty remotetransport.RemoteGitPullReviewResult
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""||
  runtime.GOOS=="windows"||
  os.Getenv("TASKDECK_REMOTE_GIT_PULL_REVIEW")!="1"{return empty,denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 stat,err:=os.Stat(root)
 if err!=nil||!stat.IsDir(){return empty,denied}

 executable:=""
 for _,path:=range []string{"/usr/bin/git","/bin/git","/opt/homebrew/bin/git","/usr/local/bin/git"}{
  if tasks.RemoteExecutableAllowed(root,path){executable=path;break}
 }
 if executable==""{return empty,denied}
 env:=[]string{
  "PATH=/usr/bin:/bin","HOME=/nonexistent","LC_ALL=C","LANG=C",
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_CONFIG_SYSTEM="+os.DevNull,"GIT_OPTIONAL_LOCKS=0",
  "GIT_TERMINAL_PROMPT=0","GIT_NO_LAZY_FETCH=1",
  "GIT_NO_REPLACE_OBJECTS=1","GIT_PROTOCOL_FROM_USER=0",
 }
 base:=[]string{
  "-c","core.fsmonitor=false","-c","core.untrackedCache=false",
  "-c","core.hooksPath="+os.DevNull,"-c","diff.external=",
  "-c","protocol.allow=never","-C",root,
 }
 deadline,cancel:=context.WithTimeout(ctx,remoteGitPullReviewTimeout)
 defer cancel()
 read:=func(args ...string)(string,error){
  if deadline.Err()!=nil{return "",denied}
  call:=exec.CommandContext(deadline,executable,append(append([]string(nil),base...),args...)...)
  call.Dir=root;call.Env=env
  output:=&boundedCommandOutput{remaining:1024}
  call.Stdout=output;call.Stderr=io.Discard
  prepareGitJobProcess(call)
  call.Cancel=func()error{killGitJobProcess(call);return nil}
  call.WaitDelay=time.Second
  runErr:=call.Run()
  killGitJobProcess(call)
  if runErr!=nil||deadline.Err()!=nil||output.truncated{return "",denied}
  return strings.TrimSpace(output.buf.String()),nil
 }
 top,err:=read("rev-parse","--show-toplevel")
 if err!=nil{return empty,denied}
 top,err=filepath.EvalSymlinks(top)
 if err!=nil{return empty,denied}
 top,err=filepath.Abs(top)
 if err!=nil||filepath.Clean(top)!=filepath.Clean(root){return empty,denied}
 shallow,err:=read("rev-parse","--is-shallow-repository")
 if err!=nil||shallow!="false"{return empty,denied}

 branch,err:=read("symbolic-ref","-q","HEAD")
 if err!=nil||!strings.HasPrefix(branch,"refs/heads/")||
  !remoteGitPullReviewRef(strings.TrimPrefix(branch,"refs/heads/")){return empty,denied}
 upstream,err:=read("rev-parse","--symbolic-full-name","@{upstream}")
 if err!=nil||!strings.HasPrefix(upstream,"refs/remotes/")||
  !remoteGitPullReviewRef(strings.TrimPrefix(upstream,"refs/remotes/"))||
  strings.HasSuffix(upstream,"/HEAD"){return empty,denied}
 head,err:=read("rev-parse","--verify","HEAD^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(head){return empty,denied}
 target,err:=read("rev-parse","--verify",upstream+"^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(target){return empty,denied}

 status:="up-to-date"
 if target!=head{
  if _,err:=read("merge-base","--is-ancestor",head,target);err!=nil{
   return empty,denied
  }
  status="fast-forward"
 }
 dirty,err:=read("--no-optional-locks","status","--porcelain=v1",
  "--untracked-files=all","--ignore-submodules=none")
 if err!=nil||dirty!=""{return empty,denied}

 // The review is informational only. Reject drift from external Git writes
 // while these fixed reads were running, instead of presenting stale advice.
 branchAgain,err:=read("symbolic-ref","-q","HEAD")
 if err!=nil||branchAgain!=branch{return empty,denied}
 upstreamAgain,err:=read("rev-parse","--symbolic-full-name","@{upstream}")
 if err!=nil||upstreamAgain!=upstream{return empty,denied}
 headAgain,err:=read("rev-parse","--verify","HEAD^{commit}")
 if err!=nil||headAgain!=head{return empty,denied}
 targetAgain,err:=read("rev-parse","--verify",upstream+"^{commit}")
 if err!=nil||targetAgain!=target{return empty,denied}
 value:=remotetransport.RemoteGitPullReviewResult{
  Head:head,UpstreamHead:target,Status:status,
 }
 return value,nil
}

func remoteGitPullReviewRef(value string)bool{
 if len(value)<1||len(value)>160{return false}
 parts:=strings.Split(value,"/")
 if len(parts)>7{return false}
 for _,part:=range parts{
  if !remotetransport.ValidRemoteGitRemoteName(part)||
   part=="HEAD"||strings.HasSuffix(part,".lock"){return false}
 }
 return true
}
