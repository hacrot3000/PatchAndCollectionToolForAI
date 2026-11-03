package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "os"
 "path/filepath"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// Host-owned read-only page; PHP and client cannot select a task index,
// filesystem path, project, command or execution parameter. Each page is
// bounded by the SAME original 512-byte authenticated plaintext mailbox.
// Content may change between reads; hash mismatch fails closed.
func (s *Server) readRemoteTaskPage(ctx context.Context,session *remoteTaskDeckSession,
 start int,expectedHash *[32]byte,requestID string,
)(remotetransport.RemoteTasksPreview,int,[32]byte,error){
 var empty remotetransport.RemoteTasksPreview
 var zero [32]byte
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||session==nil||
  os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")!="1"||
  s.Identity==nil||s.Workspace==""||start<0||
  session.projectKey==""||session.projectKey!=s.Config.SharedProjectID{
  return empty,0,zero,denied
 }
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled||project.Key!=session.projectKey{
  return empty,0,zero,denied
 }
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,0,zero,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,0,zero,denied}
 dir:=filepath.Join(root,".vscode")
 dirInfo,err:=os.Lstat(dir)
 if os.IsNotExist(err)&&start==0&&expectedHash==nil{
  return remotetransport.RemoteTasksPreview{Total:0,Labels:[]string{}},0,zero,nil
 }
 if err!=nil||!dirInfo.IsDir()||dirInfo.Mode()&os.ModeSymlink!=0{
  return empty,0,zero,denied
 }
 filename:=filepath.Join(dir,"tasks.json")
 info,err:=os.Lstat(filename)
 if os.IsNotExist(err)&&start==0&&expectedHash==nil{
  return remotetransport.RemoteTasksPreview{Total:0,Labels:[]string{}},0,zero,nil
 }
 if err!=nil||!info.Mode().IsRegular()||
  info.Size()<2||info.Size()>maxRemoteTasksPreviewSourceBytes{
  return empty,0,zero,denied
 }
 before,err:=os.ReadFile(filename)
 if err!=nil||len(before)<2||len(before)>maxRemoteTasksPreviewSourceBytes{
  return empty,0,zero,denied
 }
 hash:=sha256.Sum256(before)
 if expectedHash!=nil && hash!=*expectedHash{
  return empty,0,zero,denied
 }
 list,err:=tasks.Load(root)
 if err!=nil||ctx.Err()!=nil||
  len(list)>remotetransport.MaxRemoteTasksCount||start>len(list){
  return empty,0,zero,denied
 }
 // Do not trust a file that changed between fingerprinting and parsing.
 after,err:=os.ReadFile(filename)
 if err!=nil||!bytes.Equal(before,after){
  return empty,0,zero,denied
 }
 info,err=os.Lstat(filename)
 if err!=nil||!info.Mode().IsRegular()||ctx.Err()!=nil{
  return empty,0,zero,denied
 }
 preview:=remotetransport.RemoteTasksPreview{
  Total:len(list),Labels:make([]string,0,remotetransport.MaxRemoteTasksPreviewLabels),
 }
 positions:=make([]int,0,remotetransport.MaxRemoteTasksPreviewLabels)
 for i:=start;i<len(list);i++{
  if ctx.Err()!=nil{return empty,0,zero,denied}
  task:=list[i]
  label:=safeRemoteTaskPreviewLabel(task.MenuLabel)
  if label==""{label=safeRemoteTaskPreviewLabel(task.Label)}
  if label==""{continue}
  // Even a valid 64-byte string of angle brackets expands 6x in JSON.
  // Keep one label guaranteed to fit the legacy 512-byte AEAD frame.
  label=limitRemoteTaskPageLabel(label,48)
  if label==""{continue}
  preview.Labels=append(preview.Labels,label)
  positions=append(positions,i)
  if len(positions)==remotetransport.MaxRemoteTasksPreviewLabels{break}
 }
 fit,err:=remotetransport.BoundRemoteTasksPreview(preview,requestID)
 if err!=nil{return empty,0,zero,denied}
 next:=len(list)
 if len(fit.Labels)>0{next=positions[len(fit.Labels)-1]+1}
 // A page consisting entirely of rejected labels terminates safely.
 return fit,next,hash,nil
}

func limitRemoteTaskPageLabel(label string,maxBytes int)string{
 if len(label)<=maxBytes{return label}
 used:=0
 for i,r:=range label{
  n:=utf8.RuneLen(r)
  if n<1||used+n>maxBytes{return label[:i]}
  used+=n
 }
 return label
}
