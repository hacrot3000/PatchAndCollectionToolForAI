package server

import(
 "context"
 "crypto/sha256"
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

// The client never selects a Git ref, remote URL, page cursor, Git command
// or repository path. Every continuation recomputes the entire offline
// snapshot and rejects a changed canonical workspace or tracking-ref list.
func(s *Server)readRemoteGitTrackingPage(
 ctx context.Context,session *remoteTaskDeckSession,first bool,requestID string,
)(remotetransport.RemoteGitTrackingPage,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var zero remotetransport.RemoteGitTrackingPage
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||session.closed.Load()||
  session.channel==nil||!session.channel.Established()||
  session.portalGrant==nil||!session.portalGrant.ActiveForSession(session.channel){
  return zero,denied
 }
 session.gitTrackingMu.Lock()
 defer session.gitTrackingMu.Unlock()
 if !first&&!session.gitTrackingReady{return zero,denied}
 snapshot,root,err:=s.readRemoteGitTrackingSnapshot(ctx)
 if err!=nil||ctx.Err()!=nil||session.closed.Load(){
  session.gitTrackingReady=false
  return zero,denied
 }
 digest:=sha256.Sum256([]byte(root+"\x00"+snapshot))
 start,pageNo:=0,1
 if !first{
  if !session.gitTrackingReady||digest!=session.gitTrackingDigest||
   session.gitTrackingCursor>=len(snapshot){
   session.gitTrackingReady=false
   return zero,denied
  }
  start=session.gitTrackingCursor
  pageNo=session.gitTrackingPage+1
 }
 page,end,err:=remotetransport.BoundRemoteGitTrackingPage(requestID,snapshot,start,pageNo)
 if err!=nil||ctx.Err()!=nil||session.closed.Load(){
  session.gitTrackingReady=false
  return zero,denied
 }
 if page.More{
  session.gitTrackingReady=true
  session.gitTrackingCursor=end
  session.gitTrackingPage=pageNo
  session.gitTrackingDigest=digest
 }else{
  session.gitTrackingReady=false
  session.gitTrackingCursor=0
  session.gitTrackingPage=0
  session.gitTrackingDigest=[32]byte{}
 }
 return page,nil
}

func(s *Server)readRemoteGitTrackingSnapshot(ctx context.Context)(string,string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_TRACKING_REFS")!="1"{
  return "","",denied
 }
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return "","",denied}
 root,err=filepath.Abs(root)
 if err!=nil{return "","",denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return "","",denied}
 executable:=""
 for _,candidate:=range []string{"/usr/bin/git","/bin/git","/opt/homebrew/bin/git","/usr/local/bin/git"}{
  if tasks.RemoteExecutableAllowed(root,candidate){executable=candidate;break}
 }
 if executable==""{return "","",denied}
 // No credentials, user HOME/PATH, Git env overrides or network helper.
 env:=[]string{"PATH=/usr/bin:/bin","HOME=/nonexistent","LC_ALL=C","LANG=C",
  "GIT_OPTIONAL_LOCKS=0","GIT_CONFIG_NOSYSTEM=1",
  "GIT_CONFIG_GLOBAL="+os.DevNull,"GIT_CONFIG_SYSTEM="+os.DevNull,
  "GIT_NO_LAZY_FETCH=1","GIT_TERMINAL_PROMPT=0","GIT_PROTOCOL_FROM_USER=0"}
 deadline,cancel:=context.WithTimeout(ctx,7*time.Second)
 defer cancel()
 run:=func(limit int,args ...string)(string,error){
  if deadline.Err()!=nil{return "",denied}
  cmd:=exec.CommandContext(deadline,executable,args...)
  cmd.Env=env;cmd.Dir=root;cmd.Stderr=io.Discard
  output:=&boundedCommandOutput{remaining:limit}
  cmd.Stdout=output
  prepareGitJobProcess(cmd)
  cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
  cmd.WaitDelay=time.Second
  err:=cmd.Run()
  killGitJobProcess(cmd)
  if err!=nil||deadline.Err()!=nil||output.truncated{return "",denied}
  return output.buf.String(),nil
 }
 top,err:=run(4096,"-c","core.fsmonitor=false","-C",root,"rev-parse","--show-toplevel")
 if err!=nil{return "","",denied}
 canonical,err:=filepath.EvalSymlinks(strings.TrimSpace(top))
 if err!=nil{return "","",denied}
 canonical,err=filepath.Abs(canonical)
 if err!=nil||filepath.Clean(canonical)!=filepath.Clean(root){return "","",denied}
 raw,err:=run(8192,"-c","core.fsmonitor=false","-c","core.pager=cat",
  "-C",root,"--no-pager","for-each-ref","--sort=refname","--count=34",
  "--format=%(refname:lstrip=2) %(objectname)","refs/remotes")
 if err!=nil{return "","",denied}
 const header="GIT_TRACKING_V1\n"
 if raw==""{return header,root,nil}
 if !strings.HasSuffix(raw,"\n"){return "","",denied}
 lines:=strings.Split(strings.TrimSuffix(raw,"\n"),"\n")
 if len(lines)>34{return "","",denied}
 entries:=make([]string,0,len(lines))
 for _,line:=range lines{
  at:=strings.LastIndexByte(line,' ')
  if at<=0{return "","",denied}
  if strings.HasSuffix(line[:at],"/HEAD"){continue}
  entries=append(entries,line)
 }
 if len(entries)>remotetransport.MaxRemoteGitTrackingRefs{return "","",denied}
 sort.Strings(entries)
 snapshot:=header
 if len(entries)>0{snapshot+=strings.Join(entries,"\n")+"\n"}
 if !remotetransport.ValidRemoteGitTrackingSnapshot(snapshot){
  return "","",denied
 }
 return snapshot,root,nil
}
