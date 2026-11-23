package agentchat

import (
 "bytes"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

// This fixture deliberately triggers both existing archive compaction paths
// before executing a single-page done mutation. No hand-made archive proofs
// or unverified model narrative are used as acceptance evidence.
func archivedSegmentDoneFixture(t *testing.T)(*Store,Session){
 t.Helper()
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Long-lived verified historical evidence",planStepFixtures(1024))
 if err!=nil{t.Fatal(err)}
 x.History=append(x.History,HistoryItem{Type:"user",Content:"Original owner task goal"})
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 for i:=0;i<30;i++{
  id:=fmt.Sprintf("archive-patch-%d",i)
  x.History=append(x.History,
   HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_submit_patch",Arguments:`{"name":"Prior verified patch"}`},
   HistoryItem{Type:"function_call_output",CallID:id,Content:`{"status":"PASS","rc":0}`})
  x.ToolCalls=append(x.ToolCalls,ToolCallRecord{
   CallID:id,Name:"taskdeck_submit_patch",Status:"done",
   Result:`{"status":"PASS","rc":0}`,FeedbackSent:true,FeedbackStatus:"acknowledged",
  })
 }
 for i:=0;i<160;i++{
  id:=fmt.Sprintf("archive-collect-%d",i)
  x.History=append(x.History,
   HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_collect_workspace",
    Arguments:`{"actions":[{"type":"read","path":"src/main.go"}]}`},
   HistoryItem{Type:"function_call_output",CallID:id,
    Content:`{"results":[{"files":[{"path":"src/main.go"}]}]}`})
 }
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatalf("archive compaction save: %v",err)}
 if x.HistoryArchiveParts==0||x.PatchArchiveParts==0||
  len(x.TaskPlanSegments)!=32{
  t.Fatalf("did not stage 1024-step plan plus both archives: segments=%d patch=%d history=%d",
   len(x.TaskPlanSegments),x.PatchArchiveParts,x.HistoryArchiveParts)
 }
 if _,err=VerifyTaskPlanEvidence(x,"patch","archive-patch-0","note");err==nil{
  t.Fatal("old patch must be archived, not active")
 }
 if _,err=VerifyTaskPlanEvidence(x,"collection","archive-collect-0","note");err==nil{
  t.Fatal("old collection must be archived, not active")
 }
 return store,x
}

func TestSegmentDoneRechecksArchivedPatchAndCollectionWithoutFullPlanRewrite(t *testing.T){
 store,x:=archivedSegmentDoneFixture(t)
 prior:=append([]TaskPlanSegmentRef(nil),x.TaskPlanSegments...)
 for _,tc:=range []struct{step,kind,ref string;index int}{
  {"step-0501","patch","archive-patch-0",501},
  {"step-0875","collection","archive-collect-0",875},
 }{
  page,err:=store.SetSegmentTaskPlanStepDone(x.ID,"local",x.Revision,
   tc.step,tc.kind,tc.ref,"Owner inspected immutable receipt",tc.index-5,24)
  if err!=nil{t.Fatalf("archived %s proof refused: %v",tc.kind,err)}
  if page.Revision!=x.Revision+1||page.Done<1||page.Total!=1024||
   page.Plan==nil||page.Plan.State!="active"||page.AllDone||page.Completed{
   t.Fatalf("bad page after archive %s completion: %+v",tc.kind,page)
  }
  got,err:=store.Get(x.ID,"local")
  if err!=nil{t.Fatal(err)}
  step:=got.TaskPlan.Steps[tc.index]
  if step.Status!="done"||len(step.Evidence)!=1||
   step.Evidence[0].Kind!=tc.kind||step.Evidence[0].Ref!=tc.ref||
   len(step.Evidence[0].Digest)!=64{
   t.Fatalf("archived %s not securely attached to step: %+v",tc.kind,step)
  }
  for i,ref:=range got.TaskPlanSegments{
   if i==tc.index/taskPlanSegmentSize{
    if ref==prior[i]||ref.Revision!=page.Revision{
     t.Fatal("changed archive-evidenced milestone page was not rewritten")
    }
   }else if ref!=prior[i]{
    t.Fatalf("unrelated page %d changed on archive completion",i)
   }
  }
  prior=append([]TaskPlanSegmentRef(nil),got.TaskPlanSegments...)
  x=got
 }
 if x.TaskPlan.Steps[501].Evidence[0].Kind!="patch"||
  x.TaskPlan.Steps[875].Evidence[0].Kind!="collection"{
  t.Fatal("subsequent archived Collection completion lost previous Patch proof")
 }
 for _,tc:=range []struct{owner string;revision uint64;want error}{
  {"other-owner",x.Revision,ErrNotFound},
  {"local",x.Revision-1,ErrStaleRevision},
 }{
  _,err:=store.SetSegmentTaskPlanStepDone(x.ID,tc.owner,tc.revision,
   "step-0880","collection","archive-collect-0","Owner note",864,24)
  if !errors.Is(err,tc.want){t.Fatalf("invalid owner/revision admitted: %v",err)}
 }
 handoff,rev,err:=store.Handoff(x.ID,"local")
 if err!=nil||rev!=x.Revision||!strings.Contains(handoff,"2/1024"){
  t.Fatalf("handoff lost archived proof progress: %v",err)
 }
}

func TestSegmentDoneRejectsForgedOrTamperedArchiveBeforeCommit(t *testing.T){
 store,x:=archivedSegmentDoneFixture(t)
 before,err:=os.ReadFile(store.file(x.ID))
 if err!=nil{t.Fatal(err)}
 changed:=func() {
  t.Helper()
  now,err:=os.ReadFile(store.file(x.ID))
  if err!=nil{t.Fatal(err)}
  if !bytes.Equal(before,now){
   t.Fatal("bad archive evidence modified authoritative journal")
  }
 }
 for _,tc:=range []struct{kind,ref string}{
  {"patch","invented-patch"},
  {"collection","invented-collection"},
 }{
  _,err:=store.SetSegmentTaskPlanStepDone(x.ID,"local",x.Revision,
   "step-0501",tc.kind,tc.ref,"Owner note",480,24)
  if !errors.Is(err,ErrTaskPlanCompletionProofRejected){
   t.Fatalf("forged %s proof admitted/fallbacked: %v",tc.kind,err)
  }
  changed()
 }
 for _,tc:=range []struct{kind,ref,path string}{
  {"patch","archive-patch-0",store.patchArchivePath(x.ID,1)},
  {"collection","archive-collect-0",store.historyArchivePath(x.ID,1)},
 }{
  saved,err:=os.ReadFile(tc.path)
  if err!=nil{t.Fatal(err)}
  if err=os.WriteFile(tc.path,[]byte(`{"format":"taskdeck-agent-forged","entries":[]}`),0600);err!=nil{
   t.Fatal(err)
  }
  _,err=store.SetSegmentTaskPlanStepDone(x.ID,"local",x.Revision,
   "step-0501",tc.kind,tc.ref,"Owner note",480,24)
  if !errors.Is(err,ErrTaskPlanCompletionProofRejected){
   t.Fatalf("tampered %s archive became proof: %v",tc.kind,err)
  }
  changed()
  if err=os.WriteFile(tc.path,saved,0600);err!=nil{t.Fatal(err)}
 }
 // Preserve the predeclared suite requirement: no archived Patch/Collection
 // can downgrade a user-bound exact named Go-test checklist.
 now:=x.TaskPlan.CreatedAt
 x.TaskPlan.Steps[501].TaskCommandGuard=&TaskCommandGuard{
  TaskID:1,CommandSHA256:strings.Repeat("a",64),BoundAt:now,
 }
 x.TaskPlan.Steps[501].GoTestRequirements=&TaskGoTestRequirements{
  Package:"example/pkg",Tests:[]string{"TestExact"},BoundAt:now,
 }
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 _,err=store.SetSegmentTaskPlanStepDone(x.ID,"local",x.Revision,
  "step-0501","patch","archive-patch-0","Owner note",480,24)
 if !errors.Is(err,ErrTaskPlanSegmentWriteUnsupported){
  t.Fatalf("archived Patch bypassed required Go suite: %v",err)
 }
}
