package server

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "path/filepath"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// Verify a different, user-launched Task execution independently of Native
// Patch feedback. Browser-supplied log text, provider messages and the Task
// display preview are NEVER the success authority. Task session metadata is
// collected from the native SessionService after process exit.
func (s *Server) verifyNativeTaskRunEvidence(x agentchat.Session, ref string, taskID int, note string)(agentchat.TaskPlanEvidence,error){
 if len(ref)==0||len(ref)>256||strings.TrimSpace(ref)!=ref||taskID<1||len(note)>512{
  return agentchat.TaskPlanEvidence{},errors.New("exact Task session ID and positive Task ID are required")
 }
 if s==nil||s.Sessions==nil{return agentchat.TaskPlanEvidence{},errors.New("native Task session service unavailable")}
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace)
 if err!=nil||identity.ID!=x.ProjectID{
  return agentchat.TaskPlanEvidence{},errors.New("Task plan project identity has changed")
 }
 meta,ok:=s.Sessions.Metadata(ref)
 if !ok||meta.ID!=ref{
  // A completed Task may have been evicted from the native broker. Only a
  // host-private, MAC-authenticated receipt created while its metadata was
  // still authoritative can serve as the fallback, never workspace history.
  meta,err=s.verifiedNativeTaskRunReceipt(ref,x.ProjectID)
  if err!=nil{return agentchat.TaskPlanEvidence{},errors.New("native Task session and authenticated completion receipt are unavailable")}
 }
 return verifyNativeTaskRunMetadata(s.Workspace,s.Config.SharedServerEnabled,x,meta,taskID,note)
}

func verifyNativeTaskRunMetadata(workspace string,shared bool,x agentchat.Session,meta session.Metadata,taskID int,note string)(agentchat.TaskPlanEvidence,error){
 failed:=func(reason string)(agentchat.TaskPlanEvidence,error){return agentchat.TaskPlanEvidence{},errors.New(reason)}
 if x.TaskPlan==nil||taskID<1||meta.ID==""||meta.TaskID!=taskID||meta.TaskID<1{
  return failed("Task ID or plan identity does not match")
 }
 if meta.Kind!=tasks.SessionKindTask && !(meta.Kind==""&&x.Owner=="local"&&!shared) {
  return failed("only native Task executions can prove a Task result")
 }
 if x.Owner=="local"&&!shared {
  if meta.OwnerUserID!=""||meta.ProjectID!=""{return failed("Task ownership does not match this local conversation")}
 }else if meta.OwnerUserID==""||meta.ProjectID!=x.ProjectID||x.Owner!="user:"+meta.OwnerUserID {
  return failed("Task owner or project identity does not match")
 }
 if meta.Status!="exited"||meta.ExitCode==nil||*meta.ExitCode!=0{
  return failed("native Task is running, stopped or exited unsuccessfully")
 }
 start,startErr:=time.Parse(time.RFC3339,meta.StartedAt)
 end,endErr:=time.Parse(time.RFC3339,meta.EndedAt)
 if startErr!=nil||endErr!=nil||end.Before(start)||start.Before(x.TaskPlan.CreatedAt.Add(-2*time.Second)){
  return failed("Task timestamps are missing, inconsistent or older than the plan")
 }
 if meta.CommandPreview==""||len(meta.CommandSHA256)!=64{
  return failed("native Task has no immutable executed-command identity")
 }
 if _,err:=hex.DecodeString(meta.CommandSHA256);err!=nil{
  return failed("invalid native command digest")
 }
 root,err:=filepath.EvalSymlinks(workspace)
 if err!=nil{return failed("project workspace cannot be resolved")}
 cwd,err:=filepath.EvalSymlinks(meta.Cwd)
 if err!=nil{return failed("Task working directory cannot be resolved")}
 root,err=filepath.Abs(root);if err!=nil{return failed("project workspace is invalid")}
 cwd,err=filepath.Abs(cwd);if err!=nil{return failed("Task working directory is invalid")}
 rel,err:=filepath.Rel(root,cwd)
 if err!=nil||filepath.IsAbs(rel)||rel==".."||strings.HasPrefix(rel,".."+string(filepath.Separator)){
  return failed("Task ran outside the verified project workspace")
 }
 receipt:=struct {
  Version int `json:"version"`
  SessionID string `json:"session_id"`
  TaskID int `json:"task_id"`
  Owner string `json:"owner"`
  ProjectID string `json:"project_id"`
  CommandSHA256 string `json:"command_sha256"`
  Cwd string `json:"cwd"`
  Status string `json:"status"`
  ExitCode int `json:"exit_code"`
  StartedAt string `json:"started_at"`
  EndedAt string `json:"ended_at"`
 }{1,meta.ID,meta.TaskID,x.Owner,x.ProjectID,meta.CommandSHA256,cwd,meta.Status,*meta.ExitCode,meta.StartedAt,meta.EndedAt}
 payload,err:=json.Marshal(receipt)
 if err!=nil{return failed("cannot encode verified Task receipt")}
 digest:=sha256.Sum256(payload)
 evidenceNote:=fmt.Sprintf("Verified native Task %d exited 0; independently review its test scope, not proof of app correctness",meta.TaskID)
 if strings.TrimSpace(note)!="" {evidenceNote+="; user note: "+strings.TrimSpace(note)}
 if len(evidenceNote)>512{return failed("Task evidence note too long")}
 return agentchat.TaskPlanEvidence{Kind:"task_run",Ref:meta.ID,Digest:hex.EncodeToString(digest[:]),Note:evidenceNote},nil
}
