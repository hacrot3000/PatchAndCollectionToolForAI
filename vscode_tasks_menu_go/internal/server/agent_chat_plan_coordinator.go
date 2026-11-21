package server

import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Automatic plan continuation is strictly an explicit local-user opt-in.
// It invokes the SAME message API and its provider consent, project identity,
// idempotency, native Patch approval and egress journal; there is no second
// model executor and no tool-result re-send path.
func (s *Server) autoContinueTaskPlanOne(ctx context.Context,store *agentchat.Store,id string){
 if ctx.Err()!=nil||s==nil||s.Config.SharedServerEnabled{return}
 original,err:=store.Get(id,"local")
 if err!=nil||original.Workspace!=s.Workspace||
  !agentchat.TaskPlanAutoContinueCandidate(original){return}
 step:=agentchat.TaskPlanNext(original.TaskPlan)
 if step==nil{return}
 before:=agentchat.VerifiedWorkSignature(original)
 requestID:=fmt.Sprintf("plan-%s-r%d",step.ID,original.Revision)
 message:=fmt.Sprintf(
  "TaskDeck user-authorized automatic continuation of the ORIGINAL task plan. "+
   "Next unmet milestone %q: %s. Acceptance criteria: %s. "+
   "Inspect the workspace with read-only Collection if needed; propose only "+
   "reviewable Native Patches under existing approval permissions. Do not "+
   "declare the plan complete or repeat the same answer without new evidence. "+
   "If blocked, explain the exact missing permission/decision.",
  step.ID,step.Title,step.Acceptance)
 body,err:=json.Marshal(map[string]any{
  "message":message,"confirm_external":true,
  "expected_revision":original.Revision,"request_id":requestID,
 })
 if err!=nil{return}
 req:=httptest.NewRequest(http.MethodPost,
  "/api/agent/sessions/"+id+"/messages",bytes.NewReader(body)).WithContext(ctx)
 req.Header.Set("Content-Type","application/json")
 w:=httptest.NewRecorder()
 // No session lock here: agentChatMessageAPI owns it and performs a
 // compare-and-swap before every provider request. Other scheduler ticks
 // are deduplicated by the bounded per-session background worker pool.
 s.agentChatMessageAPI(w,req,id)
 if w.Code!=http.StatusOK{
  // Admission backoff is safe only if the request was rejected BEFORE
  // provider egress. Any ambiguous provider response already changes
  // the canonical session status and cannot be scheduled again.
  if ctx.Err()==nil{s.deferTaskPlanAdmission(store,id,w.Code)}
  return
 }
 release,acquired:=lockAgentTurn(s.Workspace,"local",id)
 if !acquired{return}
 defer release()
 current,err:=store.Get(id,"local")
 if err!=nil||current.TaskPlan==nil||current.TaskPlan.State!="active"||
  !current.TaskPlan.AutoContinue{return}
 p:=current.TaskPlan
 after:=agentchat.VerifiedWorkSignature(current)
 // A verified read or Patch may have been adopted by the background
 // reconciler between scheduled turns. Compare to the previous durable
 // signature, not only the state immediately before this model call.
 reference:=p.LastWorkSignature
 if p.LastContinuationKey==""{reference=before}
 if after==reference {
  p.NoProgressTurns++
 }else{
  p.NoProgressTurns=0
 }
 p.LastWorkSignature=after
 p.LastContinuationKey=requestID
 p.RetryStreak=0
 p.NextRetryAt=nil
 if p.NoProgressTurns>=3{
  // This is a safety brake against recurring billable text-only answers,
  // not a limit on legitimate thousands of Collection/Patch operations.
  p.State="paused"
  p.AutoContinue=false
  current.AppendEvent("plan_stalled",current.Status,"","three_turns_without_verified_work")
 }else{
  current.AppendEvent("plan_continued",current.Status,"",step.ID)
 }
 p.UpdatedAt=time.Now().UTC()
 _,_=store.Save(current,current.Revision)
}

// Admission failures occur before the model provider is contacted. Retry
// them with a durable exponential delay instead of hitting the scheduler
// every three seconds. Post-egress failures are NEVER retried from here.
func (s *Server) deferTaskPlanAdmission(store *agentchat.Store,id string,httpCode int){
 release,ok:=lockAgentTurn(s.Workspace,"local",id)
 if !ok{return}
 defer release()
 x,err:=store.Get(id,"local")
 if err!=nil||x.TaskPlan==nil||x.TaskPlan.State!="active"||
  !x.TaskPlan.AutoContinue||x.Status!="completed"{return}
 p:=x.TaskPlan
 if httpCode==429||httpCode==500||httpCode==503{
  if p.RetryStreak<8{p.RetryStreak++}
  shift:=p.RetryStreak-1
  if shift>6{shift=6}
  delay:=10*time.Second*time.Duration(1<<shift)
  if delay>10*time.Minute{delay=10*time.Minute}
  until:=time.Now().UTC().Add(delay)
  p.NextRetryAt=&until
  x.AppendEvent("plan_deferred",x.Status,"",fmt.Sprintf("http_%d_retry_after_%ds",httpCode,int(delay.Seconds())))
 }else if httpCode!=409{
  // Invalid or unauthorized plan requests need an explicit user decision.
  // Never keep retrying an impossible prompt or exhausted context.
  p.AutoContinue=false
  p.State="paused"
  x.AppendEvent("plan_blocked",x.Status,"",fmt.Sprintf("http_%d_requires_review",httpCode))
 }else{
  return
 }
 p.UpdatedAt=time.Now().UTC()
 _,_=store.Save(x,x.Revision)
}
