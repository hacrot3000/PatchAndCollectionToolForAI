package server

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strconv"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

const maxAgentTaskPlanPageSize = 32
const defaultAgentTaskPlanPageSize = 24

// The task plan is user-owned state. Provider text is never interpreted as an
// instruction to mark a milestone complete or change AutoContinue permission.
// Edits are owner/project scoped and revision-fenced against concurrent turns.
func (s *Server) agentChatTaskPlanAPI(w http.ResponseWriter,r *http.Request,id string){
 if r.Method!=http.MethodGet && r.Method!=http.MethodPost{
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
 }
 // Optional POST-only pagination is parsed before mutation. Unpaged
 // clients still receive the original complete canonical plan.
 postOffset,postLimit,postPaged:=0,defaultAgentTaskPlanPageSize,false
 if r.Method==http.MethodPost {
  q:=r.URL.Query()
  _,hasOffset:=q["offset"]
  _,hasLimit:=q["limit"]
  postPaged=hasOffset||hasLimit
  if postPaged {
   if len(q["offset"])>1||len(q["limit"])>1 {
    http.Error(w,"duplicate pagination parameter",400);return
   }
   if hasOffset {
    n,e:=strconv.Atoi(q.Get("offset"))
    if e!=nil||n<0{http.Error(w,"invalid task plan page offset",400);return}
    postOffset=n
   }
   if hasLimit {
    n,e:=strconv.Atoi(q.Get("limit"))
    if e!=nil||n<1||n>maxAgentTaskPlanPageSize{
     http.Error(w,"invalid task plan page limit",400);return
    }
    postLimit=n
   }
  }
 }
 owner:=aiPlanOwner(r)
 var req struct {
  Action string `json:"action"`
  ExpectedRevision uint64 `json:"expected_revision"`
  Goal string `json:"goal,omitempty"`
  Steps []struct {
   ID string `json:"id"`
   Title string `json:"title"`
   Acceptance string `json:"acceptance"`
  } `json:"steps,omitempty"`
  StepID string `json:"step_id,omitempty"`
  Title string `json:"title,omitempty"`
  Acceptance string `json:"acceptance,omitempty"`
  Status string `json:"status,omitempty"`
  EvidenceKind string `json:"evidence_kind,omitempty"`
  EvidenceRef string `json:"evidence_ref,omitempty"`
  EvidenceNote string `json:"evidence_note,omitempty"`
  EvidenceTaskID int `json:"evidence_task_id,omitempty"`
  EvidenceGoPackage string `json:"evidence_go_package,omitempty"`
  EvidenceGoTest string `json:"evidence_go_test,omitempty"`
  RequiredGoTests []string `json:"required_go_tests,omitempty"`
  ConfirmGoTests bool `json:"confirm_go_tests,omitempty"`
  AcceptanceCriteria []struct {
   ID string `json:"id"`
   Description string `json:"description"`
   RequiredTests []string `json:"required_tests,omitempty"`
  } `json:"acceptance_criteria,omitempty"`
  CriterionID string `json:"criterion_id,omitempty"`
  CoverageReviewNote string `json:"coverage_review_note,omitempty"`
  ConfirmCoverage bool `json:"confirm_coverage,omitempty"`
  ExpectedCommandSHA256 string `json:"expected_command_sha256,omitempty"`
  ConfirmCommandGuard bool `json:"confirm_command_guard,omitempty"`
  Enabled bool `json:"enabled,omitempty"`
  ConfirmExternal bool `json:"confirm_external,omitempty"`
  ConfirmAutoAccept bool `json:"confirm_auto_accept,omitempty"`
  AutoKind string `json:"auto_kind,omitempty"`
  AutoCallID string `json:"auto_call_id,omitempty"`
  AutoPath string `json:"auto_path,omitempty"`
 }
 if r.Method==http.MethodPost{
  r.Body=http.MaxBytesReader(w,r.Body,64<<10)
  dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
  if dec.Decode(&req)!=nil{http.Error(w,"invalid task plan request",400);return}
  var extra any
  if err:=dec.Decode(&extra);!errors.Is(err,io.EOF){
   http.Error(w,"one plan operation per request",400);return
  }
 }
 release:=func(){}
 if r.Method==http.MethodPost{
  unlock,ok:=lockAgentTurn(s.Workspace,owner,id)
  if !ok{http.Error(w,"Agent turn busy; retry plan edit after the current request",409);return}
  release=unlock
 }
 defer release()
 store,err:=s.agentChatStore()
 if err!=nil{http.Error(w,"session journal unavailable",500);return}
 // Paged GET uses Store.TaskPlanPage's bounded memory representation. It
 // validates every referenced segment SHA before reporting global progress
 // and avoids materializing the complete 1,024-step Task plan.
 if r.Method==http.MethodGet{
  q:=r.URL.Query()
  _,hasLimit:=q["limit"]
  _,hasOffset:=q["offset"]
  if hasLimit||hasOffset{
   if len(q["limit"])>1||len(q["offset"])>1{
    http.Error(w,"duplicate pagination parameter",400);return
   }
   limit,offset:=defaultAgentTaskPlanPageSize,0
   if hasLimit{
    value,e:=strconv.Atoi(q.Get("limit"))
    if e!=nil||value<1||value>maxAgentTaskPlanPageSize{
     http.Error(w,"invalid task plan page limit",400);return
    }
    limit=value
   }
   if hasOffset{
    value,e:=strconv.Atoi(q.Get("offset"))
    if e!=nil||value<0{
     http.Error(w,"invalid task plan page offset",400);return
    }
    offset=value
   }
   page,e:=store.TaskPlanPage(id,owner,offset,limit)
   if errors.Is(e,agentchat.ErrNotFound){http.Error(w,"session not found",404);return}
   if errors.Is(e,agentchat.ErrTaskPlanPageOutOfRange){
    http.Error(w,"task plan page offset exceeds plan length",400);return
   }
   if e!=nil{http.Error(w,"session journal unavailable",500);return}
   w.Header().Set("Cache-Control","no-store")
   writeJSON(w,200,page)
   return
  }
 }
 // Only the explicitly confirmed, owner-owned acceptance edits and
 // verified status/done transitions use the bounded 32-step CAS writer.
 // Native Task/Go proof and all unpaged/inline operations retain the full
 // validator path below. The Store itself validates ALL offscreen pages.
 eligiblePagedStep:=req.Action=="set_step"&&
  (req.Status=="pending"||req.Status=="active"||req.Status=="blocked"||
   (req.Status=="done"&&(req.EvidenceKind=="manual"||
    req.EvidenceKind=="patch"||req.EvidenceKind=="collection")))
 eligiblePagedAcceptance:=req.Action=="set_acceptance_criteria"||
  req.Action=="review_acceptance_criterion"
 if r.Method==http.MethodPost&&postPaged&&(eligiblePagedStep||eligiblePagedAcceptance){
  var page agentchat.TaskPlanPageResult
  var e error
  switch req.Action{
  case "set_step":
   if req.Status=="done"{
    page,e=store.SetSegmentTaskPlanStepDone(
     id,owner,req.ExpectedRevision,req.StepID,req.EvidenceKind,
     req.EvidenceRef,req.EvidenceNote,postOffset,postLimit)
   }else{
    page,e=store.SetSegmentTaskPlanStepStatus(
     id,owner,req.ExpectedRevision,req.StepID,req.Status,postOffset,postLimit)
   }
  case "set_acceptance_criteria":
   if !req.ConfirmCoverage{
    http.Error(w,"explicit owner confirmation required to replace acceptance matrix",403);return
   }
   if len(req.AcceptanceCriteria)>agentchat.MaxTaskAcceptanceCriteria{
    http.Error(w,"too many acceptance criteria",400);return
   }
   criteria:=make([]agentchat.TaskAcceptanceCriterion,0,len(req.AcceptanceCriteria))
   for _,v:=range req.AcceptanceCriteria{
    criteria=append(criteria,agentchat.TaskAcceptanceCriterion{
     ID:v.ID,Description:strings.TrimSpace(v.Description),
     RequiredTests:append([]string(nil),v.RequiredTests...),
    })
   }
   page,e=store.SetSegmentTaskPlanAcceptanceCriteria(
    id,owner,req.ExpectedRevision,req.StepID,criteria,postOffset,postLimit)
  case "review_acceptance_criterion":
   if !req.ConfirmCoverage{
    http.Error(w,"explicit owner review confirmation required",403);return
   }
   note:=strings.TrimSpace(req.CoverageReviewNote)
   if len(note)==0||len(note)>512{
    http.Error(w,"owner review note is required and at most 512 bytes",400);return
   }
   page,e=store.ReviewSegmentTaskPlanAcceptanceCriterion(
    id,owner,req.ExpectedRevision,req.StepID,
    req.CriterionID,note,postOffset,postLimit)
  }
  if e==nil{
   w.Header().Set("Cache-Control","no-store")
   writeJSON(w,200,page)
   return
  }
  if !errors.Is(e,agentchat.ErrTaskPlanSegmentWriteUnsupported){
   switch {
   case errors.Is(e,agentchat.ErrNotFound):
    http.Error(w,"session not found",404)
   case errors.Is(e,agentchat.ErrStaleRevision):
    http.Error(w,"stale task plan revision",409)
   case errors.Is(e,agentchat.ErrTaskPlanPageOutOfRange):
    http.Error(w,"task plan page offset exceeds plan length",400)
   case errors.Is(e,agentchat.ErrTaskPlanNotEditable):
    http.Error(w,"reopen the completed plan explicitly before editing",409)
   case errors.Is(e,agentchat.ErrTaskPlanStepNotFound):
    http.Error(w,"invalid task plan step: unknown task plan step",409)
   case errors.Is(e,agentchat.ErrTaskPlanCompletionProofRejected):
    http.Error(w,"completion evidence not verified or owner acceptance criteria not reviewed",409)
   case errors.Is(e,agentchat.ErrTaskPlanAcceptanceInvalid):
    http.Error(w,"invalid acceptance criteria or unmapped required Go test",400)
   case errors.Is(e,agentchat.ErrTaskPlanCriterionNotFound):
    http.Error(w,"unknown acceptance criterion for milestone",404)
   case errors.Is(e,agentchat.ErrTaskPlanAcceptanceLocked):
    http.Error(w,"reopen accepted milestone before editing acceptance criteria",409)
   default:
    http.Error(w,"unable to persist task plan",409)
   }
   return
  }
 }
 x,err:=store.Get(id,owner)
 if errors.Is(err,agentchat.ErrNotFound){http.Error(w,"session not found",404);return}
 if err!=nil{http.Error(w,"session journal unavailable",500);return}
 if x.Workspace!=s.Workspace{http.Error(w,"workspace mismatch",403);return}
 if r.Method==http.MethodPost && postPaged {
  // Any out-of-range response page is rejected before the CAS mutation.
  total:=0
  if x.TaskPlan!=nil{total=len(x.TaskPlan.Steps)}
  if postOffset>total{
   http.Error(w,"task plan page offset exceeds plan length",400);return
  }
 }
 if r.Method==http.MethodGet{
  done,total:=agentchat.TaskPlanProgress(x.TaskPlan)
  result:=map[string]any{
   "session_id":id,"revision":x.Revision,"plan":x.TaskPlan,
   "done":done,"total":total,"all_done":agentchat.TaskPlanAllDone(x.TaskPlan),
   "completed":x.TaskPlan!=nil&&x.TaskPlan.State=="completed",
  }
  w.Header().Set("Cache-Control","no-store")
  writeJSON(w,200,result)
  return
 }
 if x.Revision!=req.ExpectedRevision{
  http.Error(w,"stale task plan revision",409);return
 }
 now:=time.Now().UTC()
 switch req.Action{
 case "create":
  if x.TaskPlan!=nil{http.Error(w,"task plan already exists",409);return}
  steps:=make([]agentchat.TaskPlanStep,0,len(req.Steps))
  for _,step:=range req.Steps {
   steps=append(steps,agentchat.TaskPlanStep{ID:step.ID,Title:step.Title,Acceptance:step.Acceptance})
  }
  plan,e:=agentchat.NewTaskPlan(req.Goal,steps)
  if e!=nil{http.Error(w,"invalid task plan",400);return}
  x.TaskPlan=plan
 case "add_step":
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{
   http.Error(w,"cannot add a step without an active plan",409);return
  }
  x.TaskPlan.Steps=append(x.TaskPlan.Steps,agentchat.TaskPlanStep{
   ID:req.StepID,Title:strings.TrimSpace(req.Title),Acceptance:strings.TrimSpace(req.Acceptance),
   Status:"pending",UpdatedAt:now})
  x.TaskPlan.UpdatedAt=now
 case "set_step":
  if x.TaskPlan==nil{http.Error(w,"create a task plan first",409);return}
  if x.TaskPlan.State=="completed"{http.Error(w,"reopen the completed plan explicitly before editing evidence",409);return}
  var proof *agentchat.TaskPlanEvidence
  if req.Status=="done"{
   // An owner-bound required-test checklist must not silently degrade to
   // plain exit-zero or an opportunistically selected single test.
   for _,step:=range x.TaskPlan.Steps{
    if step.ID==req.StepID&&step.GoTestRequirements!=nil&&req.EvidenceKind!="go_test_suite"{
     http.Error(w,"remove the expected Go tests explicitly before using weaker evidence",409);return
    }
   }
   var verified agentchat.TaskPlanEvidence
   var e error
   if req.EvidenceKind=="task_run"{
    verified,e=s.verifyNativeTaskRunEvidence(x,req.EvidenceRef,req.EvidenceTaskID,req.EvidenceNote,req.StepID)
   }else if req.EvidenceKind=="go_test_case"{
    verified,e=s.verifyNativeGoTestCaseEvidence(x,req.EvidenceRef,req.EvidenceTaskID,
      req.StepID,req.EvidenceGoPackage,req.EvidenceGoTest,req.EvidenceNote)
   }else if req.EvidenceKind=="go_test_suite"{
    if req.EvidenceGoPackage!=""||req.EvidenceGoTest!=""||len(req.RequiredGoTests)!=0{
     http.Error(w,"expected Go test suite is read from the owner-bound plan only",400);return
    }
    verified,e=s.verifyNativeGoTestSuiteEvidence(x,req.EvidenceRef,req.EvidenceTaskID,
      req.StepID,req.EvidenceNote)
   }else{
    verified,e=agentchat.VerifyArchivedTaskPlanEvidence(store,x,req.EvidenceKind,req.EvidenceRef,req.EvidenceNote)
   }
   if e!=nil{http.Error(w,"completion evidence not verified: "+e.Error(),409);return}
   proof=&verified
  }
  if e:=agentchat.UpdateTaskPlanStep(&x,req.StepID,req.Status,proof);e!=nil{
   http.Error(w,"invalid task plan step: "+e.Error(),409);return
  }
 case "set_task_command_guard":
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{http.Error(w,"no editable task plan",409);return}
  if !req.ConfirmCommandGuard{http.Error(w,"explicit owner confirmation required",403);return}
  found:=false
  for i:=range x.TaskPlan.Steps {
   step:=&x.TaskPlan.Steps[i]
   if step.ID!=req.StepID{continue}
   found=true
   if step.Status=="done"{http.Error(w,"reopen accepted milestone before editing test command",409);return}
   if step.GoTestRequirements!=nil{http.Error(w,"remove expected Go tests explicitly before changing command lock",409);return}
   if len(step.AcceptanceCriteria)>0{
    http.Error(w,"remove the owner acceptance matrix explicitly before changing command lock",409);return
   }
   if !req.Enabled{step.TaskCommandGuard=nil
   }else{
    digest:=req.ExpectedCommandSHA256
    if req.EvidenceTaskID<1||len(digest)!=64||strings.ToLower(digest)!=digest{
     http.Error(w,"invalid expected native command SHA-256 or Task ID",400);return
    }
    step.TaskCommandGuard=&agentchat.TaskCommandGuard{
     TaskID:req.EvidenceTaskID,CommandSHA256:digest,BoundAt:now,
    }
   }
   step.UpdatedAt=now;x.TaskPlan.UpdatedAt=now
   break
  }
  if !found{http.Error(w,"unknown milestone",404);return}
 case "set_go_test_requirements":
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{
   http.Error(w,"no editable Task plan",409);return
  }
  if !req.ConfirmGoTests{
   http.Error(w,"explicit owner confirmation required for expected Go tests",403);return
  }
  found:=false
  for i:=range x.TaskPlan.Steps{
   step:=&x.TaskPlan.Steps[i]
   if step.ID!=req.StepID{continue}
   found=true
   if step.Status=="done"{
    http.Error(w,"reopen accepted milestone before changing test requirements",409);return
   }
   if len(step.AcceptanceCriteria)>0{
    http.Error(w,"remove the owner acceptance matrix explicitly before changing expected Go tests",409);return
   }
   if !req.Enabled{
    step.GoTestRequirements=nil
   }else{
    if step.TaskCommandGuard==nil{
     http.Error(w,"lock the expected native test command before selecting required tests",409);return
    }
    candidate:=&agentchat.TaskGoTestRequirements{
     Package:req.EvidenceGoPackage,Tests:req.RequiredGoTests,BoundAt:now,
    }
    if err:=agentchat.ValidateTaskGoTestRequirements(candidate);err!=nil{
     http.Error(w,"invalid or duplicate expected Go test names",400);return
    }
    step.GoTestRequirements=candidate
   }
   step.UpdatedAt=now
   x.TaskPlan.UpdatedAt=now
   break
  }
  if !found{http.Error(w,"unknown milestone",404);return}
 case "set_acceptance_criteria":
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{
   http.Error(w,"no editable Task plan",409);return
  }
  if !req.ConfirmCoverage{
   http.Error(w,"explicit owner confirmation required to replace acceptance matrix",403);return
  }
  if len(req.AcceptanceCriteria)>agentchat.MaxTaskAcceptanceCriteria{
   http.Error(w,"too many acceptance criteria",400);return
  }
  found:=false
  for i:=range x.TaskPlan.Steps{
   step:=&x.TaskPlan.Steps[i]
   if step.ID!=req.StepID{continue}
   found=true
   if step.Status=="done"{
    http.Error(w,"reopen accepted milestone before changing acceptance matrix",409);return
   }
   // Any change resets ALL reviews, so previous review notes can never
   // attach to altered criterion text or a different expected test.
   candidate:=make([]agentchat.TaskAcceptanceCriterion,0,len(req.AcceptanceCriteria))
   for _,v:=range req.AcceptanceCriteria{
    candidate=append(candidate,agentchat.TaskAcceptanceCriterion{
     ID:v.ID,Description:strings.TrimSpace(v.Description),
     RequiredTests:append([]string(nil),v.RequiredTests...),
    })
   }
   temp:=*step
   temp.AcceptanceCriteria=candidate
   if e:=agentchat.ValidateTaskAcceptanceCriteria(temp);e!=nil{
    http.Error(w,"invalid acceptance criteria or unmapped required Go test",400);return
   }
   step.AcceptanceCriteria=candidate
   step.UpdatedAt=now;x.TaskPlan.UpdatedAt=now
   break
  }
  if !found{http.Error(w,"unknown milestone",404);return}
 case "review_acceptance_criterion":
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{
   http.Error(w,"no editable Task plan",409);return
  }
  if !req.ConfirmCoverage{
   http.Error(w,"explicit owner review confirmation required",403);return
  }
  note:=strings.TrimSpace(req.CoverageReviewNote)
  if len(note)==0||len(note)>512{
   http.Error(w,"owner review note is required and at most 512 bytes",400);return
  }
  found:=false
  for i:=range x.TaskPlan.Steps{
   step:=&x.TaskPlan.Steps[i]
   if step.ID!=req.StepID{continue}
   if step.Status=="done"{
    http.Error(w,"reopen accepted milestone before reviewing criteria",409);return
   }
   for j:=range step.AcceptanceCriteria{
    criterion:=&step.AcceptanceCriteria[j]
    if criterion.ID!=req.CriterionID{continue}
    // This records an OWNER statement of semantic coverage only. It never
    // fetches provider content, starts tests, or mints Native Task proof.
    criterion.ReviewNote=note
    at:=now
    criterion.ReviewedAt=&at
    found=true
    step.UpdatedAt=now;x.TaskPlan.UpdatedAt=now
    break
   }
   break
  }
  if !found{http.Error(w,"unknown acceptance criterion for milestone",404);return}
 case "set_auto_check":
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{
   http.Error(w,"reopen the plan before changing automatic acceptance",409);return
  }
  found:=false
  for i:=range x.TaskPlan.Steps{
   step:=&x.TaskPlan.Steps[i]
   if step.ID!=req.StepID{continue}
   found=true
   if step.Status=="done"{http.Error(w,"accepted task evidence is immutable unless the step is reopened",409);return}
   if !req.Enabled{step.AutoCheck=nil;step.UpdatedAt=now;break}
   if !req.ConfirmAutoAccept{
    http.Error(w,"explicit owner confirmation required for mechanical auto-verification",403);return
   }
   rule:=&agentchat.TaskAutoCheck{Kind:req.AutoKind,CallID:req.AutoCallID,Path:req.AutoPath}
   if err:=agentchat.ValidateTaskAutoCheck(rule);err!=nil{
    http.Error(w,"invalid automatic evidence rule",400);return
   }
   // Baseline archive generations before accepting the rule. Future
   // compaction will wake the worker if the exact receipt moved to disk.
   rule.CheckedHistoryParts=x.HistoryArchiveParts
   rule.CheckedPatchParts=x.PatchArchiveParts
   step.AutoCheck=rule
   step.UpdatedAt=now
   // A historical native receipt may already be in an archived segment.
   // A checked, immutable result can be recorded immediately; otherwise the
   // background scheduler examines this exact rule after future tool work.
   if agentchat.TaskPlanStepCriteriaReviewed(*step) {
    if evidence,err:=agentchat.VerifyArchivedTaskAutoCheck(store,x,rule);err==nil{
    if err:=agentchat.UpdateTaskPlanStep(&x,step.ID,"done",&evidence);err!=nil{
     http.Error(w,"could not persist verified evidence",409);return
    }
   }
   }
   break
  }
  if !found{http.Error(w,"unknown task milestone",404);return}
  x.TaskPlan.UpdatedAt=now
 case "set_auto_continue":
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{
   http.Error(w,"no active task plan",409);return
  }
  if req.Enabled{
   if !req.ConfirmExternal{
    http.Error(w,"explicit ongoing provider-sharing authorization required",403);return
   }
   if owner!="local"||s.Config.SharedServerEnabled{
    http.Error(w,"automatic plan continuation is local-owner only",403);return
   }
   if x.TaskPlan.State!="active"{http.Error(w,"resume plan before enabling automatic continuation",409);return}
   if agentchat.TaskPlanNext(x.TaskPlan)==nil{http.Error(w,"no actionable plan step remains",409);return}
   if x.ProviderRun!=nil && (x.ProviderRun.Phase=="unknown"||x.ProviderRun.Phase=="sending"){
    http.Error(w,"resolve ambiguous provider acknowledgement before enabling Auto continue",409);return
   }
   for _,call:=range x.ToolCalls{
    if call.Status=="unknown"||call.FeedbackStatus=="unknown"||
     call.FeedbackStatus=="sending"{
     http.Error(w,"resolve ambiguous Native Patch/feedback before enabling Auto continue",409);return
    }
   }
  }
  x.TaskPlan.AutoContinue=req.Enabled
  x.TaskPlan.NoProgressTurns=0
  x.TaskPlan.LastWorkSignature=""
  x.TaskPlan.RetryStreak=0
  x.TaskPlan.NextRetryAt=nil
  x.TaskPlan.UpdatedAt=now
 case "pause":
  if x.TaskPlan==nil||x.TaskPlan.State=="completed"{
   http.Error(w,"no active task plan",409);return
  }
  x.TaskPlan.State="paused"
  x.TaskPlan.AutoContinue=false
  x.TaskPlan.UpdatedAt=now
 case "resume":
  if x.TaskPlan==nil||x.TaskPlan.State!="paused"{
   http.Error(w,"plan is not paused",409);return
  }
  x.TaskPlan.State="active"
  x.TaskPlan.RetryStreak=0
  x.TaskPlan.NextRetryAt=nil
  x.TaskPlan.UpdatedAt=now
 case "reopen":
  if x.TaskPlan==nil||x.TaskPlan.State!="completed"{
   http.Error(w,"only a completed plan can be reopened",409);return
  }
  x.TaskPlan.State="paused"
  x.TaskPlan.AutoContinue=false
  x.TaskPlan.NoProgressTurns=0
  x.TaskPlan.RetryStreak=0
  x.TaskPlan.NextRetryAt=nil
  x.TaskPlan.UpdatedAt=now
 case "complete":
  if x.TaskPlan==nil||!agentchat.TaskPlanAllDone(x.TaskPlan){
   http.Error(w,"all steps must have completion evidence",409);return
  }
  unresolved:=agentchat.HasUnresolvedWork(x)
  for _,call:=range x.ToolCalls{
   if (call.Status=="done"||call.Status=="error"||call.Status=="rejected")&&
    (!call.FeedbackSent||call.FeedbackStatus!="acknowledged"){unresolved=true}
  }
  if unresolved||
   (x.ProviderRun!=nil&&(x.ProviderRun.Phase=="prepared"||x.ProviderRun.Phase=="sending"||x.ProviderRun.Phase=="unknown")){
   http.Error(w,"unresolved provider or native Patch work blocks task completion",409);return
  }
  x.TaskPlan.State="completed"
  x.TaskPlan.AutoContinue=false
  x.TaskPlan.UpdatedAt=now
 default:
  http.Error(w,"unsupported task plan action",400);return
 }
 if err:=agentchat.ValidateTaskPlan(x.TaskPlan);err!=nil{
  http.Error(w,"invalid task plan state",400);return
 }
 x.AppendEvent("plan_updated",x.Status,"",req.Action)
 x,err=store.Save(x,x.Revision)
 if errors.Is(err,agentchat.ErrStaleRevision){http.Error(w,"stale task plan revision",409);return}
 if err!=nil{http.Error(w,"unable to persist task plan",409);return}
 done,total:=agentchat.TaskPlanProgress(x.TaskPlan)
 response:=map[string]any{"session_id":id,"revision":x.Revision,
  "plan":x.TaskPlan,"done":done,"total":total,
  "all_done":agentchat.TaskPlanAllDone(x.TaskPlan),
  "completed":x.TaskPlan!=nil&&x.TaskPlan.State=="completed"}
 if postPaged {
  end:=postOffset+postLimit
  if end>total{end=total}
  if x.TaskPlan!=nil{
   page:=*x.TaskPlan
   page.Steps=append([]agentchat.TaskPlanStep{},x.TaskPlan.Steps[postOffset:end]...)
   response["plan"]=&page
  }
  response["page"]=map[string]any{
   "offset":postOffset,"limit":postLimit,"total_steps":total,
   "has_next":end<total,
  }
 }
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,200,response)
}
