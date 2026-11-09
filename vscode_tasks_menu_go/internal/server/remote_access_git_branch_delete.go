package server

import (
 "bytes"
 "context"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// Only a redundant TaskDeck-owned, ordinary LOOSE local branch whose tip
// is the current HEAD may be deleted. A reviewed host-only plan, exclusive
// HEAD.lock, an exact-ref CAS and an audit are required. There is no force,
// arbitrary path, packed-ref deletion, checkout, reset or network access.
func(s *Server)readRemoteGitBranchDeleteReview(ctx context.Context,
 owner *remoteTaskDeckSession)(remotetransport.RemoteGitBranchDeleteResult,error){
 return s.remoteGitBranchDelete(ctx,owner,false)
}
func(s *Server)executeRemoteGitBranchDelete(ctx context.Context,
 owner *remoteTaskDeckSession)(remotetransport.RemoteGitBranchDeleteResult,error){
 return s.remoteGitBranchDelete(ctx,owner,true)
}
func(s *Server)remoteGitBranchDelete(ctx context.Context,
 owner *remoteTaskDeckSession,apply bool)(remotetransport.RemoteGitBranchDeleteResult,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteGitBranchDeleteResult
 if s==nil||ctx==nil||ctx.Err()!=nil||owner==nil||
  runtime.GOOS=="windows"||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_BRANCH_DELETE")!="1"||
  owner.closed.Load()||owner.channel==nil||
  !owner.channel.Established()||owner.portalGrant==nil||
  !owner.portalGrant.ActiveForSession(owner.channel){return empty,denied}
 slug:=os.Getenv("TASKDECK_REMOTE_GIT_BRANCH_DELETE_NAME")
 branch:="taskdeck/"+slug
 if !remotetransport.ValidRemoteGitBranchDeleteName(branch){return empty,denied}
 if !owner.gitDeleteMu.TryLock(){return empty,denied}
 defer owner.gitDeleteMu.Unlock()
 previous,ready:=owner.gitDeletePreview,owner.gitDeletePreviewReady
 owner.gitDeletePreviewReady=false
 if apply&&!ready{return empty,denied}
 release,err:=owner.reserveRemoteGitMutation(ctx)
 if err!=nil{return empty,denied}
 defer release()
 deadline,cancel:=context.WithTimeout(ctx,12*time.Second)
 defer cancel()
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 if fi,err:=os.Stat(root);err!=nil||!fi.IsDir(){return empty,denied}
 gitDir:=filepath.Join(root,".git")
 gi,err:=os.Lstat(gitDir)
 if err!=nil||!gi.IsDir()||gi.Mode()&os.ModeSymlink!=0{return empty,denied}
 // Linked worktrees can check out the target without changing THIS HEAD.
 // Do not guess from Git metadata; reject the entire repository category.
 if _,err:=os.Lstat(filepath.Join(gitDir,"worktrees"));err==nil||!os.IsNotExist(err){
  return empty,denied
 }
 // Reject symlinked ref/reflog ancestor directories; Lstat of the final
 // branch leaf alone would not detect an escaping parent symlink.
 for _,dir:=range []string{
  filepath.Join(gitDir,"refs"),filepath.Join(gitDir,"refs","heads"),
  filepath.Join(gitDir,"refs","heads","taskdeck"),
 }{
  fi,e:=os.Lstat(dir)
  if e!=nil||!fi.IsDir()||fi.Mode()&os.ModeSymlink!=0{return empty,denied}
 }
 for _,dir:=range []string{
  filepath.Join(gitDir,"logs"),filepath.Join(gitDir,"logs","refs"),
  filepath.Join(gitDir,"logs","refs","heads"),
  filepath.Join(gitDir,"logs","refs","heads","taskdeck"),
 }{
  fi,e:=os.Lstat(dir)
  if os.IsNotExist(e){break}
  if e!=nil||!fi.IsDir()||fi.Mode()&os.ModeSymlink!=0{return empty,denied}
 }
 executable:=""
 for _,candidate:=range []string{
  "/usr/bin/git","/bin/git","/opt/homebrew/bin/git","/usr/local/bin/git",
 }{
  if tasks.RemoteExecutableAllowed(root,candidate){executable=candidate;break}
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
  "-c","core.hooksPath="+os.DevNull,"-c","core.logAllRefUpdates=false",
  "-c","diff.external=","-c","protocol.allow=never","-C",root,
 }
 run:=func(limit int,args ...string)(string,error){
  if deadline.Err()!=nil||owner.closed.Load(){return "",denied}
  cmd:=exec.CommandContext(deadline,executable,append(append([]string(nil),base...),args...)...)
  cmd.Dir=root;cmd.Env=env
  var output *boundedCommandOutput
  if limit>0{output=&boundedCommandOutput{remaining:limit};cmd.Stdout=output
  }else{cmd.Stdout=io.Discard}
  cmd.Stderr=io.Discard
  prepareGitJobProcess(cmd)
  cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
  cmd.WaitDelay=time.Second
  err:=cmd.Run()
  killGitJobProcess(cmd)
  if err!=nil||deadline.Err()!=nil||owner.closed.Load(){return "",denied}
  if output==nil{return "",nil}
  if output.truncated{return "",denied}
  return strings.TrimSpace(output.String()),nil
 }
 // A repository config can change Git's worktree/gitdir despite a
 // normal-looking .git directory at the TaskDeck workspace. Explicitly
 // pin BOTH canonical roots before any Git ref may be mutated.
 top,err:=run(4096,"rev-parse","--show-toplevel")
 if err!=nil{return empty,denied}
 top,err=filepath.EvalSymlinks(top)
 if err!=nil{return empty,denied}
 top,err=filepath.Abs(top)
 if err!=nil||filepath.Clean(top)!=filepath.Clean(root){return empty,denied}
 resolvedGitDir,err:=run(4096,"rev-parse","--absolute-git-dir")
 if err!=nil{return empty,denied}
 resolvedGitDir,err=filepath.EvalSymlinks(resolvedGitDir)
 if err!=nil{return empty,denied}
 resolvedGitDir,err=filepath.Abs(resolvedGitDir)
 if err!=nil||filepath.Clean(resolvedGitDir)!=filepath.Clean(gitDir){
  return empty,denied
 }
 ref:="refs/heads/"+branch
 leaf:=filepath.Join(gitDir,"refs","heads","taskdeck",slug)
 leafInfo,err:=os.Lstat(leaf)
 if err!=nil||!leafInfo.Mode().IsRegular()||
  leafInfo.Mode()&os.ModeSymlink!=0||leafInfo.Size()>80{return empty,denied}
 reflog:=filepath.Join(gitDir,"logs","refs","heads","taskdeck",slug)
 if rl,err:=os.Lstat(reflog);err==nil{
  if !rl.Mode().IsRegular()||rl.Mode()&os.ModeSymlink!=0{return empty,denied}
 }else if !os.IsNotExist(err){return empty,denied}
 current,err:=run(128,"symbolic-ref","--quiet","--short","HEAD")
 if err!=nil||!remotetransport.ValidRemoteGitSwitchFrom(current)||
  current==branch{return empty,denied}
 head,err:=run(128,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(head){return empty,denied}
 targetOID,err:=run(128,"rev-parse","--verify",ref+"^{commit}")
 if err!=nil||targetOID!=head{return empty,denied}
 rawLeaf,err:=os.ReadFile(leaf)
 if err!=nil||string(rawLeaf)!=head+"\n"{return empty,denied}
 headLeaf:=filepath.Join(gitDir,"HEAD")
 headInfo,err:=os.Lstat(headLeaf)
 if err!=nil||!headInfo.Mode().IsRegular()||headInfo.Mode()&os.ModeSymlink!=0||
  headInfo.Size()>256{return empty,denied}
 actualHead,err:=os.ReadFile(headLeaf)
 if err!=nil||string(actualHead)!="ref: refs/heads/"+current+"\n"{
  return empty,denied
 }
 indexLeaf:=filepath.Join(gitDir,"index")
 indexInfo,err:=os.Lstat(indexLeaf)
 if err!=nil||!indexInfo.Mode().IsRegular()||
  indexInfo.Mode()&os.ModeSymlink!=0||indexInfo.Size()>1<<20||
  indexInfo.Size()<1{return empty,denied}
 initialIndex,err:=os.ReadFile(indexLeaf)
 if err!=nil||len(initialIndex)>1<<20{return empty,denied}
 // The read-only preview and the destructive confirmation both call
 // git status. A repository-local filter must never run just because
 // the remote user reviewed or removed an otherwise redundant ref.
 if !remoteGitNoExecutableFilters(deadline,root,env){return empty,denied}
 status,err:=run(256,"status","--porcelain=v1","--untracked-files=all",
  "--ignore-submodules=none","--no-renames")
 if err!=nil||status!=""{return empty,denied}
 // Detect non-cooperating writers before approving even the preview.
 current2,err:=run(128,"symbolic-ref","--quiet","--short","HEAD")
 if err!=nil||current2!=current{return empty,denied}
 head2,err:=run(128,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||head2!=head{return empty,denied}
 leaf2,err:=os.ReadFile(leaf)
 if err!=nil||!bytes.Equal(leaf2,rawLeaf){return empty,denied}
 index2,err:=os.ReadFile(indexLeaf)
 if err!=nil||!bytes.Equal(index2,initialIndex){return empty,denied}
 plan:=remotetransport.RemoteGitBranchDeleteResult{
  Current:current,Branch:branch,Head:head,
 }
 if !remotetransport.ValidRemoteGitBranchDeleteResult(plan){return empty,denied}
 if !apply{
  if err:=s.auditRemoteReadOnlyAction(owner,
   remotetransport.RemoteActionGitBranchDeleteReview);err!=nil{return empty,denied}
  if deadline.Err()!=nil||owner.closed.Load(){return empty,denied}
  owner.gitDeletePreview=plan
  owner.gitDeletePreviewReady=true
  return plan,nil
 }
 if previous!=plan{return empty,denied}
 // Lock HEAD exclusively: cooperative Git checkout/switch writers respect it.
 // A lock owned by a different operation is a denial, never overwritten.
 headLock:=filepath.Join(gitDir,"HEAD.lock")
 lock,err:=os.OpenFile(headLock,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
 if err!=nil{return empty,denied}
 lockInfo,err:=lock.Stat()
 if err!=nil{_ = lock.Close();return empty,denied}
 defer func(){
  if info,e:=os.Lstat(headLock);e==nil&&os.SameFile(info,lockInfo){
   _=os.Remove(headLock)
  }
  _=lock.Close()
 }()
 // Repeat the meaningful checks under HEAD.lock. Target ref itself is
 // protected by Git's compare-and-delete update-ref transaction below.
 headNow,err:=os.ReadFile(headLeaf)
 if err!=nil||!bytes.Equal(headNow,actualHead){return empty,denied}
 indexNow,err:=os.ReadFile(indexLeaf)
 if err!=nil||!bytes.Equal(indexNow,initialIndex){return empty,denied}
 leafNow,err:=os.ReadFile(leaf)
 if err!=nil||!bytes.Equal(leafNow,rawLeaf){return empty,denied}
 finalStatus,err:=run(256,"status","--porcelain=v1","--untracked-files=all",
  "--ignore-submodules=none","--no-renames")
 if err!=nil||finalStatus!=""{return empty,denied}
 if deadline.Err()!=nil||owner.closed.Load()||
  !owner.portalGrant.ActiveForSession(owner.channel){return empty,denied}
 if err:=s.auditRemoteGitBranchDeleteAttempt(deadline,owner);err!=nil{return empty,denied}
 // Exact old OID CAS: if any other writer moves the ref, Git refuses.
 _,err=run(0,"update-ref","--no-deref","-d",ref,head)
 if err!=nil{return empty,denied}
 if _,err:=os.Lstat(leaf);!os.IsNotExist(err){return empty,denied}
 headAfter,err:=run(128,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||headAfter!=head{return empty,denied}
 branchAfter,err:=run(128,"symbolic-ref","--quiet","--short","HEAD")
 if err!=nil||branchAfter!=current{return empty,denied}
 return plan,nil
}
func(s *Server)auditRemoteGitBranchDeleteAttempt(ctx context.Context,
 owner *remoteTaskDeckSession)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||s.Identity==nil||owner==nil||owner.userID==""||
  ctx==nil||ctx.Err()!=nil{return denied}
 project,err:=s.Identity.ProjectByKey(ctx,owner.projectKey)
 if err!=nil||!project.Enabled{return denied}
 id,err:=identity.NewID()
 if err!=nil{return denied}
 userID,projectID:=owner.userID,project.ID
 if err:=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:id,Timestamp:time.Now().UTC(),UserID:&userID,ProjectID:&projectID,
  Action:"remote.action.git_branch_delete",ResourceType:"git",Result:"attempted",
  Details:`{"transport":"e2ee","fixed_taskdeck_ref":true,"same_head":true,"cas":true}`,
 });err!=nil||ctx.Err()!=nil{return denied}
 return nil
}
