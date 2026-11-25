package agentchat

import (
 "encoding/json"
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
