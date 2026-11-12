package remotetransport

import (
 "bytes"
 "encoding/json"
 "strconv"
 "strings"
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
 Step int
 StepTotal int
 Attempt int
 AttemptTotal int
 StepStatus string
 StepHistory string // Optional host-owned step status summary.
 StepDetails string // Exit code and milliseconds for each finished step.
 Parallel bool // Authenticated host-selected two-worker join.
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
 Step int `json:"step,omitempty"`
 StepTotal int `json:"step_total,omitempty"`
 Attempt int `json:"attempt,omitempty"`
 AttemptTotal int `json:"attempt_total,omitempty"`
 StepStatus string `json:"step_status,omitempty"`
 StepHistory string `json:"step_history,omitempty"`
 StepDetails string `json:"step_details,omitempty"`
 Parallel bool `json:"parallel,omitempty"`
}
// Compact, canonical per-step exit codes and elapsed milliseconds.
func validLongTaskStepDetails(p RemoteLongTaskProgress)bool{
 if p.StepDetails==""{return true}
 if p.StepHistory==""{return false}
 parts:=strings.Split(p.StepDetails,"|")
 if len(parts)!=p.StepTotal{return false}
 for i,part:=range parts{
  state:=p.StepHistory[i]
  if state=='R'||state=='-'||state=='S'{
   if part!="-" {return false}
   continue
  }
  rawCode,rawMillis,ok:=strings.Cut(part,":")
  if !ok||len(rawCode)>3||len(rawMillis)>5{return false}
  code,err:=strconv.Atoi(rawCode)
  if err!=nil||code< -1||code>255||strconv.Itoa(code)!=rawCode{return false}
  millis,err:=strconv.Atoi(rawMillis)
  if err!=nil||millis<0||millis>90000||
   strconv.Itoa(millis)!=rawMillis{return false}
  if state=='C'&&code!=0{return false}
 }
 return true
}


// This recognizes ONLY an opted-in fork of two through four independent
// leaves and one final root (3-5 steps). No arbitrary DAG state or new
// job command is accepted from a native client; active leaves can overlap.
func validParallelForkProgress(p RemoteLongTaskProgress,expected rune)bool{
 if (p.StepTotal<3||p.StepTotal>5)||
  p.Step<1||p.Step>=p.StepTotal||
  p.Attempt!=1||p.AttemptTotal!=1||
  len(p.StepHistory)!=p.StepTotal||
  p.StepHistory[p.StepTotal-1]!='-'||
  p.StepStatus=="skipped"||p.Status=="completed"{
  return false
 }
 for i:=0;i<p.StepTotal-1;i++{
  switch p.StepHistory[i]{
  case '-','R','C','F','X','T':
  default:return false
  }
 }
 if strings.Count(p.StepHistory[:p.StepTotal-1],"-")==p.StepTotal-1||
  rune(p.StepHistory[p.Step-1])!=expected{return false}
 if (p.Status=="failed"||p.Status=="cancelled"||
  p.Status=="timed_out")&&strings.Contains(p.StepHistory[:p.StepTotal-1],"R"){
  return false
 }
 return true
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
 // Start, Cancel and Clear are acknowledgements with no captured task
 // output. Only authenticated Poll may advance the host cursor or report
 // capture truncation; rejecting impossible ACK payloads prevents a
 // compromised localhost bridge from inventing task text or backlog.
 if p.Status=="started"||p.Status=="cancel_requested"||p.Status=="cleared"{
  if p.Output!=""||p.More||p.Truncated{return false}
 }
 workflow:=p.Step!=0||p.StepTotal!=0||p.Attempt!=0||
  p.AttemptTotal!=0||p.StepStatus!=""||p.StepHistory!=""||
  p.StepDetails!=""
 if workflow{
  if p.Status=="started"||p.Status=="cancel_requested"||
   p.Status=="cleared"||p.Step<1||p.StepTotal<2||
   p.StepTotal>5||p.Step>p.StepTotal||
   p.Attempt<1||p.AttemptTotal<1||p.AttemptTotal>2||
   p.Attempt>p.AttemptTotal||len(p.Output)>64||
   (p.Parallel&&p.StepTotal==4&&len(p.Output)>48)||
   (p.Parallel&&p.StepTotal==5&&len(p.Output)>16)||
   (p.StepTotal==5&&len(p.Output)>32){return false}
  expected:='?'
  switch p.StepStatus{
  case "running":expected='R'
  case "completed":expected='C'
  case "skipped":
   expected='S'
   // Poll can observe the final step becoming skipped before the
   // aggregate job status is published. Both states are authenticated
   // under the same job mutex, so permit this temporary running state.
   if p.Step!=p.StepTotal||
    (p.Status!="completed"&&p.Status!="running")||
    p.StepHistory==""{return false}
  case "failed":expected='F'
  case "cancelled":expected='X'
  case "timed_out":expected='T'
  default:return false
  }
  if p.Parallel&&((p.StepTotal<3||p.StepTotal>5)||p.Attempt!=1||
   p.AttemptTotal!=1||p.StepStatus=="skipped"||p.StepHistory==""){
   return false
  }
  if p.StepHistory!=""{
   if len(p.StepHistory)!=p.StepTotal{return false}
   parallelFork:=p.Parallel&&p.Step<p.StepTotal&&validParallelForkProgress(p,expected)
   if p.Parallel&&p.Step<p.StepTotal&&!parallelFork{return false}
   for i:=0;i<len(p.StepHistory);i++{
    if parallelFork{continue}
    switch{
    case i<p.Step-1:
     if p.StepHistory[i]!='C'&&
      !(p.StepStatus!="skipped"&&p.Step==p.StepTotal&&
       i==p.Step-2&&p.StepHistory[i]=='F'){
      return false
     }
    case i==p.Step-1:
     if rune(p.StepHistory[i])!=expected{return false}
    default:
     if p.StepHistory[i]!='-'{return false}
    }
   }
  }
  if !validLongTaskStepDetails(p){return false}
 }else if p.Parallel{return false}
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
  Step:p.Step,StepTotal:p.StepTotal,Attempt:p.Attempt,
  AttemptTotal:p.AttemptTotal,StepStatus:p.StepStatus,
  StepHistory:p.StepHistory,StepDetails:p.StepDetails,
  Parallel:p.Parallel,
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
 fields,err:=decodeActionObjectWithOptionals(clear,
  []string{"type","request_id","task_id","status","exit_code",
   "output","truncated","more","remote_control_available"},
  []string{"step","step_total","attempt","attempt_total","step_status",
   "step_history","step_details","parallel"})
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
  typ!=RemoteLongTaskProgressProtocol||requestID!=id{
  ch.Close();return zero,ErrInvalidRemoteAction
 }
 optional:=[]string{"step","step_total","attempt","attempt_total","step_status"}
 present:=0
 for _,key:=range optional{
  if _,found:=fields[key];found{present++}
 }
 if present!=0&&present!=len(optional){
  ch.Close();return zero,ErrInvalidRemoteAction
 }
 if present==len(optional){
  if json.Unmarshal(fields["step"],&p.Step)!=nil||
   json.Unmarshal(fields["step_total"],&p.StepTotal)!=nil||
   json.Unmarshal(fields["attempt"],&p.Attempt)!=nil||
   json.Unmarshal(fields["attempt_total"],&p.AttemptTotal)!=nil||
   json.Unmarshal(fields["step_status"],&p.StepStatus)!=nil{
   ch.Close();return zero,ErrInvalidRemoteAction
  }
 }
 if rawHistory,found:=fields["step_history"];found{
  if present!=len(optional)||json.Unmarshal(rawHistory,&p.StepHistory)!=nil||
   p.StepHistory==""{
   ch.Close();return zero,ErrInvalidRemoteAction
  }
 }
 if raw,found:=fields["step_details"];found{
  if present!=len(optional)||json.Unmarshal(raw,&p.StepDetails)!=nil||
   p.StepDetails==""{
   ch.Close();return zero,ErrInvalidRemoteAction
  }
 }
 if raw,exists:=fields["parallel"];exists{
  // Absent = previous sequential contract. Only true opts into the
  // strict bounded concurrent-fork status shape; reject all other types.
  if string(raw)!="true"{ch.Close();return zero,ErrInvalidRemoteAction}
  p.Parallel=true
 }
 if p.TaskID!=taskID||!validLongTaskProgress(p){
  ch.Close();return zero,ErrInvalidRemoteAction
 }
 return p,nil
}
