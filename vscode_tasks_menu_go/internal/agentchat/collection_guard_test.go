package agentchat

import (
 "encoding/json"
 "errors"
 "path/filepath"
 "reflect"
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
