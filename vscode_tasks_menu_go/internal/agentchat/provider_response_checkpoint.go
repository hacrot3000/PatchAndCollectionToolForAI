package agentchat

import (
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "reflect"
 "strings"
)

// A response checkpoint contains the projected final local session, NOT a
// request to the provider. The normal journal CAS is still the commit point.
// Keep the file private and run-bound; never use it to replay network I/O.
const maxProviderResponseCheckpointBytes = 16 << 20

type providerResponseCheckpoint struct {
 Version int `json:"version"`
 RunID string `json:"run_id"`
 BaseRevision uint64 `json:"base_revision"`
 PayloadSHA256 string `json:"payload_sha256"`
 CandidateSHA256 string `json:"candidate_sha256"`
 Candidate []byte `json:"candidate"`
}

func (s *Store) providerResponseCheckpointPath(id,runID string) string {
 return filepath.Join(s.dir,id+".response."+ProviderPayloadDigest([]byte(runID))+".json")
}

func sameResponseRun(a,b *ProviderRun) bool {
 return a!=nil && b!=nil && a.ID==b.ID && a.Kind==b.Kind &&
  a.CallID==b.CallID && a.PayloadSHA256==b.PayloadSHA256 &&
  a.ProviderFingerprint==b.ProviderFingerprint
}

func validResponseProjection(base,candidate Session) bool {
 return base.Status=="requesting_ai" && !base.Archived &&
  (base.ProviderRun!=nil && (base.ProviderRun.Phase=="sending" || base.ProviderRun.Phase=="acknowledged")) &&
  sameResponseRun(base.ProviderRun,candidate.ProviderRun) &&
  candidate.ProviderRun.Phase=="completed" && candidate.Status!="requesting_ai" &&
  candidate.ID==base.ID && candidate.Owner==base.Owner && candidate.Workspace==base.Workspace &&
  candidate.ProjectID==base.ProjectID && candidate.Revision==base.Revision &&
  candidate.ProviderID==base.ProviderID && candidate.ModelID==base.ModelID &&
  candidate.Mode==base.Mode && candidate.CreatedAt.Equal(base.CreatedAt)
}

// StageProviderResponse must be called by the turn owner, after receiving and
// projecting a response and before Save. Even a projection rejected by
// validation is retained for diagnosis; it never becomes executable merely
// because this sidecar exists.
func (s *Store) StageProviderResponse(candidate Session) error {
 if !validID(candidate.ID) || candidate.ProviderRun==nil { return ErrInvalidSession }
 return s.withLock(func() error {
  base,err:=s.readLocked(candidate.ID)
  if err!=nil{return err}
  if !s.matchesProject(base)||!validResponseProjection(base,candidate){return ErrInvalidSession}
  raw,err:=json.Marshal(candidate)
  if err!=nil{return err}
  record:=providerResponseCheckpoint{Version:1,RunID:base.ProviderRun.ID,
   BaseRevision:base.Revision,PayloadSHA256:base.ProviderRun.PayloadSHA256,
   CandidateSHA256:ProviderPayloadDigest(raw),Candidate:raw}
  encoded,err:=json.Marshal(record)
  if err!=nil{return err}
  if len(encoded)>maxProviderResponseCheckpointBytes{return ErrProviderResponseCheckpointTooLarge}
  path:=s.providerResponseCheckpointPath(base.ID,record.RunID)
  // Never replace a different candidate for this same provider run.
  if _,err=os.Lstat(path);err==nil{
   old,e:=s.readProviderResponseCheckpointLocked(base)
   if e!=nil{return e}
   if old.CandidateSHA256!=record.CandidateSHA256{return ErrInvalidSession}
   return s.syncDirectory()
  }else if !os.IsNotExist(err){return err}
  f,err:=os.CreateTemp(s.dir,".agent-response-*")
  if err!=nil{return err}
  defer os.Remove(f.Name())
  if err=f.Chmod(0600);err!=nil{f.Close();return err}
  if _,err=f.Write(encoded);err!=nil{f.Close();return err}
  if err=f.Sync();err!=nil{f.Close();return err}
  if err=f.Close();err!=nil{return err}
  if err=os.Rename(f.Name(),path);err!=nil{return err}
  return s.syncDirectory()
 })
}

var ErrProviderResponseCheckpointTooLarge = errors.New("provider response checkpoint exceeds size limit")
var ErrSessionSizeLimit = errors.New("agent session exceeds size limit")

func (s *Store) readProviderResponseCheckpointLocked(base Session)(providerResponseCheckpoint,error){
 return s.readResponseCheckpointLocked(base,false)
}

func (s *Store) readResponseCheckpointLocked(base Session,closed bool)(providerResponseCheckpoint,error){
 var record providerResponseCheckpoint
 if base.ProviderRun==nil{return record,ErrInvalidSession}
 path:=s.providerResponseCheckpointPath(base.ID,base.ProviderRun.ID)
 fi,err:=os.Lstat(path)
 if err!=nil{return record,err}
 if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||fi.Size()>maxProviderResponseCheckpointBytes{return record,ErrInvalidSession}
 raw,err:=os.ReadFile(path)
 if err!=nil{return record,err}
 if json.Unmarshal(raw,&record)!=nil||record.Version!=1||
  record.RunID!=base.ProviderRun.ID||(!closed&&record.BaseRevision!=base.Revision)||(closed&&(record.BaseRevision==0||record.BaseRevision>=base.Revision))||
  record.PayloadSHA256!=base.ProviderRun.PayloadSHA256||
  len(record.Candidate)==0||record.CandidateSHA256!=ProviderPayloadDigest(record.Candidate){
  return record,ErrInvalidSession
 }
 return record,nil
}

// RecoverProviderResponse commits only the exact saved candidate under the
// original run/revision fence. Every retry decodes fresh bytes: Save may
// compact archive slices, even when its final journal rename fails.
// Caller must hold the same conversation turn lease used for provider egress.
func (s *Store) RecoverProviderResponse(id,owner string)(Session,bool,error){
 if !validID(id){return Session{},false,ErrNotFound}
 var saved Session
 found:=false
 err:=s.withLock(func()error{
  base,e:=s.readLocked(id)
  if e!=nil{return e}
  if base.Owner!=owner||!s.matchesProject(base){return ErrNotFound}
  if base.ProviderRun==nil||base.Status!="requesting_ai"||base.ProviderRun.Phase=="prepared"{return nil}
  record,e:=s.readProviderResponseCheckpointLocked(base)
  if os.IsNotExist(e){return nil}
  found=true
  if e!=nil{return e}
  var candidate Session
  if json.Unmarshal(record.Candidate,&candidate)!=nil||!validResponseProjection(base,candidate){return ErrInvalidSession}
  repairReceivedQuestionnaireProjection(base,&candidate)
  saved,e=s.saveSessionLocked(candidate,base.Revision)
  if e!=nil{return e}
  // Derived cleanup cannot invalidate the already committed journal.
  _=os.Remove(s.providerResponseCheckpointPath(id,record.RunID))
  _=s.syncDirectory()
  return nil
 })
 return saved,found,err
}

// A previous server version accidentally copied Git drift fields into a
// newly returned ask_user record. They are PATCH-only metadata, not part of
// the model's questionnaire. Repair only that newly added, still-pending
// record, and only if the unchanged question schema passes strict validation.
// Never sanitize arbitrary native execution, receipt or approval fields.
func repairReceivedQuestionnaireProjection(base Session,candidate *Session)bool{
 if candidate==nil||candidate.Status!="waiting_tool"||len(candidate.History)<=len(base.History){return false}
 for i,it:=range base.History{if candidate.History[i]!=it{return false}}
 existing:=map[string]bool{}
 for _,c:=range base.ToolCalls{existing[c.CallID]=true}
 repaired:=false
 for i,c:=range candidate.ToolCalls{
  if existing[c.CallID]||c.Name!=AskUserToolName||c.Status!="pending"{continue}
  if c.ExpectedHead==""&&c.ExpectedBranch==""&&c.WorkspaceStatusSHA256==""&&!c.GitIsRepo{continue}
  fixed:=c
  fixed.ExpectedHead="";fixed.ExpectedBranch="";fixed.WorkspaceStatusSHA256="";fixed.GitIsRepo=false
  if !ValidAskUserRecord(fixed,candidate.History){continue}
  candidate.ToolCalls[i]=fixed
  repaired=true
 }
 return repaired
}

// Recover a checkpoint already parked by the old recovery worker. The current
// run must still be the same failed message and its canonical history and
// prior tool records must be untouched. Do not revive a superseded response
// or a failed native tool; only the specific known questionnaire bug can be
// repaired here. The original private checkpoint remains until durable commit.
func (s *Store) RecoverClosedQuestionnaireResponse(id,owner string)(Session,bool,error){
 if !validID(id){return Session{},false,ErrNotFound}
 var saved Session
 found:=false
 err:=s.withLock(func()error{
  base,e:=s.readLocked(id);if e!=nil{return e}
  if base.Owner!=owner||!s.matchesProject(base){return ErrNotFound}
  if base.Archived||base.Status!="failed"||base.PendingUserTurn!=nil||
   base.ProviderRun==nil||base.ProviderRun.Kind!="message"||base.ProviderRun.Phase!="unknown"{return nil}
  interrupted:=false
  for i:=len(base.Events)-1;i>=0;i--{
   ev:=base.Events[i]
   if ev.Kind=="turn_interrupted"{
    interrupted=ev.Detail=="provider_response_save_failed";break
   }
   if ev.Kind=="provider_egress"||ev.Kind=="turn_started"{break}
  }
  if !interrupted{return nil}
  record,e:=s.readResponseCheckpointLocked(base,true)
  if os.IsNotExist(e){return nil}
  if e!=nil{return e}
  var candidate Session
  if json.Unmarshal(record.Candidate,&candidate)!=nil{return ErrInvalidSession}
  before:=base
  run:=*base.ProviderRun;run.Phase="sending";before.ProviderRun=&run
  before.Status="requesting_ai";before.Revision=record.BaseRevision
  if !validResponseProjection(before,candidate)||len(candidate.ToolCalls)!=len(base.ToolCalls)+1||
   candidate.HistoryArchiveParts!=base.HistoryArchiveParts||candidate.ArchivedHistoryItems!=base.ArchivedHistoryItems||
   candidate.PatchArchiveParts!=base.PatchArchiveParts||candidate.ArchivedPatchCalls!=base.ArchivedPatchCalls{return nil}
  for i,c:=range base.ToolCalls{if !reflect.DeepEqual(c,candidate.ToolCalls[i]){return nil}}
  if !repairReceivedQuestionnaireProjection(base,&candidate){return nil}
  question:=candidate.ToolCalls[len(base.ToolCalls)]
  if question.Name!=AskUserToolName||question.Status!="pending"||!ValidAskUserRecord(question,candidate.History){return nil}
  candidate.Revision=base.Revision
  candidate.Events=append([]SessionEvent(nil),base.Events...)
  candidate.EventSeq=base.EventSeq
  candidate.CollectionGuard=base.CollectionGuard
  candidate.AppendEvent("user_questions_pending","waiting_tool",question.CallID,"")
  candidate.AppendEvent("response_recovered","waiting_tool",question.CallID,"questionnaire_snapshot_repaired")
  found=true
  saved,e=s.saveSessionLocked(candidate,base.Revision)
  if e!=nil{return e}
  _=os.Remove(s.providerResponseCheckpointPath(id,record.RunID))
  _=s.syncDirectory()
  return nil
 })
 return saved,found,err
}

// Remove only an already-committed run's checkpoint after an ordinary Save.
// The current journal must itself prove that this run completed.
func (s *Store) ClearCommittedProviderResponse(id,owner,runID string)error{
 if !validID(id){return ErrNotFound}
 return s.withLock(func()error{
  base,e:=s.readLocked(id)
  if e!=nil{return e}
  if base.Owner!=owner||!s.matchesProject(base){return ErrNotFound}
  if base.ProviderRun==nil||base.ProviderRun.ID!=runID||base.ProviderRun.Phase!="completed"{return ErrInvalidSession}
  // Confirm the canonical journal directory entry before removing the last
 // recovery copy (especially after an ambiguous prior directory fsync).
 if e=s.syncDirectory();e!=nil{return e}
 e=os.Remove(s.providerResponseCheckpointPath(id,runID))
  if os.IsNotExist(e){return nil}
  if e!=nil{return e}
  return s.syncDirectory()
 })
}

// Safe diagnostic categories only: never copy provider output or local
// filesystem paths into the externally visible debug stream.
func ProviderResponseSaveErrorCode(err error)string{
 var field sessionFieldError
 if errors.As(err,&field){return "invalid_session_projection:"+field.field}
 switch {
 case err==nil:return "none"
 case errors.Is(err,ErrStaleRevision):return "revision_conflict"
 case errors.Is(err,ErrJournalDurabilityUnknown):return "journal_durability_unknown"
 case errors.Is(err,ErrSessionSizeLimit):return "session_size_limit"
 case errors.Is(err,ErrProviderResponseCheckpointTooLarge):return "response_checkpoint_size_limit"
 case errors.Is(err,ErrInvalidSession):return "invalid_session_projection"
 case errors.Is(err,os.ErrPermission):return "filesystem_permission"
 case strings.Contains(err.Error(),"history item exceeds limit"):return "history_item_size_limit"
 case strings.Contains(err.Error(),"archive batch is unexpectedly oversized"):return "archive_batch_size_limit"
 default:return "local_storage_or_validation_error"
 }
}
