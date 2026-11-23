package server

import (
 "encoding/json"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentPagedSegmentDoneRequiresVerifiedOwnerAttestation(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 steps:=make([]agentchat.TaskPlanStep,1024)
 for i:=range steps{
  steps[i]=agentchat.TaskPlanStep{
   ID:strings.Repeat("x",1),Title:"Bounded milestone",Acceptance:"Owner confirms evidence",
  }
  // Unique, stable IDs are part of the canonical Task Plan contract.
  steps[i].ID="step-"+formatPlanStepIndex(i)
 }
 x.TaskPlan,err=agentchat.NewTaskPlan("Complete milestone with owner evidence",steps)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan?offset=480&limit=24"
 bad:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_step","expected_revision":x.Revision,
  "step_id":"step-0501","status":"done","evidence_kind":"patch",
  "evidence_ref":"forged-native-patch","evidence_note":"cannot forge a receipt",
 })
 if bad.Code!=http.StatusConflict{
  t.Fatalf("unverified native receipt completed paged step: %d %s",bad.Code,bad.Body)
 }
 empty:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_step","expected_revision":x.Revision,
  "step_id":"step-0501","status":"done","evidence_kind":"manual",
  "evidence_note":"  ",
 })
 if empty.Code!=http.StatusConflict{
  t.Fatalf("empty owner acceptance admitted: %d %s",empty.Code,empty.Body)
 }
 success:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_step","expected_revision":x.Revision,
  "step_id":"step-0501","status":"done","evidence_kind":"manual",
  "evidence_note":"Owner directly verified the acceptance criterion",
 })
 if success.Code!=http.StatusOK{
  t.Fatalf("paged manual acceptance failed: %d %s",success.Code,success.Body)
 }
 var page agentchat.TaskPlanPageResult
 if err=json.Unmarshal(success.Body.Bytes(),&page);err!=nil{t.Fatal(err)}
 if page.Revision!=x.Revision+1||page.Done!=1||page.Total!=1024||
  page.Completed||page.AllDone||page.Plan==nil||len(page.Plan.Steps)!=24||
  page.Plan.Steps[21].ID!="step-0501"||
  page.Plan.Steps[21].Status!="done"||
  len(page.Plan.Steps[21].Evidence)!=1||
  page.Plan.Steps[21].Evidence[0].Kind!="manual"{
  t.Fatalf("wrong paged done response: %+v",page)
 }
 latest,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if latest.Revision!=page.Revision||
  latest.TaskPlan.Steps[501].Evidence[0].Note!="Owner directly verified the acceptance criterion"{
  t.Fatal("owner acceptance was not stored in the canonical plan")
 }
 stale:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_step","expected_revision":x.Revision,
  "step_id":"step-0502","status":"done","evidence_kind":"manual",
  "evidence_note":"stale",
 })
 if stale.Code!=http.StatusConflict{
  t.Fatalf("stale paged done was admitted: %d",stale.Code)
 }
}

func formatPlanStepIndex(i int)string{
 return string([]byte{'0'+byte(i/1000%10),'0'+byte(i/100%10),
  '0'+byte(i/10%10),'0'+byte(i%10)})
}
