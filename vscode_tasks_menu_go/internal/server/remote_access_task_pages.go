package server

import (
 "bytes"
 "context"
 "os"
 "path/filepath"
 "strings"
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
  return remotetransport.RemoteTasksPreview{Total:0,Labels:[]string{},IDs:[]int{}},0,zero,nil
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
 // Fingerprint BOTH the canonical host workspace and its task source.
 // Identical tasks.json bytes in a switched workspace are not the same
 // authorized page sequence. Never transmit this hash or path to PHP.
 hash:=remoteTaskSourceDigest(root,before)
 if expectedHash!=nil && hash!=*expectedHash{
  return empty,0,zero,denied
 }
 list,err:=tasks.Load(root)
 if err!=nil||ctx.Err()!=nil||
  len(list)>remotetransport.MaxRemoteTasksCount||start>len(list)||
  (expectedHash!=nil && start>=len(list)){
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
  Labels:make([]string,0,remotetransport.MaxRemoteTasksPreviewLabels),
  IDs:make([]int,0,remotetransport.MaxRemoteTasksPreviewLabels),
 }
 idCount:=make(map[int]int,len(list))
 for _,task:=range list{idCount[task.ID]++}
 positions:=make([]int,0,remotetransport.MaxRemoteTasksPreviewLabels)
 for i,task:=range list{
  if ctx.Err()!=nil{return empty,0,zero,denied}
  label:=safeRemoteTaskPreviewLabel(task.MenuLabel)
  if label==""{label=safeRemoteTaskPreviewLabel(task.Label)}
  if label==""{continue}
  // Even a valid 64-byte string of angle brackets expands 6x in JSON.
  // Keep one label guaranteed to fit the legacy 512-byte AEAD frame.
  label=strings.TrimSpace(limitRemoteTaskPageLabel(label,48))
  if label==""{continue}
  preview.Total++ // count only safely displayable task labels
  if i<start||len(positions)==remotetransport.MaxRemoteTasksPreviewLabels{
   continue
  }
  preview.Labels=append(preview.Labels,label)
  taskID:=task.ID
  if idCount[taskID]!=1{taskID=0} // duplicates never selectable
  preview.IDs=append(preview.IDs,taskID)
  positions=append(positions,i)
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
