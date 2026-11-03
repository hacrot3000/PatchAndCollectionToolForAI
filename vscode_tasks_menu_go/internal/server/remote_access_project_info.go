package server

import (
 "context"
 "os"
 "path/filepath"
 "runtime"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// readRemoteProjectInfo only returns short metadata for the one TaskDeck
// project the OWNER has already selected. It cannot enumerate projects,
// paths, files or TaskDeck API endpoints and has no caller-supplied args.
func (s *Server) readRemoteProjectInfo(ctx context.Context,session *remoteTaskDeckSession)(
 remotetransport.RemoteProjectInfo,error,
){
 var empty remotetransport.RemoteProjectInfo
 deny:=remotetransport.ErrRemoteActionDenied
 // An older daemon opted in for session.ping does not implicitly
 // disclose the host's project/workspace metadata.
 if os.Getenv("TASKDECK_REMOTE_PROJECT_INFO")!="1" || s==nil ||
  session==nil || ctx==nil || ctx.Err()!=nil ||
  s.Identity==nil || s.Workspace=="" || session.projectKey=="" ||
  session.projectKey!=s.Config.SharedProjectID{
  return empty,deny
 }
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil || !project.Enabled || project.Key!=session.projectKey ||
  ctx.Err()!=nil{
  return empty,deny
 }
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,deny}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,deny}
 info,err:=os.Stat(root)
 if err!=nil || !info.IsDir(){return empty,deny}
 workspaceName:=filepath.Base(root)
 if workspaceName=="." || workspaceName==string(filepath.Separator) ||
  workspaceName=="" {
  return empty,deny
 }
 name:=strings.TrimSpace(project.DisplayName)
 if name=="" {name=workspaceName}
 result:=remotetransport.RemoteProjectInfo{
  ProjectName:name,WorkspaceName:workspaceName,HostOS:runtime.GOOS,
 }
 // Validate on the Host, before ever encrypting or auditing success;
 // serializer separately validates on the native E2EE client.
 if len(result.ProjectName)>96 || len(result.WorkspaceName)>96{
  return empty,deny
 }
 return result,nil
}
