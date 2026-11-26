package agentchat

import (
 "encoding/json"
 "os"
 "path/filepath"
)

// CacheEpoch is a private, bounded derived projection. It is never used
// as proof of tool execution, approval, delivery, or canonical history.
type CacheEpoch struct {
 ID string `json:"id"`
 Scope string `json:"scope"`
 SourceCount int `json:"source_count"`
 SourceDigest string `json:"source_digest"`
 Visible []HistoryItem `json:"visible"`
}
type cacheEpochRecord struct{
 Version int `json:"version"`
 Binding string `json:"binding"`
 Epoch CacheEpoch `json:"epoch"`
 Digest string `json:"digest"`
}
const maxCacheEpochBytes=2<<20
func(s *Store)cacheEpochPath(id string)string{return filepath.Join(s.dir,id+".cache-epoch.json")}
func cacheEpochBinding(x Session)string{
 raw,_:=json.Marshal([]string{x.ID,x.Owner,x.Workspace,x.ProjectID})
 return BodyDigest(raw)
}
func validCacheEpoch(e CacheEpoch)bool{
 if !validAffinityKey(e.ID)||!validAffinityKey(e.Scope)||!validAffinityKey(e.SourceDigest)||e.SourceCount<1||e.SourceCount>maxItems||len(e.Visible)<1||len(e.Visible)>maxItems{return false}
 for _,it:=range e.Visible{if validHistoryItem(it)!=nil{return false}}
 return validateStatelessToolPairs(e.Visible)==nil
}
func(s *Store)ReadCacheEpoch(id,owner string,revision uint64)(*CacheEpoch,error){
 if !validID(id){return nil,ErrNotFound}
 var result *CacheEpoch
 err:=s.withLock(func()error{
  x,e:=s.readLocked(id);if e!=nil{return e}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  if x.Revision!=revision{return ErrStaleRevision}
  p:=s.cacheEpochPath(id);fi,e:=os.Lstat(p)
  if os.IsNotExist(e){return nil};if e!=nil{return e}
  if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||fi.Size()<2||fi.Size()>maxCacheEpochBytes{return ErrInvalidSession}
  raw,e:=os.ReadFile(p);if e!=nil{return e}
  var rec cacheEpochRecord
  if len(raw)>maxCacheEpochBytes||json.Unmarshal(raw,&rec)!=nil||rec.Version!=1||rec.Binding!=cacheEpochBinding(x)||!validCacheEpoch(rec.Epoch){return ErrInvalidSession}
  b,_:=json.Marshal(rec.Epoch)
  if BodyDigest(b)!=rec.Digest{return ErrInvalidSession}
  result=&rec.Epoch;return nil
 })
 return result,err
}
func(s *Store)WriteCacheEpoch(id,owner string,revision uint64,epoch *CacheEpoch)error{
 if !validID(id){return ErrNotFound}
 return s.withLock(func()error{
  x,e:=s.readLocked(id);if e!=nil{return e}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  if x.Revision!=revision{return ErrStaleRevision}
  path:=s.cacheEpochPath(id)
  if epoch==nil{if e=os.Remove(path);e!=nil&&!os.IsNotExist(e){return e};return nil}
  if !validCacheEpoch(*epoch){return ErrInvalidSession}
  b,e:=json.Marshal(epoch);if e!=nil{return e}
  data,e:=json.Marshal(cacheEpochRecord{Version:1,Binding:cacheEpochBinding(x),Epoch:*epoch,Digest:BodyDigest(b)})
  if e!=nil{return e};if len(data)>maxCacheEpochBytes{return ErrInvalidSession}
  tmp,e:=os.CreateTemp(s.dir,".agent-cache-epoch-*");if e!=nil{return e}
  defer os.Remove(tmp.Name());defer tmp.Close()
  if e=tmp.Chmod(0600);e!=nil{return e}
  if _,e=tmp.Write(data);e!=nil{return e}
  if e=tmp.Sync();e!=nil{return e}
  if e=tmp.Close();e!=nil{return e}
  if e=os.Rename(tmp.Name(),path);e!=nil{return e}
  return s.syncDirectory()
 })
}


// Invalidate only the derived projection after a retry changes the request.
// The staged provider payload and canonical session are never modified.
// Fence this operation to the exact currently SENDING provider run.
func(s *Store)InvalidateCacheEpochForRun(id,owner,runID string)error{
 if !validID(id)||runID==""{return ErrInvalidSession}
 return s.withLock(func()error{
  x,e:=s.readLocked(id);if e!=nil{return e}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  if x.ProviderRun==nil||x.ProviderRun.ID!=runID||x.ProviderRun.Phase!="sending"||x.Status!="requesting_ai"{return ErrStaleRevision}
  if e=os.Remove(s.cacheEpochPath(id));e!=nil&&!os.IsNotExist(e){return e}
  return s.syncDirectory()
 })
}
