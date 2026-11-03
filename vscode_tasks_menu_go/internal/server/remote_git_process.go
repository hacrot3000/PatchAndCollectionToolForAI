package server

import (
 "context"
 "errors"
 "io"
 "os"
 "os/exec"
 "sort"
 "strings"

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
 case remotetransport.RemoteActionGitTrackingRefs:
  // Only LOCAL, previously fetched refs/remotes/*: never contact network.
  // Ref names/OIDs are sorted, bounded and checked before E2EE sealing.
  args=[]string{"-c","core.fsmonitor=false","-c","core.untrackedCache=false",
   "-C",workspace,"--no-pager","for-each-ref","--sort=refname","--count=33",
   "--format=%(refname:lstrip=2) %(objectname)","refs/remotes"}
 case remotetransport.RemoteActionGitRemotes:
  // A fixed, offline Git builtin: NEVER use "remote -v", "get-url",
  // "ls-remote" or any network command. Only names are returned; credential-
  // bearing URLs, SSH commands, helpers and local paths must not leak.
  args=[]string{"-c","core.fsmonitor=false","-c","core.untrackedCache=false",
   "-C",workspace,"--no-pager","remote"}
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
 if operation==remotetransport.RemoteActionGitTrackingRefs{
  raw:=output.Bytes()
  if len(raw)>4096{return nil,remotetransport.ErrRemoteActionDenied}
  if len(raw)==0{return []byte("GIT_TRACKING_V1\n"),nil}
  if raw[len(raw)-1]!='\n'{return nil,remotetransport.ErrRemoteActionDenied}
  lines:=strings.Split(strings.TrimSuffix(string(raw),"\n"),"\n")
  if len(lines)>32{return nil,remotetransport.ErrRemoteActionDenied}
  seen:=map[string]bool{}
  var entries []string
  for _,line:=range lines{
   i:=strings.LastIndexByte(line,' ')
   if i<=0||i>=len(line)-1{return nil,remotetransport.ErrRemoteActionDenied}
   name,oid:=line[:i],line[i+1:]
   if strings.HasSuffix(name,"/HEAD"){continue} // symbolic alias, not a branch
   if !validRemoteTrackingName(name)||!validRemoteTrackingOID(oid)||seen[name]{
    return nil,remotetransport.ErrRemoteActionDenied
   }
   seen[name]=true
   entries=append(entries,name+" "+oid)
  }
  if len(entries)==0{return []byte("GIT_TRACKING_V1\n"),nil}
  return []byte("GIT_TRACKING_V1\n"+strings.Join(entries,"\n")+"\n"),nil
 }
 if operation==remotetransport.RemoteActionGitRemotes{
  // Git permits fairly broad remote names. For this remote capability,
  // accept only a small safe ASCII subset and refuse the entire result
  // otherwise; never show partial state that might mislead an operator.
  raw:=output.Bytes()
  if len(raw)>1024{return nil,remotetransport.ErrRemoteActionDenied}
  if len(raw)==0{return []byte("GIT_REMOTES_V1\n"),nil}
  if raw[len(raw)-1]!='\n'{return nil,remotetransport.ErrRemoteActionDenied}
  names:=strings.Split(strings.TrimSuffix(string(raw),"\n"),"\n")
  if len(names)>16{return nil,remotetransport.ErrRemoteActionDenied}
  seen:=make(map[string]bool,len(names))
  for _,name:=range names{
   if !validRemoteGitName(name)||seen[name]{
    return nil,remotetransport.ErrRemoteActionDenied
   }
   seen[name]=true
  }
  sort.Strings(names)
  return []byte("GIT_REMOTES_V1\n"+strings.Join(names,"\n")+"\n"),nil
 }
 return append([]byte(nil),output.Bytes()...),nil
}

// validRemoteGitName deliberately refuses whitespace, control/Bidi,
// delimiters, URLs, Git argv flags and unusual config-derived remote names.
func validRemoteGitName(name string)bool{
 if len(name)<1||len(name)>64{return false}
 for i:=0;i<len(name);i++{
  c:=name[i]
  asciiLetter:=(c>='a'&&c<='z')||(c>='A'&&c<='Z')
  digit:=c>='0'&&c<='9'
  if asciiLetter||digit{continue}
  if i==0||i==len(name)-1{return false}
  if c!='-'&&c!='_'&&c!='.'{return false}
 }
 return !strings.Contains(name,"..")
}

func validRemoteTrackingName(name string)bool{
 if len(name)<3||len(name)>160{return false}
 parts:=strings.Split(name,"/")
 if len(parts)<2||len(parts)>7{return false}
 for _,part:=range parts{
  if !validRemoteGitName(part)||strings.HasSuffix(part,".lock"){return false}
 }
 return true
}
func validRemoteTrackingOID(oid string)bool{
 if len(oid)!=40&&len(oid)!=64{return false}
 for i:=0;i<len(oid);i++{
  if !(oid[i]>='0'&&oid[i]<='9')&&!(oid[i]>='a'&&oid[i]<='f'){return false}
 }
 return true
}
