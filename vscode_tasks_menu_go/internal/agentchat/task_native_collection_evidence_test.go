package agentchat

import (
 "encoding/json"
 "testing"
)

func TestNativeCollectionReceiptTaskEvidence(t *testing.T) {
 receipt:=map[string]any{
  "source":"native_patch_tool_collect","project":map[string]string{"id":"project-a","key":"fixture"},
  "status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,
  "partial_evidence_available":false,"native_session_id":"native-session","run_id":"agent_collect_fixture",
  "result_zip":"artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_fixture.zip",
  "result_text":"artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_fixture.txt",
  "text_excerpt_truncated":true,
 }
 makeSession:=func(changes map[string]any)Session{
  data:=map[string]any{}
  for k,v:=range receipt{data[k]=v}
  for k,v:=range changes{if v==nil{delete(data,k)}else{data[k]=v}}
  raw,_:=json.Marshal(data)
  call:=HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"exact",OriginRunID:"run-a",ProviderID:"provider-a",ModelID:"model-a",ParentTurnID:"turn-a",Arguments:"{\"actions\":[{\"type\":\"file\",\"path\":\"main.go\"}]}"}
  result:=call
  result.Type="function_call_output";result.Name="";result.Arguments="";result.Content=string(raw)
  return Session{ProjectID:"project-a",History:[]HistoryItem{call,result}}
 }
 x:=makeSession(nil)
 evidence,err:=VerifyTaskPlanEvidence(x,"collection","exact","Reviewed native observation")
 if err!=nil||evidence.Kind!="collection"||evidence.Ref!="exact"||evidence.Digest!=taskPlanDigest(x.History[1].Content){t.Fatalf("native wrapper PASS not recognized: %+v %v",evidence,err)}
 for _,tc:=range []struct{name string;changes map[string]any}{
  {"partial",map[string]any{"partial_evidence_available":true}},
  {"missing partial flag",map[string]any{"partial_evidence_available":nil}},
  {"missing rc",map[string]any{"rc":nil}},
  {"nonzero rc",map[string]any{"rc":3}},
  {"incomplete",map[string]any{"status":"INCOMPLETE"}},
  {"native failure",map[string]any{"native_status":"FAIL"}},
  {"unverified",map[string]any{"evidence_verified":false}},
  {"foreign project",map[string]any{"project":map[string]string{"id":"project-b"}}},
  {"missing project",map[string]any{"project":nil}},
  {"missing native session",map[string]any{"native_session_id":nil}},
  {"missing run",map[string]any{"run_id":nil}},
  {"missing companion",map[string]any{"result_text":nil}},
  {"wrong companion",map[string]any{"result_text":"artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_other.txt"}},
  {"unsafe artifact",map[string]any{"result_zip":"artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_x/../evil.zip"}},
  {"artifact error",map[string]any{"artifact_error":"corrupt"}},
  {"text error",map[string]any{"text_error":"unavailable"}},
  {"failure reason",map[string]any{"failure_reason":"timeout"}},
  {"error with fake inline results",map[string]any{"error":"failed","results":[]any{map[string]any{}}}},
 }{
  t.Run(tc.name,func(t *testing.T){
   if _,err:=VerifyTaskPlanEvidence(makeSession(tc.changes),"collection","exact","");err==nil{t.Fatal("invalid native result accepted")}
  })
 }
 for _,tc:=range []struct{name string;mutate func(*Session)}{
  {"other run",func(x *Session){x.History[1].OriginRunID="run-b"}},
  {"other provider",func(x *Session){x.History[1].ProviderID="provider-b"}},
  {"other parent",func(x *Session){x.History[1].ParentTurnID="turn-b"}},
  {"duplicate output",func(x *Session){x.History=append(x.History,x.History[1])}},
  {"duplicate call",func(x *Session){x.History=append([]HistoryItem{x.History[0]},x.History...)}},
  {"reverse order",func(x *Session){x.History[0],x.History[1]=x.History[1],x.History[0]}},
  {"assistant narration",func(x *Session){x.History[1].Type="assistant"}},
 }{
  t.Run(tc.name,func(t *testing.T){x:=makeSession(nil);tc.mutate(&x);if _,err:=VerifyTaskPlanEvidence(x,"collection","exact","");err==nil{t.Fatal("ambiguous receipt accepted")}})
 }
 // Even a complete native archive receipt is not proof of the old inline
 // full-file read predicate. An embedded fabricated results array cannot
 // make this different receipt schema satisfy an exact full-file rule.
 x=makeSession(map[string]any{"results":[]any{map[string]any{"type":"read","files":[]any{map[string]any{"path":"main.go","start_line":"1","total_lines":"1","content":"x","truncated":"false"}}}}})
 x.History[0].Arguments="{\"actions\":[{\"type\":\"read\",\"path\":\"main.go\"}]}"
 if _,err:=VerifyTaskAutoCheck(x,&TaskAutoCheck{Kind:"full_file_read",CallID:"exact",Path:"main.go"});err==nil{t.Fatal("native wrapper accepted as full file proof")}
}
