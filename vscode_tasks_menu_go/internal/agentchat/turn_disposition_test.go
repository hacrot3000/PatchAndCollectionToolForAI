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
 RecordTurnDisposition(&x,"Generic answer with no unfinished task marker.")
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
