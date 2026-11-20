package server

import (
 "context"
 "errors"
 "encoding/json"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentAutomationOptionsPerConversation(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/automation"
 unauthorized:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"patch","enabled":true,"expected_revision":x.Revision,
 })
 if unauthorized.Code!=403 {t.Fatalf("Auto Patch opt-in needs explicit setting authorization: %d",unauthorized.Code)}
 activated:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"patch","enabled":true,"confirm_execution":true,"expected_revision":x.Revision,
 })
 if activated.Code!=200 {t.Fatalf("Auto Patch opt-in failed: %d %s",activated.Code,activated.Body)}
 x,err=store.Get(id,"local")
 if err!=nil||!x.AutoPatch {t.Fatalf("session option was not saved: %+v %v",x,err)}
 disabledCollection:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"collection","enabled":false,"expected_revision":x.Revision,
 })
 if disabledCollection.Code!=200 {t.Fatalf("Collection toggle failed: %d %s",disabledCollection.Code,disabledCollection.Body)}
 x,err=store.Get(id,"local")
 if err!=nil||x.AutoCollection==nil||*x.AutoCollection {t.Fatalf("Collection opt-out not persisted: %+v %v",x.AutoCollection,err)}
 noConsent:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"feedback","enabled":true,"expected_revision":x.Revision,
 })
 if noConsent.Code!=403 {t.Fatalf("provider egress opt-in lacked external authorization: %d",noConsent.Code)}
 enabledFeedback:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"feedback","enabled":true,"confirm_external":true,"expected_revision":x.Revision,
 })
 if enabledFeedback.Code!=200 {t.Fatalf("Auto feedback opt-in failed: %d %s",enabledFeedback.Code,enabledFeedback.Body)}
 x,err=store.Get(id,"local")
 if err!=nil||!x.AutoFeedback||!x.AutoFeedbackPersistent||x.AutoFeedbackUntil!=nil {t.Fatalf("Persistent Auto feedback not saved: %+v %v",x,err)}
 otherID:=createAgentSessionWithSameFixture(t,s)
 other,err:=store.Get(otherID,"local")
 if err!=nil {t.Fatal(err)}
 if other.AutoPatch||other.AutoFeedback||other.AutoCollection!=nil {t.Fatal("automation permissions leaked across conversations")}
}

func createAgentSessionWithSameFixture(t *testing.T,s *Server) string {
 t.Helper()
 p:=agentProviderFixture()
 r:=invokeAI(t,s.agentChatSessionsAPI,http.MethodPost,"/api/agent/sessions",map[string]any{
  "mode":"stateless","provider_id":p.ID,"model_id":p.Models[0].ID,
 })
 if r.Code!=201 {t.Fatalf("second session create failed: %d %s",r.Code,r.Body)}
 var data struct{ Session agentchat.Session `json:"session"` }
 if err:=json.Unmarshal(r.Body.Bytes(),&data);err!=nil {t.Fatal(err)}
 return data.Session.ID
}

func TestAgentAutoPatchUsesNativeQueueOnceWithoutPerCallConfirmation(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if call,ok:=agentAutoPatchCandidate(x);ok||call!="" {t.Fatal("Auto Patch must default OFF")}
 s.runAgentAutoFeedback(context.Background())
 x,err=store.Get(id,"local")
 if err!=nil||x.ToolCalls[0].Status!="pending" {t.Fatal("Patch unexpectedly auto-approved before opt-in")}
 enabled:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/automation",map[string]any{
  "option":"patch","enabled":true,"confirm_execution":true,"expected_revision":x.Revision,
 })
 if enabled.Code!=200 {t.Fatalf("Auto Patch enable failed: %d %s",enabled.Code,enabled.Body)}
 s.runAgentAutoFeedback(context.Background())
 x,err=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if x.ToolCalls[0].Status!="queued" || x.ToolCalls[0].QueuePath=="" {
  t.Fatalf("Native Patch was not queued under Auto Patch: %+v",x.ToolCalls[0])
 }
 queued:=filepath.Join(s.Workspace,filepath.FromSlash(x.ToolCalls[0].QueuePath))
 if _,err:=os.Stat(queued);err!=nil {t.Fatal(err)}
 s.runAgentAutoFeedback(context.Background())
 y,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if y.ToolCalls[0].QueuePath!=x.ToolCalls[0].QueuePath||y.Revision!=x.Revision {
  t.Fatal("Auto Patch was queued twice or changed after its idempotent handoff")
 }
 again:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/tools/"+x.ToolCalls[0].CallID+"/decision",map[string]any{
  "action":"approve","confirm_execution":true,"automatic":true,"expected_revision":y.Revision,
 })
 if again.Code!=409 {t.Fatalf("duplicate automatic Patch execution was allowed: %d",again.Code)}
}

func TestAgentDeleteConversationDoesNotRemoveRunningNativeWork(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 endpoint:="/api/agent/sessions/"+id
 noDelete:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodDelete,endpoint,map[string]any{"expected_revision":x.Revision})
 if noDelete.Code!=409 {t.Fatalf("pending Patch conversation was deleted: %d",noDelete.Code)}
 if _,err:=store.Get(id,"local");err!=nil {t.Fatal("failed delete damaged the session")}
 decision:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,endpoint+"/tools/"+x.ToolCalls[0].CallID+"/decision",map[string]any{
  "action":"reject","expected_revision":x.Revision,
 })
 if decision.Code!=200 {t.Fatalf("reject failed: %d %s",decision.Code,decision.Body)}
 x,err=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 stale:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodDelete,endpoint,map[string]any{"expected_revision":x.Revision-1})
 if stale.Code!=409 {t.Fatalf("stale delete accepted: %d",stale.Code)}
 deleted:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodDelete,endpoint,map[string]any{"expected_revision":x.Revision})
 if deleted.Code!=200||!strings.Contains(deleted.Body.String(),`"deleted":true`) {
  t.Fatalf("conversation delete failed: %d %s",deleted.Code,deleted.Body)
 }
 if _,err:=store.Get(id,"local");!errors.Is(err,agentchat.ErrNotFound) {
  t.Fatalf("deleted conversation remains available: %v",err)
 }
}
