package server

import (
 "context"
 "errors"
 "io"
 "os"
 "os/exec"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// remoteTrustedGitExecutable chooses a predictable OS-managed executable,
// never an executable from the daemon's PATH or the editable workspace.
//
// On macOS /usr/bin/git may be Apple's toolchain shim. The Homebrew paths
// are fallback only if the OS-managed Git is unavailable. The binary must
// be executable, outside this workspace even after resolving symlinks,
// and not group/world-writable. A missing safe Git fails closed.
func remoteTrustedGitExecutable(workspace string)(string,error){
 for _,candidate:=range []string{
  "/usr/bin/git",
  "/bin/git",
  "/opt/homebrew/bin/git",
  "/usr/local/bin/git",
 }{
  if tasks.RemoteExecutableAllowed(workspace,candidate){
   return candidate,nil
  }
 }
 return "",remotetransport.ErrRemoteActionDenied
}

// Never forward daemon credentials, HOME, GIT_CONFIG_COUNT,
// GIT_EXEC_PATH, GIT_TRACE, LD_PRELOAD or DYLD_* to a Git subprocess.
// They can turn an otherwise read-only Git command into arbitrary code
// execution or secret disclosure. No default os.Environ fallback.
func isolatedRemoteGitEnvironment() []string{
 return []string{
  "PATH=/usr/bin:/bin",
  "HOME=/nonexistent",
  "LANG=C",
  "LC_ALL=C",
  "GIT_CONFIG_NOSYSTEM=1",
  "GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_OPTIONAL_LOCKS=0",
  "GIT_TERMINAL_PROMPT=0",
 }
}

 
// remoteGitReadOnlyConfigSafe denies Git *read* actions in repositories
// defining executable clean/process/smudge drivers. Git status and even
// diff --no-ext-diff can run such a clean filter merely to inspect a tracked
// file. Therefore GitStatus/FilesRead cannot ever be treated as a safe
// command permission until this check succeeds. Includes are resolved by
// Git itself; a nonexistent global/system config is enforced by Env.
// Repositories with filter drivers must use local Git, not remote Git reads.
func remoteGitReadOnlyConfigSafe(ctx context.Context,root,executable string)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||root==""||executable==""{return denied}
 cmd:=exec.CommandContext(ctx,executable,
  "-c","core.fsmonitor=false","-C",root,
  "config","--includes","--name-only","--get-regexp",
  "^filter\\..*\\.(clean|process|smudge)$")
 cmd.Env=isolatedRemoteGitEnvironment()
 cmd.Stderr=io.Discard
 var output cappedRemoteGitDiffOutput
 cmd.Stdout=&output
 err:=cmd.Run()
 if ctx.Err()!=nil||len(output.Bytes())!=0{return denied}
 if err==nil{return denied} // any defined executable filter is forbidden
 var exit *exec.ExitError
 if errors.As(err,&exit)&&exit.ExitCode()==1{return nil} // no matching keys
 return denied // bad config, I/O, oversized output or any other error
}

// Fixed read-only Git builtins; no remote command or arguments are accepted.
// The output is either complete and <= 4KiB or the action is refused.
func remoteGitMetadata(ctx context.Context,workspace,operation string)([]byte,error){
 if ctx==nil||ctx.Err()!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 executable,err:=remoteTrustedGitExecutable(workspace)
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 if err:=remoteGitReadOnlyConfigSafe(ctx,workspace,executable);err!=nil{return nil,err}
 var args []string
 switch operation {
 case remotetransport.RemoteActionGitStatusFull:
  args=[]string{"-c","core.fsmonitor=false","-c","core.untrackedCache=false",
   "-C",workspace,"--no-pager","status","--short","--branch","--untracked-files=no"}
 case remotetransport.RemoteActionGitLog:
  args=[]string{"-c","core.fsmonitor=false","-c","core.untrackedCache=false",
   "-c","log.showSignature=false","-C",workspace,"--no-pager",
   "log","--no-show-signature","--no-color","--max-count=30","--format=%h %s"}
 case remotetransport.RemoteActionGitBranches:
  args=[]string{"-c","core.fsmonitor=false","-c","core.untrackedCache=false",
   "-C",workspace,"--no-pager","branch","--list","--no-color",
   "--verbose","--no-abbrev"}
 default:
  return nil,remotetransport.ErrRemoteActionDenied
 }
 cmd:=exec.CommandContext(ctx,executable,args...)
 cmd.Env=isolatedRemoteGitEnvironment()
 cmd.Stderr=io.Discard
 var output cappedRemoteGitDiffOutput
 cmd.Stdout=&output
 if err:=cmd.Run();err!=nil||ctx.Err()!=nil{
  return nil,remotetransport.ErrRemoteActionDenied
 }
 return append([]byte(nil),output.Bytes()...),nil
}
