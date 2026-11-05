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
 GitPull bool
 GitCommit bool
 GitPush bool
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
 GitPull bool `json:"git_pull,omitempty"`
 GitCommit bool `json:"git_commit,omitempty"`
 GitPush bool `json:"git_push,omitempty"`
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
  GitPull:capabilities.GitPull,
  GitCommit:capabilities.GitCommit,
  GitPush:capabilities.GitPush,
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
 // Strict exact-key decoding: optional flags are independently omitted by
 // legacy/partially opted-in agents. Enumerate each subset; unknown and
 // duplicate fields still poison the E2EE channel.
 required:=[]string{"type","request_id","session_ping","project_info",
  "tasks_preview","task_run","git_status","remote_control_available"}
 optional:=[]string{"task_long","git_pull","git_commit","git_push"}
 var fields map[string]json.RawMessage
 for mask:=0;mask<(1<<len(optional));mask++{
  keys:=append([]string(nil),required...)
  for i,key:=range optional{
   if mask&(1<<i)!=0{keys=append(keys,key)}
  }
  fields,err=decodeActionObject(clear,keys...)
  if err==nil{break}
 }
 if err!=nil{
  // Pre-task-run native Agents did not include task_run.
  fields,err=decodeActionObject(clear,"type","request_id",
   "session_ping","project_info","tasks_preview","git_status",
   "remote_control_available")
 }
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
 if token,ok:=fields["git_pull"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["git_commit"];ok && string(token)!="true"&&string(token)!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 if token,ok:=fields["git_push"];ok && string(token)!="true"&&string(token)!="false"{
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
 if token,ok:=fields["git_pull"];ok{
  if json.Unmarshal(token,&caps.GitPull)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_commit"];ok{
  if json.Unmarshal(token,&caps.GitCommit)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 if token,ok:=fields["git_push"];ok{
  if json.Unmarshal(token,&caps.GitPush)!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 }
 return caps,nil
}
