package agentchat

import (
 "encoding/json"
 "os"
 "path/filepath"
 "time"
)

// ProviderHeartbeat is a liveness hint for an in-process worker. A heartbeat
// NEVER proves an AI provider acknowledged bytes or tool execution succeeded.
// Do not use it as authority to replay an UNKNOWN/SENDING request.
type ProviderHeartbeat struct {
 RunID string `json:"run_id"`
 At time.Time `json:"at"`
}

func (s *Store) providerHeartbeatFile(id string)string{
 return filepath.Join(s.dir,id+".provider-heartbeat.json")
}

// TouchActiveProviderHeartbeat updates a private sidecar without rewriting the
// canonical session revision, so a multi-day provider request never conflicts
// with Collection/Patch compare-and-swap journal updates.
func (s *Store) TouchActiveProviderHeartbeat(id,owner string)error{
 if !validID(id){return ErrNotFound}
 return s.withLock(func()error{
  x,err:=s.readLocked(id);if err!=nil{return err}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  if x.Status!="requesting_ai"||x.ProviderRun==nil||
   x.ProviderRun.Phase!="sending"{return ErrInvalidSession}
  raw,err:=json.Marshal(ProviderHeartbeat{RunID:x.ProviderRun.ID,At:time.Now().UTC()})
  if err!=nil{return err}
  temp,err:=os.CreateTemp(s.dir,".agent-provider-heartbeat-*")
  if err!=nil{return err}
  defer os.Remove(temp.Name())
  if err=temp.Chmod(0600);err!=nil{temp.Close();return err}
  if _,err=temp.Write(raw);err!=nil{temp.Close();return err}
  if err=temp.Sync();err!=nil{temp.Close();return err}
  if err=temp.Close();err!=nil{return err}
  return os.Rename(temp.Name(),s.providerHeartbeatFile(id))
 })
}

// LatestProviderHeartbeat never infers current liveness from a stale timestamp.
// Caller may choose a threshold; only a matching run ID can be shown.
func (s *Store) LatestProviderHeartbeat(id,owner string)(ProviderHeartbeat,error){
 if !validID(id){return ProviderHeartbeat{},ErrNotFound}
 var h ProviderHeartbeat
 err:=s.withLock(func()error{
  x,err:=s.readLocked(id);if err!=nil{return err}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  if x.ProviderRun==nil{return ErrNotFound}
  fi,err:=os.Lstat(s.providerHeartbeatFile(id));if err!=nil{return err}
  if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||fi.Size()>1024{return ErrInvalidSession}
  raw,err:=os.ReadFile(s.providerHeartbeatFile(id));if err!=nil{return err}
  if err=json.Unmarshal(raw,&h);err!=nil{return ErrInvalidSession}
  if h.RunID!=x.ProviderRun.ID||h.At.IsZero(){return ErrNotFound}
  return nil
 })
 return h,err
}
