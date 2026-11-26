package contextcompact

import (
 "reflect"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestCacheEpochAppendOnlyAndMandatoryReceipt(t *testing.T){
 goal:=agentchat.HistoryItem{Type:"user",Content:"Original owner goal"}
 p:=Policy{MaxInputTokens:65536,Instructions:"trusted policy",Next:[]agentchat.HistoryItem{{Type:"user",Content:"Continue the audit"}}}
 history:=[]agentchat.HistoryItem{goal}
 scope:=agentchat.BodyDigest([]byte("scope"))
 visible,_:=Build(history,p)
 e:=NewCacheEpoch(history,visible,p,scope,"")
 if e==nil{t.Fatal("epoch missing")}
 canonical:=append(append([]agentchat.HistoryItem(nil),history...),p.Next...)
 // UI display metadata is not part of the actual provider prompt.
 canonical[1].RequestAt="2026-11-26T00:00:00Z"
 call:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"read-1",Arguments:"{}"}
 canonical=append(canonical,call)
 receipt:=agentchat.HistoryItem{Type:"function_call_output",CallID:"read-1",Content:"exact native evidence"}
 p.Next=[]agentchat.HistoryItem{receipt}
 got,reason,ok:=ReuseCacheEpoch(canonical,p,scope,e)
 if !ok||reason!="append_only"||!reflect.DeepEqual(got[:len(e.Visible)],e.Visible)||got[len(got)-1]!=call{t.Fatalf("reuse failed: %s %+v",reason,got)}
 body,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{Model:"model",Instructions:p.Instructions,Mode:agentchat.Stateless,History:got,Next:p.Next})
 if err!=nil||strings.Count(string(body),"exact native evidence")!=1{t.Fatalf("mandatory receipt not exact: %v",err)}
 next:=NewCacheEpoch(canonical,got,p,scope,e.ID)
 if next==nil||next.ID!=e.ID{t.Fatal("same epoch ID not retained")}
 if _,_,ok:=ReuseCacheEpoch(canonical,p,agentchat.BodyDigest([]byte("new authority")),e);ok{t.Fatal("changed scope reused")}
 changed:=append([]agentchat.HistoryItem(nil),canonical...);changed[0].Content="different owner goal"
 if _,_,ok:=ReuseCacheEpoch(changed,p,scope,e);ok{t.Fatal("changed source reused")}
 p.MaxInputTokens=20
 if _,_,ok:=ReuseCacheEpoch(canonical,p,scope,e);ok{t.Fatal("over budget reused")}
}
func TestCacheEpochDoesNotReplayHistoricalPatchOrOrphan(t *testing.T){
 goal:=agentchat.HistoryItem{Type:"user",Content:"Fix bug"}
 p:=Policy{MaxInputTokens:65536,Instructions:"trusted policy",Next:[]agentchat.HistoryItem{{Type:"user",Content:"continue"}}}
 scope:=agentchat.BodyDigest([]byte("scope"))
 e:=NewCacheEpoch([]agentchat.HistoryItem{goal},[]agentchat.HistoryItem{goal},p,scope,"")
 h:=append([]agentchat.HistoryItem{goal},p.Next...)
 patch:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"patch",Arguments:"{}"}
 p.Next=[]agentchat.HistoryItem{{Type:"function_call_output",CallID:"patch",Content:"FAIL"}}
 h=append(h,patch)
 if _,_,ok:=ReuseCacheEpoch(h,p,scope,e);ok{t.Fatal("patch appended to frozen prefix")}
 if NewCacheEpoch(h,h,p,scope,"")!=nil{t.Fatal("full Patch receipt frozen")}
 h[len(h)-1].Name="taskdeck_collect_workspace";p.Next=[]agentchat.HistoryItem{{Type:"user",Content:"new"}}
 if _,_,ok:=ReuseCacheEpoch(h,p,scope,e);ok{t.Fatal("orphan reused")}
}


func TestCacheEpochArchiveAndMultiplePendingReceiptsRebuild(t *testing.T){
 goal:=agentchat.HistoryItem{Type:"user",Content:"Original task"}
 scope:=agentchat.BodyDigest([]byte("archive-scope"))
 p:=Policy{MaxInputTokens:65536,Instructions:"trusted",Next:[]agentchat.HistoryItem{{Type:"user",Content:"continue"}}}
 history:=[]agentchat.HistoryItem{goal}
 visible,_:=Build(history,p)
 old:=NewCacheEpoch(history,visible,p,scope,"");if old==nil{t.Fatal("missing initial checkpoint")}
 // Canonical source prefix no longer matches after archive projection.
 p.ArchivedItems=2;p.ArchivedParts=1
 if _,_,ok:=ReuseCacheEpoch([]agentchat.HistoryItem{goal},p,scope,old);ok{t.Fatal("shortened source accepted")}
 history=append(history,p.Next...)
 a:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"c1",Arguments:"{}"}
 b:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"c2",Arguments:"{}"}
 history=append(history,a,b)
 p.Next=[]agentchat.HistoryItem{{Type:"function_call_output",CallID:"c1",Content:"exact first receipt"},{Type:"function_call_output",CallID:"c2",Content:"exact second receipt"}}
 if _,_,ok:=ReuseCacheEpoch(history,p,scope,old);ok{t.Fatal("nonadjacent current call pairs accepted by append-only path")}
 projected,_:=Build(history,p)
 body,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:p.Instructions,History:projected,Next:p.Next})
 if err!=nil||strings.Count(string(body),"exact first receipt")!=1||strings.Count(string(body),"exact second receipt")!=1{t.Fatalf("rebuild lost mandatory receipts: %v",err)}
 // Archived evidence catalog is inserted before the first pending call.
 if len(projected)<3||projected[len(projected)-2]!=a||projected[len(projected)-1]!=b{t.Fatalf("pending calls split by catalog: %+v",projected)}
}


func TestFullHistoryMetadataEpochCannotSupplyReplay(t *testing.T){
 goal:=agentchat.HistoryItem{Type:"user",Content:"Original task"}
 p:=Policy{MaxInputTokens:20,Instructions:"trusted",Next:[]agentchat.HistoryItem{{Type:"user",Content:"continue"}}}
 scope:=agentchat.BodyDigest([]byte("scope"))
 history:=[]agentchat.HistoryItem{goal}
 old,why:=FullHistoryCacheEpoch(history,history,p,scope,nil)
 if old==nil||why!="full_history"||len(old.Visible)!=0||old.VisibleCount!=2{t.Fatal("metadata-only epoch missing")}
 h:=append(append([]agentchat.HistoryItem(nil),history...),p.Next...)
 h[1].RequestInputTokens=100
 p.Next=[]agentchat.HistoryItem{{Type:"user",Content:"next"}}
 next,why:=FullHistoryCacheEpoch(h,h,p,scope,old)
 if next==nil||next.ID!=old.ID||why!="append_only"{t.Fatal("display metadata or soft limit invalidated actual prefix")}
 if _,_,ok:=ReuseCacheEpoch(h,p,scope,next);ok{t.Fatal("metadata-only record admitted to replay")}
 modified:=append([]agentchat.HistoryItem(nil),h...);modified[0].OriginRunID="different-origin"
 changed,why:=FullHistoryCacheEpoch(modified,modified,p,scope,old)
 if changed==nil||changed.ID==old.ID||why!="full_history"{t.Fatal("source provenance change accepted")}
}


func TestFullHistoryAcknowledgedFailedPatchSummaryIsLedgerBound(t *testing.T){
 goal:=agentchat.HistoryItem{Type:"user",Content:"owner goal"}
 failed:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"old",Arguments:`{"patch_python":"private old source"}`}
 receipt:=agentchat.HistoryItem{Type:"function_call_output",CallID:"old",Content:`{"status":"FAIL","run_id":"old-run"}`}
 pass:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"pass",Arguments:`{"patch_python":"other source"}`}
 passOut:=agentchat.HistoryItem{Type:"function_call_output",CallID:"pass",Content:`{"status":"PASS"}`}
 current:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"current",Arguments:"{}"}
 currentOut:=agentchat.HistoryItem{Type:"function_call_output",CallID:"current",Content:`{"status":"FAIL"}`}
 h:=[]agentchat.HistoryItem{goal,failed,receipt,pass,passOut,current}
 p:=Policy{Next:[]agentchat.HistoryItem{currentOut},AcknowledgedFailedPatchReceipts:map[string]string{"old":agentchat.BodyDigest([]byte(receipt.Content)),"current":agentchat.BodyDigest([]byte(currentOut.Content))}}
 got:=FullHistory(h,p)
 if len(got)!=5||got[0]!=goal||got[2]!=pass||got[3]!=passOut||got[4]!=current||!strings.Contains(got[1].Content,"old-run")||strings.Contains(got[1].Content,"private old source"){t.Fatalf("incorrect summary or pair: %+v",got)}
 body,e:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"trusted",History:got,Next:p.Next})
 if e!=nil||strings.Count(string(body),"\"call_id\":\"current\"")!=2{t.Fatalf("current receipt lost: %v",e)}
 p.AcknowledgedFailedPatchReceipts["old"]=agentchat.BodyDigest([]byte("different"))
 if !reflect.DeepEqual(FullHistory(h,p),h){t.Fatal("unbound historical receipt suppressed")}
 if h[1]!=failed||h[2]!=receipt{t.Fatal("canonical history modified")}
}


func TestFullHistoryEpochUsesActualItemLimit(t *testing.T){
 h:=make([]agentchat.HistoryItem,255)
 for i:=range h{h[i]=agentchat.HistoryItem{Type:"assistant",Content:"retained finding"}}
 h[0]=agentchat.HistoryItem{Type:"user",Content:"owner goal"}
 scope:=agentchat.BodyDigest([]byte("scope"))
 p:=Policy{Next:[]agentchat.HistoryItem{{Type:"user",Content:"continue"}}}
 e,why:=FullHistoryCacheEpoch(h,h,p,scope,nil)
 if e==nil||why!="full_history"||e.VisibleCount!=256{t.Fatal("valid full history did not get a diagnostic checkpoint")}
 h=append(h,p.Next...)
 if e,_:=FullHistoryCacheEpoch(h,h,p,scope,e);e!=nil{t.Fatal("checkpoint accepted more than the wire item limit")}
}
