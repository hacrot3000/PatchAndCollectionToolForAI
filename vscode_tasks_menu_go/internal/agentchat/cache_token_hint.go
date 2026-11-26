package agentchat

import(
 "encoding/json"
 "os"
 "time"
)

const maxCacheTokenSamples=16
const minCacheTokenSamples=8
type cacheTokenSample struct{
 Wire int `json:"wire"`
 Input int `json:"input"`
 At time.Time `json:"at"`
}
func cacheObservationScope(x Session,p ProviderConfig,model ModelConfig,credential string)string{
 return BodyDigest([]byte(ProviderAffinityKey(x,p,model.ID)+"\x00"+RunProviderConfigVersion(p,model)+"\x00"+RunCredentialFingerprint([]byte(credential))))
}
func validCacheTokenSample(s cacheTokenSample,now time.Time)bool{
 return s.Wire>=32768&&s.Wire<=maxRequestBytes&&s.Input>=1024&&s.Input<=1000000000&&
 !s.At.IsZero()&&!s.At.After(now)&&now.Sub(s.At)<=24*time.Hour&&
 int64(s.Wire)*1000/int64(s.Input)>=1000&&int64(s.Wire)*1000/int64(s.Input)<=8000
}
func updateCacheTokenSamples(samples []cacheTokenSample,old,current cachePrefixSnapshot,wire,input int,usage ProviderCacheUsage,success bool,at time.Time)[]cacheTokenSample{
 // Only known inclusive token totals from successful stateless responses.
 // Failure, stateful mode, or changed trusted instructions/schema resets
 // confidence; do not learn a smaller input estimate from a cache hit alone.
 if !success||current.Stateful||!usage.ComparableTotal||!validReportedTokenCount(usage.ReadTokens)||*usage.ReadTokens>input{return nil}
 if old.Instructions!=current.Instructions||old.Tools!=current.Tools||old.Stateful{samples=nil}
 fresh:=cacheTokenSample{Wire:wire,Input:input,At:at}
 if !validCacheTokenSample(fresh,at){return nil}
 out:=make([]cacheTokenSample,0,maxCacheTokenSamples)
 for _,s:=range samples{if validCacheTokenSample(s,at){out=append(out,s)}}
 if len(out)>=maxCacheTokenSamples{out=out[len(out)-maxCacheTokenSamples+1:]}
 return append(out,fresh)
}

// Use the densest recent sample with a 20% bytes/token discount; never
// assume more than 2.5 bytes/token. No extrapolation to much larger/smaller
// requests. This is NOT a tokenizer, hard admission rule or provider quota.
func learnedCacheEstimate(samples []cacheTokenSample,wire int,now time.Time)(int,bool){
 if len(samples)<minCacheTokenSamples||len(samples)>maxCacheTokenSamples||wire<=0||wire>maxRequestBytes{return 0,false}
 minRatio:=int64(8000);minWire,maxWire:=maxRequestBytes,0
 for _,s:=range samples{
  if !validCacheTokenSample(s,now){return 0,false}
  r:=int64(s.Wire)*1000/int64(s.Input);if r<minRatio{minRatio=r}
  if s.Wire<minWire{minWire=s.Wire};if s.Wire>maxWire{maxWire=s.Wire}
 }
 if wire<minWire/2||int64(wire)*10>int64(maxWire)*11{return 0,false}
 ratio:=minRatio*80/100;if ratio>2500{ratio=2500};if ratio<=2000{return 0,false}
 return int((int64(wire)*1000+ratio-1)/ratio)+4,true
}
type CacheTokenHint struct{
 api string
 model string
 prefix cachePrefixSnapshot
 samples []cacheTokenSample
}
func(h *CacheTokenHint)Estimate(api string,payload []byte)(int,bool){
 if h==nil||api!=h.api{return 0,false}
 var v struct{Model string `json:"model"`}
 if json.Unmarshal(payload,&v)!=nil||v.Model!=h.model{return 0,false}
 snapshot,e:=cacheSnapshot(api,payload);if e!=nil||snapshot.Stateful||snapshot.Instructions!=h.prefix.Instructions||snapshot.Tools!=h.prefix.Tools{return 0,false}
 m,e:=MeasureProviderPayload(api,payload);if e!=nil{return 0,false}
 return learnedCacheEstimate(h.samples,m.WireBytes,time.Now().UTC())
}
func(s *Store)ReadCacheTokenHint(id,owner string,revision uint64,p ProviderConfig,modelID,credential string)(*CacheTokenHint,error){
 if !validID(id)||credential==""{return nil,ErrInvalidSession}
 model,ok:=p.FindModel(modelID);if !ok{return nil,ErrInvalidSession}
 var hint *CacheTokenHint
 err:=s.withLock(func()error{
  x,e:=s.readLocked(id);if e!=nil{return e}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  if x.Revision!=revision{return ErrStaleRevision}
  path:=s.cacheObservationPath(id)
  fi,e:=os.Lstat(path);if os.IsNotExist(e){return nil};if e!=nil{return e}
  if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||fi.Size()>maxCacheObservationBytes{return ErrInvalidSession}
  raw,e:=os.ReadFile(path);if e!=nil{return e}
  var rec cacheObservation
  if len(raw)>maxCacheObservationBytes||json.Unmarshal(raw,&rec)!=nil{return ErrInvalidSession}
  if rec.Version!=2||rec.Binding!=cacheEpochBinding(x)||rec.Scope!=cacheObservationScope(x,p,model,credential)||rec.Prefix.Stateful||len(rec.TokenSamples)<minCacheTokenSamples||len(rec.TokenSamples)>maxCacheTokenSamples{return nil}
  now:=time.Now().UTC()
  for _,sample:=range rec.TokenSamples{if !validCacheTokenSample(sample,now){return nil}}
  hint=&CacheTokenHint{api:p.APIType,model:modelID,prefix:rec.Prefix,samples:append([]cacheTokenSample(nil),rec.TokenSamples...)}
  return nil
 })
 return hint,err
}
