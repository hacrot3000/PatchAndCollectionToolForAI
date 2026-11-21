package agentchat

import (
 "strings"
 "testing"
)

func exampleAutoPatchCall() ToolCallRecord {
 return ToolCallRecord{
  CallID:"patch-good",Name:"taskdeck_submit_patch",Status:"done",
  FeedbackSent:true,FeedbackStatus:"acknowledged",
  RunID:"run-native-0001",PatchSessionID:"native-session-1",
  PackageSHA256:strings.Repeat("a",64),PackageSize:90,
  Result:`{"status":"PASS","run_id":"run-native-0001","rc":0,"commands":[{"status":"PASS","rc":0}]}`,
 }
}
func exampleAutoCollectionRead() []HistoryItem {
 return []HistoryItem{
  {Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"collect-123",
   Arguments:`{"actions":[{"type":"read","path":"src/main.go","start_line":1,"max_lines":200}]}`},
  {Type:"function_call_output",CallID:"collect-123",
   Content:`{"results":[{"type":"read","files":[{"path":"src/main.go","content":"package main
"}]}]}`},
 }
}
func TestTaskPlanCannotAcceptFailedNativePatchAsCompleted(t *testing.T){
 success:=exampleAutoPatchCall()
 for name,result:=range map[string]string{
  "fail":`{"status":"FAIL","rc":1}`,
  "preflight":`{"status":"PREFLIGHT_FAIL","rc":0}`,
  "incomplete":`{"status":"INCOMPLETE"}`,
  "nonzero_rc":`{"status":"PASS","rc":7}`,
  "nested_fail":`{"status":"PASS","rc":0,"commands":[{"status":"FAIL","rc":1}]}`,
  "nested_timeout":`{"status":"PASS","rc":0,"commands":[{"status":"PASS","timed_out":true}]}`,
  "unparseable":"patch passed",
 } {
  t.Run(name,func(t *testing.T){
   bad:=success;bad.Result=result
   if _,err:=VerifyTaskPlanEvidence(Session{ToolCalls:[]ToolCallRecord{bad}},"patch",bad.CallID,"");err==nil{
    t.Fatal("unverified or failing Native Patch was accepted as milestone completion")
   }
   if _,err:=VerifyTaskAutoCheck(Session{ToolCalls:[]ToolCallRecord{bad}},
    &TaskAutoCheck{Kind:"patch_pass",CallID:bad.CallID});err==nil {
    t.Fatal("automatic milestone accepted failed Patch")
   }
  })
 }
 if _,err:=VerifyTaskPlanEvidence(Session{ToolCalls:[]ToolCallRecord{success}},
  "patch",success.CallID,"");err!=nil{t.Fatalf("verified PASS was rejected: %v",err)}
 if _,err:=VerifyTaskAutoCheck(Session{ToolCalls:[]ToolCallRecord{success}},
  &TaskAutoCheck{Kind:"patch_pass",CallID:success.CallID});err!=nil{
  t.Fatalf("exact receipt did not satisfy machine rule: %v",err)
 }
 success.RunID="different-run"
 if _,err:=VerifyTaskAutoCheck(Session{ToolCalls:[]ToolCallRecord{success}},
  &TaskAutoCheck{Kind:"patch_pass",CallID:success.CallID});err==nil{
  t.Fatal("mismatched native receipt was accepted")
 }
}
func TestTaskPlanCollectionProofRejectsFailuresAndAutoRequiresExactFullRead(t *testing.T){
 items:=exampleAutoCollectionRead()
 x:=Session{History:items}
 valid:=&TaskAutoCheck{Kind:"full_file_read",CallID:"collect-123",Path:"src/main.go"}
 if _,err:=VerifyTaskAutoCheck(x,valid);err!=nil{t.Fatalf("good full read failed: %v",err)}
 if _,err:=VerifyTaskAutoCheck(x,&TaskAutoCheck{Kind:"full_file_read",CallID:"collect-123",Path:"src/other.go"});err==nil{
  t.Fatal("wrong file accepted")
 }
 for name,bad:=range map[string]string{
  "empty":`{"results":[]}`,
  "top_error":`{"error":{"code":"COLLECTION_RESULT_BUDGET_EXCEEDED"}}`,
  "action_error":`{"results":[{"type":"read","error":"path not found"}]}`,
  "file_error":`{"results":[{"type":"read","files":[{"path":"src/main.go","error":"permission denied"}]}]}`,
  "truncated":`{"response_truncated":true,"results":[{"type":"read","files":[{"path":"src/main.go","content":"partial"}]}]}`,
  "paged":`{"results":[{"type":"read","files":[{"path":"src/main.go","content":"partial","next_line":"201"}]}]}`,
  "file_truncated":`{"results":[{"type":"read","files":[{"path":"src/main.go","content":"partial","truncated":"true"}]}]}`,
 } {
  t.Run(name,func(t *testing.T){
   content:=exampleAutoCollectionRead()
   content[1].Content=bad
   if _,err:=VerifyTaskAutoCheck(Session{History:content},valid);err==nil{
    t.Fatal("partial, erroneous or empty Collection was auto accepted")
   }
   if name=="empty"||name=="top_error"||name=="action_error"||name=="file_error"{
    if _,err:=VerifyTaskPlanEvidence(Session{History:content},"collection","collect-123","");err==nil{
     t.Fatal("failed Collection was accepted as manual completion proof")
    }
   }
  })
 }
}
func TestAutoRulesRequireExactOwnerSpecifiedCallAndSafePath(t *testing.T){
 for _,rule:=range []*TaskAutoCheck{
  {Kind:"patch_pass",CallID:""}, {Kind:"patch_pass",CallID:"call",Path:"src/a.go"},
  {Kind:"full_file_read",CallID:"call",Path:"../passwords"},
  {Kind:"full_file_read",CallID:"call",Path:"/etc/passwd"},
  {Kind:"full_file_read",CallID:"call",Path:"src\bad"},
  {Kind:"full_file_read",CallID:"call",Path:"src/../file"},
  {Kind:"unknown",CallID:"call"},
 } {
  if ValidateTaskAutoCheck(rule)==nil{t.Fatalf("unsafe automatic acceptance rule passed: %+v",rule)}
 }
 good:=&TaskAutoCheck{Kind:"full_file_read",CallID:"collect-123",Path:"src/main.go"}
 if err:=ValidateTaskAutoCheck(good);err!=nil{t.Fatalf("safe exact rule rejected: %v",err)}
}
