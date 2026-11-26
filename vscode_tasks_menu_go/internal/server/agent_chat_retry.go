package server

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/url"
 "strconv"
 "sync"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentchat/contextcompact"
)

// Retry preferences are durable in the Agent Session. The running attempt
// progress is process-local and never alters the canonical provider journal
// while a turn is in-flight (which would break its revision CAS).
type agentRetryProgress struct {
 Mode agentchat.RetryMode `json:"mode"`
 Attempt int `json:"attempt"`
 Remaining int `json:"remaining,omitempty"`
 Phase string `json:"phase"`
 NextAt *time.Time `json:"next_at,omitempty"`
 LastCode string `json:"last_code,omitempty"`
 SameDigestFailures int `json:"same_digest_failures,omitempty"`
 Outcome agentchat.ProviderAttemptOutcome `json:"outcome,omitempty"`
 RecoveryGeneration int `json:"recovery_generation,omitempty"`
}
var agentRetryDelay = 10 * time.Second // tests may shorten this; production stays 10 seconds
var agentRetryProgressMu sync.Mutex
var agentRetryProgressBySession=map[string]agentRetryProgress{}
func agentRetryKey(workspace,owner,id string) string{return workspace+"\x00"+owner+"\x00"+id}
func agentRetrySnapshot(workspace,owner,id string) *agentRetryProgress{
 agentRetryProgressMu.Lock();defer agentRetryProgressMu.Unlock()
 value,ok:=agentRetryProgressBySession[agentRetryKey(workspace,owner,id)]
 if !ok {return nil}
 return &value
}
func agentRetryPublish(workspace,owner,id string,record agentRetryProgress){
 agentRetryProgressMu.Lock();defer agentRetryProgressMu.Unlock()
 if record.Phase=="idle"{delete(agentRetryProgressBySession,agentRetryKey(workspace,owner,id));return}
 agentRetryProgressBySession[agentRetryKey(workspace,owner,id)]=record
}
func agentRetryModeForSession(s *Server,owner,id string) agentchat.RetryMode{
 store,err:=s.agentChatStore()
 if err!=nil{return agentchat.RetryOff}
 x,err:=store.Get(id,owner)
 if err!=nil||x.Workspace!=s.Workspace{return agentchat.RetryOff}
 switch x.RetryMode{
 case agentchat.RetryLimited,agentchat.RetryUnlimited:return x.RetryMode
 default:return agentchat.RetryOff
 }
}
// A user's Collection task has no cumulative 7/30-minute deadline.
// Preserve 600s per network attempt, retry-mode limits and explicit Stop.
func agentRetryTurnContext(parent context.Context,_ agentchat.RetryMode)(context.Context,context.CancelFunc){
 return context.WithCancel(parent)
}
// HTTP 499 ("context canceled") is an exception: gateways frequently emit
// it after terminating a slow upstream request. Keep retrying only while an
// expressly enabled mode is active; never replay a Native Patch.
// Permanent 4xx model/auth errors and invalid local request payloads must not
// be repeated forever. For transient failure, the user's explicit setting
// permits replay of the provider request; a remote processing ambiguity may
// incur duplicate inference, but never automatically reruns native Patch.
func agentProviderRetryable(err error)bool{return agentchat.ProviderTransportRetryable(err)}
func agentRetryFailureDetail(err error)string{
 var pe *agentchat.ProviderError
 if errors.As(err,&pe){
  if pe.ProviderErrorCode!=""{return pe.Code+":"+pe.ProviderErrorCode}
  if pe.Code!=""{return pe.Code}
 }
 return agentRetryCode(err)
}
func agentRetryCode(err error)string{
 var pe *agentchat.ProviderError
 if errors.As(err,&pe) && pe.Code!=""{return pe.Code}
 if errors.Is(err,context.DeadlineExceeded){return "PROVIDER_TIMEOUT"}
 return "TRANSIENT_TRANSPORT_FAILURE"
}
// A model may exhaust max_output_tokens on a large task, including hidden
// reasoning. On the FIRST such error, retry ONCE (only when retries are opted
// in) with both more headroom and a succinct, explicit completion budget.
// No untrusted partial output is inserted into history, no tool call from an
// incomplete response is executed, and the durable canonical payload is kept.
// The next incomplete error is terminal EVEN for RetryUnlimited: rerunning
// the same enormous task indefinitely wastes tokens and never makes progress.
const agentMaxContextRecoveries = agentchat.MaxContextRecoveries

func agentContextLimitRejected(err error) bool { return agentchat.ContextLimitRejected(err) }
func agentRetryConstrainedOutputPayload(payload []byte,p agentchat.ProviderConfig)([]byte,int,bool){
 return agentchat.RetryConstrainedOutputPayload(payload,p)
}

// Recompact the already constructed stateless wire history, preserving the
// current input and its source call(s) byte-exact at the field level. Never
// rewrite the session journal or send another native tool execution.
func agentRetryContextPayload(payload []byte)([]byte,bool){
 var req map[string]json.RawMessage
 if json.Unmarshal(payload,&req)!=nil||req==nil{return nil,false}
 if _,ok:=req["previous_response_id"];ok{return nil,false}
 if _,ok:=req["conversation"];ok{return nil,false}
 var stored bool
 if raw,ok:=req["store"];ok&&(json.Unmarshal(raw,&stored)!=nil||stored){return nil,false}
 var wire []struct{
  Type string `json:"type"`
  Role string `json:"role"`
  Content string `json:"content"`
  CallID string `json:"call_id"`
  Name string `json:"name"`
  Arguments string `json:"arguments"`
  Output string `json:"output"`
 }
 var instructions,model string
 if json.Unmarshal(req["input"],&wire)!=nil||len(wire)==0||
  json.Unmarshal(req["instructions"],&instructions)!=nil||
  json.Unmarshal(req["model"],&model)!=nil{return nil,false}
 items:=make([]agentchat.HistoryItem,0,len(wire))
 for _,w:=range wire{
  switch {
  case w.Type==""&&(w.Role=="user"||w.Role=="assistant"):
   items=append(items,agentchat.HistoryItem{Type:w.Role,Content:w.Content})
  case w.Type=="function_call":
   items=append(items,agentchat.HistoryItem{Type:w.Type,CallID:w.CallID,Name:w.Name,Arguments:w.Arguments})
  case w.Type=="function_call_output":
   items=append(items,agentchat.HistoryItem{Type:w.Type,CallID:w.CallID,Content:w.Output})
  default:return nil,false
  }
 }
 // The latest user message or trailing batch of verified tool outputs is
 // mandatory. The compactor keeps each source call for Next in full.
 if items[len(items)-1].Type!="function_call_output"&&items[len(items)-1].Type!="user"{return nil,false}
 cut:=agentMandatoryRetrySuffix(items)
 next:=items[cut:]
 budget:=contextcompact.Tokens(string(payload))/2
 if budget<4096{budget=4096}
 visible,_:=contextcompact.Build(items[:cut],contextcompact.Policy{
  MaxInputTokens:budget,CollectionPercent:5,Instructions:instructions,Next:next,
 })
 // Reuse the canonical validator for every call/output pair. Take only its
 // input field, retaining the original tools, instructions and provider options.
 checked,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{
  Model:model,Instructions:instructions,Mode:agentchat.Stateless,History:visible,Next:next,
 })
 if err!=nil{return nil,false}
 var validated map[string]json.RawMessage
 if json.Unmarshal(checked,&validated)!=nil{return nil,false}
 req["input"]=validated["input"]
 // Context limits may include reserved output. Reduce only for this rejected
 // request; do not change the user's persistent model configuration.
 var limit int
 if json.Unmarshal(req["max_output_tokens"],&limit)==nil&&limit>8192{
  limit/=2
  if limit<8192{limit=8192}
  req["max_output_tokens"],_=json.Marshal(limit)
 }
 b,err:=json.Marshal(req)
 if err!=nil||len(b)>4<<20{return nil,false}
 // Never retry an equivalent or larger context unless output reservation
 // actually decreased. Compare the input field, not JSON formatting.
 var old map[string]json.RawMessage
 if json.Unmarshal(payload,&old)!=nil{return nil,false}
 var oldLimit int
 _=json.Unmarshal(old["max_output_tokens"],&oldLimit)
 // A summary can be larger than a short original history. In that case
 // retain the complete original input and reduce only output reservation.
 // Do not abandon an otherwise safe structured context recovery.
 if len(req["input"])>len(old["input"]){return agentchat.RetryReducedOutputReservation(payload)}
 if limit>oldLimit{return nil,false}
 if len(req["input"])==len(old["input"])&&!(limit>0&&limit<oldLimit){return nil,false}
 return b,true
}

// agentChatProviderWithRetry runs one 600-second-scoped provider attempt at a
// time. A limited retry means ONE initial attempt + FIVE retries 10s apart.
// Unlimited transport retries stop on success, Stop or shutdown. A repeated
// max_output_tokens error is different: at most one guided recovery request
// is allowed, even in unlimited mode. Native Patch execution is NEVER replayed.
func (s *Server) agentChatProviderWithRetry(ctx context.Context,owner,sessionID string,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error,string){
 return s.agentChatProviderWithRetryUsingWait(ctx,owner,sessionID,p,key,payload,waitLegacyProviderRetry)
}

// A wait seam allows deterministic tests of the actual Legacy retry loop
// without sleeping or changing process-global timing policy.
func waitLegacyProviderRetry(ctx context.Context,delay time.Duration)error{
 timer:=time.NewTimer(delay)
 defer timer.Stop()
 select{case <-ctx.Done():return ctx.Err();case <-timer.C:return nil}
}

func (s *Server) agentChatProviderWithRetryUsingWait(ctx context.Context,owner,sessionID string,p agentchat.ProviderConfig,key string,payload []byte,wait func(context.Context,time.Duration)error)(agentchat.ResponsesResult,error,string){
 mode:=agentRetryModeForSession(s,owner,sessionID)
 replayPayload:=payload
 incompleteRecoverySent:=false
 contextRecoveries:=0
 var contextRejectedPayload []byte
 lastFailedDigest:=""
 sameDigestFailures:=0
 // The durable SENDING journal is the only origin of correlation data.
 // Direct capability probes and legacy tests without a live egress marker
 // keep using the existing transport without manufacturing a fake receipt.
 var attemptStore *agentchat.Store
 var runID string
 var affinityKey string
 if sessionID!=""&&owner!=""{
  if candidate,e:=s.agentChatStore();e==nil{
   if x,e:=candidate.Get(sessionID,owner);e==nil&&x.Workspace==s.Workspace&&
    x.Status=="requesting_ai"&&x.ProviderRun!=nil&&x.ProviderRun.Phase=="sending"{
    attemptStore=candidate
    runID=x.ProviderRun.ID
    affinityKey=agentchat.ProviderAffinityKey(x,p,modelFromAgentPayload(payload))
   }
  }
 }
 parsedURL,_:=url.Parse(p.BaseURL)
 headerSupported:=parsedURL!=nil&&agentchat.SupportsClientRequestID(parsedURL.Hostname())
 defer agentRetryPublish(s.Workspace,owner,sessionID,agentRetryProgress{Phase:"idle"})
 for attempt:=1; ;attempt++{
  if err:=ctx.Err();err!=nil{return agentchat.ResponsesResult{},err,""}
  remaining:=0
  if mode==agentchat.RetryLimited{remaining=6-attempt}
  generation:=contextRecoveries
  if incompleteRecoverySent{generation++}
  digest:=agentchat.ProviderPayloadDigest(replayPayload)
  if m,e:=agentchat.MeasureProviderPayload(p.APIType,replayPayload);e==nil{
   s.recordAgentDebugEvent(owner,sessionID,"","provider_retry","payload.budget","","canonical_bytes="+strconv.Itoa(m.CanonicalBytes)+" wire_template_bytes="+strconv.Itoa(m.WireBytes)+" estimated_input_tokens="+strconv.Itoa(m.EstimatedInputTokens)+" output_budget="+strconv.Itoa(m.OutputBudget)+" instruction_bytes="+strconv.Itoa(m.InstructionsBytes)+" input_bytes="+strconv.Itoa(m.InputBytes)+" tools_bytes="+strconv.Itoa(m.ToolsBytes)+" other_bytes="+strconv.Itoa(m.OtherBytes)+" stateful="+strconv.FormatBool(m.Stateful))
  }
  agentRetryPublish(s.Workspace,owner,sessionID,agentRetryProgress{Mode:mode,Attempt:attempt,Remaining:remaining,Phase:"sending",SameDigestFailures:sameDigestFailures,RecoveryGeneration:generation})
  callCtx,cancel:=context.WithTimeout(ctx,agentchat.ProviderAttemptTimeout)
  if attemptStore!=nil{
   // A reduced/reformatted retry is not the original cache projection.
   // Never mutate the immutable PREPARED payload or native history.
   if digest!=agentchat.ProviderPayloadDigest(payload){
    if e:=attemptStore.InvalidateCacheEpochForRun(sessionID,owner,runID);e!=nil{
     s.recordAgentDebugEvent(owner,sessionID,"","provider_cache","cache.epoch.invalidate_failed","","Derived checkpoint could not be removed after request rewrite; source/policy validation still applies.")
    }
   }
   // Flush the audit sidecar BEFORE each individual network attempt.
   // All retry attempts get distinct IDs, even if output budgets change.
   // Corrupt/missing storage blocks the attempt rather than losing the
   // correlation that a provider support investigation will need.
   clientID,e:=attemptStore.RecordProviderAttempt(
    sessionID,owner,runID,uint64(attempt),replayPayload,headerSupported,agentchat.ProviderAttemptPolicy{Mode:mode,RecoveryGeneration:generation})
   if e!=nil{
    cancel()
    return agentchat.ResponsesResult{},e,""
   }
   if headerSupported{
    callCtx=agentchat.WithProviderClientRequestID(callCtx,clientID)
   }
  }
  callCtx=agentchat.WithProviderAffinity(callCtx,p,affinityKey)
  result,err,trace:=s.agentChatObservedCall(callCtx,owner,"provider_request",sessionID,p,key,replayPayload)
  cancel()
  outcome:=agentchat.ClassifyProviderAttemptOutcome(ctx.Err(),err)
  if attemptStore!=nil{
   if e:=attemptStore.RecordProviderAttemptOutcome(sessionID,owner,runID,uint64(attempt),digest,outcome);e!=nil{
    // Diagnostic write failure cannot change an actual returned response
    // into an ambiguous ACK or permit another native operation.
    s.recordAgentDebugEvent(owner,sessionID,"","provider_retry","attempt_outcome.unavailable","","Could not persist local transport outcome; the pre-egress attempt is not delivery proof.")
   }
  }
  if outcome.StagnantTransport(){
   if lastFailedDigest==digest{sameDigestFailures++}else{lastFailedDigest=digest;sameDigestFailures=1}
   if sameDigestFailures==3{
    s.recordAgentDebugEvent(owner,sessionID,"","provider_retry","payload.stagnation",string(outcome),"Three transport failures for the same canonical request digest. Inspect provider connectivity or Stop and start a separately authorized continuation. No automatic provider switch, payload rewrite, or native replay.")
   }
  }else{lastFailedDigest="";sameDigestFailures=0}
  // User Stop/daemon shutdown is authoritative even when the transport
  // reports a generic provider error after the context was cancelled.
  if ctx.Err()!=nil{return agentchat.ResponsesResult{},ctx.Err(),trace}
  if err==nil{
   if contextRecoveries>0{
    if hints,e:=s.agentChatStore();e==nil{
     if x,e:=hints.Get(sessionID,owner);e==nil&&agentLearnContextRecovery(hints,x,p,key,contextRejectedPayload,replayPayload){
      s.recordAgentDebugEvent(owner,sessionID,"","provider_retry","context_budget.learned","","Retained a bounded durable provider/model context and output budget hint for subsequent stateless requests")
     }
    }
   }
   return result,nil,trace
  }
  if mode==agentchat.RetryOff||(mode==agentchat.RetryLimited&&attempt>=6)||ctx.Err()!=nil{
   return result,err,trace
  }
  if agentContextLimitRejected(err){
   if contextRecoveries>=agentMaxContextRecoveries{
    s.recordAgentDebugEvent(owner,sessionID,"","provider_retry","context_budget.exhausted",agentRetryFailureDetail(err),"Stopped after three progressively reduced stateless requests were rejected; inspect model limits and mandatory current input. Native tool receipt preserved.")
    return result,err,trace
   }
   reduced,ok:=agentRetryContextPayload(replayPayload)
   if !ok{
    s.recordAgentDebugEvent(owner,sessionID,"","provider_retry","context_budget.minimum",agentRetryFailureDetail(err),"No safely smaller stateless request remains; mandatory current input and receipt cannot be discarded. Check provider limits or use a narrower next task.")
    return result,err,trace
   }
   if _,e:=agentchat.MeasureProviderPayload(p.APIType,reduced);e!=nil{return result,err,trace}
   contextRejectedPayload=replayPayload
   replayPayload=reduced
   contextRecoveries++
   s.recordAgentDebugEvent(owner,sessionID,"","provider_retry","context_budget.recovery",agentRetryFailureDetail(err),"Progressive context recovery "+strconv.Itoa(contextRecoveries)+"/"+strconv.Itoa(agentMaxContextRecoveries)+"; exact current tool receipt preserved")
  }else if !agentProviderRetryable(err){
   return result,err,trace
  }
  var pe *agentchat.ProviderError
  if errors.As(err,&pe)&&pe.Code=="RESPONSE_INCOMPLETE"&&pe.ProviderErrorCode=="max_output_tokens"{
   if incompleteRecoverySent{
    pe.Retryable=false
    pe.Message="Provider exhausted max_output_tokens again after one bounded recovery attempt; split the task into smaller steps or check the model output limit"
    s.recordAgentDebugEvent(owner,sessionID,"","provider_retry","output_budget.exhausted","RESPONSE_INCOMPLETE",pe.Message)
    return result,err,trace
   }
   constrained,nextLimit,ok:=agentRetryConstrainedOutputPayload(replayPayload,p)
   if !ok{return result,err,trace}
   if _,e:=agentchat.MeasureProviderPayload(p.APIType,constrained);e!=nil{return result,err,trace}
   replayPayload=constrained
   incompleteRecoverySent=true
   s.recordAgentDebugEvent(owner,sessionID,"","provider_retry","output_budget.recovery","","Retrying one bounded response with output budget "+strconv.Itoa(nextLimit)+" and concise completion guidance; no incomplete tool result was executed")
  }
  next:=time.Now().UTC().Add(agentRetryDelay)
  agentRetryPublish(s.Workspace,owner,sessionID,agentRetryProgress{
   Mode:mode,Attempt:attempt,Remaining:remaining,Phase:"waiting",
   NextAt:&next,LastCode:agentRetryCode(err),SameDigestFailures:sameDigestFailures,Outcome:outcome,RecoveryGeneration:generation,
  })
  if err:=wait(ctx,agentRetryDelay);err!=nil{return agentchat.ResponsesResult{},err,trace}
 }
}
func (s *Server) agentChatRetryAPI(w http.ResponseWriter,r *http.Request,id string){
 if r.Method!=http.MethodPost {http.Error(w,"method not allowed",405);return}
 r.Body=http.MaxBytesReader(w,r.Body,2048)
 var req struct{
  Mode agentchat.RetryMode `json:"mode"`
  ExpectedRevision uint64 `json:"expected_revision"`
  ConfirmExternal bool `json:"confirm_external"`
 }
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if dec.Decode(&req)!=nil{http.Error(w,"invalid retry setting",400);return}
 var trailing any
 if err:=dec.Decode(&trailing);!errors.Is(err,io.EOF){http.Error(w,"exactly one setting required",400);return}
 if req.Mode!=agentchat.RetryOff && req.Mode!=agentchat.RetryLimited && req.Mode!=agentchat.RetryUnlimited{
  http.Error(w,"mode must be off, limited or unlimited",400);return
 }
 if req.Mode!=agentchat.RetryOff&&!req.ConfirmExternal{
  http.Error(w,"explicit provider retry consent required",403);return
 }
 owner:=aiPlanOwner(r)
 store,err:=s.agentChatStore()
 if err!=nil{http.Error(w,"agent journal unavailable",500);return}
 x,err:=store.UpdateAutomation(id,owner,s.Workspace,req.ExpectedRevision,"retry",false,req.Mode)
 if errors.Is(err,agentchat.ErrNotFound){http.Error(w,"session not found",404);return}
 if err!=nil{http.Error(w,"cannot persist retry setting",409);return}
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,200,map[string]any{"session":x,"retry_mode":x.RetryMode})
}
