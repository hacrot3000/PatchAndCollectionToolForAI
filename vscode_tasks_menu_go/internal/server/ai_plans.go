package server

// L3 preview/approval journal. Execution is intentionally absent: an approved
// proposal is NOT authorization to execute on a shell or a production server.
import (
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "time"
)

type aiPlanTarget struct {
 SessionID string `json:"session_id"`
 Host string `json:"host"`
 CWD string `json:"cwd"`
 Remote bool `json:"remote"`
}
type aiPlanStep struct {
 Command string `json:"command"`
 Reason string `json:"reason"`
 Risk string `json:"risk"`
 Decision string `json:"decision"`
}
type aiGuidedPlan struct {
 ID string `json:"id"`
 Owner string `json:"owner"`
 Target aiPlanTarget `json:"target"`
 Steps []aiPlanStep `json:"steps"`
 CreatedAt time.Time `json:"created_at"`
 UpdatedAt time.Time `json:"updated_at"`
 ExecutionEnabled bool `json:"execution_enabled"`
 Digest string `json:"digest"`
 ExpiresAt time.Time `json:"expires_at"`
}
type aiPlanDecision struct {
 ID string `json:"id"`
 Step int `json:"step"`
 Decision string `json:"decision"`
 Digest string `json:"digest"`
}
type aiPlanRequest struct {
 Target aiPlanTarget `json:"target"`
 Steps []aiPlanStep `json:"steps"`
}
const aiPlanApprovalTTL = 30 * time.Minute
// Approvals bind to the immutable command text, risk, reason, verified target,
// owner and creation time. Decisions themselves do not change the fingerprint.
func aiDigestForPlan(plan aiGuidedPlan)string{
 type exactStep struct{Command, Reason, Risk string}
 steps:=make([]exactStep,0,len(plan.Steps))
 for _,v:=range plan.Steps{steps=append(steps,exactStep{v.Command,v.Reason,v.Risk})}
 payload,_:=json.Marshal(struct{
  ID string;Owner string;Target aiPlanTarget;Steps []exactStep;CreatedAt time.Time
 }{plan.ID,plan.Owner,plan.Target,steps,plan.CreatedAt})
 sum:=sha256.Sum256(payload)
 return hex.EncodeToString(sum[:])
}
var aiPlanLock sync.Mutex // serializes the private journal, including across server handlers
func aiPlanOwner(r *http.Request)string {
 if principal,ok:=PrincipalFromContext(r.Context());ok{return "user:"+string(principal.UserID)}
 return "local"
}
func aiPlansPath(workspace string)(string,error){
 d,e:=aiPrivatePath(workspace);if e!=nil{return "",e}
 return strings.TrimSuffix(d,".json")+"-plans.json",nil
}
func readAIPlans(workspace string)([]aiGuidedPlan,error){
 p,e:=aiPlansPath(workspace);if e!=nil{return nil,e}
 b,e:=os.ReadFile(p);if os.IsNotExist(e){return []aiGuidedPlan{},nil};if e!=nil{return nil,e}
 if len(b)>256<<10{return nil,errors.New("AI plan journal size exceeded")}
 var plans []aiGuidedPlan
 if e=json.Unmarshal(b,&plans);e!=nil{return nil,e}
 return plans,nil
}
func saveAIPlans(workspace string,plans []aiGuidedPlan)error{
 p,e:=aiPlansPath(workspace);if e!=nil{return e}
 if e=os.MkdirAll(filepath.Dir(p),0700);e!=nil{return e}
 b,e:=json.Marshal(plans);if e!=nil{return e}
 tmp,e:=os.CreateTemp(filepath.Dir(p),".ai-plan-*");if e!=nil{return e}
 defer os.Remove(tmp.Name())
 if e=tmp.Chmod(0600);e!=nil{tmp.Close();return e}
 if _,e=tmp.Write(b);e!=nil{tmp.Close();return e}
 if e=tmp.Sync();e!=nil{tmp.Close();return e}
 if e=tmp.Close();e!=nil{return e}
 return os.Rename(tmp.Name(),p)
}
func (s *Server) aiPlansAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet&&r.Method!=http.MethodPost {http.Error(w,"method not allowed",405);return}
 cfg,e:=s.readAISettings();if e!=nil{http.Error(w,"settings unavailable",500);return}
 if !cfg.Enabled {http.Error(w,"AI Assistant is disabled",403);return}
 aiPlanLock.Lock();defer aiPlanLock.Unlock()
 plans,e:=readAIPlans(s.Workspace);if e!=nil{http.Error(w,"AI plans unavailable",500);return}
 owner:=aiPlanOwner(r)
 if r.Method==http.MethodGet {
  filtered:=[]aiGuidedPlan{}
  for _,p:=range plans{if p.Owner==owner{filtered=append(filtered,p)}}
  w.Header().Set("Content-Type","application/json");w.Header().Set("Cache-Control","no-store")
  json.NewEncoder(w).Encode(map[string]any{"plans":filtered,"execution_enabled":false});return
 }
 r.Body=http.MaxBytesReader(w,r.Body,8192)
 var req aiPlanRequest
 decoder:=json.NewDecoder(r.Body);decoder.DisallowUnknownFields()
 if decoder.Decode(&req)!=nil{http.Error(w,"invalid plan request",400);return}
 if len(req.Steps)==0||len(req.Steps)>8 {http.Error(w,"plan must contain 1-8 steps",400);return}
 if len(req.Target.Host)>256||len(req.Target.CWD)>1024||len(req.Target.SessionID)>128{http.Error(w,"invalid plan target",400);return}
 verifiedTarget,verified:=s.attestAITerminal(w,r,req.Target.SessionID)
 if !verified{return}
 // Ignore client-provided host, CWD, and remote flags; only server metadata is authoritative.
 req.Target=verifiedTarget
 for i:=range req.Steps{
  step:=&req.Steps[i]
  if len(step.Command)==0||len(step.Command)>400||len(step.Reason)>700{http.Error(w,"invalid plan step",400);return}
  step.Command=scrubAIContext(step.Command,400)
  step.Reason=scrubAIContext(step.Reason,700)
  // Never rely on LLM-provided read-only/low risk labels for unknown commands.
  step.Risk=aiProposalRisk(step.Command,step.Risk)
  step.Decision="pending"
 }
 raw:=make([]byte,16);if _,e=rand.Read(raw);e!=nil{http.Error(w,"plan ID generation failed",500);return}
 now:=time.Now().UTC()
 plan:=aiGuidedPlan{ID:hex.EncodeToString(raw),Owner:owner,Target:req.Target,Steps:req.Steps,CreatedAt:now,UpdatedAt:now,ExpiresAt:now.Add(aiPlanApprovalTTL),ExecutionEnabled:false}
 plan.Digest=aiDigestForPlan(plan)
 plans=append(plans,plan);if len(plans)>60{plans=plans[len(plans)-60:]}
 if e=saveAIPlans(s.Workspace,plans);e!=nil{http.Error(w,"failed to save AI plan",500);return}
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 w.WriteHeader(http.StatusCreated);json.NewEncoder(w).Encode(plan)
}
func (s *Server) aiPlanDecisionAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodPost {http.Error(w,"method not allowed",405);return}
 r.Body=http.MaxBytesReader(w,r.Body,1024)
 var req aiPlanDecision
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if dec.Decode(&req)!=nil{http.Error(w,"invalid decision",400);return}
 if req.Decision!="approved"&&req.Decision!="rejected"{http.Error(w,"decision must be approved or rejected",400);return}
 aiPlanLock.Lock();defer aiPlanLock.Unlock()
 plans,e:=readAIPlans(s.Workspace);if e!=nil{http.Error(w,"plans unavailable",500);return}
 for i:=range plans{
  p:=&plans[i]
  if p.ID!=req.ID||p.Owner!=aiPlanOwner(r){continue}
  currentTarget,allowed:=s.attestAITerminal(w,r,p.Target.SessionID)
  if !allowed{return}
  if currentTarget!=p.Target{http.Error(w,"terminal target changed; plan is stale",http.StatusConflict);return}
  if p.ExpiresAt.IsZero()||!time.Now().Before(p.ExpiresAt){http.Error(w,"plan approval expired",http.StatusConflict);return}
  if p.Digest==""||req.Digest!=p.Digest||aiDigestForPlan(*p)!=p.Digest{
   http.Error(w,"plan digest mismatch; reload and review current plan",http.StatusConflict);return
  }
  if req.Step<0||req.Step>=len(p.Steps){http.Error(w,"invalid step",400);return}
  if p.Steps[req.Step].Decision!="pending"{http.Error(w,"step already decided",409);return}
  p.Steps[req.Step].Decision=req.Decision;p.UpdatedAt=time.Now().UTC()
  if e=saveAIPlans(s.Workspace,plans);e!=nil{http.Error(w,"unable to save decision",500);return}
  w.Header().Set("Content-Type","application/json")
  w.Header().Set("Cache-Control","no-store")
  json.NewEncoder(w).Encode(map[string]any{"plan":p,"execution_enabled":false})
  return
 }
 http.Error(w,"plan not found",404)
}
