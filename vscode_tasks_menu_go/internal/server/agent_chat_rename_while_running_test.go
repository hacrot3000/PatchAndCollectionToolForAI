package server

import (
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Rename is metadata-only and must not contend with a provider/native turn.
// A concurrent turn may hold a stale in-memory snapshot; its subsequent Save
// must keep the owner's latest display title and the exact turn CAS.
func TestAgentChatRenameWhileTurnLockedAndInFlightSavePreservesTitle(t *testing.T) {
 s, id := newAgentConversationFixture(t, "stateless")
 store, err := s.agentChatStore()
 if err != nil { t.Fatal(err) }
 before, err := store.Get(id, "local")
 if err != nil { t.Fatal(err) }

 release, acquired := lockAgentTurn(s.Workspace, "local", id)
 if !acquired { t.Fatal("fixture Agent turn already locked") }
 defer release()

 path := "/api/agent/sessions/" + id + "/metadata"
 response := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPatch, path,
  map[string]any{"title":"  Cải tiến UI Agent  ","expected_revision":before.Revision})
 if response.Code!=200 || !strings.Contains(response.Body.String(),"Cải tiến UI Agent") {
  t.Fatalf("active Agent turn blocked rename: HTTP %d %s",response.Code,response.Body)
 }
 persisted, err := store.Get(id, "local")
 if err!=nil {t.Fatal(err)}
 if persisted.Title!="Cải tiến UI Agent"||persisted.Revision!=before.Revision {
  t.Fatalf("rename changed active turn CAS or did not save metadata: %+v",persisted)
 }

 // The running Agent still holds 'before'. A valid update must not overwrite
 // the owner's title even if the snapshot predates the rename.
 before.Status="requesting_ai"
 before.AppendEvent("turn_started","requesting_ai","","provider in flight")
 saved, err:=store.Save(before,before.Revision)
 if err!=nil{t.Fatalf("Agent turn Save broke after metadata rename: %v",err)}
 if saved.Title!="Cải tiến UI Agent"||saved.Revision!=before.Revision+1||
  saved.Status!="requesting_ai"||len(saved.Events)==0 {
  t.Fatalf("active Agent journal update or metadata was lost: %+v",saved)
 }
 // The UI can observe an earlier turn revision while a provider is
 // actively committing a new one; it must still be allowed to rename.
 second:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPatch,path,
  map[string]any{"title":"Second active rename","expected_revision":before.Revision})
 if second.Code!=200{
  t.Fatalf("active Agent progress invalidated independent rename: %d %s",second.Code,second.Body)
 }
 current,err:=store.Get(id,"local")
 if err!=nil||current.Title!="Second active rename"||current.Revision!=saved.Revision {
  t.Fatalf("second metadata-only rename changed turn state: %+v / %v",current,err)
 }
 rows:=invokeAI(t,s.agentChatSessionsAPI,http.MethodGet,"/api/agent/sessions",nil)
 if rows.Code!=200||!strings.Contains(rows.Body.String(),"Second active rename"){
  t.Fatalf("History list does not reflect renamed title: %d %s",rows.Code,rows.Body)
 }
}

func TestAgentChatRenameDoesNotArchiveOrDisturbPendingNativeWork(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 x.Status="waiting_tool"
 x.History=append(x.History,agentchat.HistoryItem{Type:"function_call",CallID:"call_pending",Name:"taskdeck_submit_patch",Arguments:`{"name":"test","patch_python":"pass"}`})
 x.ToolCalls=append(x.ToolCalls,agentchat.ToolCallRecord{CallID:"call_pending",Name:"taskdeck_submit_patch",Status:"pending"})
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 response:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPatch,"/api/agent/sessions/"+id+"/metadata",
  map[string]any{"title":"Pending patch display title","expected_revision":x.Revision})
 if response.Code!=200{t.Fatalf("pending Patch blocked rename: %d %s",response.Code,response.Body)}
 after,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 if after.Title!="Pending patch display title"||after.Revision!=x.Revision||
  len(after.ToolCalls)!=1||after.ToolCalls[0].Status!="pending"{
  t.Fatalf("native pending work changed during rename: %+v",after)
 }
}
