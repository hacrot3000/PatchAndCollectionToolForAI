package server

import (
 "crypto/sha256"
 "encoding/json"
 "fmt"
 "regexp"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

const agentCollectionRepairBudget = 4
var agentCollectionFieldPattern = regexp.MustCompile(`action\[[0-9]+\](?:\.[a-zA-Z_][a-zA-Z_0-9]*|: git\.operations(?:\[[0-9]+\])?(?:\.[a-zA-Z_][a-zA-Z_0-9]*)?)?`)

func agentCollectionRepairError(observed string) (string,string,bool) {
 var d struct { Error struct { Code string `json:"code"`; Message string `json:"message"` } `json:"error"` }
 if json.Unmarshal([]byte(observed), &d)!=nil {return "","",false}
 switch d.Error.Code {
 case "COLLECT_SCHEMA_INVALID","INVALID_COLLECTION_REQUEST","COLLECTION_NO_PROGRESS","COLLECTION_IDENTITY_REJECTED":
  return d.Error.Code,d.Error.Message,true
 }
 return "","",false
}

func agentCollectionRepairFeedback(code,message string,attempt int) string {
 field:=agentCollectionFieldPattern.FindString(message)
 expected:="Use the required/allowed fields in the current native COLLECT schema."
 example:=`{"actions":[{"type":"overview","path":"."}]}`
 if strings.Contains(message,"paths must be a non-empty array"){
  expected="Non-empty JSON array of project-relative strings. Native action indexes are one-based."
  example=`{"type":"pack","paths":["known-relative-file"]}`
 }else if strings.Contains(message,"git.operations"){
  expected="Non-empty array of objects with op, not strings. Native operation indexes are one-based."
  example=`{"type":"git","operations":[{"op":"status"}]}`
 }else if strings.Contains(message,"project")&&strings.Contains(message,"identity"){
  field="project";expected="Typed calls omit project; standalone text must use the verified object with key and id, never a string."
 }
 semantic:=regexp.MustCompile(`\[[0-9]+\]`).ReplaceAllString(message,"[]")
 digest:=sha256.Sum256([]byte(code+"\n"+semantic))
 action:="Repair the original invalid field; preserve valid actions, never invent paths. Examples demonstrate shapes only."
 if attempt>=2 {action+=" Refresh your understanding using the current native COLLECT contract."}
 if attempt>=3 {action+=" Submit only one small native action at a time."}
 if attempt>=4 {action+=" Last automatic repair opportunity: use a different valid native action (overview/find for unknown paths), or checkpoint unresolved work without claiming completion."}
 allowed:=attempt<=agentCollectionRepairBudget
 if !allowed {action="Automatic Collection repair budget exhausted; resume from the preserved conversation checkpoint. This rejected call provided no evidence."}
 // NO_PROGRESS may refer to a previously executed valid read, so do not
 // claim that it was never queued. Only schema preflight proves no queue.
 raw:=map[string]any{"status":"REJECTED","error":map[string]any{"code":code,"message":message,"retry_allowed":allowed,"action":action},
  "validation":map[string]any{"field":field,"expected":expected,"example":example,"error_fingerprint":fmt.Sprintf("%x",digest)},"repair_attempt":attempt}
 if code!="COLLECTION_NO_PROGRESS" {raw["executed"]=false;raw["queued"]=false}
 b,_:=json.Marshal(raw)
 return string(b)
}

// Count the consecutive invalid observations belonging to the exact run
// parent. Changed request metadata or alternating errors do not reset it.
func agentRunCollectionRepairCount(history []agentchat.HistoryItem,parent string) int {
 calls:=map[string]bool{}
 for _,item:=range history {
  if item.Type=="function_call"&&item.Name=="taskdeck_collect_workspace"&&item.ParentTurnID==parent {calls[item.CallID]=true}
 }
 n:=0
 for i:=len(history)-1;i>=0;i--{
  item:=history[i]
  if item.Type=="user" {break}
  if item.Type!="function_call_output"{continue}
  if !calls[item.CallID]{break}
  if _,_,ok:=agentCollectionRepairError(item.Content);!ok{break}
  n++
 }
 return n
}

// Ignore display-only request and action metadata, not scope, arguments,
// limits, project identity or execution-affecting ai_context fields.
func agentCollectionRequestFingerprint(raw []byte)string{
 var obj map[string]json.RawMessage
 if json.Unmarshal(raw,&obj)!=nil{return fmt.Sprintf("%x",sha256.Sum256(raw))}
 delete(obj,"id");delete(obj,"title")
 var actions []map[string]json.RawMessage
 if json.Unmarshal(obj["actions"],&actions)==nil{
  for _,a:=range actions{delete(a,"id");delete(a,"title")}
  b,_:=json.Marshal(actions);obj["actions"]=b
 }
 // Decode/re-encode nested objects to normalize key order and whitespace
 // without rounding numbers or changing array order.
 b,_:=json.Marshal(obj)
 dec:=json.NewDecoder(strings.NewReader(string(b)));dec.UseNumber()
 var value any
 if dec.Decode(&value)==nil{b,_=json.Marshal(value)}
 return fmt.Sprintf("%x",sha256.Sum256(b))
}

func agentConsecutiveMatchingCollectionReads(history []agentchat.HistoryItem,call agentchat.HistoryItem,request []byte,identity patchtool.ProjectIdentity)int{
 calls:=map[string]agentchat.HistoryItem{}
 for _,it:=range history{if it.Type=="function_call"{calls[it.CallID]=it}}
 count:=0
 signature:=agentCollectionRequestFingerprint(request)
 for i:=len(history)-1;i>=0;i--{
  it:=history[i]
  if it.Type=="user"{break}
  if it.Type!="function_call_output"{continue}
  prior,ok:=calls[it.CallID]
  if !ok||prior.Name!=call.Name||prior.ParentTurnID!=call.ParentTurnID{break}
  raw,err:=agentNativeCollectionRequest(prior.Arguments,identity,false)
  if err!=nil||agentCollectionRequestFingerprint(raw)!=signature{break}
  var observation struct{Error struct{Code string `json:"code"`} `json:"error"`}
  if json.Unmarshal([]byte(it.Content),&observation)!=nil{break}
  if observation.Error.Code!=""&&observation.Error.Code!="COLLECTION_NO_PROGRESS"{break}
  count++
 }
 return count
}
