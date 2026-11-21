package server

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "regexp"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// This is a narrowly scoped structured test result, not a generic test-suite
// coverage claim. Native go test -json wraps test stdout inside JSON events;
// plain shell log text can never activate this verifier.
const maxNativeGoTestOutput = 2 << 20

var goTestPackagePattern=regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./-]{0,199}$`)
var goNamedTestPattern=regexp.MustCompile(`^Test[A-Za-z0-9_]+(/[A-Za-z0-9_=.,-]+)*$`)

type nativeGoTestEvent struct {
 Action string `json:"Action"`
 Package string `json:"Package"`
 Test string `json:"Test,omitempty"`
}

// verifyNamedGoTestJSON accepts only observed Go runner events, in order:
// run exact case -> pass exact case -> pass its package. An isolated pass line,
// skipped case, wrong package and incomplete output are not enough.
func verifyNamedGoTestJSON(raw []byte,pkg,test string)error {
 if !goTestPackagePattern.MatchString(pkg)||!goNamedTestPattern.MatchString(test)||
  len(raw)==0||len(raw)>maxNativeGoTestOutput{
  return errors.New("invalid package/test identity or oversized native output")
 }
 ran,passed,packagePassed:=false,false,false
 lines:=bytes.Split(raw,[]byte{'\n'})
 if len(lines)>20000{return errors.New("native test output has too many events")}
 for _,line:=range lines{
  line=bytes.TrimSpace(line)
  if len(line)==0{continue}
  if len(line)>128<<10{return errors.New("native Go test event exceeds size limit")}
  if line[0]!='{'{continue} // Task header and native process exit footer
  var event nativeGoTestEvent
  if err:=json.Unmarshal(line,&event);err!=nil{
   return errors.New("malformed native Go test JSON event")
  }
  if event.Package!=pkg{continue}
  if event.Test==test{
   switch event.Action{
   case "run":
    if ran||passed{return errors.New("duplicate or out-of-order named Go test run")}
    ran=true
   case "pass":
    if !ran||passed{return errors.New("named test PASS lacks a preceding RUN")}
    passed=true
   case "skip","fail":
    return errors.New("named test skipped or failed")
   }
  }else if event.Test==""{
   if event.Action=="fail"{return errors.New("Go test package failed")}
   if event.Action=="pass"&&passed{packagePassed=true}
  }
 }
 if !ran||!passed||!packagePassed{
  return errors.New("exact named test RUN/PASS and package PASS were not observed")
 }
 return nil
}

// verifyNativeGoTestCaseEvidence requires a USER-BOUND expected Task command
// (PR #167), a live completed SessionService run and its full untruncated
// native output. The prior signed Task metadata receipt cannot establish
// the actual test case result if its native output has been evicted.
func (s *Server) verifyNativeGoTestCaseEvidence(x agentchat.Session,
 ref string,taskID int,stepID,pkg,test,note string)(agentchat.TaskPlanEvidence,error){
 if s==nil||s.Sessions==nil||ref==""||len(ref)>256||taskID<1||
  !goTestPackagePattern.MatchString(pkg)||!goNamedTestPattern.MatchString(test){
  return agentchat.TaskPlanEvidence{},errors.New("exact live Task, Go package and named test required")
 }
 if x.TaskPlan==nil{return agentchat.TaskPlanEvidence{},errors.New("no user-owned task plan")}
 foundGuard:=false
 for _,step:=range x.TaskPlan.Steps{
  if step.ID!=stepID{continue}
  foundGuard=step.TaskCommandGuard!=nil
  break
 }
 if !foundGuard{
  return agentchat.TaskPlanEvidence{},errors.New("bind expected Task ID and command SHA-256 before requesting structured test proof")
 }
 // Reuse the exact owner/project/workspace, command and post-bind timeline
 // verification already used for native Task evidence; no separate bypass.
 basic,err:=s.verifyNativeTaskRunEvidence(x,ref,taskID,note,stepID)
 if err!=nil{return agentchat.TaskPlanEvidence{},err}
 meta,ok:=s.Sessions.Metadata(ref)
 if !ok||meta.ID!=ref||meta.Status!="exited"||meta.TestRunner!="go_test_json"{
  return agentchat.TaskPlanEvidence{},errors.New("live native go test -json session required for structured proof")
 }
 if meta.OutputBytes<=0||meta.RetainedBytes<=0||meta.OutputBytes!=meta.RetainedBytes||
  meta.OutputBytes>maxNativeGoTestOutput{
  return agentchat.TaskPlanEvidence{},errors.New("native test output missing or truncated")
 }
 output,_,cancel,err:=s.Sessions.Subscribe(ref)
 if err!=nil{return agentchat.TaskPlanEvidence{},errors.New("cannot read authoritative native test output")}
 defer cancel()
 if int64(len(output))!=meta.OutputBytes{
  return agentchat.TaskPlanEvidence{},errors.New("native test output was truncated or changed")
 }
 if err:=verifyNamedGoTestJSON(output,pkg,test);err!=nil{
  return agentchat.TaskPlanEvidence{},err
 }
 outputSum:=sha256.Sum256(output)
 statement:=struct{
  Version int `json:"version"`
  NativeTaskDigest string `json:"native_task_digest"`
  NativeOutputSHA256 string `json:"native_output_sha256"`
  Package string `json:"package"`
  Test string `json:"test"`
 }{1,basic.Digest,hex.EncodeToString(outputSum[:]),pkg,test}
 raw,err:=json.Marshal(statement);if err!=nil{return agentchat.TaskPlanEvidence{},err}
 digest:=sha256.Sum256(raw)
 evidenceNote:=fmt.Sprintf("Native go test -json: %s in %s RUN/PASS, package PASS; full output verified, not proof of test coverage",test,pkg)
 if strings.TrimSpace(note)!=""{evidenceNote+="; user note: "+strings.TrimSpace(note)}
 if len(evidenceNote)>512{return agentchat.TaskPlanEvidence{},errors.New("Go test evidence note too long")}
 return agentchat.TaskPlanEvidence{Kind:"go_test_case",Ref:ref,
  Digest:hex.EncodeToString(digest[:]),Note:evidenceNote},nil
}
