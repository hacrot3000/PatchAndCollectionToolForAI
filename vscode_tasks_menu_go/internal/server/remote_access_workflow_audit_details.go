package server

import (
 "encoding/json"
 "io"
 "strings"
)

// Durable step audit is untrusted JSON. Reject duplicate keys, extra
// properties, noncanonical integers, and any command/output fields.
type remoteWorkflowStepAuditDetail struct{
 RootTaskID int
 TaskID int
 Step int
 Total int
 Attempt int
 ExitCode int
 RunID string
}
// Only a lowercase, fixed-length public correlation label, never an
// authorization key. Disallow Unicode, separators, controls and whitespace.
func validRemoteWorkflowRunID(value string)bool{
 if len(value)!=16{return false}
 for i:=0;i<len(value);i++{
  c:=value[i]
  if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return false}
 }
 return true
}
func decodeRemoteWorkflowStepAuditDetails(raw string)(remoteWorkflowStepAuditDetail,bool){
 var denied remoteWorkflowStepAuditDetail
 if len(raw)<2||len(raw)>256{return denied,false}
 d:=json.NewDecoder(strings.NewReader(raw))
 first,err:=d.Token()
 if err!=nil||first!=json.Delim('{'){return denied,false}
 var detail remoteWorkflowStepAuditDetail
 seen:=make(map[string]bool,8)
 for d.More(){
  token,e:=d.Token()
  key,ok:=token.(string)
  if e!=nil||!ok||seen[key]{return denied,false}
  seen[key]=true
  if key=="transport"{
   var transport string
   if d.Decode(&transport)!=nil||transport!="e2ee"{return denied,false}
   continue
  }
  if key=="run_id"{
   if d.Decode(&detail.RunID)!=nil||
    !validRemoteWorkflowRunID(detail.RunID){return denied,false}
   continue
  }
  var value json.RawMessage
  if d.Decode(&value)!=nil{return denied,false}
  number,ok:=parseCanonicalAuditInteger(value)
  if !ok{return denied,false}
  switch key{
  case "root_task_id":
   if number<1||number>0x7fffffff{return denied,false}
   detail.RootTaskID=int(number)
  case "task_id":
   if number<1||number>0x7fffffff{return denied,false}
   detail.TaskID=int(number)
  case "step":
   if number<1||number>4{return denied,false}
   detail.Step=int(number)
  case "total":
   if number<2||number>4{return denied,false}
   detail.Total=int(number)
  case "attempt":
   if number<1||number>2{return denied,false}
   detail.Attempt=int(number)
  case "exit_code":
   if number< -1||number>255{return denied,false}
   detail.ExitCode=int(number)
  default:
   return denied,false
  }
 }
 end,err:=d.Token()
 if err!=nil||end!=json.Delim('}')||
  (len(seen)!=7&&len(seen)!=8)||
  (len(seen)==8&&!seen["run_id"])||
  detail.Step>detail.Total{return denied,false}
 if _,err=d.Token();err!=io.EOF{return denied,false}
 return detail,true
}
