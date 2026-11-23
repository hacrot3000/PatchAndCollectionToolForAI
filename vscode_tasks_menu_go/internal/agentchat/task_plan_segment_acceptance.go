package agentchat

import (
 "errors"
 "encoding/hex"
 "strings"
 "time"
)

// An acceptance-matrix edit is always a deliberate owner action. It must
// never be synthesized from provider text or an unverified model suggestion.
// Only the HTTP owner API decides whether the explicit confirmation was given.
var ErrTaskPlanAcceptanceInvalid=errors.New("invalid task plan acceptance matrix or review")
var ErrTaskPlanCriterionNotFound=errors.New("task plan acceptance criterion not found")
var ErrTaskPlanAcceptanceLocked=errors.New("accepted milestone must be reopened before editing its criteria")
var ErrTaskPlanGoBindingInvalid=errors.New("invalid owner-selected native command or Go test checklist")
var ErrTaskPlanGoBindingConflict=errors.New("remove dependent owner Go tests or acceptance criteria first")
var ErrTaskPlanGoBindingMissing=errors.New("lock expected native test command before selecting required Go tests")

type segmentAcceptanceEdit struct {
 action string
 criteria []TaskAcceptanceCriterion
 criterionID string
 note string
 enabled bool
 taskID int
 commandSHA256 string
 goPackage string
 goTests []string
}

// apply mutates only the selected, already SHA-verified 32-step page while
// the Store's directory lock is held. The caller validates all offscreen
// canonical pages and revisions before calling this method.
func (edit *segmentAcceptanceEdit) apply(step *TaskPlanStep,now time.Time)error{
 if edit==nil||step==nil{return ErrInvalidSession}
 if step.Status=="done"{return ErrTaskPlanAcceptanceLocked}
 switch edit.action{
 case "set_acceptance_criteria":
  if len(edit.criteria)>MaxTaskAcceptanceCriteria{return ErrTaskPlanAcceptanceInvalid}
  // Never retain any review note or timestamp from the request. Replacing
  // the matrix resets ALL reviews even if an item has the same ID, as on
  // the original full-plan endpoint. This prevents old attestations from
  // being inherited by changed acceptance text or Go-test mappings.
  candidate:=make([]TaskAcceptanceCriterion,0,len(edit.criteria))
  for _,c:=range edit.criteria{
   candidate=append(candidate,TaskAcceptanceCriterion{
    ID:c.ID,
    Description:strings.TrimSpace(c.Description),
    RequiredTests:append([]string(nil),c.RequiredTests...),
   })
  }
  revised:=*step
  revised.AcceptanceCriteria=candidate
  if err:=ValidateTaskAcceptanceCriteria(revised);err!=nil{
   return ErrTaskPlanAcceptanceInvalid
  }
  step.AcceptanceCriteria=candidate
 case "review_acceptance_criterion":
  note:=strings.TrimSpace(edit.note)
  if len(note)==0||len(note)>512{return ErrTaskPlanAcceptanceInvalid}
  found:=false
  for i:=range step.AcceptanceCriteria{
   if step.AcceptanceCriteria[i].ID!=edit.criterionID{continue}
   step.AcceptanceCriteria[i].ReviewNote=note
   at:=now
   step.AcceptanceCriteria[i].ReviewedAt=&at
   found=true
   break
  }
  if !found{return ErrTaskPlanCriterionNotFound}
 case "set_task_command_guard":
  // A locked command identifies the exact Task ID, argv, cwd and digest
  // selected by the OWNER, never by a model-produced PASS message.
  if step.GoTestRequirements!=nil||len(step.AcceptanceCriteria)>0{
   return ErrTaskPlanGoBindingConflict
  }
  if !edit.enabled{
   step.TaskCommandGuard=nil
  }else{
   digest:=edit.commandSHA256
   if edit.taskID<1||len(digest)!=64||strings.ToLower(digest)!=digest{
    return ErrTaskPlanGoBindingInvalid
   }
   if _,err:=hex.DecodeString(digest);err!=nil{return ErrTaskPlanGoBindingInvalid}
   step.TaskCommandGuard=&TaskCommandGuard{
    TaskID:edit.taskID,CommandSHA256:digest,BoundAt:now,
   }
  }
 case "set_go_test_requirements":
  // Replacing an expected named-test checklist must never preserve semantic
  // coverage attestations for a different checklist or bypass a locked
  // command. The caller must first remove the acceptance matrix explicitly.
  if len(step.AcceptanceCriteria)>0{return ErrTaskPlanGoBindingConflict}
  if !edit.enabled{
   step.GoTestRequirements=nil
  }else{
   if step.TaskCommandGuard==nil{return ErrTaskPlanGoBindingMissing}
   candidate:=&TaskGoTestRequirements{
    Package:edit.goPackage,Tests:append([]string(nil),edit.goTests...),BoundAt:now,
   }
   if err:=ValidateTaskGoTestRequirements(candidate);err!=nil{
    return ErrTaskPlanGoBindingInvalid
   }
   step.GoTestRequirements=candidate
  }
 default:
  return ErrTaskPlanSegmentWriteUnsupported
 }
 step.UpdatedAt=now
 return nil
}

// SetSegmentTaskPlanAcceptanceCriteria replaces the owner-defined acceptance
// matrix for a single milestone. The replacement MUST reset all reviews and
// may only map tests selected in that milestone's existing owner-bound Go
// checklist. The HTTP handler checks confirm_coverage before calling here.
func (s *Store) SetSegmentTaskPlanAcceptanceCriteria(
 id,owner string,expectedRevision uint64,stepID string,
 criteria []TaskAcceptanceCriterion,offset,limit int,
)(TaskPlanPageResult,error){
 return s.setSegmentTaskPlanStep(id,owner,expectedRevision,stepID,"","","","",
  offset,limit,&segmentAcceptanceEdit{
   action:"set_acceptance_criteria",
   criteria:criteria,
  })
}

// ReviewSegmentTaskPlanAcceptanceCriterion records an explicit human review
// of one existing owner criterion. It is NOT evidence that Go tests ran or
// a substitute for verified Patch, Collection or native Task success.
func (s *Store) ReviewSegmentTaskPlanAcceptanceCriterion(
 id,owner string,expectedRevision uint64,stepID,criterionID,note string,
 offset,limit int,
)(TaskPlanPageResult,error){
 return s.setSegmentTaskPlanStep(id,owner,expectedRevision,stepID,"","","","",
  offset,limit,&segmentAcceptanceEdit{
   action:"review_acceptance_criterion",
   criterionID:criterionID,
   note:note,
  })
}

// SetSegmentTaskPlanCommandGuard stores or clears an exact owner-confirmed
// native Task command identity on one immutable Task plan page. Any pinned
// expected Go checklist or acceptance matrix must be explicitly removed
// first. The HTTP endpoint enforces confirm_command_guard.
func (s *Store) SetSegmentTaskPlanCommandGuard(
 id,owner string,expectedRevision uint64,stepID string,
 enabled bool,taskID int,commandSHA256 string,offset,limit int,
)(TaskPlanPageResult,error){
 return s.setSegmentTaskPlanStep(id,owner,expectedRevision,stepID,"","","","",
  offset,limit,&segmentAcceptanceEdit{
   action:"set_task_command_guard",enabled:enabled,
   taskID:taskID,commandSHA256:commandSHA256,
  })
}

// SetSegmentTaskPlanGoTestRequirements persists the OWNER's bounded, exact
// named-test checklist tied to the previously bound Task command. It does
// NOT record any test execution and cannot mark a milestone complete.
// The HTTP endpoint enforces confirm_go_tests.
func (s *Store) SetSegmentTaskPlanGoTestRequirements(
 id,owner string,expectedRevision uint64,stepID string,
 enabled bool,goPackage string,tests []string,offset,limit int,
)(TaskPlanPageResult,error){
 return s.setSegmentTaskPlanStep(id,owner,expectedRevision,stepID,"","","","",
  offset,limit,&segmentAcceptanceEdit{
   action:"set_go_test_requirements",enabled:enabled,
   goPackage:goPackage,goTests:append([]string(nil),tests...),
  })
}
