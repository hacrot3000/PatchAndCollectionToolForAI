package server

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// The task plan is user-owned state. Provider text is never interpreted as an
// instruction to mark a milestone complete or change AutoContinue permission.
// Edits are owner/project scoped and revision-fenced against concurrent turns.
func (s *Server) agentChatTaskPlanAPI(w http.ResponseWriter,r *http.Request,id string){
 if r.Method!=http.MethodGet && r.Method!=http.MethodPost{
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
 }
 owner:=aiPlanOwner(r)
 var req struct {
  Action string `json:"action"`
  ExpectedRevision uint64 `json:"expected_revision"`
  Goal string `json:"goal,omitempty"`
  Steps []struct {
   ID string `json:"id"`
   Title string `json:"title"`
   Acceptance string `json:"acceptance"`
  } `json:"steps,omitempty"`
  StepID string `json:"step_id,omitempty"`
  Title string `json:"title,omitempty"`
  Acceptance string `json:"acceptance,omitempty"`
  Status string `json:"status,omitempty"`
  EvidenceKind string `json:"evidence_kind,omitempty"`
  EvidenceRef string `json:"evidence_ref,omitempty"`
  EvidenceNote string `json:"evidence_note,omitempty"`
  Enabled bool `json:"enabled,omitempty"`
  ConfirmExternal bool `json:"confirm_external,omitempty"`
 }
 if r.Method==http.MethodPost{
  r.Body=http.MaxBytesReader(w,r.Body,64<<10)
  dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
  if dec.Decode(&req)!=nil{http.Error(w,"invalid task plan request",400);return}
  var extra any
  if err:=dec.Decode(&extra);!errors.Is(err,io.EOF){
   http.Error(w,"one plan operation per request",400);return
  }
 }
 release:=func(){}
 if r.Method==http.MethodPost{
  unlock,ok:=lockAgentTurn(s.Workspace,owner,id)
  if !ok{http.Error(w,"Agent turn busy; retry plan edit after the current request",409);return}
  release=unlock
 }
 defer release()
 store,err:=s.agentChatStore()
 if err!=nil{http.Error(w,"session journal unavailable",500);return}
 x,err:=store.Get(id,owner)
 if errors.Is(err,agentchat.ErrNotFound){http.Error(w,"session not found",404);return}
 if err!=nil{http.Error(w,"session journal unavailable",500);return}
 if x.Workspace!=s.Workspace{http.Error(w,"workspace mismatch",403);return}
 if r.Method==http.MethodGet{
  done,total:=agentchat.TaskPlanProgress(x.TaskPlan)
  w.Header().Set("Cache-Control","no-store")
  writeJSON(w,200,map[string]any{"session_id":id,"revision":x.Revision,
   "plan":x.TaskPlan,"done":done,"total":total,"completed":x.TaskPlan!=nil&&x.TaskPlan.State=="completed"})
  return
 }
 if x.Revision!=req.ExpectedRevision{
  http.Error(w,"stale task plan revision",409);return
 }
 now:=time.Now().UTC()
 switch req.Action{
 case "create":
  if x.TaskPlan!=nil{http.Error(w,"task plan already exists",409);return}
  steps:=make([]agentchat.TaskPlanStep,0,len(req.Steps))
  for _,step:=range req.Steps {
   steps=append(steps,agentchat.TaskPlanStep{ID:step.ID,Title:step.Title,Acceptance:step.Acceptance})
  }
  plan,e:=agentchat.NewTaskPlan(req.Goal,steps)
  if e!=nil{http.Error(w,"invalid task plan",400);return}
  x.TaskPlan=plan
 case "add_step":
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{
   http.Error(w,"cannot add a step without an active plan",409);return
  }
  x.TaskPlan.Steps=append(x.TaskPlan.Steps,agentchat.TaskPlanStep{
   ID:req.StepID,Title:strings.TrimSpace(req.Title),Acceptance:strings.TrimSpace(req.Acceptance),
   Status:"pending",UpdatedAt:now})
  x.TaskPlan.UpdatedAt=now
 case "set_step":
  if x.TaskPlan==nil{http.Error(w,"create a task plan first",409);return}
  var proof *agentchat.TaskPlanEvidence
  if req.Status=="done"{
   verified,e:=agentchat.VerifyArchivedTaskPlanEvidence(store,x,req.EvidenceKind,req.EvidenceRef,req.EvidenceNote)
   if e!=nil{http.Error(w,"completion evidence not verified: "+e.Error(),409);return}
   proof=&verified
  }
  if e:=agentchat.UpdateTaskPlanStep(&x,req.StepID,req.Status,proof);e!=nil{
   http.Error(w,"invalid task plan step: "+e.Error(),409);return
  }
 case "set_auto_continue":
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{
   http.Error(w,"no active task plan",409);return
  }
  if req.Enabled{
   if !req.ConfirmExternal{
    http.Error(w,"explicit ongoing provider-sharing authorization required",403);return
   }
   if owner!="local"||s.Config.SharedServerEnabled{
    http.Error(w,"automatic plan continuation is local-owner only",403);return
   }
   if x.TaskPlan.State!="active"{http.Error(w,"resume plan before enabling automatic continuation",409);return}
   if agentchat.TaskPlanNext(x.TaskPlan)==nil{http.Error(w,"no actionable plan step remains",409);return}
  }
  x.TaskPlan.AutoContinue=req.Enabled
  x.TaskPlan.NoProgressTurns=0
  x.TaskPlan.LastWorkSignature=""
  x.TaskPlan.UpdatedAt=now
 case "pause":
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{
   http.Error(w,"no active task plan",409);return
  }
  x.TaskPlan.State="paused"
  x.TaskPlan.AutoContinue=false
  x.TaskPlan.UpdatedAt=now
 case "resume":
  if x.TaskPlan==nil||x.TaskPlan.State!="paused"{
   http.Error(w,"plan is not paused",409);return
  }
  x.TaskPlan.State="active"
  x.TaskPlan.UpdatedAt=now
 case "complete":
  if x.TaskPlan==nil||!agentchat.TaskPlanAllDone(x.TaskPlan){
   http.Error(w,"all steps must have completion evidence",409);return
  }
  unresolved:=agentchat.HasUnresolvedWork(x)
  for _,call:=range x.ToolCalls{
   if (call.Status=="done"||call.Status=="error"||call.Status=="rejected")&&
    (!call.FeedbackSent||call.FeedbackStatus!="acknowledged"){unresolved=true}
  }
  if unresolved||
   (x.ProviderRun!=nil&&(x.ProviderRun.Phase=="prepared"||x.ProviderRun.Phase=="sending"||x.ProviderRun.Phase=="unknown")){
   http.Error(w,"unresolved provider or native Patch work blocks task completion",409);return
  }
  x.TaskPlan.State="completed"
  x.TaskPlan.AutoContinue=false
  x.TaskPlan.UpdatedAt=now
 default:
  http.Error(w,"unsupported task plan action",400);return
 }
 if err:=agentchat.ValidateTaskPlan(x.TaskPlan);err!=nil{
  http.Error(w,"invalid task plan state",400);return
 }
 x.AppendEvent("plan_updated",x.Status,"",req.Action)
 x,err=store.Save(x,x.Revision)
 if errors.Is(err,agentchat.ErrStaleRevision){http.Error(w,"stale task plan revision",409);return}
 if err!=nil{http.Error(w,"unable to persist task plan",409);return}
 done,total:=agentchat.TaskPlanProgress(x.TaskPlan)
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,200,map[string]any{"session_id":id,"revision":x.Revision,
  "plan":x.TaskPlan,"done":done,"total":total,"completed":x.TaskPlan!=nil&&x.TaskPlan.State=="completed"})
}
