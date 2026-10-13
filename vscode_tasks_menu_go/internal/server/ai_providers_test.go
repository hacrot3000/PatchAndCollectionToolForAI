package server

import (
 "bytes"
 "encoding/json"
 "net/http"
 "strings"
 "testing"
)

func TestAIProviderFixedEndpointsAndDefaults(t *testing.T){
 tests:=[]struct{id,endpoint,model string}{
  {"openai","https://api.openai.com/v1/chat/completions","gpt-5.4-mini"},
  {"anthropic","https://api.anthropic.com/v1/messages","claude-sonnet-5-5"},
  {"gemini","https://generativelanguage.googleapis.com/v1beta/openai/chat/completions","gemini-3.8-flash"},
  {"deepseek","https://api.deepseek.com/chat/completions","deepseek-chat"},
  {"groq","https://api.groq.com/openai/v1/chat/completions","llama-3.3-70b-versatile"},
  {"openrouter","https://openrouter.ai/api/v1/chat/completions","openai/gpt-5.4-mini"},
 }
 for _,tc:=range tests{
  got,err:=aiProviderConfig(tc.id)
  if err!=nil||got.endpoint!=tc.endpoint||got.defaultModel!=tc.model{
   t.Errorf("provider %s: %+v %v",tc.id,got,err)
  }
  cfg,err:=normalizeAISettings(aiSettings{Provider:tc.id})
  if err!=nil||cfg.Model!=tc.model||cfg.Provider!=tc.id{
   t.Errorf("defaults for %s: %+v %v",tc.id,cfg,err)
  }
  if !strings.HasPrefix(got.endpoint,"https://"){t.Fatalf("non-TLS AI endpoint %q",got.endpoint)}
 }
 for _,id:=range []string{"","localhost","http://127.0.0.1","../escape","openai@example","OPENAI"}{
  if id==""{continue}
  if _,err:=aiProviderConfig(id);err==nil{t.Errorf("unlisted provider %q accepted",id)}
 }
 if _,err:=normalizeAISettings(aiSettings{Provider:"fake"});err==nil{t.Fatal("invalid provider saved")}
}

func TestAIProviderRequestProtocolsAndResponses(t *testing.T){
 for _,provider:=range []string{"openai","anthropic","gemini","deepseek","groq","openrouter"}{
  reqBody,err:=aiProviderRequest(provider,"model/example","Policy","The user prompt")
  if err!=nil{t.Fatal(err)}
  var decoded map[string]any
  if err=json.Unmarshal(reqBody,&decoded);err!=nil{t.Fatal(err)}
  if decoded["model"]!="model/example"||decoded["messages"]==nil{t.Errorf("%s missing model/messages",provider)}
  if provider=="anthropic"{
   if decoded["system"]!="Policy"||decoded["max_tokens"]==nil{t.Fatalf("Claude native Messages payload: %+v",decoded)}
   content:=[]byte(`{"content":[{"type":"thinking","text":"not public"},{"type":"text","text":"Answer"},{"type":"text","text":"Next"}]}`)
   answer,e:=aiProviderAnswer(provider,content)
   if e!=nil||answer!="Answer\nNext"{t.Fatalf("Claude text extraction: %q %v",answer,e)}
  }else{
   if _,ok:=decoded["system"];ok{t.Fatalf("%s must use Chat Completions payload",provider)}
   tokenField:="max_tokens";if provider=="openai"{tokenField="max_completion_tokens"}
   if decoded[tokenField]==nil{t.Errorf("%s missing %s",provider,tokenField)}
   answer,e:=aiProviderAnswer(provider,[]byte(`{"choices":[{"message":{"content":"Working"}}]}`))
   if e!=nil||answer!="Working"{t.Errorf("%s answer: %q %v",provider,answer,e)}
  }
 }
}

func TestAIProviderHeadersAndCredentials(t *testing.T){
 for _,provider:=range []string{"openai","anthropic","gemini","deepseek","groq","openrouter"}{
  spec,_:=aiProviderConfig(provider)
  req,err:=http.NewRequest(http.MethodPost,spec.endpoint,bytes.NewReader(nil))
  if err!=nil{t.Fatal(err)}
  aiProviderHeaders(req,provider,[]byte("test-secret"))
  if req.Header.Get("Content-Type")!="application/json"{t.Fatal("content type")}
  if provider=="anthropic"{
   if req.Header.Get("x-api-key")!="test-secret"||req.Header.Get("anthropic-version")!="2023-06-01"{t.Fatal("Claude API authentication")}
   if req.Header.Get("Authorization")!=""{t.Fatal("duplicate auth")}
  }else if req.Header.Get("Authorization")!="Bearer test-secret"{t.Fatalf("%s bearer auth",provider)}
 }
}

func TestAIProviderSecretIsolationAndLegacyOpenAIKey(t *testing.T){
 s:=newAIServerForTest(t)
 if got:=aiProviderSecretID(s.Workspace,"openai");got!=aiSecretID(s.Workspace){t.Fatal("legacy OpenAI key lost")}
 for _,provider:=range []string{"openai","anthropic","gemini","deepseek","groq","openrouter"}{
  c:=aiSettings{Provider:provider,Enabled:true,Consent:true,APIKey:"private-"+provider}
  rr:=invokeAI(t,s.aiSettingsAPI,"PUT","/api/ai/settings",c)
  if rr.Code!=200{t.Fatalf("save %s: %d %s",provider,rr.Code,rr.Body)}
  if strings.Contains(rr.Body.String(),"private-"){t.Fatalf("secret returned to frontend for %s",provider)}
 }
 for _,provider:=range []string{"openai","anthropic","gemini","deepseek","groq","openrouter"}{
  key,err:=s.ConnectionSecrets.Get(aiProviderSecretID(s.Workspace,provider))
  if err!=nil||string(key)!="private-"+provider{t.Fatalf("wrong stored key %s: %q %v",provider,key,err)}
 }
 // Switching providers must not overwrite or delete the previously stored key.
 get:=invokeAI(t,s.aiSettingsAPI,"GET","/api/ai/settings",nil)
 var active aiSettings
 if err:=json.Unmarshal(get.Body.Bytes(),&active);err!=nil{t.Fatal(err)}
 if active.Provider!="openrouter"||!active.HasAPIKey||active.APIKey!=""{t.Fatalf("current key status: %+v",active)}
 if _,err:=normalizeAISettings(aiSettings{Provider:"openrouter",Model:"anthropic/claude-sonnet-5-5"});err!=nil{t.Fatal("slash model ID must be allowed",err)}
 if _,err:=normalizeAISettings(aiSettings{Provider:"openai",Model:"model name with spaces"});err==nil{t.Fatal("unsafe model ID was accepted")}
}
