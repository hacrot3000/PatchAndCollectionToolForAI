package server

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/url"
 "strings"
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
func agentProviderRetryable(err error)bool{
 if err==nil{return false}
 if errors.Is(err,context.Canceled){return false}
 var pe *agentchat.ProviderError
 if errors.As(err,&pe){
  if pe.Retryable{return true}
  // A 499 response means the upstream gateway/client canceled an
  // in-flight request. It is transient for an explicitly authorized
  // retrying turn even when a provider did not set Retryable=true.
  // User Stop still wins: ctx.Err() is checked after the HTTP call.
  if pe.HTTPStatus==499||pe.HTTPStatus==429||pe.HTTPStatus==408||pe.HTTPStatus>=500{return true}
  // Provider_parse can be transient (partial stream); avoid auto replaying
  // malformed JSON/unsupported feature responses without a status.
  return pe.Ambiguous && pe.HTTPStatus==0
 }
 return errors.Is(err,context.DeadlineExceeded) ||
  errors.Is(err,io.EOF) || errors.Is(err,io.ErrUnexpectedEOF)
}
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
const agentIncompleteRecoveryNote = "\nOUTPUT-BOUND RECOVERY (prior provider response hit max_output_tokens): " +
 "This is a fresh response. No partial output or tool call from the previous attempt was accepted or executed. " +
 "Use your remaining output budget sparingly. Complete ONE small, verifiable part of the original task per response. " +
 "If all work cannot fit, provide a concise checkpoint listing completed and remaining work. " +
 "If using a function tool, submit ONE complete, valid function call or a smaller complete Patch; " +
 "NEVER truncate Python, JSON or tool arguments, and never claim unverified execution. " +
 "Do not dump large source files or repeat prior evidence; stay within max_output_tokens."

func agentRetryConstrainedOutputPayload(payload []byte,p agentchat.ProviderConfig)([]byte,int,bool){
 var req map[string]json.RawMessage
 if json.Unmarshal(payload,&req)!=nil||req==nil{return nil,0,false}
 var current int
 var modelID string
 if json.Unmarshal(req["max_output_tokens"],&current)!=nil||current<=0||
  json.Unmarshal(req["model"],&modelID)!=nil||modelID==""{return nil,0,false}
 var instructions string
 if raw,exists:=req["instructions"];exists&&json.Unmarshal(raw,&instructions)!=nil{return nil,0,false}
 if strings.Contains(instructions,"OUTPUT-BOUND RECOVERY"){return nil,0,false}
 if len(instructions)+len(agentIncompleteRecoveryNote)>(1<<20){return nil,0,false}
 cap:=32768
 for _,m:=range p.Models{
  if m.ID==modelID{
   if m.MaxOutputTokens>0{cap=m.MaxOutputTokens}
   break
  }
 }
 if cap>200000{cap=200000}
 if current>cap{cap=current}
 next:=current
 if current<cap{
  next=current*2
  if next>cap{next=cap}
 }
 req["max_output_tokens"],_=json.Marshal(next)
 req["instructions"],_=json.Marshal(instructions+agentIncompleteRecoveryNote)
 b,err:=json.Marshal(req)
 if err!=nil||len(b)>4<<20{return nil,0,false}
 return b,next,true
}

// Only a structured context-limit rejection authorizes a changed request.
// A generic 400, lost response or unknown provider state is not proof.
func agentContextLimitRejected(err error) bool {
 var pe *agentchat.ProviderError
 if !errors.As(err,&pe)||pe.HTTPStatus!=400||pe.Ambiguous {return false}
 switch pe.ProviderErrorCode {
 case "context_length_exceeded","context_window_exceeded":return true
 default:return false
 }
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
 cut:=len(items)-1
 if items[cut].Type=="function_call_output"{
  for cut>0&&items[cut-1].Type=="function_call_output"{cut--}
 }else if items[cut].Type!="user"{return nil,false}
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
 if len(req["input"])>=len(old["input"])&&!(limit>0&&limit<oldLimit){return nil,false}
 return b,true
}

// agentChatProviderWithRetry runs one 600-second-scoped provider attempt at a
// time. A limited retry means ONE initial attempt + FIVE retries 10s apart.
// Unlimited transport retries stop on success, Stop or shutdown. A repeated
// max_output_tokens error is different: at most one guided recovery request
// is allowed, even in unlimited mode. Native Patch execution is NEVER replayed.
func (s *Server) agentChatProviderWithRetry(ctx context.Context,owner,sessionID string,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error,string){
 mode:=agentRetryModeForSession(s,owner,sessionID)
 replayPayload:=payload
 incompleteRecoverySent:=false
 contextRecoverySent:=false
 // The durable SENDING journal is the only origin of correlation data.
 // Direct capability probes and legacy tests without a live egress marker
 // keep using the existing transport without manufacturing a fake receipt.
 var attemptStore *agentchat.Store
 var runID string
 if sessionID!=""&&owner!=""{
  if candidate,e:=s.agentChatStore();e==nil{
   if x,e:=candidate.Get(sessionID,owner);e==nil&&x.Workspace==s.Workspace&&
    x.Status=="requesting_ai"&&x.ProviderRun!=nil&&x.ProviderRun.Phase=="sending"{
    attemptStore=candidate
    runID=x.ProviderRun.ID
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
  agentRetryPublish(s.Workspace,owner,sessionID,agentRetryProgress{Mode:mode,Attempt:attempt,Remaining:remaining,Phase:"sending"})
  callCtx,cancel:=context.WithTimeout(ctx,agentchat.ProviderAttemptTimeout)
  if attemptStore!=nil{
   // Flush the audit sidecar BEFORE each individual network attempt.
   // All retry attempts get distinct IDs, even if output budgets change.
   // Corrupt/missing storage blocks the attempt rather than losing the
   // correlation that a provider support investigation will need.
   clientID,e:=attemptStore.RecordProviderAttempt(
    sessionID,owner,runID,uint64(attempt),replayPayload,headerSupported)
   if e!=nil{
    cancel()
    return agentchat.ResponsesResult{},e,""
   }
   if headerSupported{
    callCtx=agentchat.WithProviderClientRequestID(callCtx,clientID)
   }
  }
  result,err,trace:=s.agentChatObservedCall(callCtx,owner,"provider_request",sessionID,p,key,replayPayload)
  cancel()
  // User Stop/daemon shutdown is authoritative even when the transport
  // reports a generic provider error after the context was cancelled.
  if ctx.Err()!=nil{return agentchat.ResponsesResult{},ctx.Err(),trace}
  if err==nil{return result,nil,trace}
  if mode==agentchat.RetryOff||(mode==agentchat.RetryLimited&&attempt>=6)||ctx.Err()!=nil{
   return result,err,trace
  }
  if agentContextLimitRejected(err){
   if contextRecoverySent{return result,err,trace}
   reduced,ok:=agentRetryContextPayload(replayPayload)
   if !ok{return result,err,trace}
   replayPayload=reduced
   contextRecoverySent=true
   s.recordAgentDebugEvent(owner,sessionID,"","provider_retry","context_budget.recovery",agentRetryFailureDetail(err),"Retrying one smaller stateless context after explicit provider rejection; native tool receipt preserved")
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
   replayPayload=constrained
   incompleteRecoverySent=true
   s.recordAgentDebugEvent(owner,sessionID,"","provider_retry","output_budget.recovery","","Retrying one bounded response with output budget "+strconv.Itoa(nextLimit)+" and concise completion guidance; no incomplete tool result was executed")
  }
  next:=time.Now().UTC().Add(agentRetryDelay)
  agentRetryPublish(s.Workspace,owner,sessionID,agentRetryProgress{
   Mode:mode,Attempt:attempt,Remaining:remaining,Phase:"waiting",
   NextAt:&next,LastCode:agentRetryCode(err),
  })
  timer:=time.NewTimer(agentRetryDelay)
  select{
  case <-ctx.Done():
   if !timer.Stop(){select{case <-timer.C:default:}}
   return agentchat.ResponsesResult{},ctx.Err(),trace
  case <-timer.C:
  }
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
 release,ok:=lockAgentTurn(s.Workspace,owner,id)
 if !ok{http.Error(w,"conversation currently busy; press Stop before changing retry settings",409);return}
 defer release()
 store,err:=s.agentChatStore()
 if err!=nil{http.Error(w,"agent journal unavailable",500);return}
 x,err:=store.Get(id,owner)
 if err!=nil{http.Error(w,"session not found",404);return}
 if x.Workspace!=s.Workspace||x.Revision!=req.ExpectedRevision{
  http.Error(w,"stale session or workspace",409);return
 }
 x.RetryMode=req.Mode
 x.AppendEvent("retry_setting",x.Status,"",string(req.Mode))
 x,err=store.Save(x,x.Revision)
 if err!=nil{http.Error(w,"cannot persist retry setting",409);return}
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,200,map[string]any{"session":x,"retry_mode":x.RetryMode})
}
