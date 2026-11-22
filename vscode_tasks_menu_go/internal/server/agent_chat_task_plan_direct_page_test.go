package server

import (
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentPlanDirectPageReads1024WithCompleteGlobalProgress(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 steps:=make([]agentchat.TaskPlanStep,1024)
 for i:=range steps{
  steps[i]=agentchat.TaskPlanStep{
   ID:fmt.Sprintf("step-%04d",i),
   Title:fmt.Sprintf("Action %d",i),
   Acceptance:fmt.Sprintf("Verified owner acceptance for action %d",i),
  }
 }
 plan,err:=agentchat.NewTaskPlan("Long test plan with 1024 steps",steps)
 if err!=nil{t.Fatal(err)}
 for i:=0;i<350;i++{
  plan.Steps[i].Status="done"
  plan.Steps[i].Evidence=[]agentchat.TaskPlanEvidence{{
   Kind:"manual",Note:fmt.Sprintf("Owner verified milestone %d",i),
  }}
 }
 x.TaskPlan=plan
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 if len(x.TaskPlanSegments)!=32{t.Fatalf("expected 32 pages, got %d",len(x.TaskPlanSegments))}
 url:="/api/agent/sessions/"+id+"/plan"
 check:=func(query string,from,n int,hasNext bool){
  t.Helper()
  w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,url+query,nil)
  if w.Code!=200{t.Fatalf("paged GET %s failed: %d %s",query,w.Code,w.Body)}
  var body taskPlanPaginationResponse
  if e:=json.Unmarshal(w.Body.Bytes(),&body);e!=nil{t.Fatal(e)}
  if body.SessionID!=id||body.Revision!=x.Revision||body.Done!=350||
   body.Total!=1024||body.AllDone||body.Completed||
   body.Plan==nil||body.Page==nil||
   body.Page.Offset!=from||body.Page.TotalSteps!=1024||
   body.Page.HasNext!=hasNext||len(body.Plan.Steps)!=n{
   t.Fatalf("wrong full-plan/only-page response %s: %+v",query,body)
  }
  if n>0&&body.Plan.Steps[0].ID!=fmt.Sprintf("step-%04d",from){
   t.Fatalf("wrong selected milestone at offset %d: %+v",from,body.Plan.Steps[0])
  }
  if from==336&&body.Plan.Steps[0].Evidence[0].Note!="Owner verified milestone 336"{
   t.Fatal("partial page lost owner-bound acceptance evidence")
  }
  if w.Header().Get("Cache-Control")!="no-store"{
   t.Fatal("paged private result missing cache fence")
  }
 }
 check("?offset=0&limit=24",0,24,true)
 check("?offset=336&limit=32",336,32,true)
 check("?offset=1008&limit=24",1008,16,false)
 check("?offset=1024&limit=24",1024,0,false)
 // An invalid page is a client error, not a signal to invent an empty plan.
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,url+"?offset=1025&limit=24",nil)
 if w.Code!=400{t.Fatalf("out-of-range page returned %d",w.Code)}
 _,err=store.TaskPlanPage(id,"different-owner",0,24)
 if !errors.Is(err,agentchat.ErrNotFound){
  t.Fatalf("cross-owner page read leaked data: %v",err)
 }
 // POST is still revision-fenced against a full canonical plan, never a page.
 paused:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url,map[string]any{
  "action":"pause","expected_revision":x.Revision,
 })
 if paused.Code!=200{t.Fatalf("full-plan POST after paged GET failed: %d %s",paused.Code,paused.Body)}
 var posted taskPlanPaginationResponse
 if e:=json.Unmarshal(paused.Body.Bytes(),&posted);e!=nil{t.Fatal(e)}
 if len(posted.Plan.Steps)!=1024||posted.Page!=nil||posted.Total!=1024{
  t.Fatal("legacy POST response/canonical plan was unintentionally paged")
 }
}

