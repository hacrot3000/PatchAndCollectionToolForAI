package server

import (
 "encoding/json"
 "fmt"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentPagedAcceptanceMatrixAndOwnerReviewRequireExplicitConfirmation(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 steps:=make([]agentchat.TaskPlanStep,160)
 for i:=range steps{
  steps[i]=agentchat.TaskPlanStep{
   ID:fmt.Sprintf("step-%04d",i),Title:"Acceptance scenario",Acceptance:"Owner verifies behavior",
  }
 }
 x.TaskPlan,err=agentchat.NewTaskPlan("Review acceptance for owner plan",steps)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan?offset=128&limit=24"
 matrix:=[]map[string]any{
  {"id":"workflow","description":"User scenario works"},
  {"id":"compat","description":"Legacy users remain supported"},
 }
 set:=func(rev uint64,confirmed bool)int{
  result:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
   "action":"set_acceptance_criteria","expected_revision":rev,
   "step_id":"step-0135","acceptance_criteria":matrix,
   "confirm_coverage":confirmed,
  })
  return result.Code
 }
 if code:=set(x.Revision,false);code!=http.StatusForbidden{
  t.Fatalf("missing owner confirmation replaced acceptance matrix: %d",code)
 }
 if code:=set(x.Revision,true);code!=http.StatusOK{
  t.Fatalf("confirmed matrix rejected: %d",code)
 }
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if len(x.TaskPlan.Steps[135].AcceptanceCriteria)!=2||
  x.TaskPlan.Steps[135].AcceptanceCriteria[0].ReviewNote!=""{
  t.Fatal("owner acceptance matrix absent or silently reviewed")
 }
 response:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_step","step_id":"step-0135","status":"done",
  "evidence_kind":"manual","evidence_note":"Unreviewed criteria bypass",
  "expected_revision":x.Revision,
 })
 if response.Code!=http.StatusConflict{
  t.Fatalf("manual evidence bypassed unreviewed owner coverage: %d %s",
   response.Code,response.Body)
 }
 if resp:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"review_acceptance_criterion","expected_revision":x.Revision,
  "step_id":"step-0135","criterion_id":"workflow",
  "coverage_review_note":"Owner checked scenario","confirm_coverage":false,
 });resp.Code!=http.StatusForbidden{
  t.Fatalf("review allowed without owner confirmation: %d",resp.Code)
 }
 if resp:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"review_acceptance_criterion","expected_revision":x.Revision,
  "step_id":"step-0135","criterion_id":"nonexistent",
  "coverage_review_note":"Owner checked scenario","confirm_coverage":true,
 });resp.Code!=http.StatusNotFound{
  t.Fatalf("unknown criterion did not return 404: %d",resp.Code)
 }
 for _,tc:=range []struct{id,note string}{
  {"workflow","Owner checked scenario A"},
  {"compat","Owner checked compatibility"},
 }{
  res:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
   "action":"review_acceptance_criterion","expected_revision":x.Revision,
   "step_id":"step-0135","criterion_id":tc.id,
   "coverage_review_note":tc.note,"confirm_coverage":true,
  })
  if res.Code!=http.StatusOK{
   t.Fatalf("confirmed criterion review failed: %d %s",res.Code,res.Body)
  }
  var page agentchat.TaskPlanPageResult
  if err=json.Unmarshal(res.Body.Bytes(),&page);err!=nil{t.Fatal(err)}
  if page.Revision!=x.Revision+1||page.Done!=0||page.Total!=160||
   len(page.Plan.Steps)!=24||page.Plan.Steps[7].Status!="pending"||
   len(page.Plan.Steps[7].Evidence)!=0{
   t.Fatalf("review changed unrelated milestone state: %+v",page)
  }
  x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 }
 if !agentchat.TaskPlanStepCriteriaReviewed(x.TaskPlan.Steps[135]){
  t.Fatal("both confirmed owner reviews were not durable")
 }
 success:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_step","expected_revision":x.Revision,
  "step_id":"step-0135","status":"done","evidence_kind":"manual",
  "evidence_note":"Owner explicitly accepted all reviewed criteria",
 })
 if success.Code!=http.StatusOK{
  t.Fatalf("accepted milestone not completable after owner review: %d %s",
   success.Code,success.Body)
 }
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan.Steps[135].Status!="done"||x.TaskPlan.State=="completed"||
  len(x.TaskPlan.Steps[135].Evidence)!=1{
  t.Fatal("reviewed milestone not accepted with explicit evidence or plan auto-completed")
 }
 locked:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_acceptance_criteria","expected_revision":x.Revision,
  "step_id":"step-0135","acceptance_criteria":matrix,"confirm_coverage":true,
 })
 if locked.Code!=http.StatusConflict{
  t.Fatalf("accepted milestone matrix was silently editable: %d",locked.Code)
 }
 handoff,rev,err:=store.Handoff(id,"local")
 if err!=nil||rev!=x.Revision||!strings.Contains(handoff,"1/160"){
  t.Fatalf("canonical handoff lost milestone acceptance: %v",err)
 }
}

func TestAgentPagedAcceptanceCriteriaValidationAndLegacyFallback(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 steps:=[]agentchat.TaskPlanStep{
  {ID:"first",Title:"Owner acceptance",Acceptance:"Reviewed scenario"},
  {ID:"second",Title:"Second milestone",Acceptance:"Must not change"},
 }
 x.TaskPlan,err=agentchat.NewTaskPlan("Legacy inline plan",steps)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan?offset=0&limit=24"
 confirm:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_acceptance_criteria","expected_revision":x.Revision,
  "step_id":"first","confirm_coverage":true,
  "acceptance_criteria":[]map[string]any{{"id":"scenario","description":"Legacy review"}},
 })
 if confirm.Code!=http.StatusOK{
  t.Fatalf("legacy inline owner matrix fallback failed: %d %s",confirm.Code,confirm.Body)
 }
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if len(x.TaskPlan.Steps[0].AcceptanceCriteria)!=1||
  x.TaskPlan.Steps[1].Status!="pending"{
  t.Fatal("legacy full-plan fallback lost coverage")
 }
 invalid:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_acceptance_criteria","expected_revision":x.Revision,
  "step_id":"first","confirm_coverage":true,
  "acceptance_criteria":[]map[string]any{{
   "id":"scenario","description":"Unmapped test",
   "required_tests":[]string{"TestMissing"},
  }},
 })
 if invalid.Code!=http.StatusBadRequest{
  t.Fatalf("unmapped Go test in criteria was accepted: %d %s",invalid.Code,invalid.Body)
 }
}
