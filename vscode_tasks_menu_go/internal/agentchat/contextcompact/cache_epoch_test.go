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
