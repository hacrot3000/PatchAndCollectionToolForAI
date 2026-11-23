package server

import (
 "encoding/json"
 "fmt"
 "net/http"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentTaskPlanSegmentStatusPostUsesOnlyChangedPage(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 steps:=make([]agentchat.TaskPlanStep,1024)
 for i:=range steps{
  steps[i]=agentchat.TaskPlanStep{
   ID:fmt.Sprintf("s-%04d",i),Title:fmt.Sprintf("Milestone %d",i),
   Acceptance:fmt.Sprintf("Owner accepts milestone %d",i),
  }
 }
 x.TaskPlan,err=agentchat.NewTaskPlan("Direct segmented status CAS with unchanged proofs",steps)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan.Steps[0].Status="done"
 x.TaskPlan.Steps[0].Evidence=[]agentchat.TaskPlanEvidence{{Kind:"manual",Note:"Owner explicitly attests"}}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 previous:=append([]agentchat.TaskPlanSegmentRef(nil),x.TaskPlanSegments...)
 url:="/api/agent/sessions/"+id+"/plan"
 updated:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url+"?offset=480&limit=24",map[string]any{
  "action":"set_step","expected_revision":x.Revision,
  "step_id":"s-0501","status":"blocked",
 })
 if updated.Code!=200{t.Fatalf("segmented status update rejected: %d %s",updated.Code,updated.Body)}
 var reply taskPlanPaginationResponse
 if err=json.Unmarshal(updated.Body.Bytes(),&reply);err!=nil{t.Fatal(err)}
 if reply.Revision!=x.Revision+1||reply.Total!=1024||reply.Done!=1||
  reply.AllDone||reply.Completed||reply.Page==nil||reply.Page.Offset!=480||
  reply.Page.TotalSteps!=1024||len(reply.Plan.Steps)!=24||
  reply.Plan.Steps[21].ID!="s-0501"||reply.Plan.Steps[21].Status!="blocked"{
  t.Fatalf("wrong paged status CAS response: %+v",reply)
 }
 restored,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 for i,ref:=range restored.TaskPlanSegments{
  if i==501/32{
   if ref.Revision!=restored.Revision||ref==previous[i]{
    t.Fatal("target page did not receive a new immutable revision")
   }
  }else if ref!=previous[i]{
   t.Fatalf("unexpected rewrite of page %d for one-step status change",i)
  }
 }
 if restored.TaskPlan.Steps[0].Evidence[0].Note!="Owner explicitly attests"||
  restored.TaskPlan.Steps[501].Status!="blocked"{
  t.Fatal("segmented CAS dropped unrelated owner evidence")
 }
 stale:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url+"?offset=480&limit=24",map[string]any{
  "action":"set_step","expected_revision":x.Revision,
  "step_id":"s-0501","status":"active",
 })
 if stale.Code!=http.StatusConflict{t.Fatalf("stale segmented CAS returned %d",stale.Code)}
 // "done" with manual evidence MUST use the existing full-plan proof path,
 // not the streamlined non-completing status path.
 done:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url+"?offset=480&limit=24",map[string]any{
  "action":"set_step","expected_revision":restored.Revision,
  "step_id":"s-0501","status":"done",
  "evidence_kind":"manual","evidence_note":"Human accepted the blocked milestone",
 })
 if done.Code!=200{t.Fatalf("proof-verified full path rejected a valid done: %d %s",done.Code,done.Body)}
 final,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if final.TaskPlan.Steps[501].Status!="done"||
  len(final.TaskPlan.Steps[501].Evidence)!=1||
  final.TaskPlan.Steps[501].Evidence[0].Note!="Human accepted the blocked milestone"{
  t.Fatal("existing evidence-verified done path regressed")
 }
}

func TestAgentTaskPlanSegmentStatusRejectedOnWrongPageOrCorruptProof(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 steps:=make([]agentchat.TaskPlanStep,140)
 for i:=range steps{
  steps[i]=agentchat.TaskPlanStep{ID:fmt.Sprintf("test-%03d",i),Title:"Milestone",Acceptance:"Owner approval required"}
 }
 x.TaskPlan,err=agentchat.NewTaskPlan("Fenced status updates",steps)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 url:="/api/agent/sessions/"+id+"/plan"
 for _,query:=range []string{"?offset=-1&limit=24","?offset=141&limit=24","?limit=0","?limit=33"}{
  bad:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url+query,map[string]any{
   "action":"set_step","step_id":"test-100","status":"blocked","expected_revision":x.Revision,
  })
  if bad.Code!=400{t.Fatalf("bad page %s returned %d",query,bad.Code)}
  got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  if got.Revision!=x.Revision||got.TaskPlan.Steps[100].Status!="pending"{
   t.Fatal("invalid page mutated a referenced plan segment")
  }
 }
}
