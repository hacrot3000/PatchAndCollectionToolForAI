package server

import (
 "encoding/json"
 "net/http"
 "strings"
 "testing"
 "time"
)

func createAIIntegrityPlan(t *testing.T,s *Server)aiGuidedPlan{
 t.Helper()
 request:=aiPlanRequest{
  Target:aiPlanTarget{SessionID:"a"},
  Steps:[]aiPlanStep{
   {Command:"pwd",Reason:"Confirm context",Risk:"read-only"},
   {Command:"echo inspect",Reason:"Propose harmless inspection",Risk:"low"},
  },
 }
 response:=requestAIWithUser(t,s.aiPlansAPI,http.MethodPost,"/api/ai/plans",request,"alice")
 if response.Code!=http.StatusCreated{t.Fatalf("plan creation: %d %s",response.Code,response.Body.String())}
 var plan aiGuidedPlan
 if err:=json.Unmarshal(response.Body.Bytes(),&plan);err!=nil{t.Fatal(err)}
 if len(plan.Digest)!=64||plan.ExpiresAt.IsZero()||plan.ExecutionEnabled{t.Fatalf("missing integrity fields: %+v",plan)}
 return plan
}
func TestAIPlanIntegrityRejectsWrongOrMissingFingerprint(t *testing.T){
 s:=prepareAITargetTest(t)
 plan:=createAIIntegrityPlan(t,s)
 for _,digest:=range []string{"",strings.Repeat("0",64)}{
  resp:=requestAIWithUser(t,s.aiPlanDecisionAPI,http.MethodPost,"/api/ai/plans/decision",
   aiPlanDecision{ID:plan.ID,Step:0,Decision:"approved",Digest:digest},"alice")
  if resp.Code!=http.StatusConflict{t.Errorf("bad digest %q accepted: %d",digest,resp.Code)}
 }
 for step:=0;step<2;step++{
  resp:=requestAIWithUser(t,s.aiPlanDecisionAPI,http.MethodPost,"/api/ai/plans/decision",
   aiPlanDecision{ID:plan.ID,Step:step,Decision:"approved",Digest:plan.Digest},"alice")
  if resp.Code!=http.StatusOK{t.Fatalf("correct digest step %d rejected: %d %s",step,resp.Code,resp.Body.String())}
  if !strings.Contains(resp.Body.String(),"\"execution_enabled\":false"){t.Fatal("approval activated execution")}
 }
}
func TestAIPlanIntegrityRejectsModifiedStoredCommand(t *testing.T){
 s:=prepareAITargetTest(t)
 plan:=createAIIntegrityPlan(t,s)
 saved,err:=readAIPlans(s.Workspace);if err!=nil{t.Fatal(err)}
 saved[len(saved)-1].Steps[0].Command="rm -rf /tmp/data"
 if err=saveAIPlans(s.Workspace,saved);err!=nil{t.Fatal(err)}
 resp:=requestAIWithUser(t,s.aiPlanDecisionAPI,http.MethodPost,"/api/ai/plans/decision",
  aiPlanDecision{ID:plan.ID,Step:0,Decision:"approved",Digest:plan.Digest},"alice")
 if resp.Code!=http.StatusConflict{t.Fatalf("tampered command accepted: %d %s",resp.Code,resp.Body.String())}
}
func TestAIPlanIntegrityRejectsExpiredApproval(t *testing.T){
 s:=prepareAITargetTest(t)
 plan:=createAIIntegrityPlan(t,s)
 saved,err:=readAIPlans(s.Workspace);if err!=nil{t.Fatal(err)}
 saved[len(saved)-1].ExpiresAt=time.Now().Add(-time.Minute)
 if err=saveAIPlans(s.Workspace,saved);err!=nil{t.Fatal(err)}
 resp:=requestAIWithUser(t,s.aiPlanDecisionAPI,http.MethodPost,"/api/ai/plans/decision",
  aiPlanDecision{ID:plan.ID,Step:0,Decision:"approved",Digest:plan.Digest},"alice")
 if resp.Code!=http.StatusConflict{t.Fatalf("expired approval accepted: %d %s",resp.Code,resp.Body.String())}
}

func TestAIPlanIntegrityRejectsExtendedApprovalWindow(t *testing.T){
 s:=prepareAITargetTest(t)
 plan:=createAIIntegrityPlan(t,s)
 saved,err:=readAIPlans(s.Workspace);if err!=nil{t.Fatal(err)}
 saved[len(saved)-1].ExpiresAt=plan.ExpiresAt.Add(24*time.Hour)
 if err=saveAIPlans(s.Workspace,saved);err!=nil{t.Fatal(err)}
 response:=requestAIWithUser(t,s.aiPlanDecisionAPI,http.MethodPost,"/api/ai/plans/decision",
  aiPlanDecision{ID:plan.ID,Step:0,Decision:"approved",Digest:plan.Digest},"alice")
 if response.Code!=http.StatusConflict{
  t.Fatalf("tampered extension accepted: %d %s",response.Code,response.Body.String())
 }
}
