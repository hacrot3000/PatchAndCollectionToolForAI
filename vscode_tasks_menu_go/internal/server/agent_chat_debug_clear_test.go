package server

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentDebugClearHidesOldDurableJournalWithoutDeletingAgentOrPatchEvidence(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 journal,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 old,err:=journal.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 old.History=append(old.History,agentchat.HistoryItem{Type:"user",Content:"old chat remains in canonical journal"})
 old.AppendEvent("old_turn","done","","old event")
 old,err=journal.Save(old,old.Revision)
 if err!=nil{t.Fatal(err)}
 _,oldTrace:=s.startAgentDebug("local","old_provider_request",id,
  agentchat.ProviderConfig{ID:"old-provider",APIType:"responses"},"old-model")

 report:=func()map[string]any{
  t.Helper()
  w:=httptest.NewRecorder()
  s.agentChatDebugReportAPI(w,httptest.NewRequest(http.MethodGet,"/api/agent/debug/report",nil))
  if w.Code!=200{t.Fatalf("debug report HTTP %d: %s",w.Code,w.Body)}
  var result map[string]any
  if err=json.Unmarshal(w.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
  return result
 }
 initial:=report()
 if initial["provider_trace_count"].(float64)!=1 ||
  initial["session_count"].(float64)!=1 {t.Fatalf("fixture missing old debug evidence: %+v",initial)}

 clear:=httptest.NewRecorder()
 s.agentChatDebugClearAPI(clear,httptest.NewRequest(http.MethodPost,"/api/agent/debug/clear",strings.NewReader("{}")))
 if clear.Code!=200||!strings.Contains(clear.Body.String(),`"cleared":true`){
  t.Fatalf("Clear debug was refused: %d %s",clear.Code,clear.Body)
 }
 after:=report()
 if after["provider_trace_count"].(float64)!=0||after["session_count"].(float64)!=0||
  after["cleared_at"]==nil{
  t.Fatalf("old traces or journal resurfaced after Clear: %+v",after)
 }

 // Destroy the ephemeral trace store as on restart. The owner-scoped marker
 // is persisted and must still keep the previous report hidden.
 s.agentDebugMu.Lock();s.agentDebug=nil;s.agentDebugMu.Unlock()
 restarted:=report()
 if restarted["provider_trace_count"].(float64)!=0||
  restarted["session_count"].(float64)!=0{
  t.Fatalf("old debug returned after restart: %+v",restarted)
 }
 if _,err=journal.Get(id,"local");err!=nil{
  t.Fatalf("Clear Debug deleted canonical conversation: %v",err)
 }
 marker,err:=s.readAgentDebugClearMarker("local")
 if err!=nil||marker.At.IsZero()||marker.Sessions[id].HistoryItems!=1{
  t.Fatalf("clear marker lost cursor or data: %+v / %v",marker,err)
 }

 // Fresh provider events and a continuation of the pre-existing session
 // should be visible without old history or old event entries.
 _,newTrace:=s.startAgentDebug("local","new_provider_request",id,
  agentchat.ProviderConfig{ID:"new-provider",APIType:"responses"},"new-model")
 newTurn,err:=journal.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 newTurn.AppendEvent("fresh_turn","done","","fresh event")
 newTurn.History=append(newTurn.History,agentchat.HistoryItem{Type:"assistant",Content:"new response"})
 newTurn,err=journal.Save(newTurn,newTurn.Revision)
 if err!=nil{t.Fatal(err)}
 newReport:=report()
 if newReport["provider_trace_count"].(float64)!=1||
  newReport["session_count"].(float64)!=1{
  t.Fatalf("new activity lost after Clear: %+v",newReport)
 }
 encoded,_:=json.Marshal(newReport)
 if strings.Contains(string(encoded),oldTrace) || !strings.Contains(string(encoded),newTrace) ||
  strings.Contains(string(encoded),"old_turn")||!strings.Contains(string(encoded),"fresh_turn"){
  t.Fatalf("Clear report contains old events or drops new evidence: %s",encoded)
 }
 summaries:=newReport["sessions"].([]any)
 summary:=summaries[0].(map[string]any)
 if summary["history_item_count"].(float64)!=1 {
  t.Fatalf("new tail must include only post-clear history item: %v",summary)
 }
 canonical,err:=journal.Get(id,"local")
 if err!=nil||len(canonical.History)!=2||len(canonical.Events)<2{
  t.Fatalf("debug clear altered canonical journal: %+v / %v",canonical,err)
 }
}
