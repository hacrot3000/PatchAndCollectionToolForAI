package agentchat

import (
 "context"
 "net/http"
 "strings"
)

// Provider request correlation is a SUPPORT LOOKUP HINT, not a provider
// acknowledgement, idempotency key or permission to retry ambiguous egress.
// The request-scoped context value is injected only by TaskDeck's trusted
// server after an owner/project-scoped SENDING ledger CAS was recorded.
type providerClientRequestIDKey struct{}

func WithProviderClientRequestID(ctx context.Context,id string)context.Context{
 if len(id)<1||len(id)>128||!strings.HasPrefix(id,"td-"){return ctx}
 for _,c:=range id{
  if (c<'a'||c>'z')&&(c<'0'||c>'9')&&c!='-'{return ctx}
 }
 return context.WithValue(ctx,providerClientRequestIDKey{},id)
}

// SupportsClientRequestID is intentionally limited to OpenAI-operated API
// hosts. Custom compatible endpoints may ignore or reject unfamiliar headers.
// A client ID on any host is NOT proof that the request was accepted.
func SupportsClientRequestID(host string)bool{
 switch strings.ToLower(host){
 case "api.openai.com","us.api.openai.com","eu.api.openai.com":
  return true
 }
 return false
}
func applyProviderClientRequestID(req *http.Request){
 if req==nil||req.URL==nil||!SupportsClientRequestID(req.URL.Hostname()){return}
 id,_:=req.Context().Value(providerClientRequestIDKey{}).(string)
 if id!=""{req.Header.Set("X-Client-Request-Id",id)}
}
