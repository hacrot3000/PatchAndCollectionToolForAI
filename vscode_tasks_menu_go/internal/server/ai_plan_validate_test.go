package server

import (
 "encoding/json"
 "net/http"
 "testing"
)

func TestAIPlanValidationClassifiesOnlyExactLocalReadOnlyTools(t *testing.T){
 s:=prepareAITargetTest(t)
 plan:=createAIIntegrityPlan(t,s) // pwd and echo inspect, session "a" (local)
 w:=requestAIWithUser(t,s.aiPlanValidateAPI,http.MethodPost,
  "/api/ai/plans/validate",aiPlanValidationRequest{ID:plan.ID,Digest:plan.Digest},"alice")
 if w.Code!=http.StatusOK{t.Fatalf("validation: %d %s",w.Code,w.Body.String())}
 var parsed struct{
  Checks []aiToolCandidate `json:"checks"`
  ExecutionEnabled bool `json:"execution_enabled"`
  Executed bool `json:"executed"`
 }
 if err:=json.Unmarshal(w.Body.Bytes(),&parsed);err!=nil{t.Fatal(err)}
 if len(parsed.Checks)!=2{t.Fatalf("want 2 steps: %+v",parsed)}
 if !parsed.Checks[0].Candidate||parsed.Checks[0].Tool!="terminal.pwd"{
  t.Fatalf("pwd not recognized: %+v",parsed.Checks[0])
 }
 if parsed.Checks[1].Candidate||parsed.ExecutionEnabled||parsed.Executed{
  t.Fatalf("unknown command or execution incorrectly enabled: %+v",parsed)
 }
}
func TestAIPlanValidationRejectsRemoteAndTamperedApprovals(t *testing.T){
 s:=prepareAITargetTest(t)
 create:=requestAIWithUser(t,s.aiPlansAPI,http.MethodPost,"/api/ai/plans",
  aiPlanRequest{Target:aiPlanTarget{SessionID:"session-A"},Steps:[]aiPlanStep{{Command:"pwd",Risk:"read-only"}}},"alice")
 if create.Code!=201{t.Fatalf("create %d %s",create.Code,create.Body.String())}
 var plan aiGuidedPlan;json.Unmarshal(create.Body.Bytes(),&plan)
 wrong:=requestAIWithUser(t,s.aiPlanValidateAPI,http.MethodPost,
  "/api/ai/plans/validate",aiPlanValidationRequest{ID:plan.ID,Digest:"invalid"},"alice")
 if wrong.Code!=http.StatusConflict{t.Fatalf("invalid fingerprint accepted: %d",wrong.Code)}
 remote:=requestAIWithUser(t,s.aiPlanValidateAPI,http.MethodPost,
  "/api/ai/plans/validate",aiPlanValidationRequest{ID:plan.ID,Digest:plan.Digest},"alice")
 if remote.Code!=http.StatusOK{t.Fatalf("remote validation: %d",remote.Code)}
 var parsed struct{Checks []aiToolCandidate `json:"checks"`}
 json.Unmarshal(remote.Body.Bytes(),&parsed)
 if len(parsed.Checks)!=1||parsed.Checks[0].Candidate{t.Fatalf("remote diagnostic was marked eligible: %+v",parsed)}
 other:=requestAIWithUser(t,s.aiPlanValidateAPI,http.MethodPost,
  "/api/ai/plans/validate",aiPlanValidationRequest{ID:plan.ID,Digest:plan.Digest},"bob")
 if other.Code!=http.StatusNotFound{t.Fatalf("cross-owner plan validation leaked: %d",other.Code)}
}
