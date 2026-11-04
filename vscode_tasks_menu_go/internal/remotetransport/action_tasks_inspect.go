package remotetransport

import (
 "encoding/json"
 "strings"
 "unicode"
)

// An authenticated inspection proves that the selected stable task ID was
// published by this TaskDeck host in this E2EE session and the underlying
// task definition has not changed. It never authorizes remote execution.
const RemoteTaskInspectResultProtocol="taskdeck-e2ee-task-inspect-result-v1"

type RemoteTaskInspect struct {
 TaskID int
 Eligible bool
 InputID string
 Choices []string
}
type remoteTaskInspectWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 TaskID int `json:"task_id"`
 Eligible bool `json:"eligible"`
 InputID string `json:"input_id,omitempty"`
 Choices []string `json:"choices,omitempty"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

const MaxRemoteTaskPickChoices=6

// This deliberately supports only fixed, short, non-secret values. It is
// not a general-purpose client-to-shell parameter transport.
func ValidRemoteTaskPickInput(id string,choices []string)bool {
 if id==""||len(id)>32||len(choices)<1||len(choices)>MaxRemoteTaskPickChoices {
  return false
 }
 for _,r:=range id {
  if !((r>='a'&&r<='z')||(r>='A'&&r<='Z')||
   (r>='0'&&r<='9')||r=='_'||r=='.'||r=='-'){return false}
 }
 seen:=make(map[string]bool,len(choices))
 for _,choice:=range choices {
  if choice==""||len(choice)>32||seen[choice]||
   strings.TrimSpace(choice)!=choice{return false}
  for j,r:=range choice{
   if j==0 && !unicode.IsLetter(r) && !unicode.IsDigit(r){return false}
   if r>127||unicode.IsControl(r)||!(unicode.IsLetter(r)||unicode.IsDigit(r)||
     strings.ContainsRune(" _./:@+=-",r)){return false}
  }
  seen[choice]=true
 }
 return true
}
func validRemoteTaskInspect(value RemoteTaskInspect)bool{
 if !validSelectedTaskID(value.TaskID)||!value.Eligible{return false}
 if value.InputID=="" {return len(value.Choices)==0}
 return ValidRemoteTaskPickInput(value.InputID,value.Choices)
}

func SealRemoteActionTasksInspectResult(ch *Channel,id string,result RemoteTaskInspect)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteTaskInspect(result){
  return nil,ErrInvalidRemoteAction
 }
 raw,err:=json.Marshal(remoteTaskInspectWire{
  Type:RemoteTaskInspectResultProtocol,RequestID:id,TaskID:result.TaskID,
  Eligible:true,InputID:result.InputID,Choices:result.Choices,
  RemoteControlAvailable:false,
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
  "eligible","input_id","choices","remote_control_available")
 hasChoiceFields:=err==nil
 if err!=nil {
  fields,err=decodeActionObject(raw,"type","request_id","task_id",
   "eligible","remote_control_available")
 }
 if err!=nil{ch.Close();return denied,ErrInvalidRemoteAction}
 var typ,id string
 var selected int
 var eligible,available bool
 var inputID string
 var choices []string
 if hasChoiceFields{
  if json.Unmarshal(fields["input_id"],&inputID)!=nil||
   json.Unmarshal(fields["choices"],&choices)!=nil {
   ch.Close();return denied,ErrInvalidRemoteAction
  }
 }
 // Security booleans must be canonical JSON boolean literals, not null.
 if (string(fields["eligible"])!="true" && string(fields["eligible"])!="false") ||
  string(fields["remote_control_available"])!="false" ||
  json.Unmarshal(fields["type"],&typ)!=nil ||
  json.Unmarshal(fields["request_id"],&id)!=nil ||
  json.Unmarshal(fields["task_id"],&selected)!=nil ||
  json.Unmarshal(fields["eligible"],&eligible)!=nil ||
  json.Unmarshal(fields["remote_control_available"],&available)!=nil ||
  typ!=RemoteTaskInspectResultProtocol || id!=wantID ||
  selected!=taskID || !eligible || available ||
  !validRemoteTaskInspect(RemoteTaskInspect{
   TaskID:selected,Eligible:eligible,InputID:inputID,Choices:choices,
  }) {
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return RemoteTaskInspect{
  TaskID:selected,Eligible:eligible,InputID:inputID,Choices:choices,
 },nil
}
