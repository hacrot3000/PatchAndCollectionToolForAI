package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "strings"
 "time"
)

// ErrTaskPlanSegmentWriteUnsupported asks the HTTP layer to use the legacy
// full-plan mutation for inline sessions or unsupported operations. It never
// signals a successful/partially committed write.
var ErrTaskPlanSegmentWriteUnsupported=errors.New("task plan segment status edit not applicable")
var ErrTaskPlanStepNotFound=errors.New("task plan milestone not found")
var ErrTaskPlanNotEditable=errors.New("task plan must be reopened before editing")
var ErrTaskPlanCompletionProofRejected=errors.New("task plan completion requires verified evidence and owner-reviewed criteria")

// scanTaskPlanSegmentsLocked validates ALL steps without retaining a giant
// plan in memory. The per-page canonical validator enforces ALL field-,
// evidence- and owner-test requirements, and this scan enforces cross-page
// unique IDs and suggestion references. It must run under Store.withLock.
func (s *Store) scanTaskPlanSegmentsLocked(x Session,visit func(int,TaskPlanStep)error)(done,total int,err error){
 if x.TaskPlan==nil||len(x.TaskPlan.Steps)!=0||
  len(x.TaskPlanSegments)==0||len(x.TaskPlan.Suggestions)>MaxTaskPlanSuggestions||
  len(x.TaskPlanSegments)>(MaxTaskPlanSteps+taskPlanSegmentSize-1)/taskPlanSegmentSize{
  return 0,0,ErrInvalidSession
 }
 background:=x
 background.TaskPlan=nil
 background.TaskPlanSegments=nil
 if err:=validateSession(background);err!=nil{return 0,0,err}
 ids:=make(map[string]bool,MaxTaskPlanSteps)
 for i,ref:=range x.TaskPlanSegments{
  if ref.Index!=i+1||!validTaskPlanSegmentRef(ref,x.Revision)||
   (i<len(x.TaskPlanSegments)-1&&ref.Count!=taskPlanSegmentSize){
   return 0,0,ErrInvalidSession
  }
  part,err:=s.readVerifiedTaskPlanSegment(x.ID,ref)
  if err!=nil{return 0,0,err}
  // Validate the same conditions as the full in-memory journal reader.
  // Suggestions are cross-page references, validated after all pages.
  pageMeta:=*x.TaskPlan
  pageMeta.Suggestions=nil
  pageMeta.Steps=part
  if err=ValidateTaskPlan(&pageMeta);err!=nil{return 0,0,err}
  for _,step:=range part{
   if ids[step.ID]{return 0,0,ErrInvalidSession}
   ids[step.ID]=true
   if step.Status=="done"{done++}
   if visit!=nil {
    if err:=visit(total,step);err!=nil{return 0,0,err}
   }
   total++
  }
 }
 if total<=MaxInlineTaskPlanSteps||total>MaxTaskPlanSteps{return 0,0,ErrInvalidSession}
 for _,candidate:=range x.TaskPlan.Suggestions{
  if !ids[candidate.StepID]||!taskPlanStepIDPattern.MatchString(candidate.StepID)||
   len(candidate.TurnID)==0||len(candidate.TurnID)>256||
   len(candidate.SourceDigest)!=64||candidate.At.IsZero()||
   ValidateTaskAutoCheck(&TaskAutoCheck{
    Kind:candidate.Kind,CallID:candidate.CallID,Path:candidate.Path,
   })!=nil{return 0,0,ErrInvalidSession}
 }
 if x.TaskPlan.State=="completed"&&(done!=total||x.TaskPlan.AutoContinue){
  return 0,0,ErrInvalidSession
 }
 return done,total,nil
}

// writeTaskPlanStatusPartLocked writes a NEW immutable segment and never
// mutates an existing segment. It must precede the canonical journal rename.
func (s *Store) writeTaskPlanStatusPartLocked(id string,ref TaskPlanSegmentRef,part []TaskPlanStep)error{
 raw,err:=json.Marshal(part)
 if err!=nil{return err}
 if len(raw)==0||len(raw)>taskPlanSegmentBytesLimit||len(part)!=ref.Count{
  return ErrInvalidSession
 }
 sum:=sha256.Sum256(raw)
 if hex.EncodeToString(sum[:])!=ref.SHA256{return ErrInvalidSession}
 tmp,err:=os.CreateTemp(s.dir,".agent-plan-segment-*")
 if err!=nil{return err}
 defer os.Remove(tmp.Name())
 if err=tmp.Chmod(0600);err!=nil{tmp.Close();return err}
 if _,err=tmp.Write(raw);err!=nil{tmp.Close();return err}
 if err=tmp.Sync();err!=nil{tmp.Close();return err}
 if err=tmp.Close();err!=nil{return err}
 return os.Rename(tmp.Name(),s.taskPlanSegmentPath(id,ref))
}

// SetSegmentTaskPlanStepStatus only accepts non-completing transitions.
// The companion SetSegmentTaskPlanStepDone requires in-Store verification
// of the exact user-attested or canonical in-journal completion evidence.
//
// After owner+revision checks it validates every canonical step/page, edits
// only the one matching 32-step page, commits a new immutable page and then
// atomically renames the authoritative session JSON manifest. No existing
// page is changed until the journal points at its verified replacement.
//
// Caller-provided response offset/limit are also fenced before mutation.
// Unpaged and <=128-step sessions retain the previous full-plan Save path.
func (s *Store) SetSegmentTaskPlanStepStatus(
 id,owner string,expectedRevision uint64,stepID,status string,
 offset,limit int,
)(TaskPlanPageResult,error){
 if status!="pending"&&status!="active"&&status!="blocked"{
  return TaskPlanPageResult{},ErrTaskPlanSegmentWriteUnsupported
 }
 return s.setSegmentTaskPlanStep(id,owner,expectedRevision,stepID,status,"","","",offset,limit,nil)
}

// SetSegmentTaskPlanStepDone commits a proof-verified milestone by rewriting
// only its immutable 32-step page. A manual note is an explicit OWNER
// attestation, not a machine test; active and integrity-checked archived
// Patch/Collection evidence use the SAME canonical verifier as legacy code.
// Native Task/Go-test evidence uses the companion server-verifier CAS path.
// No caller can pass a pre-manufactured TaskPlanEvidence to this method.
func (s *Store) SetSegmentTaskPlanStepDone(
 id,owner string,expectedRevision uint64,stepID,kind,ref,note string,
 offset,limit int,
)(TaskPlanPageResult,error){
 if kind!="manual"&&kind!="patch"&&kind!="collection"{
  return TaskPlanPageResult{},ErrTaskPlanSegmentWriteUnsupported
 }
 return s.setSegmentTaskPlanStep(id,owner,expectedRevision,stepID,"done",kind,ref,note,offset,limit,nil)
}

// SetSegmentTaskPlanStepDoneNative runs the server's authoritative native
// Task / structured Go test verifier against the owner-scoped milestone
// snapshot WHILE the journal CAS lock is held. The snapshot contains only
// the target milestone, but retains its original plan creation timestamp,
// owner/project and previously bound command and required Go tests. This
// prevents an out-of-date native PASS from being attached after a concurrent
// change to the milestone's acceptance or command guard.
//
// The verifier is host-owned (never provider-supplied). Only the server
// obtains it from native SessionService/host-private authenticated receipts.
// It must be side-effect-free with respect to this Store: do not re-enter
// Store.withLock or start any Task / provider request inside the callback.
func (s *Store) SetSegmentTaskPlanStepDoneNative(
 id,owner string,expectedRevision uint64,stepID,kind,ref string,
 offset,limit int,verify func(Session)(TaskPlanEvidence,error),
)(TaskPlanPageResult,error){
 if (kind!="task_run"&&kind!="go_test_case"&&kind!="go_test_suite")||verify==nil{
  return TaskPlanPageResult{},ErrTaskPlanSegmentWriteUnsupported
 }
 return s.setSegmentTaskPlanStep(id,owner,expectedRevision,stepID,"done",kind,ref,"",
  offset,limit,nil,verify)
}

func (s *Store) setSegmentTaskPlanStep(
 id,owner string,expectedRevision uint64,stepID,status,kind,ref,note string,
 offset,limit int,edit *segmentAcceptanceEdit,
 nativeVerify ...func(Session)(TaskPlanEvidence,error),
)(TaskPlanPageResult,error){
 var result TaskPlanPageResult
 if !validID(id){return result,ErrNotFound}
 if offset<0||limit<1||limit>32{return result,ErrTaskPlanPageOutOfRange}
 err:=s.withLock(func()error{
  x,e:=s.readSessionManifestLocked(id)
  if e!=nil{return e}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  if x.Revision!=expectedRevision{return ErrStaleRevision}
  if len(x.TaskPlanSegments)==0{return ErrTaskPlanSegmentWriteUnsupported}
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{return ErrTaskPlanNotEditable}
  if x.TaskPlan.State!="active"&&x.TaskPlan.State!="paused"{return ErrInvalidSession}

  index:=-1
  var old TaskPlanStep
  selected:=make([]TaskPlanStep,0,limit)
  // Stream validation of ALL canonical parts, including off-screen
  // evidence fields, before changing even one segment pointer.
  done,total,e:=s.scanTaskPlanSegmentsLocked(x,func(i int,step TaskPlanStep)error{
   if step.ID==stepID{
    index=i
    old=step
   }
   if i>=offset&&i-offset<limit{selected=append(selected,step)}
   return nil
  })
  if e!=nil{return e}
  if offset>total{return ErrTaskPlanPageOutOfRange}
  if index<0{return ErrTaskPlanStepNotFound}
  partIndex:=index/taskPlanSegmentSize
  oldRef:=x.TaskPlanSegments[partIndex]
  part,e:=s.readVerifiedTaskPlanSegment(id,oldRef)
  if e!=nil{return e}

  now:=time.Now().UTC()
  target:=&part[index%taskPlanSegmentSize]
  // Owner acceptance edits retain the current milestone status and
  // evidence. They cannot manufacture completion or reuse old reviews.
  // Ordinary status/done actions retain the original verified CAS policy.
  if edit!=nil{
   if e=edit.apply(target,now);e!=nil{return e}
  }else if status=="done"{
   // An owner-pinned exact Go checklist can ONLY be completed with the
   // native suite verifier, never a weaker Task exit or a single test.
   if old.GoTestRequirements!=nil&&
    !(kind=="go_test_suite"&&len(nativeVerify)==1){
    return ErrTaskPlanSegmentWriteUnsupported
   }
   if !TaskPlanStepCriteriaReviewed(old)||len(old.Evidence)>=MaxTaskPlanEvidence{
    return ErrTaskPlanCompletionProofRejected
   }
   var proof TaskPlanEvidence
   var verifyErr error
   if len(nativeVerify)==1{
    // The Store validated ALL pages; hand the native verifier just the
    // target step, not an untrusted, caller-assembled Task plan. The native
    // verifier still checks the exact Task ID, owner, workspace, command
    // digest, post-bind start time and full retained Go runner output.
    snapshot:=x
    plan:=*x.TaskPlan
    plan.Steps=[]TaskPlanStep{old}
    snapshot.TaskPlan=&plan
    proof,verifyErr=nativeVerify[0](snapshot)
    if verifyErr==nil{
     if proof.Kind!=kind||proof.Ref!=ref||len(proof.Digest)!=64||
      len(proof.Note)>512||strings.ToLower(proof.Digest)!=proof.Digest{
      verifyErr=ErrTaskPlanCompletionProofRejected
     }else if _,e:=hex.DecodeString(proof.Digest);e!=nil{
      verifyErr=ErrTaskPlanCompletionProofRejected
     }
    }
   }else if len(nativeVerify)==0{
    // Archive and in-journal receipts use the matching authenticated
    // verifier without re-entering Store.withLock.
    proof,verifyErr=s.verifyArchivedTaskPlanEvidenceLocked(x,kind,ref,note)
   }else{
    return ErrTaskPlanSegmentWriteUnsupported
   }
   if verifyErr!=nil{return ErrTaskPlanCompletionProofRejected}
   target.Evidence=append(target.Evidence,proof)
  }
  if edit==nil{
   target.Status=status
   target.UpdatedAt=now
   if old.Status=="done"&&status!="done"{done--}
   if old.Status!="done"&&status=="done"{done++}
  }
  // Validate the mutated page (including evidence and acceptance) using
  // the same canonical plan validator before staging any new immutable file.
  pageMeta:=*x.TaskPlan
  pageMeta.Suggestions=nil
  pageMeta.Steps=part
  if e=ValidateTaskPlan(&pageMeta);e!=nil{return e}
  if index>=offset&&index-offset<len(selected){
   selected[index-offset]=*target
  }
  x.TaskPlan.UpdatedAt=now
  detail:="set_step"
  if edit!=nil{detail=edit.action}
  x.AppendEvent("plan_updated",x.Status,"",detail)
  x.Revision=expectedRevision+1
  x.UpdatedAt=now

  // Prepare immutable changed-page bytes and verify their digest.
  raw,e:=json.Marshal(part)
  if e!=nil{return e}
  if len(raw)>taskPlanSegmentBytesLimit{return ErrInvalidSession}
  sum:=sha256.Sum256(raw)
  changed:=TaskPlanSegmentRef{
   Index:oldRef.Index,Revision:x.Revision,
   Count:len(part),SHA256:hex.EncodeToString(sum[:]),
  }
  x.TaskPlanSegments=append([]TaskPlanSegmentRef(nil),x.TaskPlanSegments...)
  x.TaskPlanSegments[partIndex]=changed

  // Do not validate a deliberately unloaded TaskPlan as though it were
  // complete: scanTaskPlanSegmentsLocked already validated every step and
  // the enclosing session fields, and only status/timestamps changed.
  background:=x
  background.TaskPlan=nil
  background.TaskPlanSegments=nil
  if e=validateSession(background);e!=nil{return e}
  if e=s.writeTaskPlanStatusPartLocked(id,changed,part);e!=nil{return e}
  // Flush the immutable page's directory entry BEFORE the canonical
  // session may reference it; failed pre-commit sync leaves only an orphan.
  if e=s.syncDirectory();e!=nil{return e}

  onDisk:=x
  meta:=*x.TaskPlan
  meta.Steps=nil
  onDisk.TaskPlan=&meta
  payload,e:=json.Marshal(onDisk)
  if e!=nil{return e}
  if len(payload)>maxSessionBytes{return errors.New("agent session exceeds size limit")}
  temp,e:=os.CreateTemp(s.dir,".agent-session-*")
  if e!=nil{return e}
  defer os.Remove(temp.Name())
  if e=temp.Chmod(0600);e!=nil{temp.Close();return e}
  if _,e=temp.Write(payload);e!=nil{temp.Close();return e}
  if e=temp.Sync();e!=nil{temp.Close();return e}
  if e=temp.Close();e!=nil{return e}
  if e=os.Rename(temp.Name(),s.file(id));e!=nil{return e}
  // Post-rename fsync failure makes the committed revision uncertain.
  // Never treat it as proof an owner action or provider egress did not occur.
  if e=s.syncDirectory();e!=nil{
   return fmt.Errorf("%w: %v",ErrJournalDurabilityUnknown,e)
  }

  // A test-only barrier exercises a real process kill after the canonical
  // manifest commit and before cleanup. The hook is never populated by
  // production stores, and cannot be configured by a request or provider.
  if s.afterSegmentStatusJournalCommit!=nil{s.afterSegmentStatusJournalCommit()}

  // The derived markdown handoff is regenerated authoritatively by
  // Store.Handoff() on demand; remove an obsolete materialized copy.
  _=os.Remove(s.handoffFile(id))
  // Cleanup only after the new referencing journal is committed.
  s.cleanupTaskPlanSegments([]TaskPlanSegmentRef{oldRef},x.TaskPlanSegments,id)

  end:=offset+limit
  if end>total{end=total}
  // The response is projected from the already-verified pre-commit
  // snapshot with the single changed step substituted. No fallible page
  // re-read may turn a successfully committed edit into an ambiguous error.
  if len(selected)!=end-offset{return ErrInvalidSession}
  responsePlan:=*x.TaskPlan
  responsePlan.Steps=selected
  result.SessionID=id
  result.Revision=x.Revision
  result.Plan=&responsePlan
  result.Done=done
  result.Total=total
  result.AllDone=done==total
  result.Completed=responsePlan.State=="completed"
  result.Page.Offset=offset
  result.Page.Limit=limit
  result.Page.TotalSteps=total
  result.Page.HasNext=end<total
  return nil
 })
 if err!=nil{return TaskPlanPageResult{},err}
 return result,nil
}

// This file intentionally does not allow arbitrary evidence injection.
// Go test/Task-run proof, command guard mutations, AutoCheck rules and
// final plan completion keep the legacy full-plan verified path.
