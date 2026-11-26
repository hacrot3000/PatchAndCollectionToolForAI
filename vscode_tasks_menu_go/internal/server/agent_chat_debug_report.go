package server

import (
 "encoding/json"
 "net/http"
 "path/filepath"
 "sort"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// One owner-scoped, read-only diagnostic bundle. No prompts, file contents,
// API keys, headers, raw payload or executable Patch code is included.
// The durable session journal is included even when the daemon's recent
// in-memory provider traces have expired or were reset by a restart.
func (s *Server) agentChatDebugReportAPI(w http.ResponseWriter, r *http.Request) {
 w.Header().Set("Cache-Control","no-store")
 if r.Method!=http.MethodGet {
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
 }
 if !s.requireSharedActionPermission(w,r,"settings.read","agent.debug.read","agent_debug") {return}
 owner:=aiPlanOwner(r)
 marker,resetErr:=s.readAgentDebugClearMarker(owner)
 if resetErr!=nil {
  http.Error(w,"AI Debug reset marker unavailable",500)
  return
 }
 report:=map[string]any{
  "format":"taskdeck.agent.debug.report.v1",
  "generated_at":time.Now().UTC(),
  "capture_mode":"automatic_safe_metadata",
  "security":"No API keys, authorization headers, request/response bodies, prompts or Patch Python are copied.",
  "workspace_name":filepath.Base(s.Workspace),
  "trace_retention":"In-memory provider traces are bounded; canonical session events remain durable but old debug summaries can be hidden by Clear history.",
 }
 if !marker.At.IsZero(){
  report["cleared_at"]=marker.At
  report["debug_window"]="after_clear"
  report["diagnostic_scope"]="Only new AI Debug data since Clear history. Original conversation/patch journals are not deleted."
 }
 store:=s.debugStore()
 store.mu.Lock()
 o:=store.owner(owner)
 store.pruneLocked(o,time.Now())
 traces:=make([]agentDebugTrace,0,len(o.Traces))
 for _,entry:=range o.Traces {
  if !marker.At.IsZero() && !entry.CreatedAt.After(marker.At){continue}
  trace:=*entry
  trace.payload=agentDebugPayload{}
  trace.Events=append([]agentDebugEvent(nil),entry.Events...)
  traces=append(traces,trace)
 }
 store.mu.Unlock()
 // Ordering is stable so a copied report is easy to correlate with an
 // error's trace ID and with the durable session journal.
 sort.Slice(traces,func(i,j int)bool{return traces[i].CreatedAt.Before(traces[j].CreatedAt)})
 report["provider_traces"]=traces
 report["provider_trace_count"]=len(traces)
 report["traces_truncated"]=len(traces)>=agentDebugMaxTraces
 journal,err:=s.agentChatStore()
 if err!=nil{
  report["journal_error"]="Unable to open the scoped Agent session journal"
 }else{
  sessions,listErr:=journal.List(owner)
  if listErr!=nil{
   report["journal_error"]="Unable to read the scoped Agent session journal"
  }else{
   unifiedRuns,runErr:=s.agentDebugUnifiedRuns(owner,sessions,marker.At)
   if runErr!=nil{report["unified_run_error"]="Unable to read scoped Unified run metadata"}
   visible:=make([]agentchat.Session,0,len(sessions))
   // Attempt sidecars advance independently from the session CAS. A live
   // retry after Clear must be visible even without a new journal event.
   freshAttempts:=map[string]*agentchat.ProviderAttemptAudit{}
   activity:=map[string]time.Time{}
   for _,sess:=range sessions {
    activity[sess.ID]=sess.UpdatedAt
    runs:=unifiedRuns[sess.ID]
    if len(runs)>0&&runs[0].UpdatedAt.After(activity[sess.ID]){activity[sess.ID]=runs[0].UpdatedAt}
    if !marker.At.IsZero() && !sess.UpdatedAt.After(marker.At){
     if sess.ProviderRun!=nil&&sess.ProviderRun.Phase=="sending"{
      audit,e:=journal.ProviderAttempts(sess.ID,owner)
      if e==nil&&audit!=nil&&audit.RunID==sess.ProviderRun.ID&&len(audit.Attempts)>0{
       last:=audit.Attempts[len(audit.Attempts)-1].RecordedAt
       if last.After(marker.At){
        freshAttempts[sess.ID]=audit
        if last.After(activity[sess.ID]){activity[sess.ID]=last}
       }
      }
     }
     if !activity[sess.ID].After(marker.At){continue}
    }
    visible=append(visible,sess)
   }
   sort.SliceStable(visible,func(i,j int)bool{return activity[visible[i].ID].After(activity[visible[j].ID])})
   limit:=len(visible)
   if limit>32{limit=32;report["sessions_truncated"]=true}
   out:=make([]map[string]any,0,limit)
   for _,sess:=range visible[:limit] {
    continuationDiagnostic:=agentSafeContinuationDiagnostic(sess)
    // Current objective counters are cumulative; do not expose a pre-clear
    // aggregate as if it were fresh debug data.
    _,preClear:=marker.Sessions[sess.ID]
    var progress *agentchat.CollectionProgressView
    if !preClear{progress=agentchat.CollectionProgressViewForSession(sess)}
    var providerRun map[string]any
    var attempts []agentchat.ProviderAttempt
    if run:=sess.ProviderRun;run!=nil{
     freshRun:=marker.At.IsZero()||run.CreatedAt.After(marker.At)
     audit:=freshAttempts[sess.ID]
     var e error
     if audit==nil{audit,e=journal.ProviderAttempts(sess.ID,owner)}
     matched:=e==nil&&audit!=nil&&audit.RunID==run.ID
     if matched{
      for _,a:=range audit.Attempts{if marker.At.IsZero()||a.RecordedAt.After(marker.At){attempts=append(attempts,a)}}
     }
     if freshRun||len(attempts)>0{
      providerRun=map[string]any{"run_id":run.ID,"kind":run.Kind,"phase":run.Phase,"call_id":run.CallID,"payload_sha256":run.PayloadSHA256,"created_at":run.CreatedAt,"updated_at":run.UpdatedAt}
      if matched{
       providerRun["attempts_truncated"]=audit.Truncated
       if freshRun{providerRun["attempt_total"]=audit.Total}else{
        providerRun["attempt_scope"]="after_clear"
        providerRun["visible_attempt_count"]=len(attempts)
       }
      }else if e!=nil{providerRun["attempt_integrity_error"]=true}
     }
    }
    if cursor,seen:=marker.Sessions[sess.ID];seen && !marker.At.IsZero(){
     // Existing sessions remain operational after clear. Expose only the
     // new journal tail; earlier history/tool metadata stays in the
     // canonical file, but no longer clutters this debug report.
     historyStart:=cursor.HistoryItems
     if historyStart>len(sess.History){historyStart=len(sess.History)}
     sess.History=sess.History[historyStart:]
     toolStart:=cursor.ToolCalls
     if toolStart>len(sess.ToolCalls){toolStart=len(sess.ToolCalls)}
     sess.ToolCalls=sess.ToolCalls[toolStart:]
     events:=sess.Events[:0:0]
     for _,event:=range sess.Events {
      if event.Seq>cursor.EventSeq && event.At.After(marker.At){
       events=append(events,event)
      }
     }
     sess.Events=events
    }
    // Transcript summaries preserve ordering and tool IDs without sending
    // potentially sensitive prompts or generated source code to clipboard.
    hist:=make([]map[string]any,0,len(sess.History))
    for _,h:=range sess.History {
     if len(hist)>=256{break}
     item:=map[string]any{"type":h.Type}
     if h.CallID!=""{item["call_id"]=h.CallID}
     if h.Name!=""{item["tool_name"]=h.Name}
     if h.Content!=""{item["content_bytes"]=len(h.Content);item["content_sha256"]=agentchat.BodyDigest([]byte(h.Content))}
     if h.Arguments!=""{item["arguments_bytes"]=len(h.Arguments);item["arguments_sha256"]=agentchat.BodyDigest([]byte(h.Arguments))}
     hist=append(hist,item)
    }
    calls:=make([]map[string]any,0,len(sess.ToolCalls))
    for _,call:=range sess.ToolCalls{
     item:=map[string]any{
      "call_id":call.CallID,"name":call.Name,"status":call.Status,
      "model_turn_id":call.ModelTurnID,"text_fallback":call.TextFallback,
      "arguments_sha256":call.ArgumentsSHA256,"package_sha256":call.PackageSHA256,
      "package_size":call.PackageSize,"patch_session_id":call.PatchSessionID,
      "prompt_id":call.PromptID,"selected_index":call.SelectedIndex,
      "run_id":call.RunID,"feedback_status":call.FeedbackStatus,
      "feedback_sent":call.FeedbackSent,
      "historical_unconfirmed":agentchat.HistoricalUnconfirmedFeedback(sess,call),
     }
     if call.Result!="" {
      var parsed any
      if json.Unmarshal([]byte(call.Result),&parsed)==nil {
       // Native Patch result is already bounded and sanitized by the
       // protocol adapter, but explicitly remove stdout/free-text fields.
       if obj,ok:=parsed.(map[string]any);ok {
        item["result_metadata"]=agentSafeDebugPatchMetadata(obj)
        item["result_size"]=len(call.Result)
        item["result_sha256"]=agentchat.BodyDigest([]byte(call.Result))
       }
      }else {item["result_size"]=len(call.Result)}
     }
     calls=append(calls,item)
    }
    feedbackDiagnostic:=agentAutoFeedbackDiagnostic(sess)
    out=append(out,map[string]any{
     "id":sess.ID,"status":sess.Status,"revision":sess.Revision,
     "provider_id":sess.ProviderID,"model_id":sess.ModelID,
     "mode":sess.Mode,"project_id":sess.ProjectID,
     "created_at":sess.CreatedAt,"updated_at":sess.UpdatedAt,
     "last_request_status":sess.LastRequestCode,
     "last_request_digest":sess.LastRequestSHA,
     "event_seq":sess.EventSeq,"events":agentSafeDebugJournalEvents(sess.Events),
     "collection_progress":progress,"provider_run":providerRun,"provider_attempts":attempts,
     "unified_runs":unifiedRuns[sess.ID],
     "history_items":hist,"history_item_count":len(sess.History),
     "tool_calls":calls,"auto_feedback_diagnostic":feedbackDiagnostic,
     "continuation_diagnostic":continuationDiagnostic,
    })
   }
   report["sessions"]=out
   report["session_count"]=len(out)
  }
 }
 if len(traces)==0 {
  if !marker.At.IsZero(){
   report["diagnostic_note"]="No new provider traces since AI Debug was cleared. Continue using the Agent to capture fresh diagnostics."
  }else{
   report["diagnostic_note"]="No in-memory provider traces available. Durable Agent session transitions appear under sessions; reproduce the failure to capture new provider HTTP diagnostics."
  }
 }
 // Limit the scope of the report to a single user's current project; avoid
 // embedding raw HTTP payload, workstation paths or unknown provider bodies.
 b,err:=agentEncodeSafeDebugReport(report)
 if err!=nil{http.Error(w,"cannot encode safe report",500);return}
 if len(b)>agentDebugReportMaxBytes{
  http.Error(w,"debug report exceeds bounded safe export limit",413);return
 }
 // The response is safe, complete within explicitly declared retention/size
 // caps, and does not depend on any capture toggle or selected UI trace.
 w.Header().Set("Content-Type","application/json; charset=utf-8")
 _,_=w.Write(b)
}

// Compile-time marker for the server's Agent journal schema referenced by the
// report. Kept intentionally independent of a provider or Patch execution.
var _=strings.TrimSpace


// Report scheduler state without exporting model-generated next actions,
// checkpoint paths or any other prompt/response content. Evaluate before the
// debug Clear history cursor removes canonical receipt/history entries.
func agentSafeContinuationDiagnostic(x agentchat.Session)map[string]any{
 out:=map[string]any{
  "auto_continue_text":x.AutoContinueText,
  "standalone_eligible":agentchat.StandaloneAutoContinueCandidate(x),
  "task_plan_present":x.TaskPlan!=nil,
 }
 if c:=x.Continuation;c!=nil{
  out["disposition"]=c.Disposition
  switch c.Reason{
  case "model_progress_metadata_unverified","model_blocker_unverified","incomplete_progress_report","next_action_in_report","model_completion_claim_unverified","unknown_model_turn","unfinished_action_announcement","stalled_without_verified_work","nonretryable_admission_failure":
   out["reason"]=c.Reason
  default:
   out["reason"]="other_or_unavailable"
  }
  out["has_next_action"]=c.NextAction!=""
  out["no_progress_turns"]=c.NoProgressTurns
  out["repeated_turns"]=c.RepeatedTurns
  out["response_digest"]=c.ResponseDigest
 }else{out["disposition"]="not_recorded"}
 return out
}


// Details in old journals may contain arbitrary native error text or
// provider-generated content. Export only exact known transition reasons,
// plus byte count/digest for everything else; preserve the canonical entry.
func agentSafeDebugJournalEvents(events []agentchat.SessionEvent)[]map[string]any{
 out:=make([]map[string]any,0,len(events))
 for _,e:=range events{
  row:=map[string]any{"seq":e.Seq,"at":e.At,"kind":e.Kind,"status":e.Status,"call_id":e.CallID,"history_index":e.HistoryIndex}
  if e.Detail!=""{
   switch e.Detail{
   case "assess_readiness","bound_next_scope","repeated_verified_evidence","recovery_budget_exhausted","recovery_not_authorized","recovery_already_recorded","user_stopped","provider_egress_unresolved","canonical_call_output_pending","invalid_recovery_state","collection_context_checkpoint","local_request_not_prepared","provider_acknowledgement_unconfirmed","provider_egress_after_restart_unknown","acknowledged_response_local_processing_interrupted","collection_continuation","collection_cancelled","queue_outcome_unconfirmed","native_patch_tool_collect","message","feedback","collection","prepared_only","recovered_pre_egress","background_reconcile":
    row["detail"]=e.Detail
   default:
    row["detail_bytes"]=len(e.Detail);row["detail_sha256"]=agentchat.BodyDigest([]byte(e.Detail))
   }
  }
  out=append(out,row)
 }
 return out
}

// Never copy unknown JSON keys or nested result structures, even if the
// native receipt was valid. This export is metadata, not an evidence reader.
func agentSafeDebugPatchMetadata(obj map[string]any)map[string]any{
 out:=map[string]any{}
 for _,k:=range []string{"rc","item_index"}{if v,ok:=obj[k].(float64);ok&&v>=-1000000&&v<=1000000{out[k]=v}}
 for _,k:=range []string{"patch_executed","patch_queued","project_unchanged","automatic_retry","manual_execution","evidence_verified","partial_evidence_available"}{if v,ok:=obj[k].(bool);ok{out[k]=v}}
 for _,k:=range []string{"status","mutation_state","source","reason_code"}{
  v,_:=obj[k].(string)
  switch v{
  case "PASS","FAIL","FAILED","INCOMPLETE","PREFLIGHT_FAIL","SKIPPED_DUPLICATE_LOCAL","REJECTED","UNKNOWN","UNCHANGED","CHANGED","native_patch_history","native_patch_history_manual","native_patch_tool_collect","PATCH_APPROVAL_STALE","INVALID_PATCH_ARGUMENTS","UNSUPPORTED_TOOL","MULTIPLE_TOOL_CALLS":out[k]=v
  }
 }
 return out
}


const agentDebugReportMaxBytes=2<<20

// Prefer a readable report, then compact JSON without loss. If the combined
// bounded journal/trace sets still exceed the cap, omit oldest whole traces
// and sessions, explicitly reporting omissions. Keep at least the latest
// trace and latest session; never silently alter a canonical journal.
func agentEncodeSafeDebugReport(report map[string]any)([]byte,error){
 b,err:=json.MarshalIndent(report,"","  ")
 if err!=nil||len(b)<=agentDebugReportMaxBytes{return b,err}
 report["compact_encoding"]=true
 b,err=json.Marshal(report)
 if err!=nil||len(b)<=agentDebugReportMaxBytes{return b,err}
 traces,_:=report["provider_traces"].([]agentDebugTrace)
 sessions,_:=report["sessions"].([]map[string]any)
 omittedTraces,omittedSessions:=0,0
 for len(b)>agentDebugReportMaxBytes&&(len(traces)>1||len(sessions)>1){
  // Alternate the two domains, retaining recent evidence from both.
  if len(traces)>1&&(len(sessions)<=1||omittedTraces<=omittedSessions){
   traces=traces[1:];omittedTraces++
   report["provider_traces"]=traces
   report["provider_trace_count"]=len(traces)
   report["traces_truncated"]=true
   report["omitted_oldest_traces"]=omittedTraces
  }else{
   sessions=sessions[:len(sessions)-1];omittedSessions++
   report["sessions"]=sessions
   report["session_count"]=len(sessions)
   report["sessions_truncated"]=true
   report["omitted_oldest_sessions"]=omittedSessions
  }
  report["diagnostic_note"]="Safe report exceeded 2 MiB; older whole diagnostic traces or session summaries were omitted. The latest evidence in each available domain remains; canonical journals are unchanged."
  b,err=json.Marshal(report)
  if err!=nil{return nil,err}
 }
 return b,nil
}
