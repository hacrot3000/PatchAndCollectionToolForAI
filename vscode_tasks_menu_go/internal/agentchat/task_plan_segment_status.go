package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "time"
)

// ErrTaskPlanSegmentWriteUnsupported asks the HTTP layer to use the legacy
// full-plan mutation for inline sessions or unsupported operations. It never
// signals a successful/partially committed write.
var ErrTaskPlanSegmentWriteUnsupported=errors.New("task plan segment status edit not applicable")
var ErrTaskPlanStepNotFound=errors.New("task plan milestone not found")
var ErrTaskPlanNotEditable=errors.New("task plan must be reopened before editing")

// scanTaskPlanSegmentsLocked validates ALL steps without retaining a giant
// plan in memory. The per-page canonical validator enforces ALL field-,
// evidence- and owner-test requirements, and this scan enforces cross-page
// unique IDs and suggestion references. It must run under Store.withLock.
func (s *Store) scanTaskPlanSegmentsLocked(x Session,visit func(int,TaskPlanStep)error)(done,total int,err error){
 if x.TaskPlan==nil||len(x.TaskPlan.Steps)!=0||
  len(x.TaskPlanSegments)==0||
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

// SetSegmentTaskPlanStepStatus is deliberately LIMITED to non-completing
// status transitions: pending, active, blocked. It never mints acceptance,
// changes a Go-test requirement, or bypasses verified Native tool receipts.
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
 var result TaskPlanPageResult
 if !validID(id){return result,ErrNotFound}
 if status!="pending"&&status!="active"&&status!="blocked"{
  return result,ErrTaskPlanSegmentWriteUnsupported
 }
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
  // Preserve prior evidence (including any accepted native receipt) when
  // marking a previously-done step pending/active/blocked, as legacy code.
  target.Status=status
  target.UpdatedAt=now
  if old.Status=="done"{done--}
  if index>=offset&&index-offset<len(selected){
   selected[index-offset]=*target
  }
  x.TaskPlan.UpdatedAt=now
  x.AppendEvent("plan_updated",x.Status,"","set_step")
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

// This file intentionally doesn't supply a generic segment-level evidence
// mutation API. "done" and any evidence/Go/AutoCheck change must continue
// through the existing full-plan, native-proof-verified endpoint.
