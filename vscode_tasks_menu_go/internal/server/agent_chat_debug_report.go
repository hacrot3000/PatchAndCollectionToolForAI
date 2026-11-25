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
   visible:=make([]agentchat.Session,0,len(sessions))
   for _,sess:=range sessions {
    if !marker.At.IsZero() && !sess.UpdatedAt.After(marker.At){continue}
    visible=append(visible,sess)
   }
   limit:=len(visible)
   if limit>32{limit=32;report["sessions_truncated"]=true}
   out:=make([]map[string]any,0,limit)
   for _,sess:=range visible[:limit] {
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
        for _,key:=range []string{"stdout_tail","output","message","failure_reason"}{delete(obj,key)}
        item["result_metadata"]=obj
       }
      }else {item["result_size"]=len(call.Result)}
     }
     calls=append(calls,item)
    }
    feedbackIndex,feedbackEligible:=agentAutoFeedbackCandidate(sess)
    feedbackDiagnostic:=map[string]any{"eligible":feedbackEligible,"auto_feedback":sess.AutoFeedback,"persistent":sess.AutoFeedbackPersistent}
    if sess.AutoFeedbackUntil!=nil{feedbackDiagnostic["grant_until"]=sess.AutoFeedbackUntil}
    if sess.ProviderRun!=nil{feedbackDiagnostic["provider_phase"]=sess.ProviderRun.Phase}
    if feedbackEligible{feedbackDiagnostic["call_id"]=sess.ToolCalls[feedbackIndex].CallID}
    out=append(out,map[string]any{
     "id":sess.ID,"status":sess.Status,"revision":sess.Revision,
     "provider_id":sess.ProviderID,"model_id":sess.ModelID,
     "mode":sess.Mode,"project_id":sess.ProjectID,
     "created_at":sess.CreatedAt,"updated_at":sess.UpdatedAt,
     "last_request_status":sess.LastRequestCode,
     "last_request_digest":sess.LastRequestSHA,
     "event_seq":sess.EventSeq,"events":sess.Events,
     "history_items":hist,"history_item_count":len(sess.History),
     "tool_calls":calls,"auto_feedback_diagnostic":feedbackDiagnostic,
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
 b,err:=json.MarshalIndent(report,"","  ")
 if err!=nil{http.Error(w,"cannot encode safe report",500);return}
 // Keep the newest trace evidence when the report reaches its 2 MiB
 // limit. Never drop all diagnostic traces just because old ones are large.
 omitted:=0
 for len(b)>2<<20 && len(traces)>1 {
  traces=traces[1:]
  omitted++
  report["provider_traces"]=traces
  report["provider_trace_count"]=len(traces)
  report["traces_truncated"]=true
  report["omitted_oldest_traces"]=omitted
  report["diagnostic_note"]="Safe report exceeded 2 MiB; oldest provider traces were omitted, newest trace evidence and durable sessions are retained."
  b,err=json.MarshalIndent(report,"","  ")
  if err!=nil{http.Error(w,"cannot encode report",500);return}
 }
 if len(b)>2<<20 {
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
