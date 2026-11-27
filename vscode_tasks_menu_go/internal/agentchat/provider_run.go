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
 RequestDisplay
 ID string `json:"id"`
 Kind string `json:"kind"` // message, collection, feedback
 Phase string `json:"phase"` // prepared, sending, acknowledged, unknown, completed, blocked
 CallID string `json:"call_id,omitempty"`
 CollectionReplan bool `json:"collection_replan,omitempty"`
 FeedbackAutoResume bool `json:"feedback_auto_resume,omitempty"` // auto PATCH feedback resume; consent rechecked at SENDING
 PayloadSHA256 string `json:"payload_sha256"`
 ProviderFingerprint string `json:"provider_fingerprint"`
 EstimatedInputTokens uint64 `json:"estimated_input_tokens,omitempty"`
 UsageRecorded bool `json:"usage_recorded,omitempty"`
 CreatedAt time.Time `json:"created_at"`
 UpdatedAt time.Time `json:"updated_at"`
}

type providerPayloadRecord struct{
 Version int `json:"version"`
 RunID string `json:"run_id"`
 SHA256 string `json:"sha256"`
 Payload []byte `json:"payload"`
}
// All canonical provider payloads accepted by the serializer must also fit
// PREPARED storage, restart validation and the pre-egress attempt ledger.
// The sidecar's 2x encoded limit includes base64 overhead.
const maxProviderRunPayload = maxRequestBytes

func ProviderPayloadDigest(payload []byte)string{
 digest:=sha256.Sum256(payload)
 return hex.EncodeToString(digest[:])
}
func validProviderRun(run *ProviderRun) bool {
 if run==nil{return true}
 if len(run.ID)<1 || len(run.ID)>96 || len(run.CallID)>256 ||
  len(run.PayloadSHA256)!=64 || len(run.ProviderFingerprint)>128 ||
  run.CreatedAt.IsZero() || run.UpdatedAt.IsZero(){return false}
 if run.CollectionReplan&&run.Kind!="collection"{return false}
 if run.FeedbackAutoResume&&run.Kind!="message"{return false}
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
  // A committed PREPARED journal still references the existing sidecar.
  if previous.ProviderRun!=nil && previous.ProviderRun.Phase=="prepared"{return ErrInvalidSession}
  return s.stageProviderPayloadLocked(x,payload)
 })
}

// stageProviderPayloadLocked requires the store lock and a validated run.
// Its directory sync makes the sidecar durable before journal admission.
func (s *Store) stageProviderPayloadLocked(x Session,payload []byte) error {
  // This helper is also used by lock-held admission paths: validate again so
  // callers cannot persist an invalid or differently bound provider payload.
  if !validID(x.ID)||x.ProviderRun==nil||!validProviderRun(x.ProviderRun)||
   x.ProviderRun.Phase!="prepared"||len(payload)==0||len(payload)>maxProviderRunPayload||
   x.ProviderRun.PayloadSHA256!=ProviderPayloadDigest(payload){return ErrInvalidSession}
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
  if err=os.Rename(f.Name(),s.providerRunFile(x.ID));err!=nil{return err}
  return s.syncDirectory()
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
  current,err:=s.readLocked(id)
  if err!=nil{return err}
  // Never delete the payload referenced by a committed PREPARED run.
  if current.ProviderRun!=nil && current.ProviderRun.Phase=="prepared"{return ErrInvalidSession}
  err=os.Remove(s.providerRunFile(id))
  if os.IsNotExist(err){return nil} // already removed; no metadata change
  if err!=nil{return err}
  // The canonical SENDING/UNKNOWN journal is authoritative regardless of
  // this sidecar's cleanup outcome. Ensure a successfully removed sensitive
  // PREPARED payload's filename is also durably unlinked on POSIX.
  if err=s.syncDirectory();err!=nil{
   return fmt.Errorf("%w: obsolete provider payload unlink: %v",ErrCleanupDurabilityUnknown,err)
  }
  return nil
 })
}


// ErrCollectionReplanRevoked means the exact PREPARED request was parked
// before crossing the durable SENDING boundary. It is not unknown egress.
var ErrCollectionReplanRevoked=errors.New("automatic Collection replan no longer authorized")

func CollectionReplanAllowed(x Session)bool{
 if x.Owner!="local"||x.Archived||(x.AutoCollection!=nil&&!*x.AutoCollection){return false}
 if x.TaskPlan!=nil{return x.TaskPlan.AutoContinue&&TaskPlanNext(x.TaskPlan)!=nil}
 return x.AutoContinueText
}

// CommitProviderSending serializes the replan consent check with owner
// metadata writes and the existing journal CAS. An opt-out that commits
// first wins; after SENDING, the normal ambiguous-egress fence applies.
// Return the durable blocked session together with the sentinel so the
// caller cannot accidentally persist a stale in-memory SENDING candidate.

// FeedbackAutoResumeAllowed re-checks live owner consent at the SENDING
// boundary for an automatic PATCH feedback resume. UpdateAutomation does
// not advance the turn revision, so admission-time consent alone is not
// sufficient; a revocation committed first must win before egress.
func FeedbackAutoResumeAllowed(x Session)bool{
 if x.Owner!="local"||x.Archived||x.Status=="cancelled"||x.Status=="failed"{return false}
 if x.TaskPlan!=nil{return false}
 return x.AutoContinueText&&x.AutoFeedback&&x.AutoFeedbackPersistent&&x.RetryMode==RetryUnlimited
}
func(s *Store) CommitProviderSending(candidate Session,expectedRevision uint64)(Session,error){
 var saved Session
 revoked:=false
 err:=s.withLock(func()error{
  current,e:=s.readLocked(candidate.ID);if e!=nil{return e}
  if current.Revision!=expectedRevision{return ErrStaleRevision}
  old,next:=current.ProviderRun,candidate.ProviderRun
  if current.Owner!=candidate.Owner||current.Workspace!=candidate.Workspace||current.ProjectID!=candidate.ProjectID||
   old==nil||next==nil||old.Phase!="prepared"||next.Phase!="sending"||
   old.ID!=next.ID||old.Kind!=next.Kind||old.CallID!=next.CallID||old.PayloadSHA256!=next.PayloadSHA256||
   old.ProviderFingerprint!=next.ProviderFingerprint||old.CollectionReplan!=next.CollectionReplan||old.FeedbackAutoResume!=next.FeedbackAutoResume{return ErrInvalidSession}
  if (old.CollectionReplan&&!CollectionReplanAllowed(current))||(old.FeedbackAutoResume&&!FeedbackAutoResumeAllowed(current)){
   current.Status="failed"
   current.ProviderRun.Phase="blocked"
   current.ProviderRun.UpdatedAt=time.Now().UTC()
   current.PreviousResponseID="";current.ConversationID=""
   current.AppendEvent("clp_checkpoint",current.Status,old.CallID,"recovery_not_authorized")
   saved,e=s.saveSessionLocked(current,expectedRevision)
   revoked=e==nil
   return e
  }
  saved,e=s.saveSessionLocked(candidate,expectedRevision)
  return e
 })
 if err!=nil{return Session{},err}
 if revoked{return saved,ErrCollectionReplanRevoked}
 return saved,nil
}
