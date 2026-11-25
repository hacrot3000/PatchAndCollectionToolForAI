package agentchat

import (
 "encoding/json"
 "strings"
 "testing"
)

func TestNativeFullFileAutoCheckRequiresExactActionReceipt(t *testing.T){
 receipt:=map[string]any{
  "source":"native_patch_tool_collect","project":map[string]string{"id":"project-a"},
  "status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"partial_evidence_available":false,
  "native_session_id":"native-session","run_id":"agent_collect_exact",
  "result_zip":"artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_exact.zip",
  "result_text":"artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_exact.txt",
  "text_excerpt_truncated":false,
 }
 proof:=map[string]any{"path":"main.go","action_index":1,"total_lines":2,"report_sha256":strings.Repeat("a",64)}
 receipt["native_full_file_reads"]=[]any{proof}
 makeSession:=func(args string)Session{
  raw,_:=json.Marshal(receipt)
  call:=HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"call-a",Arguments:args,OriginRunID:"run-a",ProviderID:"p",ModelID:"m",ParentTurnID:"t"}
  out:=call;out.Type="function_call_output";out.Name="";out.Arguments="";out.Content=string(raw)
  return Session{ProjectID:"project-a",History:[]HistoryItem{call,out}}
 }
 rule:=&TaskAutoCheck{Kind:"full_file_read",CallID:"call-a",Path:"main.go"}
 args:=`{"actions":[{"type":"file","path":"main.go"}]}`
 if e,err:=VerifyTaskAutoCheck(makeSession(args),rule);err!=nil||e.Ref!="call-a"||e.Digest==""{t.Fatalf("valid native full file rejected: %+v %v",e,err)}
 for _,bad:=range []string{
  `{"actions":[{"type":"range","path":"main.go"}]}`,
  `{"actions":[{"type":"file","path":"main.go","start_line":2}]}`,
  `{"actions":[{"type":"file","path":"main.go","end_line":2}]}`,
  `{"actions":[{"type":"file","path":"other.go"}]}`,
  `{"actions":[{"type":"head","path":"main.go"},{"type":"file","path":"main.go"}]}`,
 }{if _,err:=VerifyTaskAutoCheck(makeSession(bad),rule);err==nil{t.Fatalf("wrong action accepted: %s",bad)}}
 for _,tc:=range []struct{key string;value any}{
  {"path","other.go"},{"action_index",2},{"total_lines",-1},{"total_lines",nil},{"report_sha256","not-a-digest"},
 }{
  old:=proof[tc.key];proof[tc.key]=tc.value
  if _,err:=VerifyTaskAutoCheck(makeSession(args),rule);err==nil{t.Fatalf("invalid report metadata accepted: %s",tc.key)}
  proof[tc.key]=old
 }
 receipt["text_excerpt_truncated"]=true
 if _,err:=VerifyTaskAutoCheck(makeSession(args),rule);err==nil{t.Fatal("clipped native response accepted")}
 receipt["text_excerpt_truncated"]=false
 receipt["native_full_file_reads"]=[]any{proof,proof}
 if _,err:=VerifyTaskAutoCheck(makeSession(args),rule);err==nil{t.Fatal("ambiguous duplicate proof accepted")}
 receipt["native_full_file_reads"]=[]any{proof}
 x:=makeSession(args);x.History[1].OriginRunID="foreign"
 if _,err:=VerifyTaskAutoCheck(x,rule);err==nil{t.Fatal("foreign run output accepted")}
}
