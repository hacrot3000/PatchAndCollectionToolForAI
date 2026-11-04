package remotetransport

import "encoding/json"

// An authenticated inspection proves that the selected stable task ID was
// published by this TaskDeck host in this E2EE session and the underlying
// task definition has not changed. It never authorizes remote execution.
const RemoteTaskInspectResultProtocol="taskdeck-e2ee-task-inspect-result-v1"

type RemoteTaskInspect struct {
 TaskID int
 Eligible bool
}
type remoteTaskInspectWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 TaskID int `json:"task_id"`
 Eligible bool `json:"eligible"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func SealRemoteActionTasksInspectResult(ch *Channel,id string,result RemoteTaskInspect)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validSelectedTaskID(result.TaskID)||!result.Eligible{
  return nil,ErrInvalidRemoteAction
 }
 raw,err:=json.Marshal(remoteTaskInspectWire{
  Type:RemoteTaskInspectResultProtocol,RequestID:id,TaskID:result.TaskID,
  Eligible:true,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}

func OpenRemoteActionTasksInspectResult(ch *Channel,frame []byte,wantID string,taskID int)(RemoteTaskInspect,error){
 var denied RemoteTaskInspect
 if ch==nil||!ch.Established()||!validActionID(wantID)||
  !validSelectedTaskID(taskID){return denied,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","task_id",
  "eligible","remote_control_available")
 if err!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 var typ,id string
 var selected int
 var eligible,available bool
 // Security booleans must be canonical JSON boolean literals, not null.
 if (string(fields["eligible"])!="true" && string(fields["eligible"])!="false") ||
  string(fields["remote_control_available"])!="false" ||
  json.Unmarshal(fields["type"],&typ)!=nil ||
  json.Unmarshal(fields["request_id"],&id)!=nil ||
  json.Unmarshal(fields["task_id"],&selected)!=nil ||
  json.Unmarshal(fields["eligible"],&eligible)!=nil ||
  json.Unmarshal(fields["remote_control_available"],&available)!=nil ||
  typ!=RemoteTaskInspectResultProtocol || id!=wantID ||
  selected!=taskID || !eligible || available {
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return RemoteTaskInspect{TaskID:selected,Eligible:eligible},nil
}
