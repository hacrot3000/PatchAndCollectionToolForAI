package server

import (
 "encoding/json"
 "fmt"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentPagedGoBindingRequiresOwnerConfirmationAndNativeSuiteEvidence(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 steps:=make([]agentchat.TaskPlanStep,160)
 for i:=range steps{
  steps[i]=agentchat.TaskPlanStep{
   ID:fmt.Sprintf("step-%04d",i),Title:"Owner-defined test",Acceptance:"Suite must pass",
  }
 }
 x.TaskPlan,err=agentchat.NewTaskPlan("Bound command with exact named tests",steps)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan?offset=128&limit=24"
 post:=func(payload map[string]any)*agentchat.TaskPlanPageResult{
  t.Helper()
  r:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,payload)
  if r.Code!=http.StatusOK{t.Fatalf("paged owner binding failed: %d %s",r.Code,r.Body)}
  var p agentchat.TaskPlanPageResult
  if err:=json.Unmarshal(r.Body.Bytes(),&p);err!=nil{t.Fatal(err)}
  if p.Revision!=x.Revision+1||p.Total!=160||p.Done!=0||p.AllDone||
   p.Plan==nil||len(p.Plan.Steps)!=24||p.Plan.Steps[7].Status!="pending"||
   len(p.Plan.Steps[7].Evidence)!=0{
   t.Fatalf("paged command or Go test configuration unexpectedly completed step: %+v",p)
  }
  x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  return &p
 }
 guard:=map[string]any{
  "action":"set_task_command_guard","expected_revision":x.Revision,
  "step_id":"step-0135","enabled":true,"evidence_task_id":25,
  "expected_command_sha256":strings.Repeat("a",64),
 }
 if r:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,guard);r.Code!=http.StatusForbidden{
  t.Fatalf("unconfirmed command lock accepted: %d %s",r.Code,r.Body)
 }
 invalid:=map[string]any{
  "action":"set_task_command_guard","expected_revision":x.Revision,
  "step_id":"step-0135","enabled":true,"evidence_task_id":25,
  "expected_command_sha256":strings.Repeat("z",64),
  "confirm_command_guard":true,
 }
 if r:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,invalid);r.Code!=http.StatusBadRequest{
  t.Fatalf("invalid command fingerprint accepted: %d %s",r.Code,r.Body)
 }
 goReq:=map[string]any{
  "action":"set_go_test_requirements","expected_revision":x.Revision,
  "step_id":"step-0135","enabled":true,"evidence_go_package":"example/pkg",
  "required_go_tests":[]string{"TestOne","TestTwo"},
  "confirm_go_tests":true,
 }
 if r:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,goReq);r.Code!=http.StatusConflict{
  t.Fatalf("Go checklist without native command fingerprint accepted: %d %s",r.Code,r.Body)
 }
 guard["confirm_command_guard"]=true
 post(guard)
 if x.TaskPlan.Steps[135].TaskCommandGuard==nil{
  t.Fatal("confirmed command fingerprint was not persisted")
 }
 goReq["expected_revision"]=x.Revision
 delete(goReq,"confirm_go_tests")
 if r:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,goReq);r.Code!=http.StatusForbidden{
  t.Fatalf("unconfirmed Go checklist accepted: %d",r.Code)
 }
 goReq["confirm_go_tests"]=true
 post(goReq)
 if x.TaskPlan.Steps[135].GoTestRequirements==nil||
  len(x.TaskPlan.Steps[135].GoTestRequirements.Tests)!=2{
  t.Fatal("owner-selected Go tests were not persisted")
 }
 // The manually attested note never substitutes for a pinned Go suite.
 if r:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_step","expected_revision":x.Revision,"step_id":"step-0135",
  "status":"done","evidence_kind":"manual","evidence_note":"Owner says done",
 });r.Code!=http.StatusConflict{
  t.Fatalf("manual evidence bypassed owner-bound Go suite: %d %s",r.Code,r.Body)
 }
 if r:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_task_command_guard","expected_revision":x.Revision,
  "step_id":"step-0135","enabled":false,"confirm_command_guard":true,
 });r.Code!=http.StatusConflict{
  t.Fatalf("cleared pinned command with Go tests still bound: %d %s",r.Code,r.Body)
 }
 // Invalid duplicate Go tests also must not advance revision.
 if r:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_go_test_requirements","expected_revision":x.Revision,
  "step_id":"step-0135","enabled":true,"confirm_go_tests":true,
  "evidence_go_package":"example/pkg",
  "required_go_tests":[]string{"TestSame","TestSame"},
 });r.Code!=http.StatusBadRequest{
  t.Fatalf("duplicate required tests admitted: %d %s",r.Code,r.Body)
 }
 latest,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if latest.Revision!=x.Revision{t.Fatal("rejected Go checklist edit changed revision")}
 // Exact requirements can only be cleared with separate owner confirmation.
 post(map[string]any{
  "action":"set_go_test_requirements","expected_revision":x.Revision,
  "step_id":"step-0135","enabled":false,"confirm_go_tests":true,
 })
 post(map[string]any{
  "action":"set_task_command_guard","expected_revision":x.Revision,
  "step_id":"step-0135","enabled":false,"confirm_command_guard":true,
 })
 if x.TaskPlan.Steps[135].TaskCommandGuard!=nil||
  x.TaskPlan.Steps[135].GoTestRequirements!=nil||
  x.TaskPlan.State=="completed"{t.Fatal("explicit owner clear or final completion gate broken")}
}

func TestAgentPagedGoBindingInlinePlanKeepsLegacySemantics(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=agentchat.NewTaskPlan("Legacy inline user plan",
  []agentchat.TaskPlanStep{{ID:"single",Title:"Check CLI",Acceptance:"Native test must run"}})
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan?offset=0&limit=24"
 result:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_task_command_guard","expected_revision":x.Revision,
  "step_id":"single","enabled":true,"confirm_command_guard":true,
  "evidence_task_id":1,"expected_command_sha256":strings.Repeat("b",64),
 })
 if result.Code!=http.StatusOK{
  t.Fatalf("inline native command guard fallback rejected: %d %s",result.Code,result.Body)
 }
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan.Steps[0].TaskCommandGuard==nil{t.Fatal("inline fallback lost command guard")}
 result=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_go_test_requirements","expected_revision":x.Revision,
  "step_id":"single","enabled":true,"confirm_go_tests":true,
  "evidence_go_package":"example/pkg","required_go_tests":[]string{"TestExact"},
 })
 if result.Code!=http.StatusOK{
  t.Fatalf("inline Go checklist fallback rejected: %d %s",result.Code,result.Body)
 }
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan.Steps[0].GoTestRequirements==nil||
  x.TaskPlan.Steps[0].GoTestRequirements.Tests[0]!="TestExact"{
  t.Fatal("inline plan Go tests lost in full-plan fallback")
 }
}
