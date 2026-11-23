package server

import (
 "net/http"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentOwnerAcceptanceCriteriaGateCompletionAndResetOnReplacement(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=agentchat.NewTaskPlan("All owner requirements must be reviewed",
  []agentchat.TaskPlanStep{{
   ID:"feature",Title:"Deliver feature",Acceptance:"Owner requirements covered",
  }})
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan?offset=0&limit=24"
 action:=func(body map[string]any)int{
  t.Helper()
  body["expected_revision"]=x.Revision
  response:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,body)
  if response.Code==200{
   x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  }
  return response.Code
 }
 items:=[]map[string]any{
  {"id":"main_flow","description":"Main user flow works"},
  {"id":"edge_case","description":"Error and legacy paths are handled"},
 }
 if got:=action(map[string]any{
  "action":"set_acceptance_criteria","step_id":"feature","acceptance_criteria":items,
 });got!=403{t.Fatalf("unconfirmed owner matrix accepted: %d",got)}
 if got:=action(map[string]any{
  "action":"set_acceptance_criteria","step_id":"feature","confirm_coverage":true,
  "acceptance_criteria":items,
 });got!=200{t.Fatalf("owner matrix was not saved: %d",got)}
 if len(x.TaskPlan.Steps[0].AcceptanceCriteria)!=2{
  t.Fatal("owner-defined criteria not persisted")
 }
 if got:=action(map[string]any{
  "action":"set_step","step_id":"feature","status":"done",
  "evidence_kind":"manual","evidence_note":"A PASS result is not semantic coverage",
 });got!=409{t.Fatalf("unreviewed criteria did not block Mark done: %d",got)}
 originalRevision:=x.Revision
 if got:=action(map[string]any{
  "action":"review_acceptance_criterion","step_id":"feature",
  "criterion_id":"main_flow","coverage_review_note":"Unconfirmed",
 });got!=403{t.Fatalf("unconfirmed review accepted: %d",got)}
 if got:=action(map[string]any{
  "action":"review_acceptance_criterion","step_id":"feature","confirm_coverage":true,
  "criterion_id":"main_flow","coverage_review_note":"  ",
 });got!=400{t.Fatalf("empty owner review accepted: %d",got)}
 if x.Revision!=originalRevision{t.Fatal("bad review changed revision")}
 if got:=action(map[string]any{
  "action":"review_acceptance_criterion","step_id":"feature","confirm_coverage":true,
  "criterion_id":"main_flow","coverage_review_note":"I inspected all main flow cases",
 });got!=200{t.Fatalf("owner review failed: %d",got)}
 if got:=action(map[string]any{
  "action":"set_step","step_id":"feature","status":"done",
  "evidence_kind":"manual","evidence_note":"I separately attest the step works",
 });got!=409{t.Fatalf("one review was treated as full coverage: %d",got)}
 if got:=action(map[string]any{
  "action":"review_acceptance_criterion","step_id":"feature","confirm_coverage":true,
  "criterion_id":"edge_case","coverage_review_note":"I checked legacy behavior and failure handling",
 });got!=200{t.Fatalf("second owner review failed: %d",got)}
 if got:=action(map[string]any{
  "action":"set_step","step_id":"feature","status":"done",
  "evidence_kind":"manual","evidence_note":"Owner reviewed all criteria",
 });got!=200{t.Fatalf("fully reviewed step with explicit manual evidence denied: %d",got)}
 step:=x.TaskPlan.Steps[0]
 if step.Status!="done"||len(step.Evidence)!=1||
  len(step.AcceptanceCriteria)!=2||
  step.AcceptanceCriteria[0].ReviewedAt==nil||
  step.AcceptanceCriteria[1].ReviewedAt==nil{
  t.Fatalf("full owner evidence and criteria not persisted: %+v",step)
 }
 handoff,_,err:=store.Handoff(id,"local")
 if err!=nil||!strings.Contains(handoff,"OWNER acceptance coverage: 2/2"){
  t.Fatalf("handoff did not preserve coverage state: %v",err)
 }
 // A completed step cannot have its prior reviews silently rewritten.
 if got:=action(map[string]any{
  "action":"set_acceptance_criteria","step_id":"feature","confirm_coverage":true,
  "acceptance_criteria":[]map[string]any{{"id":"main_flow","description":"changed"}},
 });got!=409{t.Fatalf("accepted evidence was retargeted without reopening: %d",got)}
}

func TestAgentOwnerAcceptanceMappingRequiresPredeclaredGoTestsAndLocksThem(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=agentchat.NewTaskPlan("Predeclared test mapping",
  []agentchat.TaskPlanStep{{ID:"tests",Title:"Verify tests",Acceptance:"Owner coverage checklist"}})
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 x.TaskPlan.Steps[0].TaskCommandGuard=&agentchat.TaskCommandGuard{
  TaskID:31,CommandSHA256:strings.Repeat("a",64),BoundAt:now.Add(-time.Minute),
 }
 x.TaskPlan.Steps[0].GoTestRequirements=&agentchat.TaskGoTestRequirements{
  Package:"example.com/app",Tests:[]string{"TestFirst","TestSecond"},BoundAt:now,
 }
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan"
 mutate:=func(body map[string]any)int{
  t.Helper()
  body["expected_revision"]=x.Revision
  res:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,body)
  if res.Code==200{
   x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  }
  return res.Code
 }
 if got:=mutate(map[string]any{
  "action":"set_acceptance_criteria","step_id":"tests","confirm_coverage":true,
  "acceptance_criteria":[]map[string]any{{
   "id":"coverage","description":"User-visible behavior covered",
   "required_tests":[]string{"TestNotPredeclared"},
  }},
 });got!=400{t.Fatalf("unlocked test mapped to requirement: %d",got)}
 if got:=mutate(map[string]any{
  "action":"set_acceptance_criteria","step_id":"tests","confirm_coverage":true,
  "acceptance_criteria":[]map[string]any{{
   "id":"coverage","description":"User-visible behavior covered",
   "required_tests":[]string{"TestFirst","TestSecond"},
  }},
 });got!=200{t.Fatalf("owner predeclared test mapping failed: %d",got)}
 if got:=mutate(map[string]any{
  "action":"set_go_test_requirements","step_id":"tests",
  "enabled":false,"confirm_go_tests":true,
 });got!=409{t.Fatalf("Go checklist removed despite bound coverage mapping: %d",got)}
 if got:=mutate(map[string]any{
  "action":"set_task_command_guard","step_id":"tests",
  "enabled":false,"confirm_command_guard":true,
 });got!=409{t.Fatalf("command identity changed despite bound coverage: %d",got)}
 if got:=mutate(map[string]any{
  "action":"review_acceptance_criterion","step_id":"tests",
  "criterion_id":"coverage","confirm_coverage":true,
  "coverage_review_note":"I reviewed how both named tests map to the acceptance statement",
 });got!=200{t.Fatalf("owner test-mapping review failed: %d",got)}
 if got:=mutate(map[string]any{
  "action":"set_step","step_id":"tests","status":"done",
  "evidence_kind":"manual","evidence_note":"All tests passed",
 });got!=409{t.Fatalf("manual PASS bypassed the locked Go-suite evidence gate: %d",got)}
 if got:=mutate(map[string]any{
  "action":"set_acceptance_criteria","step_id":"tests",
  "confirm_coverage":true,"acceptance_criteria":[]map[string]any{{
   "id":"coverage","description":"Revised acceptance requirement",
   "required_tests":[]string{"TestSecond"},
  }},
 });got!=200{t.Fatalf("owner could not explicitly replace an unfinished matrix: %d",got)}
 if x.TaskPlan.Steps[0].AcceptanceCriteria[0].ReviewedAt!=nil||
  x.TaskPlan.Steps[0].AcceptanceCriteria[0].ReviewNote!=""{
  t.Fatal("old owner review was inherited by changed acceptance/test mapping")
 }
 if got:=mutate(map[string]any{
  "action":"set_acceptance_criteria","step_id":"tests",
  "confirm_coverage":true,"acceptance_criteria":[]map[string]any{},
 });got!=200{t.Fatalf("explicit matrix removal failed: %d",got)}
 if got:=mutate(map[string]any{
  "action":"set_go_test_requirements","step_id":"tests",
  "enabled":false,"confirm_go_tests":true,
 });got!=200{t.Fatalf("owner cannot adjust Go checklist after explicit matrix removal: %d",got)}
}
