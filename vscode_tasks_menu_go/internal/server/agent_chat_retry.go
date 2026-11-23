package server

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/url"
 "sync"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
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
// Preserve independent 180s per network attempt, retry-mode limits and explicit Stop.
func agentRetryTurnContext(parent context.Context,_ agentchat.RetryMode)(context.Context,context.CancelFunc){
 return context.WithCancel(parent)
}
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
  if pe.HTTPStatus==429||pe.HTTPStatus==408||pe.HTTPStatus>=500{return true}
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
// An incomplete max_output_tokens response requires a larger output budget,
// not an infinite loop of byte-identical requests. History and tool inputs
// remain unchanged; the ceiling is the selected model's configured maximum.
func agentRetryLargerOutputPayload(payload []byte,p agentchat.ProviderConfig)([]byte,bool){
 var req map[string]json.RawMessage
 if json.Unmarshal(payload,&req)!=nil{return nil,false}
 var current int
 var modelID string
 if json.Unmarshal(req["max_output_tokens"],&current)!=nil||current<=0||
  json.Unmarshal(req["model"],&modelID)!=nil{return nil,false}
 cap:=32768
 for _,m:=range p.Models {
  if m.ID==modelID{if m.MaxOutputTokens>0{cap=m.MaxOutputTokens};break}
 }
 if cap>200000{cap=200000}
 if current>=cap{return nil,false}
 next:=current*2
 if next>cap{next=cap}
 req["max_output_tokens"],_=json.Marshal(next)
 b,err:=json.Marshal(req)
 return b,err==nil
}

// agentChatProviderWithRetry runs one 180-second-scoped provider attempt at a
// time. A limited retry means ONE initial attempt + FIVE retries 10s apart.
// An unlimited retry stops only on success, explicit Stop or daemon shutdown.
// The same model request bytes are used for all retries; no Patch execution
// or verified tool-result egress is replayed by this helper itself.
func (s *Server) agentChatProviderWithRetry(ctx context.Context,owner,sessionID string,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error,string){
 mode:=agentRetryModeForSession(s,owner,sessionID)
 replayPayload:=payload
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
  callCtx,cancel:=context.WithTimeout(ctx,3*time.Minute)
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
  if mode==agentchat.RetryOff||!agentProviderRetryable(err)||
    (mode==agentchat.RetryLimited&&attempt>=6)||ctx.Err()!=nil{
   return result,err,trace
  }
  var pe *agentchat.ProviderError
  if errors.As(err,&pe)&&pe.Code=="RESPONSE_INCOMPLETE"&&pe.ProviderErrorCode=="max_output_tokens"{
   expanded,ok:=agentRetryLargerOutputPayload(replayPayload,p)
   if !ok{return result,err,trace}
   replayPayload=expanded
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
