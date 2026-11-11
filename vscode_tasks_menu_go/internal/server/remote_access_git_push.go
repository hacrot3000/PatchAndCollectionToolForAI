package server

import (
 "context"
 "io"
 "net/url"
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

const remoteGitPushTimeout=9*time.Second
var remoteGitPushName=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var remoteGitPushSCPURL=regexp.MustCompile(`^[A-Za-z0-9_.-]+@[A-Za-z0-9_.-]+:[A-Za-z0-9_./-]+$`)

// Only a single SSH or HTTPS remote is admitted. In particular, disallow
// file://, absolute paths, git://, ext::, unknown helpers, URL-embedded
// credentials and multiple push URLs. No remote selection comes from PHP.
func validRemoteGitPushURL(raw string)bool{
 if raw==""||len(raw)>512||strings.ContainsAny(raw,"\r\n\t\x00 "){
  return false
 }
 if remoteGitPushSCPURL.MatchString(raw){
  tail:=strings.SplitN(raw,":",2)[1]
  if strings.HasPrefix(tail,"/"){return false}
  for _,part:=range strings.Split(tail,"/"){
   if part=="."||part==".."||part==""{return false}
  }
  return true
 }
 parsed,err:=url.Parse(raw)
 if err!=nil||parsed.Hostname()==""||parsed.Opaque!=""||
  parsed.Fragment!=""||parsed.RawQuery!="" {return false}
 // URL-form SSH/HTTPS repositories need the same path traversal guard as
 // SCP-style URLs. Parse also decodes percent-escaped path components, so
 // encoded dot segments, slashes and control characters cannot bypass it.
 if !validRemoteGitPushURLPath(parsed.Path)||
  strings.Contains(strings.ToLower(parsed.EscapedPath()),"%2f")||
  strings.Contains(strings.ToLower(parsed.EscapedPath()),"%5c"){
  return false
 }
 switch parsed.Scheme{
 case "ssh":
  return parsed.User!=nil && parsed.User.Username()!="" &&
   parsed.User.String()==parsed.User.Username() && parsed.Path!=""
 case "https":
  return parsed.User==nil && parsed.Path!=""
 default:
  return false
 }
}

// Git transports can interpret dot segments (including percent-encoded
// forms) differently than Go's URL parser. Never allow a configured network
// remote to traverse or escape its explicit repository URL path.
func validRemoteGitPushURLPath(value string)bool{
 if value==""||len(value)>256||
  strings.ContainsAny(value,"\\%\x00\r\n\t "){
  return false
 }
 path:=strings.TrimPrefix(value,"/")
 if path==""||strings.HasPrefix(path,"/"){return false}
 for _,segment:=range strings.Split(path,"/"){
  if segment==""||segment=="."||segment==".."{return false}
 }
 return true
}

// Push only the exact current commit to the already configured upstream
// branch, without force, tags or client-provided remote/refspec. Local Git
// configuration supplies the remote; the E2EE client sends no arguments.
func (s *Server) executeRemoteGitPush(ctx context.Context,
 session *remoteTaskDeckSession)(remotetransport.RemoteGitPushResult,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteGitPushResult
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  runtime.GOOS=="windows"||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_PUSH")!="1"||
  session.closed.Load()||session.channel==nil||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){
  return empty,denied
 }
 // A trusted GUI disables these while a task runs, but only the host
 // can enforce exclusion against another authenticated E2EE action.
 release,err:=s.reserveRemoteGitMutation(ctx,session)
 if err!=nil{return empty,denied}
 defer release()
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return empty,denied}
 // Deliberately do not forward GIT_* inherited variables from the Agent.
 // Host SSH agent and local approved SSH configuration may be used.
 env:=[]string{
  "PATH="+os.Getenv("PATH"),"HOME="+os.Getenv("HOME"),
  "SSH_AUTH_SOCK="+os.Getenv("SSH_AUTH_SOCK"),
  "LC_ALL=C","GIT_TERMINAL_PROMPT=0","GCM_INTERACTIVE=never",
  "GIT_ASKPASS=/usr/bin/false","SSH_ASKPASS=/usr/bin/false",
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_PROTOCOL_FROM_USER=0","GIT_SSH_COMMAND=ssh -oBatchMode=yes",
 }
 base:=[]string{
  "-c","core.hooksPath="+os.DevNull,
  "-c","core.fsmonitor=false",
  // Empty override resets helpers configured by the repository; the
  // fixed GIT_ASKPASS binary denies any remaining interactive prompt.
  "-c","credential.helper=",
  "-c","protocol.allow=never",
  "-c","protocol.ssh.allow=always",
  "-c","protocol.https.allow=always",
  "-c","protocol.ext.allow=never",
  "-c","protocol.file.allow=never",
  "-c","push.followTags=false",
  // Repo-owned push.pushOption values are sent to remote receive hooks.
  // Reset them, disable repo-configured GPG executables, and forbid
  // recursive pushes to submodules beyond this audited upstream branch.
  "-c","push.pushOption=",
  "-c","push.gpgSign=false",
  "-c","push.recurseSubmodules=no",
  "-C",root,
 }
 run:=func(callCtx context.Context,args ...string)(string,error){
  if callCtx.Err()!=nil||session.closed.Load(){return "",denied}
  cmd:=exec.CommandContext(callCtx,"git",append(append([]string(nil),base...),args...)...)
  cmd.Dir=root
  cmd.Env=env
  output:=&boundedCommandOutput{remaining:512}
  cmd.Stdout=output
  cmd.Stderr=io.Discard
  prepareGitJobProcess(cmd)
  cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
  cmd.WaitDelay=time.Second
  err:=cmd.Run()
  killGitJobProcess(cmd)
  if err!=nil||callCtx.Err()!=nil||session.closed.Load()||output.truncated{
   return "",denied
  }
  return strings.TrimSpace(output.String()),nil
 }
 top,err:=run(ctx,"rev-parse","--show-toplevel")
 if err!=nil{return empty,denied}
 canonicalTop,err:=filepath.EvalSymlinks(top)
 if err!=nil{return empty,denied}
 canonicalTop,err=filepath.Abs(canonicalTop)
 if err!=nil||filepath.Clean(canonicalTop)!=filepath.Clean(root){return empty,denied}
 localRef,err:=run(ctx,"symbolic-ref","-q","HEAD")
 if err!=nil||!strings.HasPrefix(localRef,"refs/heads/"){return empty,denied}
 branch:=strings.TrimPrefix(localRef,"refs/heads/")
 if branch==""||len(branch)>200||strings.ContainsAny(branch,"\r\n"){return empty,denied}
 remote,err:=run(ctx,"config","--get","branch."+branch+".remote")
 if err!=nil||!remoteGitPushName.MatchString(remote){return empty,denied}
 merge,err:=run(ctx,"config","--get","branch."+branch+".merge")
 if err!=nil||merge!=localRef{return empty,denied}
 // branch.<name>.merge alone does not prove that the local tracking ref
 // represents that branch: remote.origin.fetch can map it to a different
 // refs/remotes destination. Refuse a misleading upstream before any
 // network side effect (the Pull preflight enforces this same binding).
 tracking,err:=run(ctx,"rev-parse","--symbolic-full-name","@{upstream}")
 if err!=nil||tracking!="refs/remotes/"+remote+"/"+branch{
  return empty,denied
 }
 // The effective push URL is resolved by Git, including url.*.pushInsteadOf.
 // Reject multiple destinations before starting a network mutation.
 pushURL,err:=run(ctx,"remote","get-url","--push","--all",remote)
 if err!=nil||!validRemoteGitPushURL(pushURL){return empty,denied}
 if !remoteGitNoRepositoryHTTPOverride(ctx,root,env){return empty,denied}
 head,err:=run(ctx,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(head){return empty,denied}
 upstream,err:=run(ctx,"rev-parse","--verify","@{upstream}^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(upstream){return empty,denied}
 // A stale tracking ref cannot turn this operation into an explicit force.
 // The server still independently enforces fast-forward against its own tip.
 ancestor:=exec.CommandContext(ctx,"git",append(append([]string(nil),base...),
  "merge-base","--is-ancestor",upstream,head)...)
 ancestor.Dir=root
 ancestor.Env=env
 ancestor.Stdout=io.Discard
 ancestor.Stderr=io.Discard
 prepareGitJobProcess(ancestor)
 ancestor.Cancel=func()error{killGitJobProcess(ancestor);return nil}
 ancestor.WaitDelay=time.Second
 err=ancestor.Run()
 killGitJobProcess(ancestor)
 if err!=nil||ctx.Err()!=nil||session.closed.Load(){return empty,denied}
 if err:=s.auditRemoteGitPushAttempt(ctx,session);err!=nil{return empty,denied}
 if !remoteGitMutationPostAuditAllowed(ctx,session,"TASKDECK_REMOTE_GIT_PUSH"){
  return empty,denied
 }
 pushCtx,cancel:=context.WithTimeout(ctx,remoteGitPushTimeout)
 defer cancel()
 if pushCtx.Err()!=nil||session.closed.Load(){return empty,denied}
 // Pin the already verified HEAD object ID to guard against concurrent
 // local branch movement. The single refspec contains no leading '+' and
 // no tag ref; hooks and non-fast-forward updates remain disabled.
 tlsFlags:=[]string{"-c","http.sslVerify=true","-c","http.followRedirects=false"}
 if strings.HasPrefix(pushURL,"https://"){
  tlsFlags=append(tlsFlags,
   "-c","http."+pushURL+".sslVerify=true",
   "-c","http."+pushURL+".followRedirects=false")
 }
 args:=append(tlsFlags,"-c","remote."+remote+".mirror=false",
  "push","--no-verify","--no-tags","--no-signed",
  "--recurse-submodules=no","--receive-pack=git-receive-pack",
  remote,head+":"+localRef)
 _,err=run(pushCtx,args...)
 if err!=nil||pushCtx.Err()!=nil||session.closed.Load(){return empty,denied}
 return remotetransport.RemoteGitPushResult{Head:head},nil
}

func (s *Server) auditRemoteGitPushAttempt(ctx context.Context,
 session *remoteTaskDeckSession)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||s.Identity==nil||session==nil||session.userID==""||
  ctx==nil||ctx.Err()!=nil{return denied}
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled{return denied}
 eventID,err:=identity.NewID()
 if err!=nil{return denied}
 userID,projectID:=session.userID,project.ID
 if err:=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:eventID,Timestamp:time.Now().UTC(),
  UserID:&userID,ProjectID:&projectID,
  Action:"remote.action.git_push",ResourceType:"git",Result:"attempted",
  Details:`{"transport":"e2ee","git_action":"push","force":false,"tags":false}`,
 });err!=nil||ctx.Err()!=nil{return denied}
 return nil
}
