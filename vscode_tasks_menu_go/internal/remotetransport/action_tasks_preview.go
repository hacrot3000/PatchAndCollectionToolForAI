package remotetransport

import (
 "encoding/json"
 "strings"
 "unicode"
 "unicode/utf8"
)

// TasksPreview includes only task menu labels, never task commands, args,
// environment variables, task IDs, secrets, workspace paths or run controls.
const RemoteTasksPreviewResultProtocol = "taskdeck-e2ee-tasks-preview-result-v1"
const MaxRemoteTasksPreviewLabels = 8
const MaxRemoteTaskLabelBytes = 64
const MaxRemoteTasksCount = 100000

type RemoteTasksPreview struct{
 Total int
 Labels []string
}
type remoteTasksPreviewWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Total int `json:"total"`
 Labels []string `json:"labels"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func ValidRemoteTaskLabel(s string)bool{
 if s=="" || len(s)>MaxRemoteTaskLabelBytes || !utf8.ValidString(s) ||
  strings.TrimSpace(s)!=s{return false}
 for _,r:=range s {
  if unicode.IsControl(r) || unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){
   return false
  }
 }
 return true
}

func validRemoteTasksPreview(result RemoteTasksPreview)bool{
 if result.Total<0 || result.Total>MaxRemoteTasksCount ||
  result.Labels==nil || len(result.Labels)>MaxRemoteTasksPreviewLabels ||
  len(result.Labels)>result.Total{return false}
 for _,label:=range result.Labels{
  if !ValidRemoteTaskLabel(label){return false}
 }
 return true
}

// fitRemoteTasksPreview is a deterministic transport-side truncation.
// The request ID has a fixed 32-byte representation, so this checks the
// exact JSON size before Seal; it does not trust client-provided limits.
func fitRemoteTasksPreview(preview RemoteTasksPreview,id string)(
 RemoteTasksPreview,error,
){
 if !validActionID(id)||!validRemoteTasksPreview(preview){
  return RemoteTasksPreview{},ErrInvalidRemoteAction
 }
 for {
  clear,err:=json.Marshal(remoteTasksPreviewWire{
   Type:RemoteTasksPreviewResultProtocol,RequestID:id,Total:preview.Total,
   Labels:preview.Labels,RemoteControlAvailable:false,
  })
  if err!=nil{return RemoteTasksPreview{},ErrInvalidRemoteAction}
  if len(clear)<=maxRemoteActionPlaintext{return preview,nil}
  if len(preview.Labels)==0{return RemoteTasksPreview{},ErrInvalidRemoteAction}
  preview.Labels=preview.Labels[:len(preview.Labels)-1]
 }
}

func SealRemoteActionTasksPreviewResult(
 ch *Channel,id string,preview RemoteTasksPreview,
)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteTasksPreview(preview){
  return nil,ErrInvalidRemoteAction
 }
 // Keep the AEAD envelope fixed-size, even when task labels contain
 // Unicode or JSON-escaped HTML-looking characters. Drop trailing labels,
 // never reduce validation or widen Relay limits to fit more data.
 var err error
 preview,err=fitRemoteTasksPreview(preview,id)
 if err!=nil{return nil,ErrInvalidRemoteAction}
 clear,err:=json.Marshal(remoteTasksPreviewWire{
  Type:RemoteTasksPreviewResultProtocol,RequestID:id,
  Total:preview.Total,Labels:preview.Labels,
  RemoteControlAvailable:false,
 })
 if err!=nil || len(clear)>maxRemoteActionPlaintext{
  return nil,ErrInvalidRemoteAction
 }
 return ch.Seal(clear)
}

func OpenRemoteActionTasksPreviewResult(
 ch *Channel,frame []byte,wantID string,
)(RemoteTasksPreview,error){
 denied:=RemoteTasksPreview{}
 if ch==nil||!ch.Established()||!validActionID(wantID){
  return denied,ErrInvalidRemoteAction
 }
 clear,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 fields,err:=decodeActionObject(clear,"type","request_id","total",
  "labels","remote_control_available")
 if err!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 var typ,id string
 var available bool
 var info RemoteTasksPreview
 if json.Unmarshal(fields["type"],&typ)!=nil ||
  json.Unmarshal(fields["request_id"],&id)!=nil ||
  json.Unmarshal(fields["total"],&info.Total)!=nil ||
  json.Unmarshal(fields["labels"],&info.Labels)!=nil ||
  json.Unmarshal(fields["remote_control_available"],&available)!=nil ||
  typ!=RemoteTasksPreviewResultProtocol || id!=wantID || available ||
  !validRemoteTasksPreview(info){
  ch.Close()
  return denied,ErrInvalidRemoteAction
 }
 return info,nil
}
