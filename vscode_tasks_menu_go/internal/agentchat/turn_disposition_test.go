package agentchat

import (
 "strings"
 "encoding/json"
 "unicode/utf8"
 "testing"
)

func TestTurnDispositionVietnameseAuditCheckpoint(t *testing.T){
 text:="Đã tiếp tục thu thập bằng chứng cho M03–M06 và lưu checkpoint bền vững. "+
  "Audit chưa hoàn tất: chưa phân loại đủ 37 trường hợp. "+
  "Bước tiếp theo: kiểm tra save/worker/runtime của M03–M06, sau đó M07–M10."
 c:=AnalyzeTurnText(text)
 if c.Disposition!=TurnNeedsContinuation || !strings.Contains(c.NextAction,"M07–M10")||
  len(c.ResponseDigest)!=64||len(c.ProgressSignature)!=64{
  t.Fatalf("unfinished audit must be resumed, got %+v",c)
 }
}
func TestTurnDispositionStrictModelMetadata(t *testing.T){
 text:="Audit progress\n"+ "~~~" // A non-fenced imitation must not be parsed as metadata.
 c:=AnalyzeTurnText(text)
 if c.Disposition!=TurnUnknown{t.Fatalf("unexpected metadata from text: %+v",c)}
 valid:="Start\n"+"\x60\x60\x60taskdeck-progress\n"+
  `{"status":"NEEDS_CONTINUATION","next_action":"Read M07-M10","checkpoint_path":"docs/audit.md","evidence_refs":["collect-real-id"]}` +
  "\n```"
 c=AnalyzeTurnText(valid)
 if c.Disposition!=TurnNeedsContinuation||c.NextAction!="Read M07-M10"||
  c.CheckpointPath!="docs/audit.md"||len(c.EvidenceRefs)!=1{
  t.Fatalf("structured progress was lost: %+v",c)
 }
 invalid:="```taskdeck-progress\n"+
  `{"status":"NEEDS_CONTINUATION","next_action":"Read","checkpoint_path":"../../outside"}` +
  "\n```"
 if got:=AnalyzeTurnText(invalid);got.CheckpointPath!=""{
  t.Fatalf("unsafe checkpoint path accepted: %+v",got)
 }
 unknownField:="```taskdeck-progress\n"+
  `{"status":"NEEDS_CONTINUATION","secret_authority":true}` +
  "\n```"
 if got:=AnalyzeTurnText(unknownField);got.Disposition!=TurnUnknown{
  t.Fatalf("unknown metadata key granted scheduling authority: %+v",got)
 }
}
func TestTurnDispositionUnknownDoesNotGrantContinuation(t *testing.T){
 x:=Session{Owner:"local",Status:"completed",AutoContinueText:true}
 RecordTurnDisposition(&x,"Here is the technical information you requested.")
 if StandaloneAutoContinueCandidate(x){t.Fatal("unconfirmed work automatically resumed")}
 RecordTurnDisposition(&x,"Task complete: all work done.")
 if StandaloneAutoContinueCandidate(x){t.Fatal("completion claim automatically resumed")}
 RecordTurnDisposition(&x,"Remaining work: verify M07. Next step: collect sources.")
 if !StandaloneAutoContinueCandidate(x){t.Fatal("unfinished task not eligible")}
 x.ProviderRun=&ProviderRun{Phase:"unknown"}
 if StandaloneAutoContinueCandidate(x){t.Fatal("unknown provider outcome replayed")}
 x.ProviderRun=nil
 x.Status="cancelled"
 if StandaloneAutoContinueCandidate(x){t.Fatal("cancelled turn resumed")}
 x.Status="completed"
 x.ToolCalls=[]ToolCallRecord{{CallID:"patch",Status:"done",FeedbackStatus:"unknown"}}
 if StandaloneAutoContinueCandidate(x){t.Fatal("ambiguous feedback bypassed")}
 x.ToolCalls=nil
 x.AutoContinueText=false
 if StandaloneAutoContinueCandidate(x){t.Fatal("no opt-in bypassed")}
}
func TestTextRephrasingDoesNotResetDurableNoProgress(t *testing.T){
 x:=Session{}
 RecordTurnDisposition(&x,"Audit chưa hoàn tất. Bước tiếp theo: đọc M07.")
 x.Continuation.NoProgressTurns=3
 RecordTurnDisposition(&x,"Audit vẫn chưa xong. Bước tiếp theo: kiểm tra M08.")
 if x.Continuation.NoProgressTurns!=3{
  t.Fatalf("rephrased text reset machine counter: %+v",x.Continuation)
 }
 if !ValidTurnContinuation(x.Continuation){t.Fatal("invalid durable continuation")}
}


func TestHistoricalUnconfirmedFeedbackRequiresNewTurnAndExactReceipt(t *testing.T){
 old:=ToolCallRecord{CallID:"old",Name:"taskdeck_submit_patch",Status:"done",Result:`{"status":"PASS"}`,FeedbackSent:true,FeedbackStatus:"unknown"}
 x:=Session{Owner:"local",Status:"completed",AutoContinueText:true,ProviderRun:&ProviderRun{Phase:"completed"},ToolCalls:[]ToolCallRecord{old},
 History:[]HistoryItem{{Type:"function_call",Name:old.Name,CallID:old.CallID},{Type:"function_call_output",CallID:old.CallID,Content:old.Result},{Type:"user",Content:"continue"},{Type:"assistant",Content:"Remaining work: inspect sources."}}}
 RecordTurnDisposition(&x,x.History[3].Content)
 if !HistoricalUnconfirmedFeedback(x,old)||!StandaloneAutoContinueCandidate(x){t.Fatal("historical ambiguity blocked new turn")}
 x.ProviderRun.Phase="unknown"
 if HistoricalUnconfirmedFeedback(x,old)||StandaloneAutoContinueCandidate(x){t.Fatal("active provider ambiguity bypassed")}
 x.ProviderRun.Phase="completed"
 x.History=x.History[:3]
 if HistoricalUnconfirmedFeedback(x,old){t.Fatal("user input without provider response treated as recovery")}
 x.History=append(x.History,HistoryItem{Type:"assistant",Content:"new response"})
 x.History[1].Content="different receipt"
 if HistoricalUnconfirmedFeedback(x,old){t.Fatal("mismatched output accepted")}
}


func TestTurnDispositionClippedMilestoneAnnouncement(t *testing.T){
 for _,text:=range []string{
  "Tiếp tục P1.1.2.3: thu thập nguồn và contract đã xác minh, rồi triển khai một đơn vị có kiểm",
  "Continue M17-02: inspect the current receipt and implement the",
  "Continuing with UAG-02: verify the source",
 }{
  x:=Session{Owner:"local",Status:"completed",AutoContinueText:true}
  RecordTurnDisposition(&x,text)
  if x.Continuation.Disposition!=TurnNeedsContinuation||x.Continuation.Reason!="unfinished_action_announcement"||x.Continuation.NextAction==""{
   t.Fatalf("unfinished milestone announcement not recognized: %+v",x.Continuation)
  }
  if !StandaloneAutoContinueCandidate(x){t.Fatal("consented unfinished milestone should be eligible")}
  x.AutoContinueText=false
  if StandaloneAutoContinueCandidate(x){t.Fatal("announcement bypassed opt-in")}
  x.AutoContinueText=true
  x.ProviderRun=&ProviderRun{Phase:"unknown"}
  if StandaloneAutoContinueCandidate(x){t.Fatal("announcement bypassed ambiguous provider fence")}
  x.ProviderRun=nil
  x.Continuation.NoProgressTurns=MaxContinuationNoWorkTurns
  if StandaloneAutoContinueCandidate(x){t.Fatal("announcement bypassed verified-work brake")}
 }
 for _,text:=range []string{
  "Tiếp tục P1.1.2.3: đã hoàn thành toàn bộ.",
  "Tiếp tục P1.1.2.3: bị chặn bởi điều kiện bắt buộc.",
  "Continue reading the documentation for more information.",
  "Here is a sample: Continue M17-02: inspect the current receipt.",
 }{
  if c:=AnalyzeTurnText(text);c.Disposition==TurnNeedsContinuation||c.Disposition==TurnInProgress{
   t.Fatalf("ordinary guidance, completion or blocker unexpectedly scheduled: %q %+v",text,c)
  }
 }
}

func TestTurnDispositionClippedMilestoneActionUTF8Bound(t *testing.T){
 c:=AnalyzeTurnText("Tiếp tục P1.1.2.3: "+strings.Repeat("kiểm ",200))
 if c.Disposition!=TurnNeedsContinuation||!ValidTurnContinuation(c)||len(c.NextAction)>500{t.Fatalf("invalid bounded announcement: %+v",c)}
}

func TestUnknownCollectionProgressRecheckBounded(t *testing.T){
 text:="The log shows the following transactions."
 x:=Session{Owner:"local",Status:"completed",AutoContinueText:true,
  ProviderRun:&ProviderRun{Kind:"collection",Phase:"completed",CallID:"read"},
  History:[]HistoryItem{{Type:"user",Content:"Audit all records"},
   {Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"read",Arguments:"{}"},
   {Type:"function_call_output",CallID:"read",Content:"{}"},
   {Type:"assistant",Content:text}}}
 RecordTurnDisposition(&x,text)
 if !UnknownCollectionProgressRecheck(x)||!StandaloneAutoContinueCandidate(x){t.Fatal("confirmed collection UNKNOWN must get status check")}
 x.AutoContinueText=false
 if StandaloneAutoContinueCandidate(x){t.Fatal("opt-out bypassed")}
 x.AutoContinueText=true
 x.ProviderRun.Phase="unknown"
 if StandaloneAutoContinueCandidate(x){t.Fatal("ambiguous provider replay")}
 x.ProviderRun.Phase="completed"
 x.ProviderRun.Kind="message"
 if StandaloneAutoContinueCandidate(x){t.Fatal("UNKNOWN status check loop")}
 x.ProviderRun.Kind="collection"
 x.History[2].CallID="other"
 if StandaloneAutoContinueCandidate(x){t.Fatal("mismatched receipt")}
 x.History[2].CallID="read"
 x.Continuation.NoProgressTurns=MaxContinuationNoWorkTurns
 if StandaloneAutoContinueCandidate(x){t.Fatal("no-progress brake bypassed")}
 x.Continuation.NoProgressTurns=0
 RecordTurnDisposition(&x,"Task complete: all work done.")
 if StandaloneAutoContinueCandidate(x){t.Fatal("completed task should not recheck")}
}

func TestTurnProgressLongVietnameseActionAndLegacyRecovery(t *testing.T){
 action:="Chỉ báo cáo dòng khác nhau theo bảng sáu cột UID, Type, buyInfo production, rewardInfo production, buyInfo backup, rewardInfo backup; JSON đầy đủ, không lặp. Danh sách UID: 272000007, 255000039, 243000474, 260000039, 219000202, 276000091, 231000071 (server 237). Chưa xác nhận UID nào hoàn tất toàn bộ. UID 231000071 đã báo cáo type 201, 301, 801; tiếp tục type 701 và kiểm tra các UID/type còn lại, kể cả dòng chỉ có một phía. Không coi grep với mẫu giới hạn là bằng chứng đầy đủ."
 if len(action)!=559||utf8.RuneCountInString(action)!=481{t.Fatal("incident fixture changed")}
 raw,err:=json.Marshal(declaredTurnProgress{Status:TurnNeedsContinuation,NextAction:action});if err!=nil{t.Fatal(err)}
 text:="Report.\n\n"+"\x60\x60\x60taskdeck-progress\n"+string(raw)+"\n\x60\x60\x60"
 got:=AnalyzeTurnText(text)
 if got.Disposition!=TurnNeedsContinuation||got.NextAction!=action||!ValidTurnContinuation(got){t.Fatalf("lost exact metadata: %+v",got)}
 x:=Session{Owner:"local",Status:"completed",AutoContinueText:true,
  History:[]HistoryItem{{Type:"assistant",Content:text}},
  Continuation:&TurnContinuation{Disposition:TurnUnknown,Reason:"unknown_model_turn",ResponseDigest:turnHash(text),NoProgressTurns:2,RepeatedTurns:2}}
 effective:=EffectiveTurnContinuation(x)
 if effective.Disposition!=TurnNeedsContinuation||effective.NextAction!=action||effective.NoProgressTurns!=2||effective.RepeatedTurns!=2||!StandaloneAutoContinueCandidate(x){t.Fatalf("legacy recovery failed: %+v",effective)}
 if x.Continuation.Disposition!=TurnUnknown{t.Fatal("read-only classifier mutated stored record")}
 x.AutoContinueText=false
 if StandaloneAutoContinueCandidate(x){t.Fatal("legacy reclassification bypassed opt-out")}
 x.AutoContinueText=true
 x.History[0].Content+=" stale"
 if StandaloneAutoContinueCandidate(x){t.Fatal("stale digest authorized continuation")}
}
func TestTurnProgressOversizeActionKeepsDisposition(t *testing.T){
 action:=strings.Repeat("kiểm tra 🚲 ",600)
 raw,err:=json.Marshal(declaredTurnProgress{Status:TurnNeedsContinuation,NextAction:action});if err!=nil{t.Fatal(err)}
 got:=AnalyzeTurnText("\x60\x60\x60taskdeck-progress\n"+string(raw)+"\n\x60\x60\x60")
 if got.Disposition!=TurnNeedsContinuation||!ValidTurnContinuation(got)||!utf8.ValidString(got.NextAction)||!strings.Contains(got.NextAction,"truncated; consult the original"){t.Fatalf("oversize advisory discarded status: %+v",got)}
}

func TestUnknownPatchCorrectionRequiresExactAcknowledgedFailure(t *testing.T){
 text:="I reviewed the previous result."
 output:=`{"status":"FAIL","rc":2,"run_id":"native-run","diagnosis_kind":"patch_payload_failed","mutation_state":"UNKNOWN"}`
 x:=Session{Owner:"local",Status:"completed",AutoContinueText:true,
  ProviderRun:&ProviderRun{Kind:"feedback",Phase:"completed",CallID:"patch"},
  History:[]HistoryItem{{Type:"user",Content:"Fix the document"},{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"patch",Arguments:"{}"},{Type:"function_call_output",CallID:"patch",Content:output},{Type:"assistant",Content:text}},
  ToolCalls:[]ToolCallRecord{{Name:"taskdeck_submit_patch",CallID:"patch",Status:"error",Result:output,RunID:"native-run",ArgumentsSHA256:turnHash("{}"),FeedbackSent:true,FeedbackStatus:"acknowledged"}}}
 RecordTurnDisposition(&x,text)
 if !UnknownPatchCorrectionRecheck(x)||!StandaloneAutoContinueCandidate(x){t.Fatal("valid failure must request correction")}
 for _,mutate:=range []func(*Session){
  func(y *Session){y.AutoContinueText=false},
  func(y *Session){y.ProviderRun.Kind="message"},
  func(y *Session){y.ProviderRun.Phase="unknown"},
  func(y *Session){y.ToolCalls[0].FeedbackStatus="unknown"},
  func(y *Session){y.ToolCalls[0].RunID="other"},
  func(y *Session){y.ToolCalls[0].Status="done"},
  func(y *Session){y.History[2].Content="different"},
  func(y *Session){y.History[1].Arguments="different"},
  func(y *Session){y.Continuation.NoProgressTurns=MaxContinuationNoWorkTurns},
  func(y *Session){y.Continuation.Disposition=TurnBlocked},
 }{
  y:=x;r:=*x.ProviderRun;y.ProviderRun=&r;d:=*x.Continuation;y.Continuation=&d
  y.History=append([]HistoryItem(nil),x.History...);y.ToolCalls=append([]ToolCallRecord(nil),x.ToolCalls...)
  mutate(&y)
  if StandaloneAutoContinueCandidate(y){t.Fatal("unsafe or repeated correction admitted")}
 }
}
func TestPatchCorrectionClassification(t *testing.T){
 for _,tc:=range []struct{status,result string;want bool}{
  {"error",`{"status":"FAIL","rc":2,"run_id":"r","diagnosis_kind":"patch_payload_failed"}`,true},
  {"error",`{"status":"FAIL","rc":1,"run_id":"r","diagnosis_kind":"anchor_mismatch"}`,true},
  {"error",`{"status":"FAIL","rc":2,"run_id":"r","diagnosis_kind":"patch_payload_timeout"}`,false},
  {"error",`{"status":"PREFLIGHT_FAIL","rc":2,"run_id":"r","diagnosis_kind":"insufficient_disk_space"}`,false},
  {"done",`{"status":"PASS","rc":0,"run_id":"r"}`,false},
  {"rejected",`{"status":"REJECTED","reason_code":"INVALID_PATCH_ARGUMENTS","retryable_by_ai":true,"patch_queued":false,"patch_executed":false}`,true},
  {"rejected",`{"status":"REJECTED","reason_code":"USER_REJECTED","retryable_by_ai":true}`,false},
 }{
  call:=ToolCallRecord{Name:"taskdeck_submit_patch",Status:tc.status,RunID:"r",Result:tc.result}
  if PatchCorrectionRequired(call)!=tc.want{t.Fatalf("wrong correction classification: %s",tc.result)}
 }
}
