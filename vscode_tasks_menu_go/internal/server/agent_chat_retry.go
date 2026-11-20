package server

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
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
// Retry mode limits network reattempts, not the number or total duration of
// successful Collection observations. A long-running user task remains active
// until the user stops it, the server shuts down, the model finishes or a
// non-retryable provider/storage error occurs. Each provider request still has
// its independent 60-second timeout in agentChatProviderWithRetry.
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
func agentRetryCode(err error)string{
 var pe *agentchat.ProviderError
 if errors.As(err,&pe) && pe.Code!=""{return pe.Code}
 if errors.Is(err,context.DeadlineExceeded){return "PROVIDER_TIMEOUT"}
 return "TRANSIENT_TRANSPORT_FAILURE"
}
// agentChatProviderWithRetry runs one 60-second-scoped provider attempt at a
// time. A limited retry means ONE initial attempt + FIVE retries 10s apart.
// An unlimited retry stops only on success, explicit Stop or daemon shutdown.
// The same model request bytes are used for all retries; no Patch execution
// or verified tool-result egress is replayed by this helper itself.
func (s *Server) agentChatProviderWithRetry(ctx context.Context,owner,sessionID string,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error,string){
 mode:=agentRetryModeForSession(s,owner,sessionID)
 defer agentRetryPublish(s.Workspace,owner,sessionID,agentRetryProgress{Phase:"idle"})
 for attempt:=1; ;attempt++{
  if err:=ctx.Err();err!=nil{return agentchat.ResponsesResult{},err,""}
  remaining:=0
  if mode==agentchat.RetryLimited{remaining=6-attempt}
  agentRetryPublish(s.Workspace,owner,sessionID,agentRetryProgress{Mode:mode,Attempt:attempt,Remaining:remaining,Phase:"sending"})
  callCtx,cancel:=context.WithTimeout(ctx,60*time.Second)
  result,err,trace:=s.agentChatObservedCall(callCtx,owner,"provider_request",sessionID,p,key,payload)
  cancel()
  // User Stop/daemon shutdown is authoritative even when the transport
  // reports a generic provider error after the context was cancelled.
  if ctx.Err()!=nil{return agentchat.ResponsesResult{},ctx.Err(),trace}
  if err==nil{return result,nil,trace}
  if mode==agentchat.RetryOff||!agentProviderRetryable(err)||
    (mode==agentchat.RetryLimited&&attempt>=6)||ctx.Err()!=nil{
   return result,err,trace
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
