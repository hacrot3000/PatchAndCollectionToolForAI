package server

import (
 "errors"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

type fakeAgentPatchProtocol struct {
 session.Service
 state session.ProtocolState
 expectedID string
}
func (f *fakeAgentPatchProtocol)ProtocolState(id string)(session.ProtocolState,error){
 if id!=f.expectedID{return session.ProtocolState{},errors.New("unknown patch session")}
 return f.state,nil
}
func TestAgentChatSyncFinalNativePatchEvidence(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.ToolCalls[0].Status="running"
 x.ToolCalls[0].QueuePath="patchs/agent_fixture.zip"
 x.ToolCalls[0].PatchSessionID="patch-session-1"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 rc:=0
 s.Sessions=&fakeAgentPatchProtocol{
  expectedID:"patch-session-1",
  state:session.ProtocolState{Available:true,Items:[]session.ProtocolItemState{{
   Name:"agent_fixture.zip",Kind:"PATCH",Status:"PASS",RC:&rc,OutputTail:"All tests passed",
  }}},
 }
 result:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/sync",map[string]any{})
 if result.Code!=200{t.Fatalf("sync failed: %d %s",result.Code,result.Body)}
 y,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if y.ToolCalls[0].Status!="done"||!strings.Contains(y.ToolCalls[0].Result,`"PASS"`){
  t.Fatalf("verified native result was not saved: %+v",y.ToolCalls[0])
 }
 oldRevision:=y.Revision
 again:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/sync",map[string]any{})
 if again.Code!=200{t.Fatalf("second poll failed: %d",again.Code)}
 z,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if z.Revision!=oldRevision{t.Fatal("no-op sync unexpectedly mutated journal")}
}
func TestAgentChatSyncDoesNotInventSuccess(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.ToolCalls[0].Status="running"
 x.ToolCalls[0].QueuePath="patchs/agent_fixture.zip"
 x.ToolCalls[0].PatchSessionID="patch-session-2"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 s.Sessions=&fakeAgentPatchProtocol{
  expectedID:"patch-session-2",
  state:session.ProtocolState{Available:true,Items:[]session.ProtocolItemState{{
   Name:"other.zip",Kind:"PATCH",Status:"PASS",
  }}},
 }
 result:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/sync",map[string]any{})
 if result.Code!=200{t.Fatalf("sync failed: %d %s",result.Code,result.Body)}
 y,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if y.ToolCalls[0].Status!="running"||y.Revision!=x.Revision{t.Fatal("unrelated native PASS was attributed to agent")}
}
func TestAgentChatSyncSharedPermission(t *testing.T){
 request:=httptest.NewRequest("POST","/api/agent/sessions/id/sync",nil)
 perms:=sharedRoutePermissions(request)
 if len(perms)!=2||perms[0]!=identity.PermissionSettingsRead||perms[1]!=identity.PermissionPatchView{
  t.Fatalf("native sync missing patch.view permission: %v",perms)
 }
}
