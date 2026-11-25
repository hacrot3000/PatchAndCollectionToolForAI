package agentchat

import (
 "encoding/json"
 "errors"
 "fmt"
 "path"
 "strings"
 "unicode"
)

// AutoCheck is explicitly selected by the owner for ONE immutable tool call.
// It cannot be supplied through provider narration or inferred from arbitrary
// text. It verifies narrowly objective facts, never semantic code correctness.
// A missing, failed, partially read or ambiguous receipt leaves the step open.
type TaskAutoCheck struct {
 Kind string `json:"kind"` // patch_pass, full_file_read
 CallID string `json:"call_id"`
 Path string `json:"path,omitempty"`
 CheckedHistoryParts uint64 `json:"checked_history_parts,omitempty"`
 CheckedPatchParts uint64 `json:"checked_patch_parts,omitempty"`
}

func ValidateTaskAutoCheck(rule *TaskAutoCheck) error{
 if rule==nil{return nil}
 if len(rule.CallID)==0||len(rule.CallID)>256||
  strings.TrimSpace(rule.CallID)!=rule.CallID||
  strings.ContainsAny(rule.CallID,"\r\n\x00"){return ErrInvalidSession}
 switch rule.Kind {
 case "patch_pass":
  if rule.Path!=""{return ErrInvalidSession}
 case "full_file_read":
  if len(rule.Path)==0||len(rule.Path)>512||path.IsAbs(rule.Path)||
   path.Clean(rule.Path)!=rule.Path||rule.Path=="."||
   rule.Path==".."||strings.HasPrefix(rule.Path,"../")||
   strings.ContainsAny(rule.Path,"\\\r\n\x00")||
   strings.IndexFunc(rule.Path,unicode.IsControl)>=0{return ErrInvalidSession}
 default:return ErrInvalidSession
 }
 return nil
}

// Verification always reads the canonical journal, never a suggested model
// "success" message. A successful Patch *wrapper* is not independent test
// verification, so the evidence note names only the mechanically proven fact.
func VerifyTaskAutoCheck(x Session,rule *TaskAutoCheck)(TaskPlanEvidence,error){
 if err:=ValidateTaskAutoCheck(rule);err!=nil||rule==nil{return TaskPlanEvidence{},ErrInvalidSession}
 switch rule.Kind {
 case "patch_pass":
  for _,call:=range x.ToolCalls{
   if call.CallID!=rule.CallID{continue}
   if call.RunID==""||call.PackageSHA256==""||call.PatchSessionID==""{
    return TaskPlanEvidence{},errors.New("native Patch lacks immutable execution receipt")
   }
   if err:=validateTaskNativePatchSuccess(call);err!=nil{return TaskPlanEvidence{},err}
   // Automatic attribution requires the exact run ID to be included in
   // TaskDeck's structured native result, not only an acknowledged text.
   var result struct{RunID string `json:"run_id"`}
   if json.Unmarshal([]byte(call.Result),&result)!=nil||result.RunID!=call.RunID{
    return TaskPlanEvidence{},errors.New("native Patch run ID not confirmed")
   }
   return TaskPlanEvidence{Kind:"patch",Ref:rule.CallID,Digest:taskPlanDigest(call.Result),
    Note:"Auto-verified Native Patch wrapper PASS; independent tests are not implied"},nil
  }
 case "full_file_read":
  for i,call:=range x.History {
   if call.Type!="function_call"||call.Name!="taskdeck_collect_workspace"||
    call.CallID!=rule.CallID{continue}
   if i+1>=len(x.History){break}
   output:=x.History[i+1]
   if output.Type!="function_call_output"||output.CallID!=rule.CallID{break}
   var native struct{Source string `json:"source"`}
   if json.Unmarshal([]byte(output.Content),&native)==nil&&native.Source=="native_patch_tool_collect"{
    if _,err:=verifyTaskCollectionPair(x.History,x.ProjectID,rule.CallID,"");err!=nil{return TaskPlanEvidence{},err}
    if err:=validateNativeFullFileRead(call.Arguments,output.Content,rule.Path);err!=nil{return TaskPlanEvidence{},err}
    return TaskPlanEvidence{Kind:"collection",Ref:rule.CallID,Digest:taskPlanDigest(output.Content),
     Note:"Auto-verified complete native file report for "+rule.Path+"; model inspection is not implied"},nil
   }
   if err:=validateTaskCompleteRead(output.Content,rule.Path);err!=nil{return TaskPlanEvidence{},err}
   if !collectionCallRequestedPath(call.Arguments,rule.Path){
    return TaskPlanEvidence{},errors.New("Collection request did not name the required file")
   }
   return TaskPlanEvidence{Kind:"collection",Ref:rule.CallID,Digest:taskPlanDigest(output.Content),
    Note:"Auto-verified an untruncated, successful full-file Collection read of "+rule.Path},nil
  }
 }
 return TaskPlanEvidence{},fmt.Errorf("specified %s proof is not yet available",rule.Kind)
}

 // The server derives these bounded report receipts directly from the native
 // ZIP. Match the report index to the exact original file action (not range,
 // head, tail, or pack). Neither the model excerpt nor its claims are proof.
 func validateNativeFullFileRead(arguments,content,required string)error{
  var request struct{Actions []struct{
   Type string `json:"type"`
   Path string `json:"path"`
   StartLine *int `json:"start_line"`
   EndLine json.RawMessage `json:"end_line"`
  } `json:"actions"`}
  var receipt struct{
   ExcerptTruncated *bool `json:"text_excerpt_truncated"`
   Reads []struct{
    Path string `json:"path"`
    Index int `json:"action_index"`
    Total *int `json:"total_lines"`
    SHA string `json:"report_sha256"`
   } `json:"native_full_file_reads"`
  }
  if json.Unmarshal([]byte(arguments),&request)!=nil||json.Unmarshal([]byte(content),&receipt)!=nil||
   receipt.ExcerptTruncated==nil||*receipt.ExcerptTruncated||len(receipt.Reads)>32{return ErrInvalidSession}
  seen:=map[int]bool{}
  matched:=false
  for _,proof:=range receipt.Reads{
   if proof.Index<1||proof.Index>len(request.Actions)||seen[proof.Index]||
    proof.Total==nil||*proof.Total<0||len(proof.SHA)!=64||
    strings.Trim(proof.SHA,"0123456789abcdef")!=""{return ErrInvalidSession}
   seen[proof.Index]=true
   action:=request.Actions[proof.Index-1]
   if action.Type=="file"&&action.Path==required&&proof.Path==required&&
    (action.StartLine==nil||*action.StartLine==1)&&len(action.EndLine)==0{matched=true}
  }
  if !matched{return errors.New("no complete exact native file report for the requested action")}
  return nil
 }

func TaskAutoCheckReady(x Session)bool{
 if x.TaskPlan==nil||x.TaskPlan.State!="active"{return false}
 for _,step:=range x.TaskPlan.Steps{
  if step.Status=="done"||step.AutoCheck==nil{continue}
  if _,err:=VerifyTaskAutoCheck(x,step.AutoCheck);err==nil{return true}
  // An exact call may have been archived during a long Collection/Patch
  // sequence before the scheduler polled. Only recheck when archive
  // generations advance, not every three seconds forever.
  if x.HistoryArchiveParts>step.AutoCheck.CheckedHistoryParts||
   x.PatchArchiveParts>step.AutoCheck.CheckedPatchParts{return true}
 }
 return false
}

// read-only helper: parse only the finite action metadata for one recorded
// Collection call, not untrusted natural language.
func collectionCallRequestedPath(arguments,required string)bool{
 var data struct{
  Actions []struct{
   Type string `json:"type"`
   Path string `json:"path"`
   StartLine int `json:"start_line"`
  } `json:"actions"`
 }
 if json.Unmarshal([]byte(arguments),&data)!=nil{return false}
 for _,action:=range data.Actions{
  // A request for just the tail of the file cannot prove a complete read.
  // Absent start_line defaults to the first line in the native executor.
  if action.Type=="read"&&action.Path==required&&action.StartLine>=0&&action.StartLine<=1{return true}
 }
 return false
}

// Old verified evidence is still usable after canonical journal compaction.
// Archive readers validate their private segment checksums and project/owner
// scope. We run the SAME objective auto-check, rather than trusting summary
// text or reissuing a provider/native request.
func VerifyArchivedTaskAutoCheck(store *Store,x Session,rule *TaskAutoCheck)(TaskPlanEvidence,error){
 current,err:=VerifyTaskAutoCheck(x,rule)
 if err==nil{return current,nil}
 if store==nil||ValidateTaskAutoCheck(rule)!=nil{return TaskPlanEvidence{},ErrInvalidSession}
 switch rule.Kind{
 case "patch_pass":
  for part:=x.PatchArchiveParts;part>0;part--{
   entries,e:=store.ArchivedPatchHistory(x.ID,x.Owner,part)
   if e!=nil{return TaskPlanEvidence{},e}
   for _,entry:=range entries{
    if entry.Call.CallID!=rule.CallID{continue}
    return VerifyTaskAutoCheck(Session{ToolCalls:[]ToolCallRecord{entry.Call}},rule)
   }
  }
 case "full_file_read":
  for part:=x.HistoryArchiveParts;part>0;part--{
   items,e:=store.ArchivedCollectionHistory(x.ID,x.Owner,part)
   if e!=nil{return TaskPlanEvidence{},e}
   for i:=0;i+1<len(items);i++{
    if items[i].Type!="function_call"||items[i].CallID!=rule.CallID{continue}
    return VerifyTaskAutoCheck(Session{ProjectID:x.ProjectID,History:items[i:i+2]},rule)
   }
  }
 }
 return TaskPlanEvidence{},err
}
