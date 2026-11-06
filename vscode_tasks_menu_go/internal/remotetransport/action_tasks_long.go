package remotetransport

import (
 "bytes"
 "encoding/json"
 "unicode"
 "unicode/utf8"
)

const RemoteLongTaskProgressProtocol="taskdeck-e2ee-task-progress-v1"
// 128 bytes fits 512-byte E2EE envelope even with every byte JSON-escaped.
const MaxRemoteLongTaskOutputBytes=128

const RemoteActionTasksStart="tasks.start"
// Separately opted-in, ID-only, host-constrained linear dependency workflow.
const RemoteActionTasksWorkflowStart="tasks.workflow.start"
const RemoteActionTasksPoll="tasks.poll"
const RemoteActionTasksCancel="tasks.cancel"
const RemoteActionTasksClear="tasks.clear"

type RemoteLongTaskProgress struct {
 TaskID int
 Status string
 ExitCode int
 Output string
 Truncated bool
 More bool // More output is buffered on Agent after this response.
}
type remoteLongTaskProgressWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 TaskID int `json:"task_id"`
 Status string `json:"status"`
 ExitCode int `json:"exit_code"`
 Output string `json:"output"`
 Truncated bool `json:"truncated"`
 More bool `json:"more"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func validLongTaskProgress(p RemoteLongTaskProgress)bool{
 if !validSelectedTaskID(p.TaskID)||p.ExitCode< -1||p.ExitCode>255||
  len(p.Output)>MaxRemoteLongTaskOutputBytes||!utf8.ValidString(p.Output) {
  return false
 }
 switch p.Status{
 case "started","running","cancel_requested","completed","failed","cancelled","timed_out","cleared":
 default:return false
 }
 if (p.Status=="started"||p.Status=="running"||
  p.Status=="cancel_requested"||p.Status=="cleared")&&p.ExitCode!= -1{return false}
 if (p.Status=="started"||p.Status=="cancel_requested"||p.Status=="cleared")&&p.More{
  return false
 }
 for _,r:=range p.Output{
  if r=='\n'||r=='\t'{continue}
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){return false}
 }
 return true
}
func SealRemoteLongTaskProgress(ch *Channel,id string,p RemoteLongTaskProgress)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!validLongTaskProgress(p){
  return nil,ErrInvalidRemoteAction
 }
 var clearBuffer bytes.Buffer
 encoder:=json.NewEncoder(&clearBuffer)
 encoder.SetEscapeHTML(false)
 err:=encoder.Encode(remoteLongTaskProgressWire{
  Type:RemoteLongTaskProgressProtocol,RequestID:id,TaskID:p.TaskID,
  Status:p.Status,ExitCode:p.ExitCode,Output:p.Output,Truncated:p.Truncated,
  More:p.More,RemoteControlAvailable:false,
 })
 if err!=nil{return nil,ErrInvalidRemoteAction}
 clear:=bytes.TrimSuffix(clearBuffer.Bytes(),[]byte{10})
 if len(clear)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(clear)
}
func OpenRemoteLongTaskProgress(ch *Channel,frame []byte,id string,taskID int)(RemoteLongTaskProgress,error){
 var zero RemoteLongTaskProgress
 if ch==nil||!ch.Established()||!validActionID(id)||!validSelectedTaskID(taskID){
  return zero,ErrInvalidRemoteAction
 }
 clear,err:=ch.Open(frame)
 if err!=nil{return zero,ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 fields,err:=decodeActionObject(clear,"type","request_id","task_id",
  "status","exit_code","output","truncated","more","remote_control_available")
 if err!=nil{ch.Close();return zero,ErrInvalidRemoteAction}
 var typ,requestID string
 var p RemoteLongTaskProgress
 if string(fields["remote_control_available"])!="false"||
  (string(fields["truncated"])!="true"&&string(fields["truncated"])!="false")||
  (string(fields["more"])!="true"&&string(fields["more"])!="false")||
  json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["task_id"],&p.TaskID)!=nil||
  json.Unmarshal(fields["status"],&p.Status)!=nil||
  json.Unmarshal(fields["exit_code"],&p.ExitCode)!=nil||
  json.Unmarshal(fields["output"],&p.Output)!=nil||
  json.Unmarshal(fields["truncated"],&p.Truncated)!=nil||
  json.Unmarshal(fields["more"],&p.More)!=nil||
  typ!=RemoteLongTaskProgressProtocol||requestID!=id||
  p.TaskID!=taskID||!validLongTaskProgress(p){
  ch.Close();return zero,ErrInvalidRemoteAction
 }
 return p,nil
}
