package server

import (
 "context"
 "encoding/json"
 "net/http"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentProviderRetryRecordsEveryAttemptBeforeProviderIOAndShowsReadOnlyAudit(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 payload:=[]byte(`{"model":"m","input":"immutable provider bytes"}`)
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.RetryMode=agentchat.RetryLimited
 x.ProviderRun=&agentchat.ProviderRun{
  ID:"r-session-unique",Kind:"message",Phase:"sending",
  PayloadSHA256:agentchat.ProviderPayloadDigest(payload),
  ProviderFingerprint:"real-provider-config",CreatedAt:now,UpdatedAt:now,
 }
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 delay:=agentRetryDelay;agentRetryDelay=time.Millisecond
 defer func(){agentRetryDelay=delay}()
 calls:=0
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,body []byte)(
  agentchat.ResponsesResult,error,
 ){
  calls++
  if key!="test-key"||string(body)!=string(payload){t.Fatal("provider payload changed across retry")}
  // The private sidecar MUST be durably committed before the mock gets
  // control, and the attempt IDs must be unique for all three calls.
  audit,e:=store.ProviderAttempts(id,"local")
  if e!=nil||audit==nil||audit.Total!=uint64(calls)||
   len(audit.Attempts)!=calls||audit.Attempts[calls-1].Attempt!=uint64(calls)||
   !audit.Attempts[calls-1].HeaderConfigured{
   t.Fatalf("attempt %d crossed network without private ledger: %v %+v",calls,e,audit)
  }
  if calls<3{
   return agentchat.ResponsesResult{},&agentchat.ProviderError{
    Code:"HTTP_503",HTTPStatus:503,Retryable:true,
   }
  }
  return agentchat.ResponsesResult{Text:"One verified provider response"},nil
 }
 provider:=agentchat.ProviderConfig{
  ID:"openai",Name:"OpenAI API",APIType:"responses",BaseURL:"https://api.openai.com/v1",
  Models:[]agentchat.ModelConfig{{ID:"m",Name:"Test model"}},
 }
 result,err,_:=s.agentChatCallForOwner(context.Background(),"local",id,provider,"test-key",payload)
 if err!=nil||calls!=3||result.Text!="One verified provider response"{
  t.Fatalf("provider retry result unexpected: calls=%d %v",calls,err)
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,
  "/api/agent/sessions/"+id,nil)
 if w.Code!=http.StatusOK{t.Fatalf("GET conversation failed: %d %s",w.Code,w.Body)}
 var info struct{
  ProviderAttempts *agentchat.ProviderAttemptAudit `json:"provider_attempts"`
  ProviderAttemptsIntegrityError bool `json:"provider_attempts_integrity_error"`
 }
 if err=json.Unmarshal(w.Body.Bytes(),&info);err!=nil{t.Fatal(err)}
 if info.ProviderAttemptsIntegrityError||info.ProviderAttempts==nil||
  info.ProviderAttempts.Total!=3||len(info.ProviderAttempts.Attempts)!=3||
  info.ProviderAttempts.RunID!=x.ProviderRun.ID{
  t.Fatalf("session GET dropped owner correlation evidence: %+v",info)
 }
 seen:=map[string]bool{}
 for _,attempt:=range info.ProviderAttempts.Attempts{
  if seen[attempt.ClientRequestID]||len(attempt.ClientRequestID)!=67||
   strings.Contains(attempt.ClientRequestID,"test-key")||
   attempt.PayloadSHA256!=agentchat.ProviderPayloadDigest(payload){
   t.Fatalf("duplicate, secret-bearing or incorrect attempt ID: %+v",attempt)
  }
  seen[attempt.ClientRequestID]=true
 }
 // After UNKNOWN on restart, provider attempts remain read-only audit
 // evidence, not an authorization to re-send the last model request.
 current,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 current.Status="failed"
 current.ProviderRun.Phase="unknown"
 current,err=store.Save(current,current.Revision);if err!=nil{t.Fatal(err)}
 s.agentReconcilePendingOnStartup(context.Background())
 if calls!=3{t.Fatal("ambiguous provider attempt was replayed after restart")}
 audit,err:=store.ProviderAttempts(id,"local")
 if err!=nil||audit.Total!=3{t.Fatalf("provider support audit lost on restart: %v",err)}
 if r:=agentchat.ProviderEgressReviewForSession(current);r==nil||
  r.AutomaticReplayAllowed||!r.RequiresReconciliation{
  t.Fatal("correlation ledger incorrectly changed provider replay policy")
 }
}
