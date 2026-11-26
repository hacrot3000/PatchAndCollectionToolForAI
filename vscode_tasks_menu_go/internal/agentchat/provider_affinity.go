package agentchat

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "net/http"
 "net/url"
 "strings"
)

// Affinity is a routing hint only, not a server-side conversation reference,
// credential, provider acknowledgement, authorization or idempotency key.
// Defaults preserve official API behavior; custom gateways receive headers.
// Extra JSON fields require explicit provider opt-in.
func validCacheAffinity(mode,header string) bool {
 switch mode {case "", "off","headers","prompt_cache_key","both":default:return false}
 switch strings.ToLower(header){case "","x-session-id","x-session-affinity","session_id","x-opencode-session":return true}
 return false
}
func providerCacheAffinityMode(p ProviderConfig) string {
 if p.CacheAffinity!=""{return p.CacheAffinity}
 u,e:=url.Parse(p.BaseURL)
 if e!=nil||SupportsClientRequestID(u.Hostname()){return "off"}
 return "headers"
}
func ProviderUsesPromptCacheKey(p ProviderConfig)bool{
 m:=providerCacheAffinityMode(p);return m=="prompt_cache_key"||m=="both"
}
func providerUsesAffinityHeaders(p ProviderConfig)bool{
 m:=providerCacheAffinityMode(p);return m=="headers"||m=="both"
}
func validAffinityKey(key string)bool{
 if len(key)!=64{return false}
 b,e:=hex.DecodeString(key)
 return e==nil&&len(b)==32&&key==strings.ToLower(key)
}

// Stable across restart, context modes, request kinds, and retry generations.
// Never send raw owner/project/path or session ID. Model/endpoint changes
// deliberately establish an independent cache namespace.
func ProviderAffinityKey(x Session,p ProviderConfig,model string)string{
 if !validID(x.ID)||x.Owner==""||x.ProjectID==""||x.Workspace==""||p.ID==""||model==""{return ""}
 raw,_:=json.Marshal([]string{"taskdeck-affinity-v1",x.ID,x.Owner,x.ProjectID,x.Workspace,p.ID,p.APIType,strings.TrimRight(p.BaseURL,"/"),model})
 sum:=sha256.Sum256(raw)
 return hex.EncodeToString(sum[:])
}

type providerAffinityContextKey struct{}
type providerAffinityContext struct{ key,endpoint,header string }

// Called from the trusted server only after a conversation is resolved from
// its owner-scoped SENDING journal. Bind the hint to the exact target URL.
func WithProviderAffinity(ctx context.Context,p ProviderConfig,key string)context.Context{
 if !validAffinityKey(key)||!validCacheAffinity(p.CacheAffinity,p.CacheSessionHeader)||!providerUsesAffinityHeaders(p){return ctx}
 endpoint,e:=p.ResponsesURL();if e!=nil{return ctx}
 return context.WithValue(ctx,providerAffinityContextKey{},providerAffinityContext{key,endpoint,p.CacheSessionHeader})
}
func applyProviderAffinity(req *http.Request){
 if req==nil||req.URL==nil{return}
 a,ok:=req.Context().Value(providerAffinityContextKey{}).(providerAffinityContext)
 if !ok||req.URL.String()!=a.endpoint||!validAffinityKey(a.key){return}
 if a.header!=""{req.Header.Set(a.header,a.key);return}
 req.Header.Set("X-Session-Id",a.key)
 req.Header.Set("X-Session-Affinity",a.key)
 req.Header.Set("Session_id",a.key)
}
