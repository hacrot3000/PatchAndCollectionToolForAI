package remotetransport

import (
 "encoding/json"
 "unicode/utf8"
)

// A managed task handle is session-scoped, never a transferable TaskDeck
// credential, filesystem path or operating-system PID. It is protected by
// the authenticated channel, the current password principal and Portal grant.
const RemoteManagedTaskMaxOutput=3200
const remoteManagedTaskResultType="taskdeck-e2ee-managed-task-result-v1"
const RemoteManagedTaskLifetimeSeconds=30

type RemoteManagedTaskResult struct {
 Handle string
 State string
 ExitCode int
 Output string
 // OutputOmitted means exit status is authenticated, but the full process
 // transcript did not fit the bounded result. It never carries a tail.
 OutputOmitted bool
}
type remoteManagedTaskResultWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 OK bool `json:"ok"`
 Handle string `json:"handle"`
 State string `json:"state"`
 ExitCode int `json:"exit_code"`
 Output string `json:"output"`
 OutputOmitted bool `json:"output_omitted"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func ValidManagedTaskHandle(v string)bool{return validActionID(v)}
func validManagedTaskResult(v RemoteManagedTaskResult)bool{
 if !ValidManagedTaskHandle(v.Handle)||len(v.Output)>RemoteManagedTaskMaxOutput||
  !utf8.ValidString(v.Output){return false}
 // An explicit omission is possible only on a terminal receipt, never on
 // a live process or alongside unverified partial stdout.
 if v.OutputOmitted&&(v.Output!=""||
  (v.State!="exited"&&v.State!="stopped")){return false}
 switch v.State {
 case "running","cancel_requested":
  return v.ExitCode==-1 && v.Output==""
 case "exited":
  return v.ExitCode>=0&&v.ExitCode<=255
 case "stopped":
  return v.ExitCode>=-1&&v.ExitCode<=255
 default:return false
 }
}
func SealRemoteManagedTaskResult(ch *Channel,id string,v RemoteManagedTaskResult)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validManagedTaskResult(v){return nil,ErrInvalidRemoteAction}
 data,err:=json.Marshal(remoteManagedTaskResultWire{
  Type:remoteManagedTaskResultType,RequestID:id,OK:true,
  Handle:v.Handle,State:v.State,ExitCode:v.ExitCode,Output:v.Output,
  OutputOmitted:v.OutputOmitted,RemoteControlAvailable:false,
 })
 if err!=nil||len(data)>RemoteGitDiffMaxPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(data)
}
func OpenRemoteManagedTaskResult(ch *Channel,frame []byte,id string)(RemoteManagedTaskResult,error){
 denied:=ErrInvalidRemoteAction
 if ch==nil||!ch.Established()||!validActionID(id){return RemoteManagedTaskResult{},denied}
 data,err:=ch.Open(frame)
 if err!=nil{return RemoteManagedTaskResult{},denied}
 defer wipeLoginBytes(data)
 raw,err:=decodeActionObjectBounded(data,RemoteGitDiffMaxPlaintext,
  "type","request_id","ok","handle","state","exit_code","output","output_omitted","remote_control_available")
 if err!=nil{ch.Close();return RemoteManagedTaskResult{},denied}
 var v remoteManagedTaskResultWire
 if json.Unmarshal(raw["type"],&v.Type)!=nil||
  json.Unmarshal(raw["request_id"],&v.RequestID)!=nil||
  json.Unmarshal(raw["ok"],&v.OK)!=nil||
  json.Unmarshal(raw["handle"],&v.Handle)!=nil||
  json.Unmarshal(raw["state"],&v.State)!=nil||
  json.Unmarshal(raw["exit_code"],&v.ExitCode)!=nil||
  json.Unmarshal(raw["output"],&v.Output)!=nil||
  json.Unmarshal(raw["output_omitted"],&v.OutputOmitted)!=nil||
  json.Unmarshal(raw["remote_control_available"],&v.RemoteControlAvailable)!=nil||
  v.Type!=remoteManagedTaskResultType||v.RequestID!=id||!v.OK||
  v.RemoteControlAvailable||
  !validManagedTaskResult(RemoteManagedTaskResult{Handle:v.Handle,State:v.State,ExitCode:v.ExitCode,Output:v.Output,OutputOmitted:v.OutputOmitted}){
  ch.Close();return RemoteManagedTaskResult{},denied
 }
 return RemoteManagedTaskResult{Handle:v.Handle,State:v.State,ExitCode:v.ExitCode,Output:v.Output,OutputOmitted:v.OutputOmitted},nil
}
