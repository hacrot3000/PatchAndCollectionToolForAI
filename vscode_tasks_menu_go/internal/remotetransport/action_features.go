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
 GitStatus bool
}

type remoteFeaturesWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 SessionPing bool `json:"session_ping"`
 ProjectInfo bool `json:"project_info"`
 TasksPreview bool `json:"tasks_preview"`
 GitStatus bool `json:"git_status"`
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
  GitStatus:capabilities.GitStatus,
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
 fields,err:=decodeActionObject(clear,"type","request_id",
  "session_ping","project_info","tasks_preview","git_status","remote_control_available")
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
 return caps,nil
}
