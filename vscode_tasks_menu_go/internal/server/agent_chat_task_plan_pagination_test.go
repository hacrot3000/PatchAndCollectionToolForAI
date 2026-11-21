package server

import (
 "encoding/json"
 "fmt"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

type taskPlanPaginationResponse struct {
 SessionID string `json:"session_id"`
 Revision uint64 `json:"revision"`
 Plan *agentchat.TaskPlan `json:"plan"`
 Done int `json:"done"`
 Total int `json:"total"`
 AllDone bool `json:"all_done"`
 Completed bool `json:"completed"`
 Page *struct {
  Offset int `json:"offset"`
  Limit int `json:"limit"`
  TotalSteps int `json:"total_steps"`
  HasNext bool `json:"has_next"`
 } `json:"page"`
}

func TestAgentTaskPlanGETPaginationAndWholePlanCompletion(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 steps:=make([]agentchat.TaskPlanStep,128)
 for i:=range steps{
  steps[i]=agentchat.TaskPlanStep{
   ID:fmt.Sprintf("step-%03d",i),Title:fmt.Sprintf("Milestone %03d",i),
   Acceptance:fmt.Sprintf("Manual proof for step %03d",i),
  }
 }
 plan,err:=agentchat.NewTaskPlan("Durable 128 milestone plan for API pagination",steps)
 if err!=nil{t.Fatal(err)}
 // First page has evidence on ALL its rows, but step 25 is unfinished.
 // The API must not accidentally offer whole-plan completion from the page.
 for i:=0;i<24;i++{
  plan.Steps[i].Status="done"
  plan.Steps[i].Evidence=[]agentchat.TaskPlanEvidence{{Kind:"manual",Note:"owner accepted this exact milestone"}}
 }
 x.TaskPlan=plan
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 url:="/api/agent/sessions/"+id+"/plan"
 get:=func(query string)taskPlanPaginationResponse{
  t.Helper()
  w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,url+query,nil)
  if w.Code!=200{t.Fatalf("GET %s returned %d: %s",query,w.Code,w.Body.String())}
  if w.Header().Get("Cache-Control")!="no-store"{
   t.Fatalf("GET %s missed no-store header",query)
  }
  var body taskPlanPaginationResponse
  if err:=json.Unmarshal(w.Body.Bytes(),&body);err!=nil{t.Fatal(err)}
  if body.SessionID!=id||body.Revision!=x.Revision||body.Total!=128||body.Done!=24||body.Completed||body.AllDone{
   t.Fatalf("whole-plan metadata changed by pagination: %+v",body)
  }
  return body
 }
 legacy:=get("")
 if legacy.Page!=nil||legacy.Plan==nil||len(legacy.Plan.Steps)!=128{
  t.Fatalf("legacy unpaged GET was truncated: %+v",legacy.Page)
 }
 first:=get("?offset=0&limit=24")
 if first.Page==nil||first.Page.Offset!=0||first.Page.Limit!=24||first.Page.TotalSteps!=128||!first.Page.HasNext||
  len(first.Plan.Steps)!=24||first.Plan.Steps[0].ID!="step-000"||
  first.Plan.Steps[23].ID!="step-023"||len(first.Plan.Steps[23].Evidence)!=1{
  t.Fatalf("first page/accepted proof incorrect: %+v",first)
 }
 middle:=get("?offset=24&limit=24")
 if len(middle.Plan.Steps)!=24||middle.Plan.Steps[0].ID!="step-024"||
  middle.Plan.Steps[0].Status=="done"{
  t.Fatalf("middle page mixed with accepted first page: %+v",middle)
 }
 tail:=get("?offset=120&limit=24")
 if tail.Page==nil||tail.Page.HasNext||len(tail.Plan.Steps)!=8||
  tail.Plan.Steps[0].ID!="step-120"||tail.Plan.Steps[7].ID!="step-127"{
  t.Fatalf("tail page incorrect: %+v",tail)
 }
 empty:=get("?offset=128&limit=24")
 if empty.Page==nil||len(empty.Plan.Steps)!=0||empty.Page.HasNext{
  t.Fatalf("valid empty end-page was not handled: %+v",empty.Page)
 }
 defaultLimit:=get("?offset=24")
 if defaultLimit.Page==nil||defaultLimit.Page.Limit!=24||len(defaultLimit.Plan.Steps)!=24{
  t.Fatalf("default limit should be 24: %+v",defaultLimit.Page)
 }
 for _,bad:=range []string{
  "?limit=0","?limit=-1","?limit=33","?limit=bogus","?limit=",
  "?offset=-1","?offset=bogus","?offset=","?offset=129",
  "?limit=1&limit=2","?offset=0&offset=1",
  "?offset=999999999999999999999999999999",
 }{
  w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,url+bad,nil)
  if w.Code!=400{t.Fatalf("invalid page %s returned %d instead of 400: %s",bad,w.Code,w.Body)}
 }
 // A paginated read may never mutate the canonical plan. POST continues
 // to require original session CAS and returns a complete plan to old clients.
 olderRevision:=x.Revision
 changed:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url,map[string]any{
  "action":"pause","expected_revision":olderRevision,
 })
 if changed.Code!=200{t.Fatalf("pause failed after GET pagination: %d %s",changed.Code,changed.Body)}
 var posted taskPlanPaginationResponse
 if err:=json.Unmarshal(changed.Body.Bytes(),&posted);err!=nil{t.Fatal(err)}
 if posted.Plan==nil||len(posted.Plan.Steps)!=128||posted.Page!=nil{
  t.Fatal("legacy POST response was accidentally paged")
 }
 invalidCAS:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url,map[string]any{
  "action":"resume","expected_revision":olderRevision,
 })
 if invalidCAS.Code!=409{t.Fatalf("stale mutation accepted after paginated read: %d",invalidCAS.Code)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan.State!="paused"||len(x.TaskPlan.Steps)!=128||
  x.TaskPlan.Steps[24].Status=="done"{
  t.Fatal("pagination mutated the canonical Task plan")
 }
 // Whole-plan all_done must only become true after all 128 milestones
 // have evidence, even when the visible 24-step window is unchanged.
 for i:=24;i<128;i++{
  x.TaskPlan.Steps[i].Status="done"
  x.TaskPlan.Steps[i].Evidence=[]agentchat.TaskPlanEvidence{{Kind:"manual",Note:"owner accepted this exact milestone"}}
 }
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,url+"?offset=0&limit=24",nil)
 if w.Code!=200{t.Fatalf("all-done first page GET: %d",w.Code)}
 var all taskPlanPaginationResponse
 if err:=json.Unmarshal(w.Body.Bytes(),&all);err!=nil{t.Fatal(err)}
 if !all.AllDone||all.Done!=128||all.Total!=128||len(all.Plan.Steps)!=24||
  all.Plan.State!="paused"||!strings.Contains(all.Plan.Goal,"128 milestone"){
  t.Fatalf("whole-plan all_done or plan metadata lost: %+v",all)
 }
}
