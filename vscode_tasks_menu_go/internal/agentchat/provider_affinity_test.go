package agentchat

import (
 "context"
 "encoding/json"
 "net/http"
 "strings"
 "testing"
)

func affinityFixture(t *testing.T)(Session,ProviderConfig){
 t.Helper()
 x,err:=NewSession("local","/project","project-id","gateway","model",Stateless);if err!=nil{t.Fatal(err)}
 p:=ProviderConfig{ID:"gateway",Name:"Gateway",APIType:"responses",BaseURL:"https://gateway.example/v1",Models:[]ModelConfig{{ID:"model",Name:"Model"}}}
 return x,p
}
func TestProviderAffinityStableScopedAndBothAPIs(t *testing.T){
 x,p:=affinityFixture(t)
 key:=ProviderAffinityKey(x,p,"model")
 if !validAffinityKey(key){t.Fatal("invalid key")}
 x.Revision++;x.Mode=PreviousResponse;x.PreviousResponseID="resp-new"
 if ProviderAffinityKey(x,p,"model")!=key{t.Fatal("mode or revision changed affinity")}
 for _,mutate:=range []func(*Session,*ProviderConfig){
  func(x *Session,p *ProviderConfig){x.ID=strings.Repeat("b",32)},
  func(x *Session,p *ProviderConfig){x.Owner="other"},
  func(x *Session,p *ProviderConfig){x.ProjectID="other-project"},
  func(x *Session,p *ProviderConfig){p.BaseURL="https://other.example/v1"},
 }{y,q:=x,p;mutate(&y,&q);if ProviderAffinityKey(y,q,"model")==key{t.Fatal("scope collision")}}
 if ProviderAffinityKey(x,p,"other-model")==key{t.Fatal("model collision")}
 for _,api:=range []string{"responses","chat_completions"}{
  p.APIType=api
  key=ProviderAffinityKey(x,p,"model")
  endpoint,err:=p.APIURL(api);if err!=nil{t.Fatal(err)}
  ctx:=WithProviderAffinity(context.Background(),p,key)
  for i:=0;i<3;i++{
   req,err:=newProviderRequest(ctx,endpoint,"test-key",[]byte("{}"));if err!=nil{t.Fatal(err)}
   for _,name:=range []string{"X-Session-Id","X-Session-Affinity","Session_id"}{if req.Header.Get(name)!=key{t.Fatalf("%s: missing %s",api,name)}}
  }
  req,_:=http.NewRequestWithContext(ctx,http.MethodPost,"https://other.example/v1/responses",nil);applyProviderAffinity(req)
  if req.Header.Get("X-Session-Id")!=""{t.Fatal("affinity leaked to another endpoint")}
 }
}
func TestProviderAffinityPolicyAndPromptCacheKey(t *testing.T){
 x,p:=affinityFixture(t)
 if ProviderUsesPromptCacheKey(p){t.Fatal("unknown custom body field enabled by default")}
 p.CacheAffinity="both";p.CacheSessionHeader="x-opencode-session"
 key:=ProviderAffinityKey(x,p,"model")
 endpoint,_:=p.APIURL(p.APIType)
 req,err:=newProviderRequest(WithProviderAffinity(context.Background(),p,key),endpoint,"key",[]byte("{}"))
 if err!=nil||req.Header.Get("x-opencode-session")!=key||req.Header.Get("X-Session-Id")!=""{t.Fatal("explicit header not respected")}
 payload,err:=BuildResponsesRequest(RequestConfig{Model:"model",Instructions:"stable",Mode:Stateless,Next:[]HistoryItem{{Type:"user",Content:"hi"}},PromptCacheKey:key})
 if err!=nil||PromptCacheKeyFingerprint(payload)==""{t.Fatalf("key missing: %v",err)}
 wire,err:=BuildChatCompletionsRequest(payload);if err!=nil{t.Fatal(err)}
 var got map[string]any
 if json.Unmarshal(wire,&got)!=nil||got["prompt_cache_key"]!=key{t.Fatal("Chat Completions dropped key")}
 p.CacheAffinity="off"
 req,_=newProviderRequest(WithProviderAffinity(context.Background(),p,key),endpoint,"key",payload)
 if req.Header.Get("x-opencode-session")!=""{t.Fatal("off sent affinity")}
 p.CacheAffinity="";p.CacheSessionHeader="";p.BaseURL="https://api.openai.com/v1"
 if providerUsesAffinityHeaders(p)||ProviderUsesPromptCacheKey(p){t.Fatal("official behavior unexpectedly changed")}
 p.CacheSessionHeader="Authorization";if ValidateProvider(p)==nil{t.Fatal("arbitrary header allowed")}
}
func TestProviderCacheUsageJSONAndSSE(t *testing.T){
 responses:=[]byte(`{"id":"r","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":100,"output_tokens":3,"input_tokens_details":{"cached_tokens":80},"cache_creation_input_tokens":10}}`)
 r,e:=ParseResponsesResult(responses)
 if e!=nil||r.CacheUsage.Summary(r.InputTokens)!="Cache Read: 80 · Cache Write: 10 · Cache Hit: 80.0%"{t.Fatalf("Responses usage: %+v %v",r,e)}
 chat:=[]byte(`{"id":"c","choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":100,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":0}}}`)
 r,e=ParseChatCompletionsResult(chat)
 if e!=nil||r.CacheUsage.Summary(r.InputTokens)!="Cache Read: 0 · Cache Write: unreported · Cache Hit: 0.0%"{t.Fatalf("Chat usage: %+v %v",r,e)}
 ev,e:=parseSSEEvent([]byte(`{"type":"response.completed","response":{"id":"r","status":"completed","usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":80}}}}`))
 if e!=nil||ev.CacheUsage.Summary(ev.InputTokens)!="Cache Read: 80 · Cache Write: unreported · Cache Hit: 80.0%"{t.Fatalf("Responses SSE usage: %+v %v",ev,e)}
 a:=newChatStreamAccumulator()
 if e=a.apply([]byte(`{"id":"c","choices":[{"finish_reason":"stop","delta":{"content":"ok"}}]}`));e!=nil{t.Fatal(e)}
 if e=a.apply([]byte(`{"id":"c","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":90}}}`));e!=nil{t.Fatal(e)}
 if e=a.apply([]byte("[DONE]"));e!=nil{t.Fatal(e)}
 r,e=a.result();if e!=nil||r.CacheUsage.Summary(r.InputTokens)!="Cache Read: 90 · Cache Write: unreported · Cache Hit: 90.0%"{t.Fatalf("Chat SSE usage: %+v %v",r,e)}
 if (ProviderCacheUsage{}).Summary(100)!="Cache Read: unreported · Cache Write: unreported · Cache Hit: unreported"{t.Fatal("missing cache usage treated as zero")}
}
