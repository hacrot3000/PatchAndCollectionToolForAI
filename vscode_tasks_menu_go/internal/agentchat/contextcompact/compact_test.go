package contextcompact

import (
 "strings"
 "fmt"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestNewPatchReceiptOnlyOnceThenSummary(t *testing.T){
 code:=strings.Repeat("python patch mutates source\n",500)
 patch:=agentchat.HistoryItem{Type:"function_call",CallID:"patch-1",Name:"taskdeck_submit_patch",Arguments:fmt.Sprintf(`{"name":"Fix PHP CLI","patch_python":%q}`,code)}
 result:=agentchat.HistoryItem{Type:"function_call_output",CallID:"patch-1",Content:`{"status":"PASS","run_id":"run-44","rc":0}`}
 history:=[]agentchat.HistoryItem{{Type:"user",Content:"Fix PHP CLI"},patch,result}
 visible,stats:=Build(history,Policy{MaxInputTokens:32768})
 if stats.SummarizedPatches!=1 {t.Fatalf("completed patch must compact: %+v",stats)}
 content:=fmt.Sprint(visible)
 if strings.Contains(content,"python patch mutates source") || strings.Contains(content,`"status":"PASS","run_id"`){
  t.Fatal("completed historical Patch source/result must not be sent a second time")
 }
 if !strings.Contains(content,"Fix PHP CLI")||!strings.Contains(content,"PASS")||!strings.Contains(content,"run-44"){
  t.Fatalf("historic Patch metadata missing: %s",content)
 }
 // A pending provider Patch call stays in full to pair with the exact new
 // function_call_output; no compacted assistant text can replace it.
 pending:=[]agentchat.HistoryItem{{Type:"user",Content:"Fix PHP CLI"},patch}
 immediate:=[]agentchat.HistoryItem{{Type:"function_call_output",CallID:"patch-1",Content:result.Content}}
 visible, _=Build(pending,Policy{MaxInputTokens:32768,Next:immediate})
 if !strings.Contains(fmt.Sprint(visible),"python patch mutates source"){t.Fatal("new Patch tool output lost its pending function-call arguments")}
 if immediate[0].Content!=result.Content{t.Fatal("new Patch result was modified")}
}

func TestRecentCollectionsSelectedBySafeTokenBudgetAndOlderEvidenceSummarized(t *testing.T){
 history:=[]agentchat.HistoryItem{{Type:"user",Content:"Study the repository and repair three modules."}}
 for i:=0;i<26;i++{
  id:=fmt.Sprintf("read-%d",i)
  path:=fmt.Sprintf("src/%02d/module.php",i)
  history=append(history,agentchat.HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_collect_workspace",Arguments:fmt.Sprintf(`{"actions":[{"type":"read","path":%q}]}`,path)},
   agentchat.HistoryItem{Type:"function_call_output",CallID:id,Content:fmt.Sprintf(`{"results":[{"files":[{"path":%q,"content":%q}]}]}`,path,strings.Repeat("<?php\nfunction f() {}\n",190))})
 }
 original:=history[2].Content
 next:=[]agentchat.HistoryItem{{Type:"user",Content:"Continue task; do not stop"}}
 visible,stats:=Build(history,Policy{MaxInputTokens:25000,CollectionPercent:30,Next:next,Instructions:strings.Repeat("policy ",200)})
 if stats.RecentFullCollections<=0 || stats.RecentFullCollections>=26 || stats.SummarizedCollections<=0{
  t.Fatalf("expected adaptive N, not all or nothing: %+v",stats)
 }
 joined:=fmt.Sprint(visible)
 if !strings.Contains(joined,"src/00/module.php")||!strings.Contains(joined,"evidence:") {
  t.Fatalf("old observations missing path/evidence list: %s",joined)
 }
 if !strings.Contains(joined,"read-25") {t.Fatal("latest full Collection lost its pair")}
 if history[2].Content!=original {t.Fatal("canonical history mutated")}
 if stats.BudgetExceeded {t.Fatalf("context exceeded conservative budget: %+v",stats)}
 for i,v:=range visible{
  if v.Type=="function_call_output"{
   found:=false
   for _,call:=range visible[:i]{if call.Type=="function_call"&&call.CallID==v.CallID{found=true;break}}
   if !found{t.Fatalf("orphan output %q",v.CallID)}
  }
 }
}
func TestUtf8SummariesAndArchiveCheckpoint(t *testing.T){
 hist:=[]agentchat.HistoryItem{{Type:"user",Content:"Original task: giữ nguyên tiếng Việt"},
  {Type:"function_call",CallID:"c",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"read","path":"a.py"}]}`},
  {Type:"function_call_output",CallID:"c",Content:`{"results":[{"files":[{"path":"a.py","content":"Xin chào thế giới"}]}]}`}}
 got,_:=Build(hist,Policy{MaxInputTokens:4000,ArchivedItems:300,ArchivedParts:2})
 s:=fmt.Sprint(got)
 if !strings.Contains(s,"Original task")||!strings.Contains(s,"300 older history")||!strings.Contains(s,"Xin chào"){
  t.Fatalf("UTF-8 or checkpoint lost: %s",s)
 }
}

func TestHistoricalOrphanPatchIsNotReplayedButAuditRemains(t *testing.T){
 // Mirrors the real 2026-11-23 debug report: call_0354... appears at
 // history index 3 with NO output; 81 subsequent history records include
 // legitimate completed Patch/Collection exchanges. The provider rejected
 // the old implementation with HTTP 400 "No tool output found".
 orphanID:="call_03541687963626754fe4b9e1"
 oldPatch:=agentchat.HistoryItem{Type:"function_call",CallID:orphanID,
  Name:"taskdeck_submit_patch",Arguments:`{"name":"old-unfinished","patch_python":"print('never confirmed')"}`}
 history:=[]agentchat.HistoryItem{
  {Type:"user",Content:"Original task"},
  {Type:"user",Content:"Continue"},
  {Type:"user",Content:"Continue"},
  oldPatch,
  {Type:"user",Content:"Prior attempt interrupted; continue without reapplying"},
 }
 for i:=0;i<38;i++{
  id:=fmt.Sprintf("collect-%d",i)
  history=append(history,
   agentchat.HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"search","query":"FBB"}]}`},
   agentchat.HistoryItem{Type:"function_call_output",CallID:id,Content:`{"status":"INCOMPLETE","partial_evidence":true}`})
 }
 if len(history)!=81{t.Fatalf("fixture must reflect real 81-item context: %d",len(history))}
 origArgs:=history[3].Arguments
 next:=[]agentchat.HistoryItem{{Type:"user",Content:"Continue current task"}}
 got,stats:=Build(history,Policy{MaxInputTokens:65536,Next:next})
 all:=fmt.Sprint(got)
 if strings.Contains(all,oldPatch.Arguments){t.Fatal("orphan Patch source replayed to model")}
 if !strings.Contains(all,orphanID)||!strings.Contains(all,"UNRESOLVED historical tool call")||
  !strings.Contains(all,"Execution status UNKNOWN"){
  t.Fatalf("orphan Patch was silently discarded or falsely marked executed: %s",all)
 }
 for _,item:=range got{if item.Type=="function_call"&&item.CallID==orphanID{
  t.Fatal("orphan function_call reached provider input; would trigger HTTP 400")
 }}
 if stats.BudgetExceeded{t.Fatalf("context compaction failed: %+v",stats)}
 if history[3].Arguments!=origArgs||history[3].CallID!=orphanID{
  t.Fatal("canonical audit history was mutated during context repair")
 }
}
func TestOrphanOutputCompactedAndCurrentFeedbackRemainsExact(t *testing.T){
 historical:=[]agentchat.HistoryItem{
  {Type:"user",Content:"test"},
  {Type:"function_call_output",CallID:"old-output",Content:`{"status":"PASS"}`},
  {Type:"user",Content:"continue"},
  {Type:"function_call",CallID:"pending-patch",Name:"taskdeck_submit_patch",
   Arguments:`{"name":"new patch","patch_python":"print('ok')"}`},
 }
 next:=[]agentchat.HistoryItem{{Type:"function_call_output",CallID:"pending-patch",Content:`{"status":"FAIL","rc":3}`}}
 got,_:=Build(historical,Policy{MaxInputTokens:32768,Next:next})
 visible:=fmt.Sprint(got)
 if !strings.Contains(visible,"UNPAIRED historical tool feedback"){
  t.Fatalf("orphan output not catalogued: %s",visible)
 }
 for _,v:=range got {
  if v.Type=="function_call_output"&&v.CallID=="old-output"{t.Fatal("orphan output reached provider")}
 }
 if got[len(got)-1].Type!="function_call"||got[len(got)-1].CallID!="pending-patch"{
  t.Fatal("fresh pending Patch call was lost; next verified output cannot pair")
 }
 if next[0].Content!=`{"status":"FAIL","rc":3}`{t.Fatal("new verified Patch feedback modified")}
}

func TestRecentCatalogRetainedAcrossRecompaction(t *testing.T){
 history:=[]agentchat.HistoryItem{{Type:"user",Content:"Finish inventory"}}
 for i:=0;i<45;i++{
  id:=fmt.Sprintf("read-%d",i)
  history=append(history,agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:id,Arguments:fmt.Sprintf(`{"actions":[{"type":"range","path":"file-%d.go","start_line":10,"end_line":20}]}`,i)},agentchat.HistoryItem{Type:"function_call_output",CallID:id,Content:fmt.Sprintf(`{"status":"FAIL","run_id":"native-%d","result_text":"artifact-%d.txt","error":{"code":"COLLECT_SCHEMA_INVALID","message":"invalid field"},"padding":%q}`,i,i,strings.Repeat("x",50000))})
 }
 first,_:=Build(history,Policy{MaxInputTokens:32768})
 text:=fmt.Sprint(first)
 for _,want:=range []string{"file-44.go","native-44","artifact-44.txt","COLLECT_SCHEMA_INVALID","lines=10-20"}{
  if !strings.Contains(text,want){t.Fatalf("recent evidence missing %s",want)}
 }
 second,_:=Build(first,Policy{MaxInputTokens:24000})
 if !strings.Contains(fmt.Sprint(second),"native-44"){t.Fatal("recent catalog lost during retry compaction")}
 if strings.Contains(fmt.Sprint(second),strings.Repeat("x",100)){t.Fatal("full old result replayed")}
}

func TestPriorityCatalogPreservesPatchAndUnknownAfterLongCollectionAndRetry(t *testing.T) {
 history:=[]agentchat.HistoryItem{
  {Type:"user",Content:"Original objective"},
  {Type:"function_call",CallID:"old-patch",Name:"taskdeck_submit_patch",Arguments:`{"name":"Fix safety","patch_python":"print('private source')"}`},
  {Type:"function_call_output",CallID:"old-patch",Content:`{"status":"PASS","run_id":"native-old"}`},
  {Type:"function_call",CallID:"orphan-patch",Name:"taskdeck_submit_patch",Arguments:`{"name":"Unconfirmed","patch_python":"print('never replay')"}`},
 }
 for i:=0;i<70;i++{
  id:=fmt.Sprintf("collection-%d",i)
  history=append(history,agentchat.HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_collect_workspace",Arguments:fmt.Sprintf(`{"actions":[{"type":"read","path":"file-%d.go"}]}`,i)},agentchat.HistoryItem{Type:"function_call_output",CallID:id,Content:strings.Repeat("source",8000)})
 }
 before:=fmt.Sprint(history)
 for _,budget:=range []int{32768,18000,10000}{
  visible,stats:=Build(history,Policy{MaxInputTokens:budget})
  for i:=0;i<2;i++{
   joined:=fmt.Sprint(visible)
   for _,want:=range []string{"Original objective","Prior Native Patch","native-old","orphan-patch","Execution status UNKNOWN","file-69.go"}{
    if !strings.Contains(joined,want){t.Fatalf("budget=%d recompaction=%d lost %q",budget,i,want)}
   }
   if strings.Contains(joined,"private source")||strings.Contains(joined,"never replay")||strings.Contains(joined,strings.Repeat("source",100)){t.Fatal("historical source leaked")}
   if stats.BudgetExceeded{t.Fatalf("budget=%d: %+v",budget,stats)}
   visible,stats=Build(visible,Policy{MaxInputTokens:budget})
  }
 }
 if fmt.Sprint(history)!=before{t.Fatal("canonical history mutated")}
}


func TestCurrentTextFallbackReceiptsDoNotConsumeHumanInstructionBudget(t *testing.T){
 history:=[]agentchat.HistoryItem{{Type:"user",Content:"Original owner goal"}}
 for i:=0;i<20;i++{
  history=append(history,agentchat.HistoryItem{Type:"assistant",Content:`{"project":{"key":"p","id":"id"},"actions":[{"type":"file","path":"a"}]}`},agentchat.HistoryItem{Type:"user",Content:agentchat.NativeCollectionTextObservationPrefix+fmt.Sprintf(`{"status":"INCOMPLETE","text":%q}`,strings.Repeat("large-receipt-",3000))})
 }
 history=append(history,agentchat.HistoryItem{Type:"user",Content:"Keep my latest instruction"})
 next:=agentchat.HistoryItem{Type:"user",Content:agentchat.NativeCollectionTextObservationPrefix+`{"status":"PASS","marker":"current receipt"}`}
 visible,stats:=Build(history,Policy{MaxInputTokens:24000,Next:[]agentchat.HistoryItem{next}})
 got:=fmt.Sprint(visible)
 if stats.BudgetExceeded||stats.SummarizedCollections!=20||strings.Contains(got,"large-receipt-")||!strings.Contains(got,"Original owner goal")||!strings.Contains(got,"Keep my latest instruction"){t.Fatalf("bad fallback projection: %+v",stats)}
 if !strings.Contains(next.Content,"current receipt"){t.Fatal("current receipt changed")}
}
