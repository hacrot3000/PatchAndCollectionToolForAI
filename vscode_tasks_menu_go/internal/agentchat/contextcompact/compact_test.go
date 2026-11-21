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
