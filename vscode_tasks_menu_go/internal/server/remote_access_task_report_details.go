package server

import (
 "encoding/json"
 "io"
 "strconv"
 "strings"
)

// Do not reuse the permissive encoding/json struct decoder for security
// audit projections: it accepts duplicate keys and silently assigns zero
// values to omitted integer fields. Only this exact, bounded schema can
// become a user-visible remote task outcome.
type remoteTaskFinishAuditDetail struct {
 TaskID int
 ExitCode int
 DurationMS int64
}

func parseCanonicalAuditInteger(raw json.RawMessage)(int64,bool){
 s:=strings.TrimSpace(string(raw))
 if s==""||len(s)>18{return 0,false}
 value,err:=strconv.ParseInt(s,10,64)
 return value,err==nil&&strconv.FormatInt(value,10)==s
}

func decodeRemoteTaskFinishAuditDetails(raw string)(
 remoteTaskFinishAuditDetail,bool,
){
 var denied remoteTaskFinishAuditDetail
 if len(raw)<2||len(raw)>256{return denied,false}
 dec:=json.NewDecoder(strings.NewReader(raw))
 first,err:=dec.Token()
 if err!=nil||first!=json.Delim('{'){return denied,false}
 var detail remoteTaskFinishAuditDetail
 seen:=make(map[string]bool,4)
 for dec.More(){
  token,err:=dec.Token()
  key,ok:=token.(string)
  if err!=nil||!ok||seen[key]{return denied,false}
  seen[key]=true
  switch key{
  case "transport":
   var value string
   if dec.Decode(&value)!=nil||value!="e2ee"{return denied,false}
  case "task_id","exit_code","duration_ms":
   var value json.RawMessage
   if dec.Decode(&value)!=nil{return denied,false}
   number,ok:=parseCanonicalAuditInteger(value)
   if !ok{return denied,false}
   switch key{
   case "task_id":
    if number<1||number>0x7fffffff{return denied,false}
    detail.TaskID=int(number)
   case "exit_code":
    if number< -1||number>255{return denied,false}
    detail.ExitCode=int(number)
   case "duration_ms":
    if number<0||number>86400000{return denied,false}
    detail.DurationMS=number
   }
  default:
   return denied,false
  }
 }
 last,err:=dec.Token()
 if err!=nil||last!=json.Delim('}')||len(seen)!=4{
  return denied,false
 }
 if _,err:=dec.Token();err!=io.EOF{return denied,false}
 return detail,true
}
