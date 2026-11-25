package agentchat

import (
 "encoding/json"
 "strings"
 "testing"
)
func TestCollectionProgressViewOnlyCommittedRedactedMetadata(t *testing.T){
 x,err:=NewSession("local","/private-worktree","project","provider","model",Stateless);if err!=nil{t.Fatal(err)}
 if CollectionProgressViewForSession(x)!=nil{t.Fatal("legacy state invented")}
 if !x.BindCollectionGuard("Fix secret-file.go","",false){t.Fatal("bind")}
 x.Status="requesting_ai";x.AppendEvent("collect_started",x.Status,"private-call","private-file")
 v:=CollectionProgressViewForSession(x)
 if v==nil||v.State!="reading"||v.Generation!=1{t.Fatalf("bad reading view: %+v",v)}
 x.AppendEvent("clp_checkpoint",x.Status,"private-call","recovery_budget_exhausted");x.Status="failed"
 v=CollectionProgressViewForSession(x)
 if v.State!="stalled"||v.Reason!="recovery_budget_exhausted"{t.Fatalf("bad checkpoint: %+v",v)}
 raw,_:=json.Marshal(v)
 for _,secret:=range []string{"private-worktree","private-call","secret-file","goal_digest","scope_digest","proof_digest"}{if strings.Contains(string(raw),secret){t.Fatalf("leaked %s",secret)}}
 x.Status="requesting_ai";x.AppendEvent("turn_started",x.Status,"","")
 if v=CollectionProgressViewForSession(x);v.State=="stalled"{t.Fatal("stale checkpoint leaked into new turn")}
 x.ProviderRun=&ProviderRun{Phase:"unknown"};x.Status="failed"
 if CollectionProgressViewForSession(x).State!="recovery_required"{t.Fatal("unknown provider hidden")}
 x.Status="cancelled"
 if CollectionProgressViewForSession(x).State!="stopped"{t.Fatal("Stop hidden")}
 x.CollectionGuard=json.RawMessage(`{"version":999,"private":"unavailable"}`)
 if CollectionProgressViewForSession(x).State!="metadata_unavailable"{t.Fatal("opaque future metadata accepted")}
}
