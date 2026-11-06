package remotetransport

import (
 "encoding/json"
 "unicode/utf8"
)

// The only executable remote action uses a previously-issued stable host
// task ID. Output remains inside the authenticated E2EE reply; never put
// process output in Portal/Relay metadata, URLs, or TaskDeck audit details.
const RemoteTaskRunResultProtocol = "taskdeck-e2ee-task-run-result-v1"
const MaxRemoteTaskOutputBytes = 192

type RemoteTaskRun struct {
 TaskID int
 ExitCode int
 Output string
 Truncated bool
}
type remoteTaskRunWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 TaskID int `json:"task_id"`
 ExitCode int `json:"exit_code"`
 Output string `json:"output"`
 Truncated bool `json:"truncated"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func validRemoteTaskRun(result RemoteTaskRun)bool{
 return validSelectedTaskID(result.TaskID) && result.ExitCode>=0 && result.ExitCode<=255 &&
  len(result.Output)<=MaxRemoteTaskOutputBytes && utf8.ValidString(result.Output)
}

func SealRemoteActionTaskRunResult(ch *Channel,id string,result RemoteTaskRun)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!validRemoteTaskRun(result){
  return nil,ErrInvalidRemoteAction
 }
 for {
  raw,err:=json.Marshal(remoteTaskRunWire{
   Type:RemoteTaskRunResultProtocol,RequestID:id,TaskID:result.TaskID,
   ExitCode:result.ExitCode,Output:result.Output,
   Truncated:result.Truncated,RemoteControlAvailable:false,
  })
  if err!=nil{return nil,ErrInvalidRemoteAction}
  if len(raw)<=maxRemoteActionPlaintext{return ch.Seal(raw)}
  if result.Output==""{return nil,ErrInvalidRemoteAction}
  _,lastSize:=utf8.DecodeLastRuneInString(result.Output)
  result.Output=result.Output[:len(result.Output)-lastSize]
  result.Truncated=true
 }
}

func OpenRemoteActionTaskRunResult(ch *Channel,frame []byte,wantID string,taskID int)(RemoteTaskRun,error){
 var denied RemoteTaskRun
 if ch==nil||!ch.Established()||!validActionID(wantID)||!validSelectedTaskID(taskID){
  return denied,ErrInvalidRemoteAction
 }
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","task_id",
  "exit_code","output","truncated","remote_control_available")
 if err!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 var typ,id string
 var result RemoteTaskRun
 if (string(fields["truncated"])!="true"&&string(fields["truncated"])!="false")||
  string(fields["remote_control_available"])!="false"||
  json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&id)!=nil||
  json.Unmarshal(fields["task_id"],&result.TaskID)!=nil||
  json.Unmarshal(fields["exit_code"],&result.ExitCode)!=nil||
  json.Unmarshal(fields["output"],&result.Output)!=nil||
  json.Unmarshal(fields["truncated"],&result.Truncated)!=nil||
  typ!=RemoteTaskRunResultProtocol||id!=wantID||result.TaskID!=taskID||
  !validRemoteTaskRun(result) {
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return result,nil
}
