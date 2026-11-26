package agentchat

import (
 "context"
 "fmt"
 "net/http"
 "strings"
)

// nil is unreported; zero is an actual provider-reported cache miss.
type ProviderCacheUsage struct {
 ReadTokens *int
 WriteTokens *int
 ComparableTotal bool
}
type CacheUsageFields struct {
 InputDetails struct{CachedTokens *int `json:"cached_tokens"`} `json:"input_tokens_details"`
 PromptDetails struct{CachedTokens *int `json:"cached_tokens"`} `json:"prompt_tokens_details"`
 PromptCacheHit *int `json:"prompt_cache_hit_tokens"`
 CacheRead *int `json:"cache_read_input_tokens"`
 CacheWrite *int `json:"cache_creation_input_tokens"`
}
func validReportedTokenCount(n *int)bool{return n!=nil&&*n>=0&&*n<=1000000000}
func(f CacheUsageFields)normalized(total int)ProviderCacheUsage{
 var out ProviderCacheUsage
 for _,v:=range []*int{f.InputDetails.CachedTokens,f.PromptDetails.CachedTokens,f.PromptCacheHit}{
  if validReportedTokenCount(v)&&total>0&&*v<=total{out.ReadTokens=v;out.ComparableTotal=true;break}
 }
 if out.ReadTokens==nil&&validReportedTokenCount(f.CacheRead){out.ReadTokens=f.CacheRead}
 if validReportedTokenCount(f.CacheWrite){out.WriteTokens=f.CacheWrite}
 return out
}
func(u ProviderCacheUsage)Reported()bool{return u.ReadTokens!=nil||u.WriteTokens!=nil}
func(u ProviderCacheUsage)Summary(total int)string{
 read,write,hit:="unreported","unreported","unreported"
 if validReportedTokenCount(u.ReadTokens){read=fmt.Sprint(*u.ReadTokens);if u.ComparableTotal&&total>0&&*u.ReadTokens<=total{hit=fmt.Sprintf("%.1f%%",100*float64(*u.ReadTokens)/float64(total))}}
 if validReportedTokenCount(u.WriteTokens){write=fmt.Sprint(*u.WriteTokens)}
 return fmt.Sprintf("Cache Read: %s · Cache Write: %s · Cache Hit: %s",read,write,hit)
}
// Compare opaque backend hints without exporting arbitrary headers, hostnames,
// account identifiers or secrets. Absence is not proof of failover.
func recordProviderBackend(ctx context.Context,h http.Header){
 for _,name:=range []string{"X-Backend-Id","X-Upstream-Id","X-Provider-Id"}{
  v:=strings.TrimSpace(h.Get(name))
  if v!=""&&len(v)<=256{
   RecordProviderDiagnostic(ctx,DiagnosticEvent{Level:"info",Stage:"provider_cache",Event:"cache.backend",SafeMessage:"backend fingerprint="+BodyDigest([]byte(name+"\x00"+v))[:16]})
   return
  }
 }
}
