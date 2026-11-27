package server

import(
 "encoding/json"
 "strings"
 "testing"
 "bletonfc/vscode_tasks_menu/internal/agentchat"
)
func TestAgentCLP20MandatoryReceiptAcrossContextRecovery(t *testing.T){
 for _,fallback:=range []bool{false,true}{
  goal:=agentchat.HistoryItem{Type:"user",Content:"Original owner goal"}
  call:=agentchat.HistoryItem{Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{"project":{"key":"p","id":"id"},"actions":[{"type":"file","path":"a"}]}`}
  receipt:=agentchat.HistoryItem{Type:"function_call_output",CallID:"read",Content:`{"status":"PASS","text":"`+strings.Repeat("exact-receipt-",14000)+`"}`}
  if fallback{call=agentchat.HistoryItem{Type:"assistant",Content:call.Arguments};receipt=agentchat.HistoryItem{Type:"user",Content:agentchat.NativeCollectionTextObservationPrefix+receipt.Content}}
  history:=[]agentchat.HistoryItem{goal}
  for i:=0;i<5;i++{history=append(history,agentchat.HistoryItem{Type:"assistant",Content:strings.Repeat("optional-old-context-",12000)})}
  next:=[]agentchat.HistoryItem{call,receipt,{Type:"user",Content:agentCollectionResumeInstruction}}
  raw,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{Model:"m",Instructions:"Trusted policy",Mode:agentchat.Stateless,History:history,Next:next,MaxOutputTokens:65536});if err!=nil{t.Fatal(err)}
  for i:=0;i<3;i++{
   reduced,ok:=agentRetryContextPayload(raw);if !ok{t.Fatalf("recovery %d failed",i)}
   var wire struct{Input []map[string]string `json:"input"`}
   if json.Unmarshal(reduced,&wire)!=nil{t.Fatal("invalid wire")}
   count:=0
   for _,it:=range wire.Input{if it["output"]==receipt.Content||it["content"]==receipt.Content{count++}}
   if count!=1||!strings.Contains(string(reduced),"Original owner goal"){t.Fatalf("fallback=%v round=%d lost mandatory context",fallback,i)}
   raw=reduced
  }
 }
}

func TestAgentLegacyEmptyPatchFeedbackRecoveryKeepsExactReceipt(t *testing.T){
 call:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"patch",Arguments:"{}"}
 out:=agentchat.HistoryItem{Type:"function_call_output",CallID:"patch",Content:`{"status":"PASS","rc":0}`}
 x:=agentchat.Session{Owner:"local",Status:"failed",AutoContinueText:true,AutoFeedback:true,AutoFeedbackPersistent:true,RetryMode:agentchat.RetryUnlimited,
  ProviderRun:&agentchat.ProviderRun{ID:"r11-e20",Kind:"feedback",Phase:"unknown",CallID:"patch",PayloadSHA256:strings.Repeat("a",64),ProviderFingerprint:"p"},
  History:[]agentchat.HistoryItem{{Type:"user",Content:"Original review"},call,out},
  ToolCalls:[]agentchat.ToolCallRecord{{CallID:"patch",Name:call.Name,Status:"done",FeedbackSent:true,FeedbackStatus:"unknown",
   Result:out.Content,RunID:"native-run",PatchSessionID:"native-session",PromptID:"prompt",SelectedIndex:1,PackageSHA256:strings.Repeat("b",64),PackageSize:100,ArgumentsSHA256:agentPatchPayloadSHA256(call.Arguments)}},
  Events:[]agentchat.SessionEvent{{Seq:20,Kind:"provider_egress",CallID:"patch",Detail:"feedback"},{Seq:21,Kind:"feedback_unknown",CallID:"patch",Status:"failed",Detail:"provider_acknowledgement_unconfirmed: JSON_PARSE_FAILED"}}}
 if !agentPatchFeedbackAutoResumeReady(x){t.Fatal("exact historical receipt not recoverable")}
 next:=agentchat.HistoryItem{Type:"user",Content:agentPatchFeedbackResumeInstruction}
 h,n,ok:=agentPatchFeedbackResumeProjection(x,x.History,next)
 if !ok||len(h)!=1||len(n)!=3||n[0]!=call||n[1]!=out||n[2]!=next{t.Fatal("exact receipt not projected as mandatory")}
 if agentMandatoryRetrySuffix(append(h,n...))!=1{t.Fatal("context retry could lose receipt")}
 if x.ToolCalls[0].FeedbackStatus!="unknown"||len(x.History)!=3{t.Fatal("recovery invented ACK or changed canonical history")}
 raw,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{Model:"m",Instructions:"policy",Mode:agentchat.Stateless,History:h,Next:n})
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(raw),`"call_id":"patch"`){t.Fatal("wire missing exact call")}
 for _,mutate:=range []func(*agentchat.Session){
  func(y *agentchat.Session){y.AutoContinueText=false},
  func(y *agentchat.Session){y.AutoFeedback=false},
  func(y *agentchat.Session){y.RetryMode=agentchat.RetryOff},
  func(y *agentchat.Session){y.Status="cancelled"},
  func(y *agentchat.Session){y.ProviderRun.Phase="sending"},
  func(y *agentchat.Session){y.History[2].Content=`{"status":"PASS","rc":1}`},
  func(y *agentchat.Session){y.ToolCalls[0].Status="running"},
  func(y *agentchat.Session){y.Events[1].Detail="provider_acknowledgement_unconfirmed: REQUEST_TIMEOUT"},
  func(y *agentchat.Session){y.Events=append(y.Events,agentchat.SessionEvent{Kind:"patch_feedback_resume_refused",Detail:y.ProviderRun.ID})},
 }{
  y:=x;r:=*x.ProviderRun;y.ProviderRun=&r
  y.History=append([]agentchat.HistoryItem(nil),x.History...)
  y.ToolCalls=append([]agentchat.ToolCallRecord(nil),x.ToolCalls...)
  y.Events=append([]agentchat.SessionEvent(nil),x.Events...)
  mutate(&y)
  if agentPatchFeedbackAutoResumeReady(y){t.Fatal("unsafe legacy recovery admitted")}
 }
}
