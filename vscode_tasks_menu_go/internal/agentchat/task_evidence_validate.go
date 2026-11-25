package agentchat

import (
 "encoding/json"
 "errors"
 "fmt"
 "path"
 "strconv"
 "strings"
)

// A verified native call's *existence* is not proof that its execution
// succeeded. Only a final, acknowledged, PASS native result can be accepted
// as evidence of successful application. It does not prove independent tests
// passed unless the acceptance rule explicitly provides separate test proof.
func validateTaskNativePatchSuccess(call ToolCallRecord) error {
 if call.Status!="done" || !call.FeedbackSent || call.FeedbackStatus!="acknowledged" || call.Result=="" {
  return errors.New("native Patch has no acknowledged successful execution")
 }
 var result struct {
  Status string `json:"status"`
  RunID string `json:"run_id"`
  RC *int `json:"rc"`
  Commands []struct {
   Status string `json:"status"`
   RC *int `json:"rc"`
   TimedOut bool `json:"timed_out"`
  } `json:"commands"`
 }
 if err:=json.Unmarshal([]byte(call.Result),&result);err!=nil {
  return fmt.Errorf("native Patch result is not structured verified JSON: %w",err)
 }
 if result.Status!="PASS"{return errors.New("native Patch did not PASS")}
 if result.RC!=nil&&*result.RC!=0{return errors.New("native Patch exited nonzero")}
 if call.RunID!=""&&result.RunID!=""&&result.RunID!=call.RunID{
  return errors.New("native Patch run ID differs from accepted receipt")
 }
 for _,cmd:=range result.Commands{
  if cmd.TimedOut||(cmd.RC!=nil&&*cmd.RC!=0){
   return errors.New("native Patch command failed or timed out")
  }
  switch strings.ToUpper(cmd.Status){
  case "FAIL","FAILED","ERROR","TIMEOUT","TIMED_OUT","CANCELLED","INCOMPLETE","PREFLIGHT_FAIL":
   return errors.New("native Patch command failed")
  }
 }
 return nil
}

// Canonical function_call_output items are written from TaskDeck's read-only
// Collection executor. Reject failed and empty observations; an error-bearing
// response must not become evidence that the requested research succeeded.
func validateTaskCollectionSuccess(content string) error {
 var envelope struct{
  Source string `json:"source"`
  Error json.RawMessage `json:"error"`
  Results []struct{
   Error json.RawMessage `json:"error"`
   Files []struct{Error string `json:"error"`} `json:"files"`
  } `json:"results"`
 }
 if json.Unmarshal([]byte(content),&envelope)!=nil||envelope.Source=="native_patch_tool_collect"||len(envelope.Results)==0{
  return errors.New("Collection observation is missing verified results")
 }
 if hasTaskEvidenceError(envelope.Error){return errors.New("Collection returned an error")}
 for _,entry:=range envelope.Results{
  if hasTaskEvidenceError(entry.Error){return errors.New("Collection action returned an error")}
  for _,file:=range entry.Files {
   if strings.TrimSpace(file.Error)!=""{return errors.New("Collection file read failed")}
  }
 }
 return nil
}

// Native COLLECT now returns a server-created artifact receipt, not the
// historical inline results array. This proves wrapper/manifest PASS only.
// It is intentionally not used for exact full-file Auto-check.
func validateTaskCollectionObservation(content, projectID string) error {
 var receipt struct {
  Source string `json:"source"`
  Project struct { ID string `json:"id"` } `json:"project"`
  Status string `json:"status"`
  NativeStatus string `json:"native_status"`
  NativeSessionID string `json:"native_session_id"`
  RunID string `json:"run_id"`
  ResultZIP string `json:"result_zip"`
  ResultText string `json:"result_text"`
  Verified bool `json:"evidence_verified"`
  Partial *bool `json:"partial_evidence_available"`
  RC *int `json:"rc"`
  Error json.RawMessage `json:"error"`
  ArtifactError json.RawMessage `json:"artifact_error"`
  TextError json.RawMessage `json:"text_error"`
  FailureReason string `json:"failure_reason"`
 }
 if json.Unmarshal([]byte(content), &receipt)!=nil {
  return errors.New("invalid Collection observation JSON")
 }
 if receipt.Source!="native_patch_tool_collect" {
  return validateTaskCollectionSuccess(content)
 }
 if projectID=="" || receipt.Project.ID!=projectID ||
  receipt.Status!="PASS" || receipt.NativeStatus!="PASS" || !receipt.Verified ||
  receipt.Partial==nil || *receipt.Partial || receipt.RC==nil || *receipt.RC!=0 ||
  strings.TrimSpace(receipt.NativeSessionID)=="" || strings.TrimSpace(receipt.RunID)=="" ||
  strings.TrimSpace(receipt.FailureReason)!="" ||
  hasTaskEvidenceError(receipt.Error) || hasTaskEvidenceError(receipt.ArtifactError) ||
  hasTaskEvidenceError(receipt.TextError) {
  return errors.New("native Collection receipt is incomplete, unsuccessful or outside the project")
 }
 validArtifact:=func(name,ext string)bool{
  const prefix="artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_"
  return strings.HasPrefix(name,prefix) && path.Clean(name)==name &&
   path.Dir(name)=="artifacts/patch_tool_code_collections" && path.Ext(name)==ext &&
   !strings.ContainsAny(name,"\\\r\n\x00") && len(name)>len(prefix)+len(ext)
 }
 if !validArtifact(receipt.ResultZIP,".zip") || !validArtifact(receipt.ResultText,".txt") ||
  strings.TrimSuffix(receipt.ResultZIP,".zip")!=strings.TrimSuffix(receipt.ResultText,".txt"){
  return errors.New("native Collection receipt lacks matching artifact identities")
 }
 return nil
}

func hasTaskEvidenceError(raw json.RawMessage)bool{
 v:=strings.TrimSpace(string(raw))
 return v!=""&&v!="null"&&v!="false"&&v!=string([]byte{34,34})
}

// For automatic read verification the user supplies the exact expected
// project-relative file path AND the immutable Collection call ID. This is
// deliberately stronger than the manual acknowledgment of any successful
// read-only Collection observation.
func validateTaskCompleteRead(content,path string)error{
 if err:=validateTaskCollectionSuccess(content);err!=nil{return err}
 var envelope struct{
  ResponseTruncated bool `json:"response_truncated"`
  Results []struct{
   Type string `json:"type"`
   OutputTruncated bool `json:"output_truncated"`
   Files []struct{
    Path string `json:"path"`
    StartLine string `json:"start_line"`
    TotalLines string `json:"total_lines"`
    Content *string `json:"content"`
    Error string `json:"error"`
    Truncated string `json:"truncated"`
    ContentTruncated string `json:"content_truncated"`
    NextLine string `json:"next_line"`
   } `json:"files"`
  } `json:"results"`
 }
 if json.Unmarshal([]byte(content),&envelope)!=nil||envelope.ResponseTruncated{
  return errors.New("Collection read response was truncated")
 }
 for _,action:=range envelope.Results{
  if action.Type!="read"||action.OutputTruncated{continue}
  for _,file:=range action.Files{
   // A final non-truncated page that STARTED midway through a file
   // is not a full-file read. Require native page metadata even for
   // a one-line/empty file rather than trusting omitted fields.
   total,err:=strconv.Atoi(file.TotalLines)
   if err!=nil||total<0||file.StartLine!="1" {continue}
   if file.Path!=path||file.Content==nil||file.Error!=""||
    file.Truncated!="false"||
    (file.ContentTruncated!=""&&file.ContentTruncated!="false")||
    file.NextLine!=""{continue}
   return nil
  }
 }
 return errors.New("expected full-file Collection read is not present")
}
