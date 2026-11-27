package agentchat

import (
 "encoding/json"
 "fmt"
 "os"
 "errors"
 "path/filepath"
 "reflect"
 "strings"
 "testing"

 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
)

func TestCollectionGuardSessionCASAndRestart(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private")
 s,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 x.History=[]HistoryItem{{Type:"user",Content:"old objective"},
  {Type:"function_call",CallID:"old-read",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"overview"}]}`},
  {Type:"function_call_output",CallID:"old-read",Content:`{"status":"PASS"}`}}
 x,err=s.Save(x,0);if err!=nil{t.Fatal(err)}
 original:=append([]HistoryItem(nil),x.History...)
 if !x.BindCollectionGuard("Fix the issue","",false){t.Fatal("bind")}
 g,ok:=x.ReadCollectionGuard();if !ok||g.Legacy.RetainedCalls!=1||g.Legacy.RetainedOutputs!=1||g.Counters.Observed!=0{t.Fatalf("bad migration: %+v",g)}
 x.AppendEvent("clp_requested",x.Status,"new-read","")
 event:=cp.CounterEvent{Sequence:x.EventSeq,Identity:guardDigest("new-read"),Kind:cp.CollectionRequested}
 g.Counters,_,_,err=cp.ApplyCounterEvent(g.Policy,g.Counters,event,true);if err!=nil{t.Fatal(err)}
 if !x.SetCollectionGuard(g){t.Fatal("guard update")}
 saved,err:=s.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if _,err=s.Save(x,x.Revision);!errors.Is(err,ErrStaleRevision){t.Fatalf("stale CAS: %v",err)}
 reopened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 got,err:=reopened.Get(x.ID,x.Owner);if err!=nil{t.Fatal(err)}
 after,ok:=got.ReadCollectionGuard()
 if !ok||!reflect.DeepEqual(g,after)||!reflect.DeepEqual(original,got.History)||got.Revision!=saved.Revision{t.Fatal("guard or canonical history lost after reopen")}
 if !got.BindCollectionGuard("","",true)||!got.BindCollectionGuard("Fix the issue","",false){t.Fatal("continuation failed")}
 same,_:=got.ReadCollectionGuard();if same.Generation!=g.Generation||same.Counters.Requested!=1{t.Fatal("continuation reset")}
 if !got.BindCollectionGuard("Fix the issue","step-2",false){t.Fatal("new step")}
 next,_:=got.ReadCollectionGuard();if next.Generation!=g.Generation+1||next.Counters.Requested!=0||next.Legacy!=g.Legacy{t.Fatal("step not isolated")}
}

func TestCollectionGuardOpaqueFutureOrInvalidDoesNotBlockCanonicalHistory(t *testing.T){
 s,err:=NewStore(filepath.Join(t.TempDir(),"private"));if err!=nil{t.Fatal(err)}
 for _,raw:=range []string{`{"version":999,"opaque":"preserve"}`,`{"version":1,"counters":"invalid"}`}{
  x,err:=NewSession("local","/workspace","project","provider","model",Stateless);if err!=nil{t.Fatal(err)}
  x.CollectionGuard=json.RawMessage(raw)
  x.History=[]HistoryItem{{Type:"user",Content:"original"}}
  x,err=s.Save(x,0);if err!=nil{t.Fatal(err)}
  if _,ok:=x.ReadCollectionGuard();ok||x.BindCollectionGuard("Fix the issue","",false){t.Fatal("opaque guard accepted or overwritten")}
  x.History=append(x.History,HistoryItem{Type:"user",Content:"continue"})
  x,err=s.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
  got,err:=s.Get(x.ID,x.Owner);if err!=nil||string(got.CollectionGuard)!=raw||len(got.History)!=2{t.Fatalf("canonical session unavailable: %v",err)}
 }
}


func TestCollectionGuardPatchTransitionsShareSessionCASWithoutFalseProgress(t *testing.T){
 s,err:=NewStore(filepath.Join(t.TempDir(),"private"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 if !x.BindCollectionGuard("Fix the issue","",false){t.Fatal("bind")}
 g,_:=x.ReadCollectionGuard()
 scope:=cp.ScopeDescriptor{ObjectiveKey:g.Counters.Objective.Key,RequestDigest:strings.Repeat("a",64)}
 proof:=cp.Evidence{Status:cp.EvidenceComplete,ProofDigest:strings.Repeat("b",64)}
 g.Evidence,_=cp.ObserveEvidence(g.Evidence,scope,proof)
 x.AppendEvent("clp_collection_observed","created","old-read","")
 g.Counters,_,_,err=cp.ApplyCounterEvent(g.Policy,g.Counters,cp.CounterEvent{Sequence:x.EventSeq,Identity:guardDigest("old-read"),Kind:cp.CollectionObserved,Novelty:cp.VerifiedNew,NativeRead:true},true)
 if err!=nil||!x.SetCollectionGuard(g){t.Fatal("seed guard")}
 x,err=s.Save(x,0);if err!=nil{t.Fatal(err)}
 x.History=append(x.History,HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"patch",Arguments:`{"name":"fixture"}`})
 x.ToolCalls=append(x.ToolCalls,ToolCallRecord{CallID:"patch",Name:"taskdeck_submit_patch",Status:"pending"})
 x,err=s.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 g,_=x.ReadCollectionGuard()
 if g.Counters.PatchProposals!=1||len(g.Evidence.Entries)!=1||g.Counters.Progress.ReadsSinceProgress!=1{t.Fatal("proposal fabricated progress or mutation")}
 x.ToolCalls[0].Status="running";x.ToolCalls[0].PatchSessionID="native-session";x.ToolCalls[0].RunID="native-run"
 x,err=s.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 g,_=x.ReadCollectionGuard()
 if len(g.Evidence.Entries)!=0||!g.Evidence.Evicted||g.Counters.PatchExecutions!=0{t.Fatal("possible mutation not invalidated or premature execution count")}
 x.ToolCalls[0].Status="done";x.ToolCalls[0].Result=`{"status":"PASS","run_id":"native-run","project_unchanged":false}`
 x,err=s.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 g,_=x.ReadCollectionGuard()
 if g.Counters.PatchExecutions!=1||g.Counters.VerifiedProgressEvents!=0||g.Counters.Progress.ReadsSinceProgress!=1{t.Fatalf("wrapper PASS treated as meaningful progress: %+v",g)}
 x.ToolCalls[0].FeedbackSent=true;x.ToolCalls[0].FeedbackStatus="acknowledged"
 x,err=s.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 g,_=x.ReadCollectionGuard()
 if g.Counters.PatchExecutions!=1||x.ToolCalls[0].FeedbackStatus!="acknowledged"{t.Fatal("ACK double-counted execution or changed feedback")}
}


func TestCollectionGuardHydratesRetainedTextPairsWithoutInventingNovelty(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"private")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 x.History=[]HistoryItem{
  {Type:"user",Content:"Old owner goal"},
  {Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"typed",Arguments:`{"actions":[{"type":"file","path":"a"}]}`},
  {Type:"function_call_output",CallID:"typed",Content:`{"status":"PASS"}`},
  {Type:"assistant",Content:`{"project":{"key":"p","id":"id"},"actions":[{"type":"file","path":"b"}]}`},
  {Type:"user",Content:NativeCollectionTextObservationPrefix+`{"status":"INCOMPLETE"}`},
  {Type:"user",Content:NativeCollectionTextObservationPrefix+`{"status":"PASS"}`},
  {Type:"user",Content:legacyCollectionTextObservationPrefix+" old format"},
 }
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 original:=append([]HistoryItem(nil),x.History...)
 if !x.BindCollectionGuard("Continue","",true){t.Fatal("hydrate")}
 g,ok:=x.ReadCollectionGuard()
 if !ok||g.Legacy.RetainedCalls!=2||g.Legacy.RetainedOutputs!=2||!g.Legacy.Incomplete||
  g.Counters.NativeReads!=0||g.Counters.NewEvidence!=0||g.Counters.DuplicateEvidence!=0||len(g.Evidence.Entries)!=0{
  t.Fatalf("historical retained counts confused with evidence: %+v",g)
 }
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 reopened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 got,err:=reopened.Get(x.ID,x.Owner);if err!=nil{t.Fatal(err)}
 if !got.BindCollectionGuard("","",true){t.Fatal("continue")}
 after,ok:=got.ReadCollectionGuard()
 if !ok||!reflect.DeepEqual(g,after)||!reflect.DeepEqual(original,got.History){t.Fatal("hydration reran or rewrote canonical history")}
 // A new explicit goal must preserve the historical lower-bound counts
 // without merging them into the new objective's current counters.
 if !got.BindCollectionGuard("Fix the issue","",false){t.Fatal("new objective")}
 fresh,ok:=got.ReadCollectionGuard()
 if !ok||fresh.Generation!=g.Generation+1||fresh.Legacy!=g.Legacy||fresh.Counters.Observed!=0{t.Fatal("historical counts reset or attributed to new goal")}
}

func TestCollectionGuardArchiveMigrationBoundedCursorAndIsolation(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 x.History=[]HistoryItem{{Type:"user",Content:"Original owner task"}}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 // Generate actual private immutable archive parts through the normal CAS.
 for batch:=0;batch<8;batch++{
  for i:=0;i<100;i++{
   id:=fmt.Sprintf("old-read-%d-%d",batch,i)
   x.History=append(x.History,HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:id,Arguments:`{"actions":[{"type":"ls","path":"."}]}`},HistoryItem{Type:"function_call_output",CallID:id,Content:`{"status":"PASS"}`})
  }
  x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 }
 for i:=0;i<28;i++{
  id:=fmt.Sprintf("old-patch-%d",i)
  x.History=append(x.History,HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:id,Arguments:"{}"},HistoryItem{Type:"function_call_output",CallID:id,Content:`{"status":"PASS"}`})
  x.ToolCalls=append(x.ToolCalls,ToolCallRecord{CallID:id,Name:"taskdeck_submit_patch",Status:"done",Result:`{"status":"PASS"}`,FeedbackSent:true,FeedbackStatus:"acknowledged"})
 }
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if x.HistoryArchiveParts<=maxLegacyArchiveParts||x.PatchArchiveParts==0{t.Fatal("fixture did not create archive bounds")}
 historicalHistory,historicalPatch:=x.HistoryArchiveParts,x.PatchArchiveParts
 // Remove part 2 before migration. Its failure is recorded, not interpreted
 // as a negative receipt, and not retried on every subsequent Save.
 missing:=store.historyArchivePath(x.ID,2)
 raw,err:=os.ReadFile(missing);if err!=nil{t.Fatal(err)}
 if err=os.Remove(missing);err!=nil{t.Fatal(err)}
 original:=append([]HistoryItem(nil),x.History...)
 if !x.BindCollectionGuard("Audit the source read-only","",false){t.Fatal("bind")}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 g,ok:=x.ReadCollectionGuard()
 if !ok||g.Legacy.HistoryPartsAtBind!=historicalHistory||g.Legacy.PatchPartsAtBind!=historicalPatch||g.Legacy.HistoryCursor!=1||g.Legacy.PatchCursor!=1||g.Legacy.ArchivedCalls==0||g.Legacy.ArchivedAcknowledgedPatches==0{t.Fatalf("first migration batch: %+v",g.Legacy)}
 old:=x
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 g,ok=x.ReadCollectionGuard()
 if !ok||g.Legacy.HistoryCursor!=2||g.Legacy.ArchiveReadErrors!=1||!g.Legacy.Incomplete{t.Fatalf("missing archive not recorded: %+v",g.Legacy)}
 if _,err=store.Save(old,old.Revision);!errors.Is(err,ErrStaleRevision){t.Fatal("stale migration CAS accepted")}
 if err=os.WriteFile(missing,raw,0600);err!=nil{t.Fatal(err)}
 reopened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err=reopened.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 for i:=0;i<6;i++{x,err=reopened.Save(x,x.Revision);if err!=nil{t.Fatal(err)}}
 g,ok=x.ReadCollectionGuard()
 if !ok||g.Legacy.HistoryCursor!=maxLegacyArchiveParts||g.Legacy.ArchiveReadErrors!=1||g.Legacy.ArchivedCalls!=g.Legacy.ArchivedOutputs||g.Counters.NativeReads!=0||g.Counters.NewEvidence!=0||g.Counters.PatchExecutions!=0||len(g.Evidence.Entries)!=0{t.Fatalf("migration exceeded bounds or fabricated evidence: %+v",g)}
 if !reflect.DeepEqual(original,x.History){t.Fatal("migration changed retained canonical history")}
 if _,err=reopened.Get(x.ID,"other-owner");!errors.Is(err,ErrNotFound){t.Fatal("migration leaked another owner's archive")}
 summary:=g.Legacy
 if !x.BindCollectionGuard("Fix the issue","",false){t.Fatal("new goal")}
 x,err=reopened.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 next,ok:=x.ReadCollectionGuard()
 if !ok||next.Legacy!=summary||next.Generation!=g.Generation+1{t.Fatal("new goal rescanned historical archive")}
 // Old guards with no captured cut must not retroactively read archives.
 other,err:=NewSession("other-owner","/workspace","project","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 if !other.BindCollectionGuard("Continue","",true){t.Fatal("other bind")}
 og,_:=other.ReadCollectionGuard()
 if og.Legacy.ArchivedCalls!=0||og.Legacy.ArchivedAcknowledgedPatches!=0{t.Fatal("cross-conversation migration")}
}


func TestCollectionGuardConcurrentLogicalReceiptCAS(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private")
 a,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 b,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 x.History=[]HistoryItem{{Type:"user",Content:"Audit the source read-only"},{Type:"function_call",CallID:"read-once",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"main.go"}]}`}}
 if !x.BindCollectionGuard("Audit the source read-only","",false){t.Fatal("bind")}
 g,_:=x.ReadCollectionGuard()
 x.AppendEvent("clp_collection_requested",x.Status,"read-once","")
 g.Counters,_,_,err=cp.ApplyCounterEvent(g.Policy,g.Counters,cp.CounterEvent{Sequence:x.EventSeq,Identity:guardDigest("read-once-request"),Kind:cp.CollectionRequested},false)
 if err!=nil||!x.SetCollectionGuard(g){t.Fatal("request checkpoint")}
 x,err=a.Save(x,0);if err!=nil{t.Fatal(err)}
 // Two independently reopened journal candidates reduce the same exact
 // logical receipt at the same sequence. Only one CAS may commit.
 const receipt=`{"status":"PASS","run_id":"native-once"}`
 prepare:=func(store *Store)Session{
  t.Helper()
  candidate,e:=store.Get(x.ID,x.Owner);if e!=nil{t.Fatal(e)}
  state,ok:=candidate.ReadCollectionGuard();if !ok{t.Fatal("missing guard")}
  candidate.History=append(candidate.History,HistoryItem{Type:"function_call_output",CallID:"read-once",Content:receipt})
  candidate.AppendEvent("clp_collection_observed",candidate.Status,"read-once","")
  state.Counters,_,_,e=cp.ApplyCounterEvent(state.Policy,state.Counters,cp.CounterEvent{Sequence:candidate.EventSeq,Identity:guardDigest("read-once-receipt"),Kind:cp.CollectionObserved,Novelty:cp.UnknownNovelty,NativeRead:true},false)
  if e!=nil||!candidate.SetCollectionGuard(state){t.Fatal("receipt projection")}
  return candidate
 }
 first,second:=prepare(a),prepare(b)
 start:=make(chan struct{})
 done:=make(chan error,2)
 go func(){<-start;_,e:=a.Save(first,first.Revision);done<-e}()
 go func(){<-start;_,e:=b.Save(second,second.Revision);done<-e}()
 close(start)
 committed,stale:=0,0
 for i:=0;i<2;i++{e:=<-done;if e==nil{committed++}else if errors.Is(e,ErrStaleRevision){stale++}else{t.Fatal(e)}}
 if committed!=1||stale!=1{t.Fatalf("committed=%d stale=%d",committed,stale)}
 reopened,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 got,err:=reopened.Get(x.ID,x.Owner);if err!=nil{t.Fatal(err)}
 state,ok:=got.ReadCollectionGuard()
 if !ok||got.Revision!=x.Revision+1||state.Counters.Requested!=1||state.Counters.Observed!=1||state.Counters.NativeReads!=1||state.Counters.UnknownEvidence!=1||len(got.History)!=3||got.History[2].Content!=receipt{t.Fatalf("receipt/counter atomicity violated: %+v",state)}
 if _,err=reopened.Save(second,second.Revision);!errors.Is(err,ErrStaleRevision){t.Fatal("stale receipt replay accepted")}
}


func TestCollectionGuardOuterWorkSignatureUsesNoveltyNotReceiptIdentity(t *testing.T){
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 if !x.BindCollectionGuard("Audit read-only","",false){t.Fatal("bind")}
 initial:=VerifiedWorkSignature(x)
 observe:=func(id string,novelty cp.Novelty,native bool){
  t.Helper()
  g,ok:=x.ReadCollectionGuard();if !ok{t.Fatal("guard")}
  x.History=append(x.History,HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:id,Arguments:"{}"},HistoryItem{Type:"function_call_output",CallID:id,Content:"receipt-"+id})
  x.AppendEvent("clp_collection_observed",x.Status,id,"")
  var e error
  g.Counters,_,_,e=cp.ApplyCounterEvent(g.Policy,g.Counters,cp.CounterEvent{Sequence:x.EventSeq,Identity:guardDigest(id),Kind:cp.CollectionObserved,Novelty:novelty,NativeRead:native},false)
  if e!=nil||!x.SetCollectionGuard(g){t.Fatal("observation")}
 }
 observe("a",cp.VerifiedNew,true)
 first:=VerifiedWorkSignature(x)
 if first==initial{t.Fatal("new evidence did not advance outer signature")}
 observe("b",cp.VerifiedDuplicate,true)
 observe("c",cp.ObservationError,false)
 observe("d",cp.UnknownNovelty,true)
 if VerifiedWorkSignature(x)!=first{t.Fatal("duplicate, error or unknown reset outer brake")}
 x.ArchivedHistoryItems=20
 x.ArchivedPatchCalls=3
 x.History=nil
 if VerifiedWorkSignature(x)!=first{t.Fatal("archive movement fabricated progress")}
 observe("e",cp.VerifiedNew,true)
 if VerifiedWorkSignature(x)==first{t.Fatal("changed source did not advance outer signature")}
 g,_:=x.ReadCollectionGuard()
 before:=VerifiedWorkSignature(x)
 x.AppendEvent("clp_patch_executed",x.Status,"patch","")
 g.Counters,_,_,err=cp.ApplyCounterEvent(g.Policy,g.Counters,cp.CounterEvent{Sequence:x.EventSeq,Identity:guardDigest("patch"),Kind:cp.PatchExecuted},false)
 if err!=nil||!x.SetCollectionGuard(g)||VerifiedWorkSignature(x)!=before{t.Fatal("wrapper execution fabricated progress")}
 x.CollectionGuard=json.RawMessage(`{"version":999}`)
 opaque:=VerifiedWorkSignature(x)
 x.History=append(x.History,HistoryItem{Type:"function_call_output",CallID:"opaque",Content:"PASS"})
 if VerifiedWorkSignature(x)!=opaque{t.Fatal("opaque guard fabricated work")}
}


func TestContinuationWorkReferenceMigrationDoesNotResetBrake(t *testing.T){
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 x.History=[]HistoryItem{{Type:"user",Content:"Audit read-only"},{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"legacy",Arguments:"{}"},{Type:"function_call_output",CallID:"legacy",Content:"historical receipt"}}
 original:=x
 prior:=VerifiedWorkSignature(original)
 if !x.BindCollectionGuard("Continue","",true){t.Fatal("bind")}
 if ContinuationWorkReference(original,x,prior)!=VerifiedWorkSignature(x){t.Fatal("first guard binding fabricated work")}
 bound:=x
 if ContinuationWorkReference(bound,x,"old-unversioned-signature")!=VerifiedWorkSignature(x){t.Fatal("upgrade signature change fabricated work")}
 g,_:=x.ReadCollectionGuard()
 x.AppendEvent("clp_collection_observed",x.Status,"new-read","")
 g.Counters,_,_,err=cp.ApplyCounterEvent(g.Policy,g.Counters,cp.CounterEvent{Sequence:x.EventSeq,Identity:guardDigest("new-read"),Kind:cp.CollectionObserved,Novelty:cp.VerifiedNew,NativeRead:true},false)
 if err!=nil||!x.SetCollectionGuard(g){t.Fatal("new evidence")}
 if ContinuationWorkReference(original,x,prior)==VerifiedWorkSignature(x){t.Fatal("migration hid actual new work")}
 if ContinuationWorkReference(bound,x,VerifiedWorkSignature(bound))==VerifiedWorkSignature(x){t.Fatal("existing guard hid actual new work")}
 beforeStep:=x
 if !x.BindCollectionGuard("Audit next step","next-step",true){t.Fatal("step")}
 if ContinuationWorkReference(beforeStep,x,VerifiedWorkSignature(beforeStep))!=VerifiedWorkSignature(x){t.Fatal("step rebinding fabricated work")}
}
