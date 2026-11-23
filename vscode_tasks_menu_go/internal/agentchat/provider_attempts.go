package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strconv"
 "time"
)

// ProviderAttempt is a DURABLE, private correlation hint for ONE outbound
// HTTP attempt. It is written BEFORE network I/O and therefore NEVER proves
// the provider received, processed or acknowledged a request. It does not
// authorize restarting or replaying SENDING/UNKNOWN.
type ProviderAttempt struct {
 Attempt uint64 `json:"attempt"`
 ClientRequestID string `json:"client_request_id"`
 PayloadSHA256 string `json:"payload_sha256"`
 HeaderSent bool `json:"header_sent"`
 RecordedAt time.Time `json:"recorded_at"`
}
type ProviderAttemptAudit struct {
 RunID string `json:"run_id"`
 Total uint64 `json:"total"`
 Truncated bool `json:"truncated"`
 Attempts []ProviderAttempt `json:"attempts"`
}
type providerAttemptSidecar struct {
 Version int `json:"version"`
 RunID string `json:"run_id"`
 Total uint64 `json:"total"`
 SHA256 string `json:"sha256"`
 Attempts []ProviderAttempt `json:"attempts"`
}
const maxProviderAttemptRecords=128
const maxProviderAttemptSidecarBytes=64<<10

func (s *Store) providerAttemptsPath(id string)string{
 return filepath.Join(s.dir,id+".provider-attempts.json")
}
func providerAttemptClientID(sessionID,runID string,attempt uint64,payloadDigest string)string{
 h:=sha256.Sum256([]byte(sessionID+"\x00"+runID+"\x00"+strconv.FormatUint(attempt,10)+"\x00"+payloadDigest))
 return "td-"+hex.EncodeToString(h[:])
}
func providerAttemptDigest(items []ProviderAttempt)(string,error){
 raw,err:=json.Marshal(items)
 if err!=nil{return "",err}
 d:=sha256.Sum256(raw)
 return hex.EncodeToString(d[:]),nil
}

// readProviderAttemptsLocked verifies integrity and 0600 file ownership
// while the Store's owner/project-scoped journal lock is already held.
func (s *Store) readProviderAttemptsLocked(id string)(providerAttemptSidecar,error){
 path:=s.providerAttemptsPath(id)
 fi,err:=os.Lstat(path)
 if err!=nil{return providerAttemptSidecar{},err}
 if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||
  fi.Size()<2||fi.Size()>maxProviderAttemptSidecarBytes{
  return providerAttemptSidecar{},ErrInvalidSession
 }
 raw,err:=os.ReadFile(path)
 if err!=nil{return providerAttemptSidecar{},err}
 var rec providerAttemptSidecar
 if json.Unmarshal(raw,&rec)!=nil||rec.Version!=1||rec.RunID==""||
  rec.Total==0||len(rec.Attempts)==0||len(rec.Attempts)>maxProviderAttemptRecords||
  uint64(len(rec.Attempts))>rec.Total{return providerAttemptSidecar{},ErrInvalidSession}
 digest,err:=providerAttemptDigest(rec.Attempts)
 if err!=nil||digest!=rec.SHA256{return providerAttemptSidecar{},ErrInvalidSession}
 for i,a:=range rec.Attempts{
  if a.Attempt==0||a.Attempt>rec.Total||a.RecordedAt.IsZero()||
   len(a.PayloadSHA256)!=64||
   a.ClientRequestID!=providerAttemptClientID(id,rec.RunID,a.Attempt,a.PayloadSHA256)||
   (i>0&&rec.Attempts[i-1].Attempt+1!=a.Attempt){
   return providerAttemptSidecar{},ErrInvalidSession
  }
 }
 if rec.Attempts[len(rec.Attempts)-1].Attempt!=rec.Total{
  return providerAttemptSidecar{},ErrInvalidSession
 }
 return rec,nil
}

// RecordProviderAttempt fences the current canonical SENDING run and writes a
// synced, atomically replaced sidecar before ONE provider network attempt.
// Separate retry attempts get DISTINCT IDs; no successful network delivery
// may be inferred from this pre-send record. The ledger is bounded to the
// latest 128 attempts, with an explicit truncated flag for unlimited retry.
func (s *Store) RecordProviderAttempt(
 id,owner,runID string,attempt uint64,payload []byte,headerSent bool,
)(string,error){
 if !validID(id)||runID==""||attempt==0||attempt>1000000000||
  len(payload)==0||len(payload)>maxProviderRunPayload{
  return "",ErrInvalidSession
 }
 var clientID string
 err:=s.withLock(func()error{
  x,e:=s.readLocked(id);if e!=nil{return e}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  if x.ProviderRun==nil||x.ProviderRun.Phase!="sending"||
   x.ProviderRun.ID!=runID||x.Status!="requesting_ai"{
   return ErrInvalidSession
  }
  var record providerAttemptSidecar
  old,e:=s.readProviderAttemptsLocked(id)
  if e==nil&&old.RunID==runID{
   record=old
  }else if e!=nil&&!os.IsNotExist(e){
   // Damaged evidence is never silently overwritten. Recovery needs an
   // owner review, not an automatic request replay with lost attempt IDs.
   return e
  }
  if record.RunID==""{
   record=providerAttemptSidecar{Version:1,RunID:runID}
  }
  if record.Total+1!=attempt{return ErrStaleRevision}
  digest:=ProviderPayloadDigest(payload)
  clientID=providerAttemptClientID(id,runID,attempt,digest)
  record.Attempts=append(record.Attempts,ProviderAttempt{
   Attempt:attempt,ClientRequestID:clientID,PayloadSHA256:digest,
   HeaderSent:headerSent,RecordedAt:time.Now().UTC(),
  })
  if len(record.Attempts)>maxProviderAttemptRecords{
   record.Attempts=append([]ProviderAttempt(nil),
    record.Attempts[len(record.Attempts)-maxProviderAttemptRecords:]...)
  }
  record.Total=attempt
  record.SHA256,e=providerAttemptDigest(record.Attempts)
  if e!=nil{return e}
  data,e:=json.Marshal(record);if e!=nil{return e}
  if len(data)>maxProviderAttemptSidecarBytes{
   return errors.New("provider attempt audit exceeds private limit")
  }
  tmp,e:=os.CreateTemp(s.dir,".agent-provider-attempt-*")
  if e!=nil{return e}
  defer os.Remove(tmp.Name())
  if e=tmp.Chmod(0600);e!=nil{tmp.Close();return e}
  if _,e=tmp.Write(data);e!=nil{tmp.Close();return e}
  if e=tmp.Sync();e!=nil{tmp.Close();return e}
  if e=tmp.Close();e!=nil{return e}
  return os.Rename(tmp.Name(),s.providerAttemptsPath(id))
 })
 if err!=nil{return "",fmt.Errorf("cannot durably record provider attempt: %w",err)}
 return clientID,nil
}

// ProviderAttempts is read-only and owner/project scoped. A legacy run
// without an audit sidecar returns nil; a corrupt existing audit fails closed.
func (s *Store) ProviderAttempts(id,owner string)(*ProviderAttemptAudit,error){
 if !validID(id){return nil,ErrNotFound}
 var result *ProviderAttemptAudit
 err:=s.withLock(func()error{
  x,e:=s.readLocked(id);if e!=nil{return e}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  if x.ProviderRun==nil{return nil}
  record,e:=s.readProviderAttemptsLocked(id)
  if os.IsNotExist(e){return nil}
  if e!=nil{return e}
  if record.RunID!=x.ProviderRun.ID{return nil}
  result=&ProviderAttemptAudit{RunID:record.RunID,Total:record.Total,
   Truncated:record.Total>uint64(len(record.Attempts)),
   Attempts:append([]ProviderAttempt(nil),record.Attempts...),
  }
  return nil
 })
 return result,err
}
