package agentchat

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestUnlimitedCollectionStoreArchivesOlderEvidenceAndKeepsOriginalGoal(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private","sessions")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/example/project","project-uuid","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.History=append(x.History,HistoryItem{Type:"user",Content:"Original user goal: inspect the code and complete the task"})
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 for n:=0;n<220;n++{
  id:=fmt.Sprintf("collect-%04d",n)
  x.History=append(x.History,
   HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"ls","path":"."}]}`},
   HistoryItem{Type:"function_call_output",CallID:id,Content:strings.Repeat("Verified read-only source observation Xin chào\n",160)})
  x,err=store.Save(x,x.Revision)
  if err!=nil{t.Fatalf("Collection #%d failed before completion: %v",n,err)}
 }
 if x.HistoryArchiveParts<2 || x.ArchivedHistoryItems<200 {
  t.Fatalf("unbounded read history was not checkpointed: parts=%d archived=%d",x.HistoryArchiveParts,x.ArchivedHistoryItems)
 }
 if len(x.History)>=archiveHistoryThreshold {t.Fatalf("live session grew without bound: %d",len(x.History))}
 if x.History[0].Type!="user" || !strings.Contains(x.History[0].Content,"Original user goal"){
  t.Fatal("initial task instructions were lost during archival")
 }
 latest:=x.History[len(x.History)-1]
 if latest.Type!="function_call_output" || latest.CallID!="collect-0219" {t.Fatal("latest verified evidence was lost")}
 opened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 got,err:=opened.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if got.ArchivedHistoryItems!=x.ArchivedHistoryItems || got.HistoryArchiveParts!=x.HistoryArchiveParts{
  t.Fatal("archive checkpoint metadata was not durable after reopening")
 }
 total:=0
 for part:=uint64(1);part<=got.HistoryArchiveParts;part++{
  items,e:=opened.ArchivedCollectionHistory(x.ID,"local",part)
  if e!=nil{t.Fatalf("cannot load verified historical part %d: %v",part,e)}
  total+=len(items)
  if _,e:=opened.ArchivedCollectionHistory(x.ID,"another-owner",part);!errors.Is(e,ErrNotFound){
   t.Fatal("historical source leaked across owners")
  }
 }
 if uint64(total)!=got.ArchivedHistoryItems {t.Fatalf("evidence count mismatch: %d vs %d",total,got.ArchivedHistoryItems)}
 if err:=opened.Delete(x.ID,"local",got.Revision);err!=nil{t.Fatal(err)}
 for part:=uint64(1);part<=got.HistoryArchiveParts;part++{
  if _,err:=os.Stat(opened.historyArchivePath(x.ID,part));!os.IsNotExist(err){
   t.Fatalf("deleting session retained private archive %d: %v",part,err)
  }
 }
}

func TestCollectionHistoryArchiveNeverSeparatesOrDropsPatchEvidence(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/example","project-uuid","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 x.History=append(x.History,HistoryItem{Type:"user",Content:"Keep my original task"})
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 for n:=0;n<145;n++{
  id:=fmt.Sprintf("read-%d",n)
  x.History=append(x.History,
    HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_collect_workspace",Arguments:"{}"},
    HistoryItem{Type:"function_call_output",CallID:id,Content:"read result"})
  if n==65{
   x.History=append(x.History,HistoryItem{Type:"function_call",CallID:"patch-1",Name:"taskdeck_submit_patch",Arguments:"{}"},
    HistoryItem{Type:"function_call_output",CallID:"patch-1",Content:`{"status":"PASS","rc":0}`})
   x.ToolCalls=append(x.ToolCalls,ToolCallRecord{CallID:"patch-1",Name:"taskdeck_submit_patch",
    Status:"done",Result:`{"status":"PASS","rc":0}`,FeedbackSent:true,FeedbackStatus:"acknowledged"})
  }
  x,err=store.Save(x,x.Revision)
  if err!=nil{t.Fatalf("failed at n=%d: %v",n,err)}
 }
 var call,output bool
 for _,it:=range x.History{
  if it.CallID=="patch-1"{
   if it.Type=="function_call"{call=true}
   if it.Type=="function_call_output"{output=true}
  }
 }
 if !call||!output||len(x.ToolCalls)!=1{t.Fatal("archiving altered a native Patch approval/result pair")}
}
