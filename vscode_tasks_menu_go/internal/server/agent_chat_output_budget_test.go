package server

import (
 "bytes"
 "context"
 "encoding/json"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentLargeTaskInitialOutputBudget(t *testing.T){
 model:=agentchat.ModelConfig{ID:"composer-2.5",MaxOutputTokens:160000}
 for _,tc:=range []struct{name,effort string;tools bool;want int}{
  {"text_low","low",false,65536},
  {"text_high","high",false,65536},
  {"patch_medium","medium",true,65536},
  {"patch_high","high",true,98304},
  {"patch_xhigh","xhigh",true,98304},
 }{
  t.Run(tc.name,func(t *testing.T){
   got:=agentInitialOutputBudget(model,tc.effort,tc.tools)
   if got!=tc.want{t.Fatalf("budget=%d, want %d",got,tc.want)}
  })
 }
 small:=agentchat.ModelConfig{ID:"small",MaxOutputTokens:4096}
 if got:=agentInitialOutputBudget(small,"high",true);got!=4096{
  t.Fatalf("ignored configured output cap: %d",got)
 }
 if got:=agentInitialOutputBudget(agentchat.ModelConfig{MaxOutputTokens:999999},"high",true);got>200000{
  t.Fatalf("accepted budget past protocol cap: %d",got)
 }
}

func TestAgentIncompleteRecoveryPreservesCanonicalInputsAndToolPolicy(t *testing.T){
 p:=agentchat.ProviderConfig{Models:[]agentchat.ModelConfig{{ID:"composer-2.5",MaxOutputTokens:160000}}}
 canonical:=[]byte(`{"model":"composer-2.5","instructions":"Project-bound rules","input":[{"role":"user","content":"Original user task"}],"tools":[{"type":"function","name":"taskdeck_collect_workspace"}],"max_output_tokens":65536,"reasoning":{"effort":"medium"},"store":false}`)
 backup:=append([]byte(nil),canonical...)
 retry,limit,ok:=agentRetryConstrainedOutputPayload(canonical,p)
 if !ok||limit<=65536||limit>160000{
  t.Fatalf("large task recovery failed, limit=%d ok=%v",limit,ok)
 }
 if !bytes.Equal(canonical,backup){t.Fatal("retry mutated durable original request")}
 var orig,next map[string]json.RawMessage
 if err:=json.Unmarshal(canonical,&orig);err!=nil{t.Fatal(err)}
 if err:=json.Unmarshal(retry,&next);err!=nil{t.Fatal(err)}
 for _,key:=range []string{"input","tools","reasoning","model","store"}{
  if !bytes.Equal(orig[key],next[key]){
   t.Fatalf("retry changed canonical %s, risking tool-call semantics",key)
  }
 }
 var instr string
 if err:=json.Unmarshal(next["instructions"],&instr);err!=nil{t.Fatal(err)}
 if !strings.Contains(instr,"OUTPUT-BOUND RECOVERY")||
  !strings.Contains(instr,"NEVER truncate Python")||
  !strings.Contains(instr,"ONE complete, valid function call"){
  t.Fatalf("missing bounded-output safety instructions: %q",instr)
 }
 if _,_,ok:=agentRetryConstrainedOutputPayload(retry,p);ok{
  t.Fatal("second bounded replay must not be prepared")
 }
 // Even if the configured cap was reached, one concise retry is permitted
 // rather than blindly repeating an identical request.
 atCap:=[]byte(`{"model":"composer-2.5","max_output_tokens":160000,"instructions":"Rule"}`)
 bounded,finalLimit,ok:=agentRetryConstrainedOutputPayload(atCap,p)
 if !ok||finalLimit!=160000||bytes.Equal(bounded,atCap){
  t.Fatalf("configured cap prevented guided recovery: limit=%d ok=%v",finalLimit,ok)
 }
}

func TestAgentIncompleteWithRetryOffDoesNotSendAnotherProviderRequest(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++
  return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"RESPONSE_INCOMPLETE",ProviderErrorCode:"max_output_tokens",Retryable:true}
 }
 // RetryOff is the default: never consume a second provider inference unless
 // the user explicitly enables limited/unlimited retry.
 result,err,_:=s.agentChatCallForOwner(context.Background(),"local",id,
  agentchat.ProviderConfig{ID:"fake",Models:[]agentchat.ModelConfig{{ID:"composer-2.5",MaxOutputTokens:160000}}},
  "key",[]byte(`{"model":"composer-2.5","max_output_tokens":65536}`))
 _=result
 if err==nil||calls!=1{t.Fatalf("RetryOff performed extra inference: calls=%d err=%v",calls,err)}
 }
