package agentchat

import (
 "errors"
 "fmt"
 "os"
 "strings"
 "testing"
)

func TestPatchReceiptsOverThirtyTwoRemainDurableWithoutReexecution(t *testing.T){
 dir:=t.TempDir()
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace/test","project-uuid","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.History=[]HistoryItem{{Type:"user",Content:"Original task: implement many independent patches"}}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 for i:=0;i<125;i++{
  id:=fmt.Sprintf("patch-%03d",i)
  src:=fmt.Sprintf(`{"name":"Step-%03d","patch_python":"print('ok')"}`,i)
  x.History=append(x.History,
   HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_submit_patch",Arguments:src},
   HistoryItem{Type:"function_call_output",CallID:id,Content:`{"status":"PASS","rc":0}`})
  x.ToolCalls=append(x.ToolCalls,ToolCallRecord{
   CallID:id,Name:"taskdeck_submit_patch",Status:"done",
   Result:`{"status":"PASS","rc":0}`,FeedbackSent:true,FeedbackStatus:"acknowledged",
  })
  x,err=store.Save(x,x.Revision)
  if err!=nil{t.Fatalf("Patch #%d failed to save: %v",i,err)}
 }
 if len(x.ToolCalls)>32||x.ArchivedPatchCalls<90||x.PatchArchiveParts==0{
  t.Fatalf("completed Patch receipts are not segmented: live=%d archived=%d parts=%d",len(x.ToolCalls),x.ArchivedPatchCalls,x.PatchArchiveParts)
 }
 if x.History[0].Content!="Original task: implement many independent patches"{t.Fatal("original goal lost")}
 fresh,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 y,err:=fresh.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if y.ArchivedPatchCalls!=x.ArchivedPatchCalls{t.Fatal("Patch archival pointer not durable after restart")}
 count:=0
 for part:=uint64(1);part<=y.PatchArchiveParts;part++{
  entries,e:=fresh.ArchivedPatchHistory(y.ID,"local",part)
  if e!=nil{t.Fatalf("archive part %d unavailable: %v",part,e)}
  for _,entry:=range entries{
   if entry.Call.Status!="done"||entry.Call.FeedbackStatus!="acknowledged"||len(entry.History)!=2{
    t.Fatalf("lost final Patch evidence: %+v",entry)
   }
  }
  count+=len(entries)
 }
 if uint64(count)!=y.ArchivedPatchCalls{t.Fatal("archive receipts not accounted for")}
 if _,err:=fresh.ArchivedPatchHistory(y.ID,"other-user",1);!errors.Is(err,ErrNotFound){
  t.Fatal("cross-owner patch archive access")
 }
 if _,err:=fresh.Save(x,x.Revision-1);!errors.Is(err,ErrStaleRevision){t.Fatal("stale CAS allowed")}
 if err:=fresh.Delete(y.ID,"local",y.Revision);err!=nil{t.Fatal(err)}
 for part:=uint64(1);part<=y.PatchArchiveParts;part++{
  if _,err:=os.Stat(fresh.patchArchivePath(y.ID,part));!os.IsNotExist(err){
   t.Fatalf("deleted conversation retained Patch archive part %d: %v",part,err)
  }
 }
}
func TestUnresolvedPatchIntentIsNotArchived(t *testing.T){
 store,err:=NewStore(t.TempDir());if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace/test","project-uuid","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.History=append(x.History,HistoryItem{Type:"user",Content:"Original plan"})
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 for i:=0;i<28;i++{
  id:=fmt.Sprintf("call-%02d",i)
  x.History=append(x.History,HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_submit_patch",Arguments:`{"name":"test"}`})
  x.ToolCalls=append(x.ToolCalls,ToolCallRecord{CallID:id,Name:"taskdeck_submit_patch",Status:"unknown"})
 }
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 if x.ArchivedPatchCalls!=0||len(x.ToolCalls)!=28 {t.Fatal("unconfirmed Patch intent was discarded")}
 if !strings.Contains(x.History[0].Content,"Original plan"){t.Fatal("goal removed")}
}
