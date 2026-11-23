package server

import (
 "encoding/json"
 "fmt"
 "net/http"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentTaskPlanPostPageReturnsOnlyRequestedStepsWithWholePlanCounts(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 steps:=make([]agentchat.TaskPlanStep,1024)
 for i:=range steps{
  steps[i]=agentchat.TaskPlanStep{
   ID:fmt.Sprintf("step-%04d",i),
   Title:fmt.Sprintf("Milestone %d",i),
   Acceptance:fmt.Sprintf("Verified acceptance for milestone %d",i),
  }
 }
 x.TaskPlan,err=agentchat.NewTaskPlan("Paginated POST must keep full canonical plan",steps)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan.Steps[0].Status="done"
 x.TaskPlan.Steps[0].Evidence=[]agentchat.TaskPlanEvidence{{Kind:"manual",Note:"Human accepted this step"}}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 url:="/api/agent/sessions/"+id+"/plan"
 initialRevision:=x.Revision
 paused:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url+"?offset=0&limit=24",map[string]any{
  "action":"pause","expected_revision":initialRevision,
 })
 if paused.Code!=200{t.Fatalf("paged pause failed: %d %s",paused.Code,paused.Body)}
 var body taskPlanPaginationResponse
 if err=json.Unmarshal(paused.Body.Bytes(),&body);err!=nil{t.Fatal(err)}
 if body.Revision!=initialRevision+1||body.Done!=1||body.Total!=1024||body.AllDone||
  body.Completed||body.Page==nil||body.Page.TotalSteps!=1024||body.Page.Offset!=0||
  body.Page.Limit!=24||!body.Page.HasNext||body.Plan==nil||
  body.Plan.State!="paused"||len(body.Plan.Steps)!=24||
  body.Plan.Steps[0].Evidence[0].Note!="Human accepted this step"{
  t.Fatalf("paged POST forgot canonical metadata or evidence: %+v",body)
 }
 if paused.Header().Get("Cache-Control")!="no-store"{
  t.Fatal("paged POST lacks no-store")
 }
 // A posted page is only a response projection. The durable journal and
 // unpaged legacy GET still expose all 1,024 milestones and identical proof.
 full:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,url,nil)
 if full.Code!=200{t.Fatalf("legacy unpaged GET failed: %d",full.Code)}
 var complete taskPlanPaginationResponse
 if err=json.Unmarshal(full.Body.Bytes(),&complete);err!=nil{t.Fatal(err)}
 if len(complete.Plan.Steps)!=1024||complete.Page!=nil||
  complete.Plan.Steps[0].Evidence[0].Note!="Human accepted this step"{
  t.Fatal("partial POST overwrote canonical milestone list")
 }
 resumed:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url,map[string]any{
  "action":"resume","expected_revision":body.Revision,
 })
 if resumed.Code!=200{t.Fatalf("legacy resume failed: %d %s",resumed.Code,resumed.Body)}
 var legacy taskPlanPaginationResponse
 if err=json.Unmarshal(resumed.Body.Bytes(),&legacy);err!=nil{t.Fatal(err)}
 if legacy.Page!=nil||len(legacy.Plan.Steps)!=1024||legacy.Revision!=body.Revision+1{
  t.Fatal("legacy unpaged POST was truncated by the new opt-in behavior")
 }
 tail:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url+"?offset=1008&limit=24",map[string]any{
  "action":"pause","expected_revision":legacy.Revision,
 })
 if tail.Code!=200{t.Fatalf("tail page pause failed: %d %s",tail.Code,tail.Body)}
 var last taskPlanPaginationResponse
 if err=json.Unmarshal(tail.Body.Bytes(),&last);err!=nil{t.Fatal(err)}
 if last.Page==nil||last.Page.HasNext||last.Page.Offset!=1008||
  len(last.Plan.Steps)!=16||last.Plan.Steps[0].ID!="step-1008"||
  last.Total!=1024||last.Done!=1{
  t.Fatalf("tail POST lost global counts: %+v",last)
 }
}

func TestAgentTaskPlanPostBadPageFailsBeforeMutation(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 plan,err:=agentchat.NewTaskPlan("Reject bad page before mutation",[]agentchat.TaskPlanStep{
  {ID:"one",Title:"First milestone",Acceptance:"Human verified"},
  {ID:"two",Title:"Second milestone",Acceptance:"Human verified"},
 })
 if err!=nil{t.Fatal(err)}
 x.TaskPlan=plan
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 url:="/api/agent/sessions/"+id+"/plan"
 for _,suffix:=range []string{
  "?offset=-1","?offset=3","?limit=0","?limit=33",
  "?limit=NaN","?offset=","?limit=","?offset=0&offset=1",
  "?limit=1&limit=2",
 }{
  bad:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url+suffix,map[string]any{
   "action":"pause","expected_revision":x.Revision,
  })
  if bad.Code!=http.StatusBadRequest{
   t.Fatalf("bad page %q changed plan or returned %d: %s",suffix,bad.Code,bad.Body)
  }
  unchanged,err:=store.Get(id,"local")
  if err!=nil{t.Fatal(err)}
  if unchanged.Revision!=x.Revision||unchanged.TaskPlan.State!="active"{
   t.Fatalf("bad page %s mutated canonical session to revision %d",suffix,unchanged.Revision)
  }
 }
}
 
func TestAgentTaskPlanPostCreateViaPage(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 url:="/api/agent/sessions/"+id+"/plan?offset=0&limit=24"
 created:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url,map[string]any{
  "action":"create","expected_revision":uint64(1),
  "goal":"Create from a paged Task plan editor",
  "steps":[]map[string]string{{"id":"one","title":"First","acceptance":"Owner verifies first step"}},
 })
 if created.Code!=200{t.Fatalf("paged create failed: %d %s",created.Code,created.Body)}
 var result taskPlanPaginationResponse
 if err:=json.Unmarshal(created.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if result.Page==nil||result.Page.TotalSteps!=1||result.Page.HasNext||
  len(result.Plan.Steps)!=1||result.Plan.Steps[0].ID!="one"||
  result.Total!=1||result.Done!=0{
  t.Fatalf("paged create response incorrect: %+v",result)
 }
}
