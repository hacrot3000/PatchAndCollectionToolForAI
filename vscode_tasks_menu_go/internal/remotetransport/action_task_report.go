package remotetransport

import (
 "encoding/json"
 "time"
)

// This argument-free read-only operation returns only sanitized outcome
// metadata for the current principal; no command, arguments, output,
// environment, file path, portal token or audit detail is transported.
const RemoteActionTasksReport="tasks.report"
const RemoteTaskReportProtocol="taskdeck-e2ee-task-report-v1"
const MaxRemoteTaskReportEntries=3

type RemoteTaskReportEntry struct{
 TaskID int `json:"task_id"`
 Status string `json:"status"`
 ExitCode int `json:"exit_code"`
 DurationMS int64 `json:"duration_ms"`
 Timestamp int64 `json:"timestamp"`
}
type RemoteTaskReport struct{
 Items []RemoteTaskReportEntry
}
type remoteTaskReportWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Items []RemoteTaskReportEntry `json:"items"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func ValidRemoteTaskReportEntry(item RemoteTaskReportEntry)bool{
 return validSelectedTaskID(item.TaskID)&&
  (item.Status=="completed"||item.Status=="failed"||
   item.Status=="cancelled"||item.Status=="timed_out")&&
  item.ExitCode>=-1&&item.ExitCode<=255&&
  item.DurationMS>=0&&item.DurationMS<=86400000&&
  item.Timestamp>=1577836800&&
  item.Timestamp<=time.Now().UTC().Add(5*time.Minute).Unix()
}
func ValidRemoteTaskReport(report RemoteTaskReport)bool{
 if len(report.Items)>MaxRemoteTaskReportEntries{return false}
 for i,item:=range report.Items{
  if !ValidRemoteTaskReportEntry(item)||
   (i>0&&item.Timestamp>report.Items[i-1].Timestamp){return false}
 }
 return true
}
func SealRemoteTaskReport(ch *Channel,id string,report RemoteTaskReport)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !ValidRemoteTaskReport(report){return nil,ErrInvalidRemoteAction}
 if report.Items==nil{report.Items=[]RemoteTaskReportEntry{}}
 clear,err:=json.Marshal(remoteTaskReportWire{
  Type:RemoteTaskReportProtocol,RequestID:id,Items:report.Items,
  RemoteControlAvailable:false,
 })
 if err!=nil||len(clear)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(clear)
}
func OpenRemoteTaskReport(ch *Channel,frame []byte,id string)(RemoteTaskReport,error){
 denied:=RemoteTaskReport{}
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 fields,err:=decodeActionObject(clear,"type","request_id","items","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,requestID string
 var rawItems []json.RawMessage
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["items"],&rawItems)!=nil||
  typ!=RemoteTaskReportProtocol||requestID!=id||
  len(rawItems)>MaxRemoteTaskReportEntries{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 result:=RemoteTaskReport{Items:make([]RemoteTaskReportEntry,0,len(rawItems))}
 for _,raw:=range rawItems{
  sub,err:=decodeActionObject(raw,"task_id","status","exit_code","duration_ms","timestamp")
  if err!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
  var item RemoteTaskReportEntry
  if json.Unmarshal(sub["task_id"],&item.TaskID)!=nil||
   json.Unmarshal(sub["status"],&item.Status)!=nil||
   json.Unmarshal(sub["exit_code"],&item.ExitCode)!=nil||
   json.Unmarshal(sub["duration_ms"],&item.DurationMS)!=nil||
   json.Unmarshal(sub["timestamp"],&item.Timestamp)!=nil{
   ch.Close();return denied,ErrInvalidRemoteAction
  }
  result.Items=append(result.Items,item)
 }
 if !ValidRemoteTaskReport(result){ch.Close();return denied,ErrInvalidRemoteAction}
 return result,nil
}
