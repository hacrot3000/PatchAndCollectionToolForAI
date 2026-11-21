package server

import (
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestModelSuggestedEvidenceRequiresOwnerRuleConsentAndNativeProof(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=agentchat.NewTaskPlan("Research real files",
  []agentchat.TaskPlanStep{{ID:"research",Title:"Inspect source",Acceptance:"Complete verified source file read"}})
 if err!=nil{t.Fatal(err)}
 // A user/attachment message containing the same JSON must not be captured:
 // only appendAgentProviderResult is authorized to parse assistant output.
 modelText:="Review candidate only.\n\x60\x60\x60taskdeck-evidence-suggestion\n"+
  `{"step_id":"research","kind":"full_file_read","call_id":"collect-proposed","path":"src/main.go"}`+"\n\x60\x60\x60"
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:modelText})
 if len(x.TaskPlan.Suggestions)!=0{t.Fatal("user-controlled content became model proposal")}
 appendAgentProviderResult(&x,agentchat.ResponsesResult{ResponseID:"resp-suggested-1",Text:modelText},true)
 if x.TaskPlan.Steps[0].AutoCheck!=nil||x.TaskPlan.Steps[0].Status=="done"{
  t.Fatal("model hint bypassed user consent")
 }
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan"
 get:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,path,nil)
 if get.Code!=200||!strings.Contains(get.Body.String(),`"call_id":"collect-proposed"`){
  t.Fatalf("untrusted model suggestion missing from owner plan GET: %d %s",get.Code,get.Body.String())
 }
 before:=x.Revision
 noConsent:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_auto_check","expected_revision":before,"step_id":"research","enabled":true,
  "auto_kind":"full_file_read","auto_call_id":"collect-proposed","auto_path":"src/main.go",
 })
 if noConsent.Code!=http.StatusForbidden{t.Fatalf("model hint auto-approved without user consent: %d",noConsent.Code)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Revision!=before||x.TaskPlan.Steps[0].Status!="pending"{t.Fatal("unapproved proposal altered canonical plan")}
 accepted:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_auto_check","expected_revision":x.Revision,"step_id":"research","enabled":true,
  "confirm_auto_accept":true,
  "auto_kind":"full_file_read","auto_call_id":"collect-proposed","auto_path":"src/main.go",
 })
 if accepted.Code!=200{t.Fatalf("owner-approved rule failed: %d %s",accepted.Code,accepted.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan.Steps[0].AutoCheck==nil||x.TaskPlan.Steps[0].Status=="done"{
  t.Fatal("missing native receipt was treated as success")
 }
 reopened,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 y,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
 if len(y.TaskPlan.Suggestions)!=1||y.TaskPlan.Suggestions[0].TurnID!="resp-suggested-1"{
  t.Fatal("model-suggested pointer did not survive durable journal reopen")
 }
}
