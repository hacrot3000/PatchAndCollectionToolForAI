package remotetransport

import (
 "encoding/json"
 "unicode/utf8"
)

// TaskDeck process completion metadata is authenticated separately from the
// untrusted process transcript. No command, stdin, PTY ID or session handle.
const RemoteTaskReportMaxOutput=3500
const remoteTaskReportResultType="taskdeck-e2ee-task-run-report-result-v1"

type RemoteTaskReport struct {
 Status string
 ExitCode int
 Output string
}
type remoteTaskReportWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 OK bool `json:"ok"`
 Status string `json:"status"`
 ExitCode int `json:"exit_code"`
 Output string `json:"output"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func validTaskReport(v RemoteTaskReport)bool{
 return v.Status=="exited"&&v.ExitCode>=0&&v.ExitCode<=255&&
  len(v.Output)<=RemoteTaskReportMaxOutput&&utf8.ValidString(v.Output)
}
func SealRemoteTaskReportResult(ch *Channel,id string,result RemoteTaskReport)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!validTaskReport(result){
  return nil,ErrInvalidRemoteAction
 }
 clear,err:=json.Marshal(remoteTaskReportWire{
  Type:remoteTaskReportResultType,RequestID:id,OK:true,
  Status:result.Status,ExitCode:result.ExitCode,Output:result.Output,
  RemoteControlAvailable:false,
 })
 if err!=nil||len(clear)>RemoteGitDiffMaxPlaintext{
  return nil,ErrInvalidRemoteAction
 }
 return ch.Seal(clear)
}
func OpenRemoteTaskReportResult(ch *Channel,frame []byte,id string)(RemoteTaskReport,error){
 denied:=ErrInvalidRemoteAction
 if ch==nil||!ch.Established()||!validActionID(id){return RemoteTaskReport{},denied}
 clear,err:=ch.Open(frame)
 if err!=nil{return RemoteTaskReport{},denied}
 defer wipeLoginBytes(clear)
 raw,err:=decodeActionObject(clear,
  "type","request_id","ok","status","exit_code","output","remote_control_available")
 if err!=nil{ch.Close();return RemoteTaskReport{},denied}
 var data remoteTaskReportWire
 if json.Unmarshal(raw["type"],&data.Type)!=nil||
  json.Unmarshal(raw["request_id"],&data.RequestID)!=nil||
  json.Unmarshal(raw["ok"],&data.OK)!=nil||
  json.Unmarshal(raw["status"],&data.Status)!=nil||
  json.Unmarshal(raw["exit_code"],&data.ExitCode)!=nil||
  json.Unmarshal(raw["output"],&data.Output)!=nil||
  json.Unmarshal(raw["remote_control_available"],&data.RemoteControlAvailable)!=nil||
  data.Type!=remoteTaskReportResultType||data.RequestID!=id||
  !data.OK||data.RemoteControlAvailable||
  !validTaskReport(RemoteTaskReport{Status:data.Status,ExitCode:data.ExitCode,Output:data.Output}){
  ch.Close();return RemoteTaskReport{},denied
 }
 return RemoteTaskReport{Status:data.Status,ExitCode:data.ExitCode,Output:data.Output},nil
}
