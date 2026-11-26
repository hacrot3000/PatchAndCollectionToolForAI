package agentchat

import (
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "time"
)

type cachePrefixPart struct{
 Digest string `json:"digest"`
 Bytes int `json:"bytes"`
}
type cachePrefixSnapshot struct{
 Instructions cachePrefixPart `json:"instructions"`
 Tools cachePrefixPart `json:"tools"`
 Input []cachePrefixPart `json:"input"`
 Stateful bool `json:"stateful"`
}
func cachePart(raw []byte)cachePrefixPart{return cachePrefixPart{Digest:BodyDigest(raw),Bytes:len(raw)}}

// Compare serialized semantic segments, not raw JSON field ordering. This
// is a local diagnostic, not the upstream token prefix or actual cache hit.
func cacheSnapshot(api string,payload []byte)(cachePrefixSnapshot,error){
 var out cachePrefixSnapshot
 if len(payload)>maxRequestBytes{return out,ErrInvalidSession}
 var m struct{
  Instructions json.RawMessage `json:"instructions"`
  Tools json.RawMessage `json:"tools"`
  Input []json.RawMessage `json:"input"`
  Previous string `json:"previous_response_id"`
  Conversation string `json:"conversation"`
 }
 if json.Unmarshal(payload,&m)!=nil||len(m.Input)>maxItems||len(m.Instructions)==0{return out,ErrInvalidSession}
 out.Stateful=m.Previous!=""||m.Conversation!=""
 if api=="chat_completions"{
  wire,e:=BuildChatCompletionsRequest(payload);if e!=nil{return out,e}
  var chat struct{Messages []json.RawMessage `json:"messages"`;Tools json.RawMessage `json:"tools"`}
  if json.Unmarshal(wire,&chat)!=nil||len(chat.Messages)==0{return out,ErrInvalidSession}
  m.Instructions=chat.Messages[0];m.Tools=chat.Tools;m.Input=chat.Messages[1:]
 }
 out.Instructions=cachePart(m.Instructions);out.Tools=cachePart(m.Tools)
 for _,raw:=range m.Input{out.Input=append(out.Input,cachePart(raw))}
 return out,nil
}
func compareCacheSnapshots(a,b cachePrefixSnapshot)(int,int,string){
 if a.Stateful||b.Stateful{return 0,0,"stateful_server_context_unobservable"}
 if a.Tools!=b.Tools{return 0,0,"tool_schema_changed"}
 if a.Instructions!=b.Instructions{return 0,0,"instructions_changed"}
 bytes:=a.Tools.Bytes+a.Instructions.Bytes
 n:=0
 for n<len(a.Input)&&n<len(b.Input)&&a.Input[n]==b.Input[n]{bytes+=a.Input[n].Bytes;n++}
 reason:="append_only"
 if n<len(a.Input){reason="history_prefix_changed"}
 return bytes,n,reason
}
type cacheObservation struct{
 Version int `json:"version"`
 Binding string `json:"binding"`
 Scope string `json:"scope"`
 At time.Time `json:"at"`
 Prefix cachePrefixSnapshot `json:"prefix"`
 Attempts uint64 `json:"attempts"`
 Successes uint64 `json:"successes"`
 Reported uint64 `json:"reported"`
 Misses uint64 `json:"misses"`
 ReadTokens uint64 `json:"read_tokens"`
 WriteTokens uint64 `json:"write_tokens"`
 ComparableRead uint64 `json:"comparable_read"`
 ComparableInput uint64 `json:"comparable_input"`
}
func(s *Store)cacheObservationPath(id string)string{return filepath.Join(s.dir,id+".cache-observation.json")}
const maxCacheObservationBytes=64<<10

// Non-authoritative, owner/project/credential/endpoint scoped metadata.
// One bounded record per conversation. No raw prompts or responses saved.
func(s *Store)RecordCacheObservation(id,owner string,p ProviderConfig,credential string,payload []byte,at time.Time,input int,usage ProviderCacheUsage,success bool)(string,string,error){
 if !validID(id)||at.IsZero(){return "","",ErrInvalidSession}
 snapshot,e:=cacheSnapshot(p.APIType,payload);if e!=nil{return "","",e}
 measurement,e:=MeasureProviderPayload(p.APIType,payload);if e!=nil{return "","",e}
 var wire struct{Model string `json:"model"`}
 if json.Unmarshal(payload,&wire)!=nil||wire.Model==""{return "","",ErrInvalidSession}
 var detail,stats string
 err:=s.withLock(func()error{
  x,e:=s.readLocked(id);if e!=nil{return e}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  if x.ProviderRun==nil||x.ProviderRun.Phase!="sending"||x.Status!="requesting_ai"{return ErrInvalidSession}
  model,ok:=p.FindModel(wire.Model);if !ok{return ErrInvalidSession}
  scope:=BodyDigest([]byte(ProviderAffinityKey(x,p,wire.Model)+"\x00"+RunProviderConfigVersion(p,model)+"\x00"+RunCredentialFingerprint([]byte(credential))))
  binding:=cacheEpochBinding(x)
  cur:=cacheObservation{Version:1,Binding:binding,Scope:scope}
  path:=s.cacheObservationPath(id)
  if fi,e:=os.Lstat(path);e==nil{
   if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||fi.Size()>maxCacheObservationBytes{return ErrInvalidSession}
   raw,e:=os.ReadFile(path);if e!=nil{return e}
   var old cacheObservation
   if len(raw)<=maxCacheObservationBytes&&json.Unmarshal(raw,&old)==nil&&old.Version==1&&old.Binding==binding&&old.Scope==scope&&len(old.Prefix.Input)<=maxItems&&old.Attempts<1000000000&&old.Successes<=old.Attempts&&old.Reported<=old.Successes&&old.Misses<=old.Reported&&old.ComparableRead<=old.ComparableInput&&old.ComparableInput<=1000000000000000000&&old.ReadTokens<=1000000000000000000&&old.WriteTokens<=1000000000000000000&&!old.At.After(at){cur=old}
  }else if !os.IsNotExist(e){return e}
  reason:="cold_or_scope_changed";bytes,n:=0,0;gap:="unreported"
  if cur.Attempts>0{
   bytes,n,reason=compareCacheSnapshots(cur.Prefix,snapshot)
   gap=fmt.Sprintf("%.1fs",at.Sub(cur.At).Seconds())
  }
  cur.At=at;cur.Prefix=snapshot;cur.Attempts++
  if success{
   cur.Successes++
   if validReportedTokenCount(usage.ReadTokens){
    cur.Reported++;cur.ReadTokens+=uint64(*usage.ReadTokens);if *usage.ReadTokens==0{cur.Misses++}
    if usage.ComparableTotal&&input>0&&input<=1000000000&&*usage.ReadTokens<=input{cur.ComparableRead+=uint64(*usage.ReadTokens);cur.ComparableInput+=uint64(input)}
   }
   if validReportedTokenCount(usage.WriteTokens){cur.WriteTokens+=uint64(*usage.WriteTokens)}
  }
  hit:="unreported";if cur.ComparableInput>0{hit=fmt.Sprintf("%.1f%%",100*float64(cur.ComparableRead)/float64(cur.ComparableInput))}
  ratio:="unreported";if input>0&&input<=1000000000&&success&&!snapshot.Stateful{ratio=fmt.Sprintf("%.2f",float64(measurement.WireBytes)/float64(input))}
  detail=fmt.Sprintf("reason=%s; instructions_digest=%s; tools_digest=%s; common_prefix_segment_bytes=%d; common_input_items=%d; previous_attempt_gap=%s; provider_input_tokens=%d; measured_wire_bytes_per_input_token=%s",reason,snapshot.Instructions.Digest,snapshot.Tools.Digest,bytes,n,gap,input,ratio)
  stats=fmt.Sprintf("scope attempts=%d; success=%d; read_reported=%d; zero_read=%d; reported_cache_read_total=%d; reported_cache_write_total=%d; comparable_weighted_hit=%s",cur.Attempts,cur.Successes,cur.Reported,cur.Misses,cur.ReadTokens,cur.WriteTokens,hit)
  data,e:=json.Marshal(cur);if e!=nil{return e};if len(data)>maxCacheObservationBytes{return ErrInvalidSession}
  tmp,e:=os.CreateTemp(s.dir,".agent-cache-observation-*");if e!=nil{return e}
  defer os.Remove(tmp.Name());defer tmp.Close()
  if e=tmp.Chmod(0600);e!=nil{return e}
  if _,e=tmp.Write(data);e!=nil{return e}
  if e=tmp.Sync();e!=nil{return e}
  if e=tmp.Close();e!=nil{return e}
  if e=os.Rename(tmp.Name(),path);e!=nil{return e}
  return s.syncDirectory()
 })
 return detail,stats,err
}
