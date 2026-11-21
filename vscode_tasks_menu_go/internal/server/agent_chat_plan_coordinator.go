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
  // Admission pressure, a revoked grant or provider failure must not be
  // disguised as a completed milestone. Session status and the durable
  // provider egress ledger remain the authority for safe recovery.
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
