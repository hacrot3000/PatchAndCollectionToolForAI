package server

import (
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "strings"
 "sync"
)

// Remote changes are observed without modifying the checkout. The optional
// update happens only for a schedule run whose Git condition actually fired.
var scheduleGitRepoLocks sync.Map

var scheduleGitRemoteNamePattern=regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._/-]*$`)

func scheduleGitCommand(ctx context.Context,repoPath string,args ...string)(string,error){
 cmd:=exec.CommandContext(ctx,"git",args...)
 cmd.Dir=repoPath
 cmd.Env=append(os.Environ(),"GIT_TERMINAL_PROMPT=0","GIT_PAGER=cat","LC_ALL=C","GCM_INTERACTIVE=never")
 output,err:=cmd.CombinedOutput()
 if err!=nil {
  detail:=strings.TrimSpace(string(output))
  if len(detail)>1024{detail=detail[:1024]}
  if detail!=""{return "",fmt.Errorf("git %s: %s: %w",args[0],detail,err)}
  return "",fmt.Errorf("git %s: %w",args[0],err)
 }
 return strings.TrimSpace(string(output)),nil
}

func scheduleGitRemoteTarget(ctx context.Context,repoPath,remote,ref string)(string,string,error){
 if remote==""{remote="origin"}
 if !scheduleGitRemoteNamePattern.MatchString(remote)||strings.Contains(remote,".."){
  return "","",errors.New("invalid Git remote name")
 }
 remotes,err:=scheduleGitCommand(ctx,repoPath,"remote")
 if err!=nil{return "","",err}
 present:=false
 for _,name:=range strings.Split(remotes,"\n"){if name==remote{present=true;break}}
 if !present{return "","",fmt.Errorf("Git remote %q is not configured",remote)}
 if ref==""||ref=="HEAD"{
  ref,err=scheduleGitCommand(ctx,repoPath,"symbolic-ref","--quiet","--short","HEAD")
  if err!=nil{return "","",errors.New("Git remote HEAD needs a checked-out local branch (detached HEAD is not supported)")}
 }else{
  ref=strings.TrimPrefix(ref,"refs/heads/")
 }
 if ref==""||strings.HasPrefix(ref,"-")||strings.ContainsAny(ref,"\r\n\x00") {
  return "","",errors.New("invalid Git branch")
 }
 if _,err=scheduleGitCommand(ctx,repoPath,"check-ref-format","--branch",ref);err!=nil{
  return "","",fmt.Errorf("invalid Git branch %q: %w",ref,err)
 }
 return remote,ref,nil
}

func scheduleGitRemoteHead(ctx context.Context,repoPath,remote,ref string)(string,error){
 remote,branch,err:=scheduleGitRemoteTarget(ctx,repoPath,remote,ref)
 if err!=nil{return "",err}
 want:="refs/heads/"+branch
 output,err:=scheduleGitCommand(ctx,repoPath,"ls-remote","--exit-code","--heads",remote,want)
 if err!=nil{return "",fmt.Errorf("cannot check Git remote branch %s/%s: %w",remote,branch,err)}
 for _,line:=range strings.Split(output,"\n"){
  fields:=strings.Fields(line)
  if len(fields)!=2||fields[1]!=want{continue}
  sha:=fields[0]
  if (len(sha)!=40&&len(sha)!=64)||strings.Trim(sha,"0123456789abcdefABCDEF")!=""{
   return "",errors.New("Git remote returned invalid commit SHA")
  }
  return strings.ToLower(sha),nil
 }
 return "",fmt.Errorf("Git remote branch %s/%s was not found",remote,branch)
}

// Never reset, switch branches, stash, auto-merge, or discard local edits.
// Only fast-forward the exact checked-out branch, using a named remote.
func (s *Server) scheduleGitPull(ctx context.Context,c scheduleCondition)(string,error){
 repoPath,err:=verifyGitRepository(s.Workspace,filepath.Join(s.Workspace,c.GitPath))
 if err!=nil{return "",err}
 lockAny,_:=scheduleGitRepoLocks.LoadOrStore(repoPath,&sync.Mutex{})
 lock:=lockAny.(*sync.Mutex);lock.Lock();defer lock.Unlock()
 remote,branch,err:=scheduleGitRemoteTarget(ctx,repoPath,c.GitRemote,c.GitRef)
 if err!=nil{return "",err}
 current,err:=scheduleGitCommand(ctx,repoPath,"symbolic-ref","--quiet","--short","HEAD")
 if err!=nil||current!=branch{return "",fmt.Errorf("cannot fast-forward %s: checked-out branch must be %s",branch,branch)}
 status,err:=scheduleGitCommand(ctx,repoPath,"status","--porcelain","--untracked-files=no")
 if err!=nil{return "",err}
 if status!=""{return "",errors.New("Git pull blocked: tracked worktree/index has local changes")}
 if _,err=scheduleGitCommand(ctx,repoPath,"fetch","--no-tags",remote,"refs/heads/"+branch);err!=nil{return "",fmt.Errorf("Git fetch failed: %w",err)}
 if _,err=scheduleGitCommand(ctx,repoPath,"merge","--ff-only","FETCH_HEAD");err!=nil{
  return "",fmt.Errorf("Git pull blocked (non-fast-forward or checkout conflict): %w",err)
 }
 sha,err:=scheduleGitCommand(ctx,repoPath,"rev-parse","--verify","HEAD")
 if err!=nil{return "",err}
 return fmt.Sprintf("Fast-forward synced %s/%s to %s",remote,branch,sha),nil
}
