package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "time"
 "unicode/utf8"
)

// HandoffMarkdown builds a deliberately compact and transferable checkpoint.
// It uses only verified journal state, never invents task completion, and
// never includes source file contents, API secrets, or complete Patch code.
func HandoffMarkdown(x Session) string {
 var b strings.Builder
 fmt.Fprintf(&b,"# TaskDeck Agent handoff\n\n")
 fmt.Fprintf(&b,"Session: %s\nProject ID: %s\nRevision: %d\nUpdated: %s\nStatus: %s\n",
  x.ID,x.ProjectID,x.Revision,x.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),x.Status)
 fmt.Fprintf(&b,"Provider/model: %s / %s\n\n",x.ProviderID,x.ModelID)
 if x.ProviderRun!=nil {
  fmt.Fprintf(&b,"Provider request: %s (%s), phase=%s, call=%s, payload_sha256=%s.\n",
   handoffTrim(x.ProviderRun.ID,90),x.ProviderRun.Kind,x.ProviderRun.Phase,
   handoffTrim(x.ProviderRun.CallID,95),x.ProviderRun.PayloadSHA256)
  switch x.ProviderRun.Phase {
  case "prepared":
   b.WriteString("PREPARED: provider payload was durably staged; TaskDeck may automatically resume only this exact unsent request.\n\n")
  case "sending","unknown":
   b.WriteString("AMBIGUOUS PROVIDER EGRESS: TaskDeck must not replay this request automatically. Reconcile provider acknowledgement first.\n\n")
  default:
   b.WriteString("The provider request phase is journaled; this is not a claim that the complete user task passed its acceptance criteria.\n\n")
  }
 }
 if len(x.CollectionResumeHistory)>0{
  b.WriteString("## Previous UNKNOWN Collection egress — retained audit, never replay\n\n")
  for _,c:=range x.CollectionResumeHistory{
   fmt.Fprintf(&b,"- Old run %s phase=%s payload_sha256=%s; new run %s; ",
    handoffTrim(c.Previous.ID,90),c.Previous.Phase,c.Previous.PayloadSHA256,handoffTrim(c.NewRunID,90))
   if c.ReceiptUnavailable{b.WriteString("canonical receipt unavailable; ")}else{fmt.Fprintf(&b,"canonical receipt_sha256=%s; ",c.ReceiptSHA256)}
   if a:=c.AttemptAudit;a!=nil{
    fmt.Fprintf(&b,"%d recorded local attempts",a.Total)
    if a.Truncated{b.WriteString(" (earlier details omitted)")}
    if len(a.Attempts)>0{
     last:=a.Attempts[len(a.Attempts)-1]
     fmt.Fprintf(&b,"; last correlation=%s, local outcome=%s",handoffTrim(last.ClientRequestID,80),last.Outcome)
    }
   }else{b.WriteString("No matching local attempt record")}
   b.WriteString(". Not provider acknowledgement.\n")
  }
  b.WriteString("\n")
 }
 if g,ok:=x.ReadCollectionGuard();ok{
  c:=g.Counters
  fmt.Fprintf(&b,"Collection guard (current objective): requested=%d, observed=%d, native_reads=%d, verified_new=%d, verified_duplicate=%d, unknown=%d, errors=%d, recorded_http_attempts=%d, patch_proposals=%d, patch_executions=%d, patch_rejections=%d, patch_uncertain=%d, replans=%d.\n\n",
   c.Requested,c.Observed,c.NativeReads,c.NewEvidence,c.DuplicateEvidence,c.UnknownEvidence,c.Errors,c.HTTPAttempts,c.PatchProposals,c.PatchExecutions,c.PatchRejections,c.PatchUncertainOutcomes,g.Recovery.Attempts)
  if g.AttemptAuditIncomplete{b.WriteString("Local attempt audit incomplete; counts are not proof of delivery.\n\n")}
 }
 b.WriteString("## Original user goal\n\n")
 foundGoal:=false
 for i,it:=range x.History {
  if it.Type!="user" || CollectionTextObservation(x.History,i) {continue}
  content:=it.Content
  if idx:=strings.LastIndex(content,"USER REQUEST:\n");idx>=0{content=content[idx+len("USER REQUEST:\n"):]}
  if idx:=strings.Index(content,"\n\nATTACHMENTS (untrusted user-provided text");idx>=0{content=content[:idx]}
  if strings.Contains(content,"TaskDeck local read-only Collection tool returned this VERIFIED observation") {continue}
  fmt.Fprintf(&b,"> %s\n\n",handoffTrim(content,1100))
  foundGoal=true
  break
 }
 if !foundGoal {b.WriteString("Original user goal is not available in active history; inspect the archived user transcript before assuming completion.\n\n")}
 if x.TaskPlan!=nil{
  plan:=x.TaskPlan
  done,total:=TaskPlanProgress(plan)
  fmt.Fprintf(&b,"## Durable task plan (%d/%d)\n\nGoal: %s\nState: %s; automatic continuation: %t; no-progress turns: %d\n\n",
   done,total,handoffTrim(plan.Goal,1300),plan.State,plan.AutoContinue,plan.NoProgressTurns)
  if plan.NextRetryAt!=nil{
   fmt.Fprintf(&b,"Automatic continuation deferred until: %s (consecutive admission delays: %d; no provider egress confirmed by this wait).\n\n",
    plan.NextRetryAt.UTC().Format(time.RFC3339),plan.RetryStreak)
  }
  if len(plan.Steps)>compactHandoffPlanThreshold{
   appendCompactTaskPlanHandoff(&b,x.ID,plan)
  }else{
  for _,step:=range plan.Steps{
   fmt.Fprintf(&b,"- [%s] %s — %s; acceptance: %s\n",
    step.Status,handoffTrim(step.ID,60),handoffTrim(step.Title,230),handoffTrim(step.Acceptance,580))
   if g:=step.TaskCommandGuard;g!=nil{
    fmt.Fprintf(&b,"  - Owner-bound expected test command: Task %d, SHA-256 %s, bound at %s; a NEW native Task run must match (not proof of test coverage).\n",
     g.TaskID,handoffTrim(g.CommandSHA256,64),g.BoundAt.UTC().Format(time.RFC3339))
   }
   if required:=step.GoTestRequirements;required!=nil{
    fmt.Fprintf(&b,"  - Owner-bound required Go tests in package %s (bound at %s; ALL required): %s. Test names must match one NEW successful native go test -json execution; not a coverage claim.\n",
     handoffTrim(required.Package,200),required.BoundAt.UTC().Format(time.RFC3339),
     handoffTrim(strings.Join(required.Tests,", "),800))
   }
   if len(step.AcceptanceCriteria)>0{
    reviewed:=0
    for _,criterion:=range step.AcceptanceCriteria{
     if criterion.ReviewedAt!=nil{reviewed++}
    }
    fmt.Fprintf(&b,"  - OWNER acceptance coverage: %d/%d criteria explicitly reviewed (human attestation, NOT automatic semantic proof).\n",
     reviewed,len(step.AcceptanceCriteria))
    for _,criterion:=range step.AcceptanceCriteria{
     state:="awaiting owner review"
     if criterion.ReviewedAt!=nil{state="owner-reviewed "+criterion.ReviewedAt.UTC().Format(time.RFC3339)}
     fmt.Fprintf(&b,"    - %s: %s; test mapping=%s; %s; owner note=%s\n",
      handoffTrim(criterion.ID,64),handoffTrim(criterion.Description,300),
      handoffTrim(strings.Join(criterion.RequiredTests,", "),250),
      state,handoffTrim(criterion.ReviewNote,250))
    }
   }
   for _,ev:=range step.Evidence{
    fmt.Fprintf(&b,"  - %s evidence: ref=%s digest=%s note=%s\n",
     ev.Kind,handoffTrim(ev.Ref,120),handoffTrim(ev.Digest,64),handoffTrim(ev.Note,300))
   }
  }
  }
  if len(plan.Suggestions)>0{
   b.WriteString("\n## Model-suggested evidence pointers (UNTRUSTED; not acceptance)\n\n")
   for _,candidate:=range plan.Suggestions{
    fmt.Fprintf(&b,"- Suggested step=%s kind=%s call=%s path=%s; model_turn=%s source_sha256=%s. Requires separate owner review, rule consent and a verified native receipt.\n",
     handoffTrim(candidate.StepID,64),handoffTrim(candidate.Kind,24),
     handoffTrim(candidate.CallID,120),handoffTrim(candidate.Path,200),
     handoffTrim(candidate.TurnID,120),handoffTrim(candidate.SourceDigest,64))
   }
   b.WriteString("\n")
  }
  if next:=TaskPlanNext(plan);next!=nil{
   fmt.Fprintf(&b,"\nNext actionable milestone: %s — %s. Do not infer completion from the last assistant reply.\n\n",next.ID,next.Acceptance)
  } else if !TaskPlanAllDone(plan) {
   b.WriteString("\nNo actionable milestone remains; blocked steps require a user decision.\n\n")
  }
 }
 if x.Continuation!=nil{
  c:=x.Continuation
  fmt.Fprintf(&b,"## Last assistant-turn continuation advisory (not acceptance)\n\nDisposition: %s; reason: %s; automatic continuation (without plan): %t\n",
   handoffTrim(c.Disposition,32),handoffTrim(c.Reason,120),x.AutoContinueText&&x.TaskPlan==nil)
  if c.Disposition==TurnBlocked{
   if StandaloneAutoContinueCandidate(x)&&RetiredPatchBlockerRecheck(x){
    b.WriteString("Lifecycle decision: eligible for one fresh automatic recovery turn after exact acknowledged retirement. No old native execution will be replayed.\n")
   }else{
    b.WriteString("Lifecycle decision: model-reported blocker (advisory); automatic scheduling is paused unless an independent authorized recovery applies. This is not proof of a native planning refusal.\n")
   }
  }
  fmt.Fprintf(&b,"Next proposed action: %s\nCheckpoint path (UNVERIFIED): %s\nResponse SHA-256: %s; action signature: %s; repeated advice: %d; no-verified-work turns: %d\n\n",
   handoffTrim(c.NextAction,500),handoffTrim(c.CheckpointPath,240),
   c.ResponseDigest,c.ProgressSignature,c.RepeatedTurns,c.NoProgressTurns)
  if c.NextRetryAt!=nil{
   fmt.Fprintf(&b,"Continuation admission deferred until: %s\n\n",c.NextRetryAt.UTC().Format(time.RFC3339))
  }
 }
 b.WriteString("## Recent journal observations (assistant narration is advisory)\n\n")
 count:=0
 for i:=len(x.History)-1;i>=0 && count<9;i-- {
  it:=x.History[i]
  if it.Type=="assistant" && len(it.Content)>0{
   fmt.Fprintf(&b,"- AI response: %s\n",handoffTrim(it.Content,280))
   count++
  }
  if it.Type=="function_call_output" {
   fmt.Fprintf(&b,"- Tool observation %s: %s\n",handoffTrim(it.CallID,95),handoffDigest(it.Content))
   count++
  }
  if it.Type=="function_call" && it.Name=="taskdeck_collect_workspace" {
   fmt.Fprintf(&b,"- Collection %s: %s\n",handoffTrim(it.CallID,95),handoffCollectionPaths(it.Arguments))
   count++
  }
 }
 if count==0 {b.WriteString("- No verified work recorded in the recent session window.\n")}
 b.WriteString("\n## Native Patch state (no implicit re-execution)\n\n")
 if len(x.ToolCalls)==0 {b.WriteString("- No live Patch commitments.\n")}
 for _,call:=range x.ToolCalls{
  fmt.Fprintf(&b,"- %s: %s; feedback=%s; run=%s; receipt=%s\n",
   handoffTrim(call.CallID,90),handoffTrim(call.Status,35),
   handoffTrim(call.FeedbackStatus,35),handoffTrim(call.RunID,90),
   handoffDigest(call.Result))
  if r:=call.FailedQueueRetirement;r!=nil&&validFailedQueueRetirement(r,call.Status,call.RunID,call.PackageSHA256){
   fmt.Fprintf(&b,"  - Native queue lifecycle (journal observation): %s; run=%s; package_sha256=%s; archive=%s; observed=%s. Not rollback, not source-integrity proof; revalidate current queue before execution.\n",
    handoffTrim(r.Status,32),handoffTrim(r.RunID,90),r.PackageSHA256,handoffTrim(r.Archive,180),r.ObservedAt.UTC().Format(time.RFC3339))
  }else if call.Name=="taskdeck_submit_patch"&&call.Status=="error"&&call.RunID!=""{
   b.WriteString("  - Queue disposition is not confirmed in this journal. Inspect exact native evidence; absence of metadata alone is not a native planning denial.\n")
  }
 }
 fmt.Fprintf(&b,"\nArchived: %d Collection/narration items in %d segments; %d acknowledged Patch calls in %d segments.\n",
  x.ArchivedHistoryItems,x.HistoryArchiveParts,x.ArchivedPatchCalls,x.PatchArchiveParts)
 for _,summary:=range x.PatchArchiveSummaries{
  fmt.Fprintf(&b,"- Earlier Patch: %s\n",handoffTrim(summary,160))
 }
 b.WriteString("\n## Recovery and next action\n\n")
 switch x.Status {
 case "requesting_ai":
  b.WriteString("A provider turn is in-flight. Do not start a duplicate request. Check the active worker and wait for a durable result, or reconcile after restart.\n")
 case "waiting_tool","tool_queuing":
  b.WriteString("Resolve the current native Patch using the existing approval and execution evidence. Do not submit a duplicate Patch to bypass a pending receipt.\n")
 case "failed","cancelled":
  b.WriteString("The previous AI turn was interrupted or failed. Inspect the final event/feedback acknowledgement before initiating any new provider request. Never automatically resend ambiguous Patch feedback.\n")
 case "completed":
  b.WriteString("The model ended a turn; this does NOT prove the user's whole plan is finished. Check acceptance criteria and outstanding tasks before continuing.\n")
 default:
  b.WriteString("Check the session state, verified evidence and remaining user task requirements before starting the next step.\n")
 }
 for _,call:=range x.ToolCalls {
  if call.FeedbackStatus=="unknown"||call.FeedbackStatus=="sending"||call.Status=="unknown"{
   fmt.Fprintf(&b,"- Needs manual/provider reconciliation: %s (%s / %s)\n",handoffTrim(call.CallID,90),call.Status,call.FeedbackStatus)
  }
 }
 if len(x.Events)>0{
  ev:=x.Events[len(x.Events)-1]
  fmt.Fprintf(&b,"Last event: #%d %s [%s] %s\n",ev.Seq,ev.Kind,ev.Status,ev.Detail)
 }
 b.WriteString("\nThis handoff is derived from the durable TaskDeck journal. It is not an instruction to replay previous tool calls. Verify the workspace before acting.\n")
 return b.String()
}
func handoffTrim(s string,limit int)string{
 s=strings.ReplaceAll(strings.ReplaceAll(s,"\r"," "),"\n"," ")
 s=strings.TrimSpace(s)
 if len(s)<=limit{return s}
 end:=limit-3
 if end<0{return ""}
 for end>0&&!utf8.ValidString(s[:end]){end--}
 return s[:end]+"..."
}
func handoffDigest(s string)string{
 if s==""{return "(none)"}
 sum:=sha256.Sum256([]byte(s))
 return hex.EncodeToString(sum[:8])
}
func handoffCollectionPaths(arguments string)string{
 var a struct{Actions []struct{
  Type string
  Path string
  Paths []string
  Query string
 }}
 if json.Unmarshal([]byte(arguments),&a)!=nil{return "(unparseable request; refer to canonical journal)"}
 paths:=make([]string,0,6)
 for _,act:=range a.Actions{
  desc:=act.Type
  if act.Path!=""{desc+=" "+handoffTrim(act.Path,90)}
  for _,p:=range act.Paths{desc+=" "+handoffTrim(p,90)}
  if act.Query!=""{desc+=" search="+handoffTrim(act.Query,90)}
  paths=append(paths,handoffTrim(desc,200))
  if len(paths)>=6{break}
 }
 return strings.Join(paths,"; ")
}

func (s *Store) handoffFile(id string)string{return filepath.Join(s.dir,id+".handoff.md")}

// The handoff is a convenience derivative, not an independent authority.
// Save() commits the canonical journal first, then best-effort materializes
// the handoff. Handoff() regenerates from the journal if a crash occurs after
// session rename but before this derivative is updated.
func (s *Store) writeHandoffLocked(x Session)error{
 temp,err:=os.CreateTemp(s.dir,".agent-handoff-*")
 if err!=nil{return err}
 defer os.Remove(temp.Name())
 if err=temp.Chmod(0600);err!=nil{temp.Close();return err}
 if _,err=temp.WriteString(HandoffMarkdown(x));err!=nil{temp.Close();return err}
 if err=temp.Sync();err!=nil{temp.Close();return err}
 if err=temp.Close();err!=nil{return err}
 return os.Rename(temp.Name(),s.handoffFile(x.ID))
}

// Handoff is owner/project scoped and always generated from the latest durable
// revision. It remains available even after a lost response or server restart.
func (s *Store) Handoff(id,owner string)(string,uint64,error){
 x,err:=s.Get(id,owner)
 if err!=nil{return "",0,err}
 content:=HandoffMarkdown(x)
 if x.ProviderRun!=nil && x.ProviderRun.Phase=="sending"{
  // A heartbeat is a hint, not an acknowledgement: report the observed
  // timestamp without declaring the provider alive or the task complete.
  if h,e:=s.LatestProviderHeartbeat(id,owner);e==nil {
   lease:="recent"
   if time.Now().UTC().After(h.LeaseUntil){lease="stale/expired"}
   content+=fmt.Sprintf("\nProvider worker heartbeat: %s (run %s; lease %s through %s). Diagnostic liveness only; provider acknowledgement remains unverified.\n",
    h.At.UTC().Format(time.RFC3339),h.RunID,lease,h.LeaseUntil.UTC().Format(time.RFC3339))
  }
 }
 return content,x.Revision,nil
}
