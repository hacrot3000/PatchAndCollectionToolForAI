package server

import (
 "encoding/json"
 "net/http"
 "time"
)

type aiPlanValidationRequest struct {
 ID string `json:"id"`
 Digest string `json:"digest"`
}
type aiToolCandidate struct {
 Step int `json:"step"`
 Tool string `json:"tool,omitempty"`
 Candidate bool `json:"candidate"`
 Reason string `json:"reason"`
}

// aiPlanValidateAPI evaluates whether each proposal matches an exact,
// read-only typed tool. The endpoint never executes any tool or shell.
func (s *Server) aiPlanValidateAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 cfg,e:=s.readAISettings()
 if e!=nil{http.Error(w,"AI settings unavailable",500);return}
 if !cfg.Enabled{http.Error(w,"AI Assistant is disabled",403);return}
 r.Body=http.MaxBytesReader(w,r.Body,1024)
 var req aiPlanValidationRequest
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if e=dec.Decode(&req);e!=nil||len(req.ID)>128||len(req.Digest)>128{
  http.Error(w,"invalid plan validation request",400);return
 }
 aiPlanLock.Lock();defer aiPlanLock.Unlock()
 plans,e:=readAIPlans(s.Workspace);if e!=nil{http.Error(w,"AI plans unavailable",500);return}
 for _,plan:=range plans{
  if plan.ID!=req.ID||plan.Owner!=aiPlanOwner(r){continue}
  verified,ok:=s.attestAITerminal(w,r,plan.Target.SessionID);if !ok{return}
  if verified!=plan.Target{http.Error(w,"plan target is stale",http.StatusConflict);return}
  if plan.ExpiresAt.IsZero()||!time.Now().Before(plan.ExpiresAt){
   http.Error(w,"plan approval expired",http.StatusConflict);return
  }
  if req.Digest==""||req.Digest!=plan.Digest||aiDigestForPlan(plan)!=plan.Digest{
   http.Error(w,"plan fingerprint mismatch",http.StatusConflict);return
  }
  checks:=make([]aiToolCandidate,0,len(plan.Steps))
  for i,step:=range plan.Steps{
   item:=aiToolCandidate{Step:i}
   tool,recognized:=aiReadOnlyToolName(step.Command)
   switch{
   case plan.Target.Remote:
    item.Reason="SSH/remote execution is not supported by the isolated tool runner"
   case !recognized:
    item.Reason="Command is not in the fixed read-only diagnostics registry"
   case aiProposalRisk(step.Command,step.Risk)!="read-only":
    item.Reason="Proposal risk must be reviewed; not a read-only candidate"
   default:
    item.Tool=tool
    item.Candidate=true
    item.Reason="Exact read-only diagnostic recognized; preview only, execution disabled"
   }
   checks=append(checks,item)
  }
  w.Header().Set("Content-Type","application/json")
  w.Header().Set("Cache-Control","no-store")
  json.NewEncoder(w).Encode(map[string]any{
   "plan_id":plan.ID,"digest":plan.Digest,"target":plan.Target,
   "checks":checks,"executed":false,"execution_enabled":false,
   "note":"Candidate classification only; no commands or tools were executed.",
  })
  return
 }
 http.Error(w,"plan not found",http.StatusNotFound)
}
