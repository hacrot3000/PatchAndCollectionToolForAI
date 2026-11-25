package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

// Opt-in automatic continuation. Consent is recorded per session (not implied
// by a provider key) and is consumed exactly once per completed tool call.
// The scheduler never queues, starts, selects or retries a patch; it only
// delivers a verified native result as function_call_output with the original
// call_id, and only while its bounded grant is valid.
const (
	agentAutoFeedbackMaxAge = 30 * time.Minute
	agentAutoFeedbackPoll   = 3 * time.Second
)

// Machine denials are protocol feedback, not approval or task evidence.
// Only explicitly opted-in continuation may schedule them in the absence
// of the separate Auto-send results grant. Never extend this to user denial.
func agentRepairableToolDenial(call agentchat.ToolCallRecord) bool {
	if call.Status != "rejected" || call.Result == "" { return false }
	return agentRepairableDenialOutput(call.Result)
}

func agentRepairableDenialOutput(output string) bool {
	var receipt struct {
		Status string `json:"status"`
		Reason string `json:"reason_code"`
		Retryable bool `json:"retryable_by_ai"`
		Queued bool `json:"patch_queued"`
		Executed bool `json:"patch_executed"`
	}
	if json.Unmarshal([]byte(output), &receipt) != nil || receipt.Status != "REJECTED" ||
		!receipt.Retryable || receipt.Queued || receipt.Executed { return false }
	switch receipt.Reason {
	case "UNSUPPORTED_TOOL", "MULTIPLE_TOOL_CALLS", "INVALID_PATCH_ARGUMENTS", "PATCH_APPROVAL_STALE":
		return true
	default:
		return false
	}
}

// Bound consecutive denial-only follow-ups, regardless of changes in tool
// names/call IDs. Real user input or a successful native observation starts
// a new repair window. Never silently replay a previously sent denial.
const agentMaxConsecutiveDenials = 3

func agentConsecutiveDenialOutputs(history []agentchat.HistoryItem) int {
	count := 0
	for i := len(history)-1; i >= 0; i-- {
		item := history[i]
		switch item.Type {
		case "user":
			// The legacy JSON Collection fallback stores a native observation
			// as user content; it is also a legitimate new inspection round.
			return count
		case "function_call_output":
			if !agentRepairableDenialOutput(item.Content) { return count }
			count++
		}
	}
	return count
}

func agentDenialAutoContinueGranted(x agentchat.Session, call agentchat.ToolCallRecord) bool {
	if x.Owner != "local" || x.Archived || !agentRepairableToolDenial(call) { return false }
	if x.TaskPlan != nil {
		p := x.TaskPlan
		return p.State == "active" && p.AutoContinue && agentchat.TaskPlanNext(p) != nil &&
			p.NoProgressTurns < agentchat.MaxContinuationNoWorkTurns
	}
	return x.AutoContinueText && (x.Continuation == nil || x.Continuation.NoProgressTurns < agentchat.MaxContinuationNoWorkTurns)
}

func agentFeedbackGrantForCall(x agentchat.Session, call agentchat.ToolCallRecord) bool {
	granted := x.AutoFeedback && (x.AutoFeedbackPersistent ||
		(x.AutoFeedbackUntil != nil && !time.Now().UTC().After(*x.AutoFeedbackUntil)))
	if agentRepairableToolDenial(call) && agentConsecutiveDenialOutputs(x.History) >= agentMaxConsecutiveDenials {
		return false
	}
	return granted || call.SendAfterRetry || agentDenialAutoContinueGranted(x, call)
}

// agentAutoFeedbackCandidate reports whether a session currently qualifies.
// It is a pure read used by both the scheduler and tests.
func agentAutoFeedbackCandidate(x agentchat.Session) (int, bool) {
 idx,reason:=agentAutoFeedbackAdmission(x)
 return idx,reason==""
}

// Single source of truth for scheduler eligibility and the UI/Debug reason.
func agentAutoFeedbackAdmission(x agentchat.Session)(int,string){
 if x.Archived{return 0,"conversation_archived"}
 if x.ProviderRun!=nil{
  switch x.ProviderRun.Phase{case "prepared","sending","unknown":return 0,"provider_delivery_unresolved"}
 }
 if x.Status!="waiting_tool"&&x.Status!="tool_rejected"{return 0,"not_awaiting_tool_result"}
 for _,call:=range x.ToolCalls{
  switch call.Status{case "queuing","running","dispatching","queued","unknown":return 0,"native_work_unresolved"}
  if call.FeedbackStatus=="prepared"||call.FeedbackStatus=="sending"||
   (call.FeedbackStatus=="unknown"&&!agentchat.HistoricalUnconfirmedFeedback(x,call)){return 0,"current_feedback_unconfirmed"}
 }
 recorded:=map[string]bool{}
 for _,item:=range x.History{if item.Type=="function_call_output"&&item.CallID!=""{recorded[item.CallID]=true}}
 for i,call:=range x.ToolCalls{
  if call.FeedbackSent||recorded[call.CallID]||!agentFeedbackCallIsCurrent(x,call.CallID){continue}
  switch call.Status{
  case "done","error","rejected":
   if call.FeedbackStatus!=""{return 0,"feedback_already_attempted"}
   if !agentFeedbackGrantForCall(x,call){return 0,"auto_feedback_not_authorized_or_repair_limit"}
   return i,""
  default:return 0,"native_work_unresolved"
  }
 }
 return 0,"no_current_pending_result"
}

func agentAutoFeedbackDiagnostic(x agentchat.Session)map[string]any{
 idx,reason:=agentAutoFeedbackAdmission(x)
 if reason==""&&x.Owner!="local"{reason="explicit_owner_action_required"}
 out:=map[string]any{"eligible":reason=="","reason":reason,"auto_feedback":x.AutoFeedback,"persistent":x.AutoFeedbackPersistent}
 if x.AutoFeedbackUntil!=nil{out["grant_until"]=x.AutoFeedbackUntil}
 if x.ProviderRun!=nil{out["provider_phase"]=x.ProviderRun.Phase}
 if reason==""{out["call_id"]=x.ToolCalls[idx].CallID}
 return out
}

// runAgentAutoFeedback performs one bounded pass. Each session is processed
// under its turn lock with a fresh owner/project/provider authorization.
func (s *Server) runAgentAutoFeedback(ctx context.Context) {
	if s == nil || s.Workspace == "" {
		return
	}
	store, err := s.agentChatStore()
	if err != nil {
		return
	}
	// The scheduler only runs for the single-user local owner. Shared-server
	// owners must opt in and continue explicitly; an implicit background pass
	// must never egress on behalf of a principal it cannot re-authenticate.
	sessions, err := store.List("local")
	if err != nil {
		return
	}
	for _, sess := range sessions {
		if ctx.Err() != nil { return }
		if sess.Owner != "local" || sess.Archived || !agentAutoFeedbackNeedsWork(sess) { continue }
		s.processAgentAutoFeedbackSession(ctx, store, sess.ID)
	}
}

// processAgentAutoFeedbackSession remains synchronous for the single-pass
// reconciler and tests. The live scheduler runs this behind a bounded,
// per-session worker guard so one multi-day AI continuation cannot starve
// all other conversations.
func (s *Server) processAgentAutoFeedbackSession(ctx context.Context, store *agentchat.Store, id string) {
	if ctx.Err() != nil { return }
	// Resume an exact legacy PREPARED request using its durable provider
	// attempt record. Never replay SENDING/UNKNOWN.
	if s.resumePreparedAgentProviderRun(ctx,store,id){return}
	// Preserve the provider-bound run fence, but a historical completed or
	// failed run must not strand a later, independently admitted legacy Patch.
	if s.agentConversationRequiresRunContinuation("local", id) {
		x, err := store.Get(id, "local")
		if err == nil && len(x.History) != 0 {
			last := x.History[len(x.History)-1]
			if s.agentLegacyPatchSchedulerAllowed(x, last.CallID) {
				s.autoPatchOne(ctx, store, id)
				s.reconcileAgentSession(ctx, store, id)
				s.autoFeedbackOne(ctx, store, id)
			}
		}
		s.autoCheckTaskPlanOne(ctx, store, id)
		return
	}
	// Auto Patch uses exactly the existing revision-fenced native approval
	// gateway; it must not reexecute an ambiguous native Patch.
	s.recoverLegacyStalePatch(ctx, store, id)
	s.autoPatchOne(ctx, store, id)
	// Adopt verified native final results without requiring a browser poll.
	s.reconcileAgentSession(ctx, store, id)
	if s.autoResumeRecoveredCollectionOne(ctx,store,id){return}
	fresh, err := store.Get(id, "local")
	if err != nil { return }
	if _, ok := agentAutoFeedbackCandidate(fresh); ok {
		s.autoFeedbackOne(ctx, store, id)
	}
	// Check exact user-authorized Native Patch/Collection receipt rules
	// before advancing to the next unfinished milestone.
	s.autoCheckTaskPlanOne(ctx,store,id)
	// A finished model turn is not an accepted whole plan. Continue only
	// when the LOCAL user explicitly enabled automatic plan continuation.
	s.autoContinueTaskPlanOne(ctx,store,id)
	// Explicit owner opt-in also supports unfinished conversations without plans.
	s.autoContinueConversationOne(ctx,store,id)
}

func agentAutoFeedbackNeedsWork(x agentchat.Session) bool {
	if x.Owner != "local" || x.Archived { return false }
	if x.Status=="requesting_ai" && x.ProviderRun!=nil && x.ProviderRun.Phase=="prepared"{return true}
	if agentRecoveredCollectionResumeCandidate(x){return true}
	if agentchat.TaskPlanAutoContinueCandidate(x){return true}
	if agentchat.StandaloneAutoContinueCandidate(x){return true}
	if agentchat.TaskAutoCheckReady(x){return true}
	if _, ok := agentAutoPatchCandidate(x); ok { return true }
	if _, ok := agentAutoFeedbackCandidate(x); ok { return true }
	if _, ok := agentRecoverableRejection(x, ""); ok { return true }
 // A copy prevents the readiness predicate from mutating the journal.
 if x.ProviderRun!=nil{run:=*x.ProviderRun;x.ProviderRun=&run;x.Events=append([]agentchat.SessionEvent(nil),x.Events...);if agentRecoverAcknowledgedCollection(&x){return true}}
	// A native operation can finish while the GUI is closed; it still
	// requires reconciliation, even when auto-feedback was not selected.
	for _, call := range x.ToolCalls {
		switch call.Status {
		case "queuing", "running", "unknown", "dispatching", "queued":
			return true
		}
	}
	return false
}

// Historical Run Manager records are not ownership of a NEW legacy call.
// Only permit the exception for a unique current legacy Patch with matching
// canonical arguments, no unresolved run work, and no run-owned call identity.
// An unavailable ledger must still fail closed. Do not use this exception for
// arbitrary text continuation or replay of a provider-bound run.
func agentLegacyPatchWithRunHistory(x agentchat.Session, records []agentrun.Record, callID string) bool {
	if x.Owner != "local" || x.Archived || callID == "" || len(x.History) == 0 {
		return false
	}
	last := x.History[len(x.History)-1]
	if last.Type != "function_call" || last.Name != "taskdeck_submit_patch" ||
		last.CallID != callID || last.OriginRunID != "" {
		return false
	}
	matches := 0
	for _, call := range x.ToolCalls {
		if call.CallID != callID { continue }
		if call.AgentRunID != "" || call.Name != last.Name ||
			call.ArgumentsSHA256 == "" || call.ArgumentsSHA256 != agentPatchPayloadSHA256(last.Arguments) {
			return false
		}
		matches++
	}
	if matches != 1 { return false }
	for _, item := range x.History[:len(x.History)-1] {
		if item.CallID == callID && (item.Type == "function_call" || item.Type == "function_call_output") {
			return false
		}
	}
	for _, run := range records {
		switch run.Status {
		case agentrun.Completed, agentrun.Failed, agentrun.Stopped, agentrun.Interrupted:
		default:
			return false
		}
		if hasUnresolvedRunToolCalls(run) { return false }
		for _, item := range run.History {
			if item.CallID == callID { return false }
		}
	}
	return true
}

func (s *Server) agentLegacyPatchSchedulerAllowed(x agentchat.Session, callID string) bool {
	for _, call := range x.ToolCalls {
		if call.CallID == callID && call.AgentRunID != "" { return false }
	}
	records, err := s.completedAgentRuns(x.Owner, x.ID)
	if err != nil { return false }
	return len(records) == 0 || agentLegacyPatchWithRunHistory(x, records, callID)
}

// Auto Patch performs a single exact-call approval under the existing native
// workspace lease, identity validation, revision fence and idempotency marker.
// No automatic retry is attempted after an ambiguous queue/dispatch result.
// A one-shot user-approved reapply is independent from persistent Auto Patch.
func agentAutoApprovalAuthorized(x agentchat.Session,callID string) bool {
 if x.AutoPatch {return true}
 for _,call:=range x.ToolCalls {
  if call.CallID==callID {return call.Status=="pending" && call.AutoRunOnce && !call.ManualRetryOnly}
 }
 return false
}

func agentAutoPatchCandidate(x agentchat.Session) (string, bool) {
	if x.Owner != "local" || x.Archived || x.Status != "waiting_tool" {
		return "",false
	}
	if x.ProviderRun != nil {
		switch x.ProviderRun.Phase {
		case "prepared", "sending", "unknown":
			return "", false
		}
	}
	for _, call := range x.ToolCalls {
		switch call.Status {
		case "queuing", "queued", "dispatching", "running", "unknown":
			return "", false
		}
	}
	for _, call := range x.ToolCalls {
		if call.Name == "taskdeck_submit_patch" && call.AgentRunID == "" &&
			call.Status == "pending" && !call.FeedbackSent && call.FeedbackStatus == "" &&
			!call.ManualRetryOnly && (x.AutoPatch || call.AutoRunOnce) &&
			agentFeedbackCallIsCurrent(x, call.CallID) {
			return call.CallID, true
		}
	}
	return "", false
}
func (s *Server) autoPatchOne(ctx context.Context, store *agentchat.Store, id string) {
	if s.Config.SharedServerEnabled || ctx.Err()!=nil {return}
	// Do not hold the turn lock: the existing approval handler acquires it,
	// then records the QUEUING marker before the native file-side effect.
	x,err:=store.Get(id,"local")
	if err!=nil || x.Workspace!=s.Workspace {return}
	callID,ok:=agentAutoPatchCandidate(x)
	if !ok {return}
	if !s.agentLegacyPatchSchedulerAllowed(x, callID) { return }
	body,err:=json.Marshal(map[string]any{
		"action":"approve","automatic":true,
		"confirm_execution":true,"expected_revision":x.Revision,
	})
	if err!=nil {return}
	req:=httptest.NewRequest(http.MethodPost,
		"/api/agent/sessions/"+id+"/tools/"+callID+"/decision",
		bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type","application/json")
	w:=httptest.NewRecorder()
	s.agentChatToolDecisionAPI(w,req,id,callID)
	if w.Code!=http.StatusOK {
        // Do not repeat a refused one-shot approval every scheduler tick.
        release,locked:=lockAgentTurn(s.Workspace,"local",id)
        if locked {
          current,e:=store.Get(id,"local")
          if e==nil {
           for i:=range current.ToolCalls {
            if current.ToolCalls[i].CallID==callID && current.ToolCalls[i].Status=="pending" && current.ToolCalls[i].AutoRunOnce {
             current.ToolCalls[i].AutoRunOnce=false
             current.ToolCalls[i].SendAfterRetry=false
             current.ToolCalls[i].ManualRetryOnly=true
             _,_=store.Save(current,current.Revision)
            }
           }
          }
          release()
        }
		s.recordAgentDebugEvent("local",id,callID,"tool","tool.auto_patch_blocked",
			"AUTO_PATCH_FAILED","Auto Patch was refused or became stale; no automatic retry")
	}
}

// reconcileAgentSession adopts final evidence directly from the native
// Session Service while holding the same per-session lock used by approval,
// sync and feedback. It never starts, selects, cancels or repeats a patch.
func (s *Server) reconcileAgentSession(ctx context.Context, store *agentchat.Store, id string) {
	release, ok := lockAgentTurn(s.Workspace, "local", id)
	if !ok {
		return
	}
	defer release()
	x, err := store.Get(id, "local")
	if err != nil || x.Owner != "local" || x.Workspace != s.Workspace {
		return
	}
	changed := agentRecoverAcknowledgedCollection(&x)
	if agentRestoreRejectedSession(&x, "") {changed=true}
	if agentParkInterruptedQueue(&x) { changed = true }
	for i, record := range x.ToolCalls {
		if ctx.Err() != nil {
			return
		}
		if record.Status != "running" && record.Status != "unknown" && record.Status != "dispatching" {
			continue
		}
		view := agentToolReconcileView{Record: record}
		s.agentReconcilePatchRecord(&view)
		if view.Changed {
			x.ToolCalls[i] = view.Record
			if view.Record.Status=="done" || view.Record.Status=="error" {x.Status="waiting_tool"}
			x.AppendEvent("tool_final", view.Record.Status, record.CallID, "background_reconcile")
			changed = true
		}
	}
	if changed {
		_, _ = store.Save(x, x.Revision)
	}
}

func (s *Server) autoFeedbackOne(ctx context.Context, store *agentchat.Store, id string) {
	release, ok := lockAgentTurn(s.Workspace, "local", id)
	if !ok {
		return
	}
	defer release()
	x, err := store.Get(id, "local")
	if err != nil || x.Archived || x.Owner != "local" {
		return
	}
	// Re-validate the candidate under the lock: another turn may have run.
	idx, still := agentAutoFeedbackCandidate(x)
	if !still || !s.agentLegacyPatchSchedulerAllowed(x, x.ToolCalls[idx].CallID) {
		return
	}
	// Consent must still be valid at the moment of egress.
	if s.Config.SharedServerEnabled || !agentFeedbackGrantForCall(x, x.ToolCalls[idx]) {
		return
	}
	if _, err = s.agentDeliverToolResult(ctx, store, x, idx, true); err != nil {
		var sc agentContinueError
		if errors.As(err, &sc) && sc.status == 429 {
			return // transient admission pressure; a later pass retries safely
		}
		// Pre-egress failures need a visible reason; never claim delivery or
		// retry native execution. Post-egress failed/unknown is already durable.
		if errors.As(err,&sc){
			s.recordAgentDebugEvent("local",id,x.ToolCalls[idx].CallID,"feedback","feedback.admission_blocked","AUTO_FEEDBACK_BLOCKED",sc.message)
		}
	}
}

// An AutoFeedback turn may run hundreds of Collections or await an unlimited
// provider retry for hours or days. Never hold the sole scheduler goroutine
// inside that turn: other conversations must still be reconciled and resumed.

const agentAutoFeedbackWorkerLimit = 4

// Keep process-local fairness cursors bounded even if TaskDeck opens many
// workspaces over multiple days. This is not a distributed scheduler lease.
const maxAgentAutoFeedbackCursorScopes = 128

// Pending scopes are refreshed by their existing three-second scheduler
// polls. A scope that stops polling must not block all other work indefinitely.
const agentAutoFeedbackPendingScopeTTL = 6 * agentAutoFeedbackPoll

type agentAutoFeedbackWorkerPool struct {
 mu sync.Mutex
 active map[string]struct{}
 lastAdmitted map[string]string
 cursorOrder []string
 // Global, process-local workspace round robin. A scope appears here only
 // while it has eligible sessions not already occupying a worker.
 pendingScopes map[string]time.Time
 lastAdmittedScope string
}

var agentAutoFeedbackWorkers = agentAutoFeedbackWorkerPool{active: map[string]struct{}{}}

// Native-only workers must not compete with long provider retry loops.
// They never deliver feedback or issue a provider request.
var agentAutoPatchWorkers = agentAutoFeedbackWorkerPool{active: map[string]struct{}{}}

func (p *agentAutoFeedbackWorkerPool) acquire(key string) bool {
 p.mu.Lock()
 defer p.mu.Unlock()
 if p.active==nil{p.active=map[string]struct{}{}}
 if _,exists:=p.active[key];exists||len(p.active)>=agentAutoFeedbackWorkerLimit{return false}
 p.active[key]=struct{}{}
 return true
}

func (p *agentAutoFeedbackWorkerPool) release(key string) {
 p.mu.Lock()
 delete(p.active,key)
 p.mu.Unlock()
}

// acquireFairBatch provides two-level process-local fairness: workspaces
// alternate for free slots, and sessions alternate within each workspace.
// Calls may arrive from independent workspace scheduler goroutines. A waiting
// scope gets a turn on its next poll; an aggressively polling scope cannot
// repeatedly take its reserved turn. This is not a durable cross-daemon queue.
func (p *agentAutoFeedbackWorkerPool) acquireFairBatch(scope string,candidates []string)[]string{
 if scope==""{return nil}
 ids:=append([]string(nil),candidates...)
 sort.Strings(ids)
 p.mu.Lock()
 defer p.mu.Unlock()
 if p.active==nil{p.active=map[string]struct{}{}}
 if p.lastAdmitted==nil{p.lastAdmitted=map[string]string{}}
 if p.pendingScopes==nil{p.pendingScopes=map[string]time.Time{}}
 now:=time.Now()
 for name,seen:=range p.pendingScopes{
  if now.Sub(seen)>agentAutoFeedbackPendingScopeTTL{delete(p.pendingScopes,name)}
 }
 // Never register a scope whose only eligible conversations already have
 // in-flight workers. An empty poll explicitly clears a stale waiter.
 ready:=false
 for _,id:=range ids{
  if _,busy:=p.active[scope+":"+id];!busy{ready=true;break}
 }
 if !ready{
  delete(p.pendingScopes,scope)
  return nil
 }
 if _,exists:=p.pendingScopes[scope];!exists&&len(p.pendingScopes)>=maxAgentAutoFeedbackCursorScopes{
  // Bound memory. Prefer dropping the stalest waiting scope; it can
  // re-register on its next normal scheduler tick.
  oldest:=""
  var oldestAt time.Time
  for name,seen:=range p.pendingScopes{
   if oldest==""||seen.Before(oldestAt)||(seen.Equal(oldestAt)&&name<oldest){
    oldest=name;oldestAt=seen
   }
  }
  delete(p.pendingScopes,oldest)
 }
 p.pendingScopes[scope]=now
 if len(p.active)>=agentAutoFeedbackWorkerLimit{return nil}

 // Admit the lexicographically next waiting workspace after the last
 // successful allocation. Busy/updated-first Store.List order is irrelevant.
 scopes:=make([]string,0,len(p.pendingScopes))
 for name:=range p.pendingScopes{scopes=append(scopes,name)}
 sort.Strings(scopes)
 next:=sort.Search(len(scopes),func(i int)bool{return scopes[i]>p.lastAdmittedScope})
 if next==len(scopes){next=0}
 if scopes[next]!=scope{return nil}

 last:=p.lastAdmitted[scope]
 first:=sort.Search(len(ids),func(i int)bool{return ids[i]>last})
 capacity:=agentAutoFeedbackWorkerLimit-len(p.active)
 // When other workspaces are waiting, reserve at most one worker this
 // turn. Another workspace receives the next available slot on its poll.
 if len(scopes)>1&&capacity>1{capacity=1}
 reserved:=make([]string,0,capacity)
 for i:=0;i<len(ids)&&len(reserved)<capacity;i++{
  id:=ids[(first+i)%len(ids)]
  key:=scope+":"+id
  if _,running:=p.active[key];running{continue}
  p.active[key]=struct{}{}
  reserved=append(reserved,id)
  if _,known:=p.lastAdmitted[scope];!known{
   if len(p.cursorOrder)>=maxAgentAutoFeedbackCursorScopes{
    oldest:=p.cursorOrder[0]
    delete(p.lastAdmitted,oldest)
    p.cursorOrder=p.cursorOrder[1:]
   }
   p.cursorOrder=append(p.cursorOrder,scope)
  }
  p.lastAdmitted[scope]=id
  p.lastAdmittedScope=scope
 }
 // Do not let an exhausted scope monopolize the global turn. The next
 // poll will register it again if a worker completes and work remains.
 stillWaiting:=false
 for _,id:=range ids{
  if _,busy:=p.active[scope+":"+id];!busy{stillWaiting=true;break}
 }
 if !stillWaiting{delete(p.pendingScopes,scope)}
 return reserved
}

func (s *Server) scheduleAgentAutoFeedback(ctx context.Context) {
 if s==nil||s.Workspace==""||ctx.Err()!=nil{return}
 store,err:=s.agentChatStore()
 if err!=nil{return}
 sessions,err:=store.List("local")
 if err!=nil{return}
 eligible:=make([]string,0,len(sessions))
 patchEligible:=make([]string,0,len(sessions))
 for _,sess:=range sessions{
  if ctx.Err()!=nil{return}
  if callID,ok:=agentAutoPatchCandidate(sess);ok && !s.Config.SharedServerEnabled {
   if s.agentLegacyPatchSchedulerAllowed(sess,callID) {
    patchEligible=append(patchEligible,sess.ID)
   }
   // No provider worker is needed while this proposal is pending.
   continue
  }
  if agentAutoFeedbackNeedsWork(sess){eligible=append(eligible,sess.ID)}
 }
 scope:=aiWorkspaceHash(s.Workspace)+":local"
 for _,id:=range agentAutoPatchWorkers.acquireFairBatch(scope,patchEligible){
  key:=scope+":"+id
  if ctx.Err()!=nil{agentAutoPatchWorkers.release(key);continue}
  go func(id,key string){
   defer agentAutoPatchWorkers.release(key)
   s.autoPatchOne(ctx,store,id)
  }(id,key)
 }
 reserved:=agentAutoFeedbackWorkers.acquireFairBatch(scope,eligible)
 for _,id:=range reserved{
  key:=scope+":"+id
  if ctx.Err()!=nil{
   agentAutoFeedbackWorkers.release(key)
   continue
  }
  go func(id,key string){
   defer agentAutoFeedbackWorkers.release(key)
   s.processAgentAutoFeedbackSession(ctx,store,id)
  }(id,key)
 }
}

// startAgentAutoFeedback keeps polling even if another session's verified
// Patch feedback is awaiting hours of read-only Collection work. Worker
// concurrency is bounded and duplicate scheduling is forbidden.
func (s *Server) startAgentAutoFeedback(ctx context.Context) {
	ticker := time.NewTicker(agentAutoFeedbackPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scheduleAgentAutoFeedback(ctx)
		}
	}
}
