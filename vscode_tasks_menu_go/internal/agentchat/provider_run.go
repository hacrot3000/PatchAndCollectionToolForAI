package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "time"
)

// ProviderRun is a durable egress marker, not a claim that a provider received
// anything. Only a PREPARED run has provably not crossed the network boundary
// and is eligible for automatic recovery after restart. SENDING/UNKNOWN must
// NEVER be automatically replayed, including on providers with retry enabled.
type ProviderRun struct {
 ID string `json:"id"`
 Kind string `json:"kind"` // message, collection, feedback
 Phase string `json:"phase"` // prepared, sending, acknowledged, unknown, completed, blocked
 CallID string `json:"call_id,omitempty"`
 PayloadSHA256 string `json:"payload_sha256"`
 ProviderFingerprint string `json:"provider_fingerprint"`
 CreatedAt time.Time `json:"created_at"`
 UpdatedAt time.Time `json:"updated_at"`
}

type providerPayloadRecord struct{
 Version int `json:"version"`
 RunID string `json:"run_id"`
 SHA256 string `json:"sha256"`
 Payload []byte `json:"payload"`
}
const maxProviderRunPayload = 2 << 20

func ProviderPayloadDigest(payload []byte)string{
 digest:=sha256.Sum256(payload)
 return hex.EncodeToString(digest[:])
}
func validProviderRun(run *ProviderRun) bool {
 if run==nil{return true}
 if len(run.ID)<1 || len(run.ID)>96 || len(run.CallID)>256 ||
  len(run.PayloadSHA256)!=64 || len(run.ProviderFingerprint)>128 ||
  run.CreatedAt.IsZero() || run.UpdatedAt.IsZero(){return false}
 switch run.Kind{case "message","collection","feedback":default:return false}
 switch run.Phase{case "prepared","sending","acknowledged","unknown","completed","blocked":default:return false}
 return true
}

func (s *Store) providerRunFile(id string) string {
 return filepath.Join(s.dir,id+".provider-run.json")
}

// StageProviderPayload is called only for a previously authorized request
// while holding the caller's session turn lock. The private sidecar is written
// BEFORE the session declares PREPARED; a crash in between leaves only an
// unreferenced file, never an incorrectly replayable request.
func (s *Store) StageProviderPayload(x Session,payload []byte)error{
 if !validID(x.ID)||!validProviderRun(x.ProviderRun)||x.ProviderRun==nil||
  x.ProviderRun.Phase!="prepared"||len(payload)==0||len(payload)>maxProviderRunPayload||
  x.ProviderRun.PayloadSHA256!=ProviderPayloadDigest(payload){
  return ErrInvalidSession
 }
 return s.withLock(func()error{
  previous,err:=s.readLocked(x.ID)
  if err!=nil{return err}
  if previous.Revision!=x.Revision{return ErrStaleRevision}
  if previous.Owner!=x.Owner||previous.Workspace!=x.Workspace||
   previous.ProjectID!=x.ProjectID||!s.matchesProject(previous){return ErrInvalidSession}
  record:=providerPayloadRecord{Version:1,RunID:x.ProviderRun.ID,
   SHA256:x.ProviderRun.PayloadSHA256,Payload:payload}
  encoded,err:=json.Marshal(record)
  if err!=nil{return err}
  if len(encoded)>(maxProviderRunPayload*2){return ErrInvalidSession}
  f,err:=os.CreateTemp(s.dir,".agent-egress-prepared-*")
  if err!=nil{return err}
  defer os.Remove(f.Name())
  if err=f.Chmod(0600);err!=nil{f.Close();return err}
  if _,err=f.Write(encoded);err!=nil{f.Close();return err}
  if err=f.Sync();err!=nil{f.Close();return err}
  if err=f.Close();err!=nil{return err}
  return os.Rename(f.Name(),s.providerRunFile(x.ID))
 })
}

// PreparedProviderPayload is a strictly read-only restart recovery lookup.
// It fails closed if a sidecar was swapped, truncated, corrupted, or the
// session has moved past the prepared phase. Only exact bytes can be replayed.
func (s *Store) PreparedProviderPayload(id,owner string)(Session,[]byte,error){
 if !validID(id){return Session{},nil,ErrNotFound}
 var x Session
 var payload []byte
 err:=s.withLock(func()error{
  current,e:=s.readLocked(id);if e!=nil{return e}
  if current.Owner!=owner||!s.matchesProject(current){return ErrNotFound}
  if current.ProviderRun==nil||current.ProviderRun.Phase!="prepared"||
   current.Status!="requesting_ai"||!validProviderRun(current.ProviderRun) {
   return ErrInvalidSession
  }
  fi,e:=os.Lstat(s.providerRunFile(id));if e!=nil{return e}
  if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||
   fi.Size()>2*maxProviderRunPayload{return ErrInvalidSession}
  raw,e:=os.ReadFile(s.providerRunFile(id));if e!=nil{return e}
  var record providerPayloadRecord
  if json.Unmarshal(raw,&record)!=nil||record.Version!=1||
   record.RunID!=current.ProviderRun.ID||
   record.SHA256!=current.ProviderRun.PayloadSHA256||
   len(record.Payload)==0||len(record.Payload)>maxProviderRunPayload||
   ProviderPayloadDigest(record.Payload)!=record.SHA256{
   return fmt.Errorf("invalid or unverified prepared provider request")
  }
  payload=record.Payload;x=current;return nil
 })
 if err!=nil{return Session{},nil,err}
 return x,payload,nil
}

// DeleteProviderPayload is a best-effort cleanup. A provider payload is not an
// execution receipt and is never needed for a sending/unknown/completed run.
func (s *Store) DeleteProviderPayload(id string) error{
 if !validID(id){return errors.New("invalid session")}
 return s.withLock(func()error{
  if err:=os.Remove(s.providerRunFile(id));err!=nil&&!os.IsNotExist(err){return err}
  return nil
 })
}
