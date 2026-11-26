package agentchat

import (
 "encoding/json"
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


func TestCurrentTextCollectionFallbackArchivesWithoutLosingHumanMessages(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/project","id","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 x.History=[]HistoryItem{{Type:"user",Content:"Original task"}}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 for i:=0;i<140;i++{
  x.History=append(x.History,HistoryItem{Type:"assistant",Content:`{"project":{"key":"p","id":"id"},"actions":[{"type":"file","path":"a"}]}`},HistoryItem{Type:"user",Content:NativeCollectionTextObservationPrefix+`{"status":"INCOMPLETE","sequence":`+fmt.Sprint(i)+`}`})
  if i==5{x.History=append(x.History,HistoryItem{Type:"user",Content:"A real owner instruction: preserve this"})}
  x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 }
 if x.HistoryArchiveParts==0{t.Fatal("text observations were not archived")}
 total:=0
 for _,it:=range x.History{if strings.HasPrefix(it.Content,NativeCollectionTextObservationPrefix){total++}}
 for i:=uint64(1);i<=x.HistoryArchiveParts;i++{
  items,err:=store.ArchivedCollectionHistory(x.ID,x.Owner,i);if err!=nil{t.Fatal(err)}
  for _,it:=range items{if strings.HasPrefix(it.Content,NativeCollectionTextObservationPrefix){total++}}
 }
 if total!=140||!strings.Contains(fmt.Sprint(x.History),"A real owner instruction: preserve this"){t.Fatal("fallback receipt or owner goal lost")}
}


// The old prefix contained 137 scheduler messages and Patch pairs, but
// no archivable Collection pairs. Adding the 257th item must not fail.
func TestArchiveStandaloneContinuationsAtLegacyHistoryLimit(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/project","id","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 x.History=[]HistoryItem{{Type:"user",Content:"Original owner goal"}}
 template:=func(i int)string{return fmt.Sprintf("TaskDeck user-authorized continuation of the ORIGINAL unfinished conversation, NOT a new task. The previous provider turn is NEEDS_CONTINUATION (advisory only). Its proposed next step is %q. Verify this against the ORIGINAL user goal and actual native receipts; inspect the claimed checkpoint with read-only Collection before relying on it. Do not repeat a generic progress report. Do the next feasible concrete action and report actual evidence. Never claim success without independent proof; never bypass native Patch approval or reissue an ambiguous tool request.",fmt.Sprintf("step %d",i))}
 generated:=map[string]int{}
 for i:=0;i<137;i++{msg:=template(i);generated[msg]++;x.History=append(x.History,HistoryItem{Type:"user",Content:msg})}
 x.History=append(x.History,HistoryItem{Type:"user",Content:"Owner instruction: do not enable pending dispatch"})
 for i:=0;i<11;i++{
  id:=fmt.Sprintf("patch-%d",i)
  x.History=append(x.History,HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:id,Arguments:"{}"},HistoryItem{Type:"function_call_output",CallID:id,Content:"PASS"})
  x.ToolCalls=append(x.ToolCalls,ToolCallRecord{Name:"taskdeck_submit_patch",CallID:id,Status:"done",Result:"PASS",FeedbackSent:true,FeedbackStatus:"acknowledged"})
 }
 for len(x.History)<255{x.History=append(x.History,HistoryItem{Type:"assistant",Content:"recent update"})}
 x.History=append(x.History,HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"pending-read",Arguments:"{}"})
 x.Status="requesting_ai"
 x.Revision=1
 if err:=validateSession(x);err!=nil{t.Fatal(err)}
 raw,err:=json.Marshal(x);if err!=nil{t.Fatal(err)}
 // Seed an old valid session directly, before new archival takes effect.
 if err=store.withLock(func() error { return os.WriteFile(store.file(x.ID),raw,0600) });err!=nil{t.Fatal(err)}
 x,err=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if len(x.History)!=256{t.Fatal("old saturation fixture not reproduced")}
 x.History=append(x.History,HistoryItem{Type:"function_call_output",CallID:"pending-read",Content:"verified native receipt"})
 x.AppendEvent("collect_observed","requesting_ai","pending-read","")
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatalf("257th item failed: %v",err)}
 if len(x.History)>=256||x.HistoryArchiveParts==0||len(x.ToolCalls)!=11{t.Fatal("history capacity or Patch commitments lost")}
 if x.History[0].Content!="Original owner goal"||!strings.Contains(fmt.Sprint(x.History),"Owner instruction: do not enable pending dispatch"){t.Fatal("owner goal lost")}
 if x.History[len(x.History)-1].Content!="verified native receipt"{t.Fatal("current receipt lost")}
 for _,it:=range x.History{if _,ok:=generated[it.Content];ok{generated[it.Content]--}}
 for part:=uint64(1);part<=x.HistoryArchiveParts;part++{
  rows,e:=store.ArchivedCollectionHistory(x.ID,"local",part);if e!=nil{t.Fatal(e)}
  for _,it:=range rows{if _,ok:=generated[it.Content];ok{generated[it.Content]--}}
 }
 for msg,n:=range generated{if n!=0{t.Fatalf("generated message not preserved exactly once: %d %s",n,msg)}}
 for i:=137;i<450;i++{
  x.History=append(x.History,HistoryItem{Type:"user",Content:template(i)})
  x,err=store.Save(x,x.Revision);if err!=nil{t.Fatalf("continuation %d: %v",i,err)}
 }
}

func TestHistoryArchiveRetainsFullHistoryBelowActualLimits(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/project","id","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 x.History=[]HistoryItem{{Type:"user",Content:"Original goal"}}
 for i:=0;i<85;i++{
  id:=fmt.Sprintf("read-%d",i)
  x.History=append(x.History,HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:id,Arguments:"{}"},HistoryItem{Type:"function_call_output",CallID:id,Content:strings.Repeat("verified source ",1400)})
 }
 expected:=len(x.History)
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 if len(x.History)!=expected||x.HistoryArchiveParts!=0{t.Fatalf("premature archive: %d/%d parts=%d",len(x.History),expected,x.HistoryArchiveParts)}
}
