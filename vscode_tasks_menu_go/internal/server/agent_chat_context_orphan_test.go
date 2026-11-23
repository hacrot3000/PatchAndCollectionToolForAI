package server

import (
 "encoding/json"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentRealSessionOrphanPatchRebaseProducesValidProviderInput(t *testing.T) {
 // Repro from session 897b4014e45014d5ea61d0638a1f6fe9:
 // a historical unanswered native Patch function_call from index 3
 // preceded many normal completed calls, then stateless continuation.
 orphan:=agentchat.HistoryItem{
  Type:"function_call",CallID:"call_03541687963626754fe4b9e1",
  Name:"taskdeck_submit_patch",
  Arguments:`{"name":"old patch","patch_python":"print('UNKNOWN')"}`,
 }
 x:=agentchat.Session{History:[]agentchat.HistoryItem{
  {Type:"user",Content:"TaskDeck original project instructions"},
  {Type:"user",Content:"Continue"},
  {Type:"user",Content:"Continue"},
  orphan,
  {Type:"user",Content:"Old patch was interrupted; investigate"},
 }}
 for i:=0;i<38;i++{
  id:="verified-tool-"+string(rune('A'+i))
  x.History=append(x.History,
   agentchat.HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_collect_workspace",
    Arguments:`{"actions":[{"type":"search","query":"verify"}]}`},
   agentchat.HistoryItem{Type:"function_call_output",CallID:id,Content:`{"status":"PASS"}`})
 }
 if len(x.History)!=81{t.Fatalf("incorrect fixture size: %d",len(x.History))}
 next:=agentchat.HistoryItem{Type:"user",Content:"Please continue the task"}
 visible:=agentContextForRequest(x,agentchat.ModelConfig{MaxInputTokens:65536},
  "TaskDeck mandatory instructions",[]agentchat.HistoryItem{next})
 raw,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{
  Model:"composer-2.5",Instructions:"TaskDeck mandatory instructions",
  Mode:agentchat.Stateless,History:visible,Next:[]agentchat.HistoryItem{next},
  EnablePatchTool:true,EnableCollectionTool:true,
 })
 if err!=nil{t.Fatalf("real session rebase still invalid: %v",err)}
 var req struct {Input []map[string]any `json:"input"`}
 if err:=json.Unmarshal(raw,&req);err!=nil{t.Fatal(err)}
 foundSummary:=false
 for _,v:=range req.Input{
  if v["type"]=="function_call" && v["call_id"]==orphan.CallID{
   t.Fatal("orphaned Patch was passed to the provider, triggering HTTP 400")
  }
  content,_:=v["content"].(string)
  if strings.Contains(content,"UNRESOLVED historical tool call") &&
   strings.Contains(content,orphan.CallID){foundSummary=true}
 }
 if !foundSummary{t.Fatal("historical orphan is not reported to the model")}
 if x.History[3].Arguments!=orphan.Arguments{t.Fatal("durable Agent journal changed")}
}

func TestAgentCurrentPatchFeedbackSurvivesRebasePairing(t *testing.T) {
 call:=agentchat.HistoryItem{Type:"function_call",CallID:"current",
  Name:"taskdeck_submit_patch",Arguments:`{"name":"current","patch_python":"print('ok')"}`}
 x:=agentchat.Session{History:[]agentchat.HistoryItem{
  {Type:"user",Content:"Current task"},call,
 }}
 next:=agentchat.HistoryItem{Type:"function_call_output",CallID:"current",
  Content:`{"status":"FAIL","rc":1,"source":"native_patch_tool"}`}
 visible:=agentContextForRequest(x,agentchat.ModelConfig{MaxInputTokens:32768},
  "TaskDeck policy",[]agentchat.HistoryItem{next})
 payload,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{
  Model:"composer-2.5",Instructions:"TaskDeck policy",Mode:agentchat.Stateless,
  History:visible,Next:[]agentchat.HistoryItem{next},
 })
 if err!=nil{t.Fatalf("fresh pending call/output lost after repair: %v",err)}
 if !strings.Contains(string(payload),`"call_id":"current"`)||
  !strings.Contains(string(payload),`native_patch_tool`){
  t.Fatal("once-only verified native Patch evidence missing")
 }
}
