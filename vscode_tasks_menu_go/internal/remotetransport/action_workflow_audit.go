package remotetransport

import (
 "encoding/json"
 "time"
)

// Workflow audit rows are persisted by the TaskDeck host, never by PHP.
// The request is argument-free and the response contains only two recent
// principal-owned final step outcomes, not raw audit details or output.
const RemoteWorkflowAuditProtocol="taskdeck-e2ee-workflow-audit-v1"
const MaxRemoteWorkflowAuditEntries=2

type RemoteWorkflowAuditEntry struct{
 RootTaskID int `json:"root_task_id"`
 TaskID int `json:"task_id"`
 Step int `json:"step"`
 Total int `json:"total"`
 Attempt int `json:"attempt"`
 Status string `json:"status"`
 ExitCode int `json:"exit_code"`
 Timestamp int64 `json:"timestamp"`
}
type RemoteWorkflowAudit struct{ Items []RemoteWorkflowAuditEntry }
type remoteWorkflowAuditWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Items []RemoteWorkflowAuditEntry `json:"items"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func ValidRemoteWorkflowAuditEntry(item RemoteWorkflowAuditEntry)bool{
 if !validSelectedTaskID(item.RootTaskID)||!validSelectedTaskID(item.TaskID)||
  item.Step<1||item.Total<2||item.Total>4||item.Step>item.Total||
  item.Attempt<1||item.Attempt>2||item.ExitCode< -1||item.ExitCode>255||
  item.Timestamp<1577836800||item.Timestamp>time.Now().UTC().Add(5*time.Minute).Unix(){
  return false
 }
 switch item.Status{
 case "completed":return item.ExitCode==0
 case "failed","cancelled","timed_out":return true
 default:return false
 }
}
func ValidRemoteWorkflowAudit(report RemoteWorkflowAudit)bool{
 if len(report.Items)>MaxRemoteWorkflowAuditEntries{return false}
 for i,item:=range report.Items{
  if !ValidRemoteWorkflowAuditEntry(item)||
   (i>0&&item.Timestamp>report.Items[i-1].Timestamp){return false}
 }
 return true
}
func SealRemoteWorkflowAudit(ch *Channel,id string,report RemoteWorkflowAudit)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !ValidRemoteWorkflowAudit(report){return nil,ErrInvalidRemoteAction}
 if report.Items==nil{report.Items=[]RemoteWorkflowAuditEntry{}}
 clear,err:=json.Marshal(remoteWorkflowAuditWire{
  Type:RemoteWorkflowAuditProtocol,RequestID:id,Items:report.Items,
  RemoteControlAvailable:false,
 })
 if err!=nil||len(clear)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(clear)
}
func OpenRemoteWorkflowAudit(ch *Channel,frame []byte,id string)(RemoteWorkflowAudit,error){
 denied:=RemoteWorkflowAudit{}
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
  typ!=RemoteWorkflowAuditProtocol||requestID!=id||
  len(rawItems)>MaxRemoteWorkflowAuditEntries{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 result:=RemoteWorkflowAudit{Items:make([]RemoteWorkflowAuditEntry,0,len(rawItems))}
 for _,raw:=range rawItems{
  sub,e:=decodeActionObject(raw,"root_task_id","task_id","step","total",
   "attempt","status","exit_code","timestamp")
  if e!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
  var item RemoteWorkflowAuditEntry
  if json.Unmarshal(sub["root_task_id"],&item.RootTaskID)!=nil||
   json.Unmarshal(sub["task_id"],&item.TaskID)!=nil||
   json.Unmarshal(sub["step"],&item.Step)!=nil||
   json.Unmarshal(sub["total"],&item.Total)!=nil||
   json.Unmarshal(sub["attempt"],&item.Attempt)!=nil||
   json.Unmarshal(sub["status"],&item.Status)!=nil||
   json.Unmarshal(sub["exit_code"],&item.ExitCode)!=nil||
   json.Unmarshal(sub["timestamp"],&item.Timestamp)!=nil{
   ch.Close();return denied,ErrInvalidRemoteAction
  }
  result.Items=append(result.Items,item)
 }
 if !ValidRemoteWorkflowAudit(result){ch.Close();return denied,ErrInvalidRemoteAction}
 return result,nil
}
