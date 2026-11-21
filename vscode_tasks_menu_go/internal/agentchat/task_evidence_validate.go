package agentchat

import (
 "encoding/json"
 "errors"
 "fmt"
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
  Error json.RawMessage `json:"error"`
  Results []struct{
   Error json.RawMessage `json:"error"`
   Files []struct{Error string `json:"error"`} `json:"files"`
  } `json:"results"`
 }
 if json.Unmarshal([]byte(content),&envelope)!=nil||len(envelope.Results)==0{
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
func hasTaskEvidenceError(raw json.RawMessage)bool{
 v:=strings.TrimSpace(string(raw))
 return v!=""&&v!="null"&&v!="false"&&v!=""""
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
   if file.Path!=path||file.Content==nil||file.Error!=""||
    file.Truncated=="true"||file.ContentTruncated=="true"||
    file.NextLine!=""{continue}
   return nil
  }
 }
 return errors.New("expected full-file Collection read is not present")
}
