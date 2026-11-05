package remotetransport

import "encoding/json"

// Features is a read-only, advisory snapshot of which already-reviewed,
// host-opted-in GUI actions the current TaskDeck principal may attempt.
// The host must still revalidate key, password user, RBAC, portal grant and
// opt-in for EACH later action; this object grants no authority.
const RemoteFeaturesResultProtocol = "taskdeck-e2ee-session-features-result-v1"

type RemoteFeatures struct {
 SessionPing bool
 ProjectInfo bool
 TasksPreview bool
 TaskRun bool
 TaskLong bool
 GitStatus bool
 GitLog bool
 GitBranches bool
 GitBranchCreate bool
 GitDiff bool
 GitPatch bool
 GitPull bool
 GitFetch bool
 GitStageTracked bool
 GitUnstageAll bool
 GitStageFile bool
 GitUnstageFile bool
 GitCommit bool
 GitPush bool
 Terminal bool
}

type remoteFeaturesWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 SessionPing bool `json:"session_ping"`
 ProjectInfo bool `json:"project_info"`
 TasksPreview bool `json:"tasks_preview"`
 TaskRun bool `json:"task_run"`
 TaskLong bool `json:"task_long,omitempty"`
 GitStatus bool `json:"git_status"`
 GitLog bool `json:"git_log,omitempty"`
 GitBranches bool `json:"git_branches,omitempty"`
 GitBranchCreate bool `json:"git_branch_create,omitempty"`
 GitDiff bool `json:"git_diff,omitempty"`
 GitPatch bool `json:"git_patch,omitempty"`
 GitPull bool `json:"git_pull,omitempty"`
 GitFetch bool `json:"git_fetch,omitempty"`
 GitStageTracked bool `json:"git_stage_tracked,omitempty"`
 GitUnstageAll bool `json:"git_unstage_all,omitempty"`
 GitStageFile bool `json:"git_stage_file,omitempty"`
 GitUnstageFile bool `json:"git_unstage_file,omitempty"`
 GitCommit bool `json:"git_commit,omitempty"`
 GitPush bool `json:"git_push,omitempty"`
 Terminal bool `json:"terminal,omitempty"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func SealRemoteActionFeaturesResult(
 ch *Channel,id string,capabilities RemoteFeatures,
)([]byte,error){
 if ch==nil || !ch.Established() || !validActionID(id){
  return nil,ErrInvalidRemoteAction
 }
 clear,err:=json.Marshal(remoteFeaturesWire{
  Type:RemoteFeaturesResultProtocol,RequestID:id,
  SessionPing:capabilities.SessionPing,
  ProjectInfo:capabilities.ProjectInfo,
  TasksPreview:capabilities.TasksPreview,
  TaskRun:capabilities.TaskRun,
  TaskLong:capabilities.TaskLong,
  GitStatus:capabilities.GitStatus,
  GitLog:capabilities.GitLog,
  GitBranches:capabilities.GitBranches,
  GitBranchCreate:capabilities.GitBranchCreate,
  GitDiff:capabilities.GitDiff,
  GitPatch:capabilities.GitPatch,
  GitPull:capabilities.GitPull,
  GitFetch:capabilities.GitFetch,
  GitStageTracked:capabilities.GitStageTracked,
  GitUnstageAll:capabilities.GitUnstageAll,
  GitStageFile:capabilities.GitStageFile,
  GitUnstageFile:capabilities.GitUnstageFile,
  GitCommit:capabilities.GitCommit,
  GitPush:capabilities.GitPush,
  Terminal:capabilities.Terminal,
  RemoteControlAvailable:false,
 })
 if err!=nil || len(clear)>maxRemoteActionPlaintext {
  return nil,ErrInvalidRemoteAction
 }
 return ch.Seal(clear)
}

func OpenRemoteActionFeaturesResult(
 ch *Channel,frame []byte,wantID string,
)(RemoteFeatures,error){
 var denied RemoteFeatures
 if ch==nil || !ch.Established() || !validActionID(wantID){
  return denied,ErrInvalidRemoteAction
 }
 clear,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 // Optional advisory flags must remain individually compatible with old
 // native Agents. Decode once, but still reject duplicate/unknown keys.
 required:=[]string{"type","request_id","session_ping","project_info",
  "tasks_preview","git_status","remote_control_available"}
 optional:=[]string{"task_run","task_long","git_pull","git_commit","git_push",
  "terminal","git_log","git_diff","git_fetch","git_stage_tracked",
  "git_unstage_all","git_stage_file","git_unstage_file","git_branches","git_patch","git_branch_create"}
 fields,err:=decodeActionObjectWithOptionals(clear,required,optional)
 if err!=nil{
  ch.Close()
  return denied,ErrInvalidRemoteAction
 }
 // encoding/json silently accepts JSON null into a bool without an error.
 // Require canonical JSON boolean tokens for each security-relevant flag.
 for _,key:=range []string{"session_ping","project_info","tasks_preview","git_status","remote_control_available"}{
  token:=string(fields[key])
  if token!="true" && token!="false"{
   ch.Close()
   return denied,ErrInvalidRemoteAction
  }
 }
 if token,ok:=fields["task_long"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["git_patch"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["git_diff"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["git_branch_create"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["git_branches"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["git_log"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 for _,key:=range []string{"git_stage_tracked","git_unstage_all","git_stage_file","git_unstage_file"}{
  if token,ok:=fields[key];ok && string(token)!="true"&&string(token)!="false"{
   ch.Close();return denied,ErrInvalidRemoteAction
  }
 }
 if token,ok:=fields["git_fetch"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["git_pull"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["git_commit"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["git_push"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["terminal"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["task_run"];ok && string(token)!="true" && string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,id string
 var remoteControl bool
 var caps RemoteFeatures
 if json.Unmarshal(fields["type"],&typ)!=nil ||
  json.Unmarshal(fields["request_id"],&id)!=nil ||
  json.Unmarshal(fields["session_ping"],&caps.SessionPing)!=nil ||
  json.Unmarshal(fields["project_info"],&caps.ProjectInfo)!=nil ||
  json.Unmarshal(fields["tasks_preview"],&caps.TasksPreview)!=nil ||
  json.Unmarshal(fields["git_status"],&caps.GitStatus)!=nil ||
  json.Unmarshal(fields["remote_control_available"],&remoteControl)!=nil ||
  typ!=RemoteFeaturesResultProtocol || id!=wantID || remoteControl{
  ch.Close()
  return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["task_run"];ok {
  if json.Unmarshal(token,&caps.TaskRun)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["task_long"];ok{
  if json.Unmarshal(token,&caps.TaskLong)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_diff"];ok{
  if json.Unmarshal(token,&caps.GitDiff)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_patch"];ok{
  if json.Unmarshal(token,&caps.GitPatch)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_log"];ok{
  if json.Unmarshal(token,&caps.GitLog)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_branches"];ok{
  if json.Unmarshal(token,&caps.GitBranches)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_branch_create"];ok{
  if json.Unmarshal(token,&caps.GitBranchCreate)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_stage_tracked"];ok{
  if json.Unmarshal(token,&caps.GitStageTracked)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_unstage_all"];ok{
  if json.Unmarshal(token,&caps.GitUnstageAll)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_stage_file"];ok{
  if json.Unmarshal(token,&caps.GitStageFile)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_unstage_file"];ok{
  if json.Unmarshal(token,&caps.GitUnstageFile)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_fetch"];ok{
  if json.Unmarshal(token,&caps.GitFetch)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_pull"];ok{
  if json.Unmarshal(token,&caps.GitPull)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_commit"];ok{
  if json.Unmarshal(token,&caps.GitCommit)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_push"];ok{
  if json.Unmarshal(token,&caps.GitPush)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["terminal"];ok{
  if json.Unmarshal(token,&caps.Terminal)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 return caps,nil
}
