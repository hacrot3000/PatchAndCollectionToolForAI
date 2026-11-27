package server

import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// This is the no-plan companion to the user-owned Task plan coordinator.
// The user must separately opt in from the conversation Automation menu.
// Provider narration is scheduling advice only: the existing message API
// remains the authority for network consent and native tool restrictions.
func (s *Server) autoContinueConversationOne(ctx context.Context,store *agentchat.Store,id string){
 if ctx.Err()!=nil||s==nil||s.Config.SharedServerEnabled{return}
 if s.agentConversationRequiresRunContinuation("local",id){return}
 original,err:=store.Get(id,"local")
 if err!=nil||original.Workspace!=s.Workspace||
  !agentchat.StandaloneAutoContinueCandidate(original){return}
 original.Continuation=agentchat.EffectiveTurnContinuation(original)
 c:=original.Continuation
 requestID:=fmt.Sprintf("conversation-%s-r%d",id,original.Revision)
 action:=strings.TrimSpace(c.NextAction)
 if action==""{
  action="Inspect the next missing requirement using read-only Collection or state a precise blocker."
 }
 message:=fmt.Sprintf(
  "TaskDeck user-authorized continuation of the ORIGINAL unfinished conversation, "+
   "NOT a new task. The previous provider turn is %s (advisory only). "+
   "Its proposed next step is %q. Verify this against the ORIGINAL user goal "+
   "and actual native receipts; inspect the claimed checkpoint with read-only "+
   "Collection before relying on it. Do not repeat a generic progress report. "+
   "Do the next feasible concrete action and report actual evidence. "+
   "Never claim success without independent proof; never bypass native "+
   "Patch approval or reissue an ambiguous tool request.",
   c.Disposition,action)
 if agentchat.UnknownCollectionProgressRecheck(original){
  message="TaskDeck user-authorized task status check for the ORIGINAL conversation. The last native COLLECT result has already been delivered and the provider reply did not establish task completion or a next action. Do not repeat the COLLECT or execute any tool in this status check. Review the original user goal and existing evidence, then return exactly one fenced taskdeck-progress JSON block. If work remains, use IN_PROGRESS or NEEDS_CONTINUATION with a concrete next_action. If the goal is fully satisfied, use CLAIMED_COMPLETE. If genuinely blocked, use BLOCKED with the precise reason in next_action. Do not invent evidence or claim completion from transport success."
 }
 if c.NoProgressTurns>=2&&!agentchat.UnknownCollectionProgressRecheck(original){
  message+=" No independently verified work has been recorded in the last "+
   "two automatic turns. Prioritize a specific read-only Collection now; "+
   "if one is impossible, report the precise blocker. Do not recycle "+
   "the same advice or a newly worded status update."
 }
 body,err:=json.Marshal(map[string]any{
  "message":message,"confirm_external":true,
  "expected_revision":original.Revision,"request_id":requestID,
 })
 if err!=nil{return}
 req:=httptest.NewRequest(http.MethodPost,
  "/api/agent/sessions/"+id+"/messages",bytes.NewReader(body)).WithContext(context.WithValue(collectionContinuationContext(ctx),standaloneContinuationContextKey{},c.ResponseDigest))
 req.Header.Set("Content-Type","application/json")
 w:=httptest.NewRecorder()
 s.agentChatMessageAPI(w,req,id)
 if w.Code!=http.StatusOK{
  if ctx.Err()==nil{s.deferStandaloneAdmission(store,id,w.Code)}
  return
 }
 release,ok:=lockAgentTurn(s.Workspace,"local",id)
 if !ok{return}
 defer release()
 current,err:=store.Get(id,"local")
 if err!=nil||!current.AutoContinueText||current.Continuation==nil||
  current.Status!="completed"{return}
 c=current.Continuation
 after:=agentchat.VerifiedWorkSignature(current)
 prior:=c.LastWorkSignature
 if c.LastContinuationKey==""{prior=""}
 reference:=agentchat.ContinuationWorkReference(original,current,prior,c.LastWorkSignatureVersion)
 if after==reference {c.NoProgressTurns++}else{c.NoProgressTurns=0}
 c.LastWorkSignature=after
 c.LastWorkSignatureVersion=agentchat.WorkSignatureVersion(current)
 c.LastContinuationKey=requestID
 c.RetryStreak=0
 c.NextRetryAt=nil
 if c.NoProgressTurns>=agentchat.MaxContinuationNoWorkTurns{
  current.AutoContinueText=false
  c.Reason="stalled_without_verified_work"
  current.AppendEvent("continuation_stalled",current.Status,"","no_verified_work_after_recovery")
 }else if c.Disposition==agentchat.TurnBlocked{
  current.AutoContinueText=false
  current.AppendEvent("continuation_blocked",current.Status,"","model_reported_blocker_requires_review")
 }else{
  current.AppendEvent("continuation_advanced",current.Status,"",c.Disposition)
 }
 _,_=store.Save(current,current.Revision)
}

type standaloneContinuationContextKey struct{}

// A scheduler admission failure may be retried only while the prior request
// was rejected before provider egress. SENDING/UNKNOWN are never replayed.
func (s *Server) deferStandaloneAdmission(store *agentchat.Store,id string,httpCode int){
 release,ok:=lockAgentTurn(s.Workspace,"local",id)
 if !ok{return}
 defer release()
 x,err:=store.Get(id,"local")
 if err!=nil||!x.AutoContinueText||x.Continuation==nil||x.Status!="completed"{return}
 c:=x.Continuation
 switch httpCode {
 case http.StatusTooManyRequests,http.StatusInternalServerError,http.StatusServiceUnavailable:
  if c.RetryStreak<8{c.RetryStreak++}
  shift:=c.RetryStreak-1
  if shift>6{shift=6}
  delay:=10*time.Second*time.Duration(1<<shift)
  if delay>10*time.Minute{delay=10*time.Minute}
  until:=time.Now().UTC().Add(delay)
  c.NextRetryAt=&until
  x.AppendEvent("continuation_deferred",x.Status,"",fmt.Sprintf("http_%d_retry_after_%ds",httpCode,int(delay.Seconds())))
 case http.StatusConflict:
  return
 default:
  x.AutoContinueText=false
  c.Reason="nonretryable_admission_failure"
  x.AppendEvent("continuation_blocked",x.Status,"",fmt.Sprintf("http_%d_requires_review",httpCode))
 }
 _,_=store.Save(x,x.Revision)
}
