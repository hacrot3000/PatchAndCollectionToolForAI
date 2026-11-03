package server

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "unicode"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

const maxRemoteTasksPreviewSourceBytes = 1024*1024

// readRemoteTasksPreview is a fixed read-only action. Unlike TaskDeck's
// regular Tasks HTTP API it never returns a command, args, input defaults,
// file contents, TaskDeck secret IDs or executable action identifiers.
//
// The host owns the exact workspace; PHP/Relay/native clients cannot choose
// a path, task ID, script, group, executable, filter, page or limit.
// This deliberately returns a small PREVIEW rather than a full task index.
func (s *Server) readRemoteTasksPreview(
 ctx context.Context,session *remoteTaskDeckSession,
)(remotetransport.RemoteTasksPreview,error){
 var empty remotetransport.RemoteTasksPreview
 denied:=remotetransport.ErrRemoteActionDenied
 if os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")!="1"||
  s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  s.Identity==nil||session.projectKey==""||
  session.projectKey!=s.Config.SharedProjectID||s.Workspace==""{
  return empty,denied
 }
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled||project.Key!=session.projectKey||
  ctx.Err()!=nil{return empty,denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 rootInfo,err:=os.Stat(root)
 if err!=nil||!rootInfo.IsDir(){return empty,denied}
 // Block .vscode/tasks.json symlinks and large/unbounded task definitions.
 // The trusted host, not the Portal or client, controls the workspace path.
 dir:=filepath.Join(root,".vscode")
 dirInfo,err:=os.Lstat(dir)
 if err!=nil||!dirInfo.IsDir()||dirInfo.Mode()&os.ModeSymlink!=0{
  return empty,denied
 }
 definition:=filepath.Join(dir,"tasks.json")
 info,err:=os.Lstat(definition)
 if err!=nil||!info.Mode().IsRegular()||
  info.Size()<2||info.Size()>maxRemoteTasksPreviewSourceBytes{
  return empty,denied
 }
 list,err:=tasks.Load(root)
 if err!=nil||ctx.Err()!=nil||
  len(list)>remotetransport.MaxRemoteTasksCount{
  return empty,denied
 }
 preview:=remotetransport.RemoteTasksPreview{
  Total:len(list),Labels:make([]string,0,remotetransport.MaxRemoteTasksPreviewLabels),
 }
 for _,task:=range list{
  if ctx.Err()!=nil{return empty,denied}
  if len(preview.Labels)==remotetransport.MaxRemoteTasksPreviewLabels{break}
  label:=safeRemoteTaskPreviewLabel(task.MenuLabel)
  if label==""{label=safeRemoteTaskPreviewLabel(task.Label)}
  if label!=""{preview.Labels=append(preview.Labels,label)}
 }
 return preview,nil
}

func safeRemoteTaskPreviewLabel(label string)string{
 if !utf8.ValidString(label){return ""}
 label=strings.TrimSpace(label)
 var b strings.Builder
 for _,r:=range label{
  // Control and bidi characters must never become remote UI text.
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){
   return ""
  }
  size:=utf8.RuneLen(r)
  if size<1||b.Len()+size>remotetransport.MaxRemoteTaskLabelBytes{
   break
  }
  b.WriteRune(r)
 }
 result:=strings.TrimSpace(b.String())
 if !remotetransport.ValidRemoteTaskLabel(result){return ""}
 return result
}
