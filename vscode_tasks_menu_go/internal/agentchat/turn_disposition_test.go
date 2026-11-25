package agentchat

import (
 "strings"
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

func TestTurnProgressProviderCompatibilityAndRecovery(t *testing.T){
 raw:=`{"status":"NEEDS_CONTINUATION","next_action":"Đọc nốt fixture mock_endpoint_switch_v1.json dòng 700-2356 (X22-X64, P01-P11, K01-K03), sau đó implement EndpointSwitchModels + SyntheticEndpointSwitch reducer + xUnit tests đủ 78 ca, chạy scripts/run_dotnet_research_tests.sh, rồi thêm integration test multi-endpoint cutover thật trên localhost trước khi cập nhật tracker P1.1.1.4.","checkpoint_path":"implementing_doc/coder_tasks/TASK_EXECUTION_STATUS.md","evidence_refs":["agent_collect_d47a2097be680c2ea65816f9","agent_collect_42c6deefbb0c3b3fb86f0a42","agent_collect_98293a3440a45cff95db6fd8"]}`
 for _,text:=range []string{raw,"Report.\ntaskdeck-progress\n"+raw,"Report.\ntaskdeck-progress\n⧉\n\n"+raw,"Report.\ntaskdeck-progress\n\x60\x60\x60json\n"+raw+"\n\x60\x60\x60","Report.\n\x60\x60\x60taskdeck-progress\n"+raw+"\n\x60\x60\x60"}{
  c:=AnalyzeTurnText(text)
  if c.Disposition!=TurnNeedsContinuation||c.Reason!="model_progress_metadata_unverified"||!strings.Contains(c.NextAction,"P1.1.1.4")||len(c.EvidenceRefs)!=3{t.Fatalf("metadata lost: %+v",c)}
 }
 for _,text:=range []string{"Example object: "+raw,"taskdeck-progress\n"+raw+"\nnot final","taskdeck-progress\n"+raw+"\ntaskdeck-progress\n"+raw}{
  if _,ok:=parseDeclaredTurnProgress(text);ok{t.Fatalf("ambiguous metadata accepted: %q",text)}
 }
 x:=Session{Owner:"local",Status:"completed",AutoContinueText:true,History:[]HistoryItem{{Type:"assistant",Content:raw}},Continuation:&TurnContinuation{Disposition:TurnUnknown,ResponseDigest:turnHash(raw),NoProgressTurns:2}}
 if !RecoverTurnProgress(&x)||!StandaloneAutoContinueCandidate(x)||x.Continuation.NoProgressTurns!=2{t.Fatalf("legacy recovery failed: %+v",x.Continuation)}
 if RecoverTurnProgress(&x){t.Fatal("recovery not idempotent")}
 x.AutoContinueText=false
 if StandaloneAutoContinueCandidate(x){t.Fatal("recovery granted consent")}
 x.Continuation=nil;x.History[0].Type="user"
 if RecoverTurnProgress(&x){t.Fatal("user input treated as model metadata")}
}
