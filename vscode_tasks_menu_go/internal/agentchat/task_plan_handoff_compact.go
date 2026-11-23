package agentchat

import (
 "crypto/sha256"
 "encoding/json"
 "fmt"
 "sort"
 "strings"
)

// A plan's canonical journal and paginated API retain every accepted proof.
// Handoff is a deliberately bounded derivative, not an export or receipt.
const (
 compactHandoffPlanThreshold = 24
 compactHandoffUnfinishedDetail = 8
 compactHandoffAcceptedDetail = 4
 compactHandoffUpcomingIDs = 20
 compactHandoffEvidencePerStep = 2
)

func appendCompactTaskPlanHandoff(b *strings.Builder,sessionID string,plan *TaskPlan){
 if b==nil||plan==nil{return}
 raw,err:=json.Marshal(plan.Steps)
 digest:="unavailable"
 if err==nil{
  sum:=sha256.Sum256(raw)
  digest=fmt.Sprintf("%x",sum[:])
 }
 statuses:=map[string]int{"pending":0,"active":0,"blocked":0,"done":0}
 selected:=make(map[int]bool,compactHandoffUnfinishedDetail+compactHandoffAcceptedDetail)
 pendingIDs:=make([]string,0,compactHandoffUpcomingIDs)
 for i,step:=range plan.Steps{
  statuses[step.Status]++
  if step.Status!="done"{
   if len(pendingIDs)<compactHandoffUpcomingIDs{
    pendingIDs=append(pendingIDs,handoffTrim(step.ID,64))
   }
   if len(selected)<compactHandoffUnfinishedDetail{selected[i]=true}
  }
 }
 accepted:=0
 for i:=len(plan.Steps)-1;i>=0&&accepted<compactHandoffAcceptedDetail;i--{
  if plan.Steps[i].Status=="done"{
   selected[i]=true
   accepted++
  }
 }
 indices:=make([]int,0,len(selected))
 for i:=range selected{indices=append(indices,i)}
 sort.Ints(indices)
 fmt.Fprintf(b,"Large plan summary: %d total milestones; pending=%d, active=%d, blocked=%d, done=%d.\n",
  len(plan.Steps),statuses["pending"],statuses["active"],statuses["blocked"],statuses["done"])
 fmt.Fprintf(b,"Canonical step-inventory SHA-256: %s (diagnostic identity, NOT evidence or acceptance).\n",
  digest)
 fmt.Fprintf(b,"Detailed entries are limited to the first %d unfinished and last %d accepted milestones; %d of %d milestones omitted from this handoff.\n",
  compactHandoffUnfinishedDetail,compactHandoffAcceptedDetail,len(plan.Steps)-len(indices),len(plan.Steps))
 if len(pendingIDs)>0{
  fmt.Fprintf(b,"Next unfinished milestone IDs (at most %d): %s\n",compactHandoffUpcomingIDs,strings.Join(pendingIDs,", "))
 }
 fmt.Fprintf(b,"For COMPLETE step details and all native receipts, use owner-authenticated GET /api/agent/sessions/%s/plan?offset=0&limit=24 and its subsequent pages. This handoff does not authorize inference of omitted steps or completion.\n\n",
  handoffTrim(sessionID,32))
 for _,i:=range indices{
  step:=plan.Steps[i]
  fmt.Fprintf(b,"- [%d/%d] [%s] %s — %s; acceptance: %s\n",
   i+1,len(plan.Steps),step.Status,handoffTrim(step.ID,64),
   handoffTrim(step.Title,120),handoffTrim(step.Acceptance,260))
  if g:=step.TaskCommandGuard;g!=nil{
   fmt.Fprintf(b,"  - Expected native Task %d, command SHA-256 %s, bound %s (identity only).\n",
    g.TaskID,handoffTrim(g.CommandSHA256,64),g.BoundAt.UTC().Format("2006-01-02T15:04:05Z"))
  }
  if req:=step.GoTestRequirements;req!=nil{
   fmt.Fprintf(b,"  - Required named tests: package=%s, bound=%s, tests=%s (full checklist in journal).\n",
    handoffTrim(req.Package,120),req.BoundAt.UTC().Format("2006-01-02T15:04:05Z"),
    handoffTrim(strings.Join(req.Tests,", "),240))
  }
  if len(step.AcceptanceCriteria)>0{
   reviewed:=0
   for _,criterion:=range step.AcceptanceCriteria{
    if criterion.ReviewedAt!=nil{reviewed++}
   }
   fmt.Fprintf(b,"  - OWNER acceptance coverage: %d/%d reviewed (human attestation, not proven semantics).\n",reviewed,len(step.AcceptanceCriteria))
   for _,criterion:=range step.AcceptanceCriteria{
    state:="unreviewed"
    if criterion.ReviewedAt!=nil{state="owner-reviewed"}
    fmt.Fprintf(b,"    - %s: %s; mapped Go tests=%s; %s; note=%s\n",
     handoffTrim(criterion.ID,64),handoffTrim(criterion.Description,140),
     handoffTrim(strings.Join(criterion.RequiredTests,", "),160),
     state,handoffTrim(criterion.ReviewNote,100))
   }
  }
  for j,ev:=range step.Evidence{
   if j>=compactHandoffEvidencePerStep{
    fmt.Fprintf(b,"  - %d further evidence receipt(s) omitted; inspect the canonical plan page.\n",
     len(step.Evidence)-compactHandoffEvidencePerStep)
    break
   }
   fmt.Fprintf(b,"  - %s evidence: ref=%s digest=%s note=%s\n",
    handoffTrim(ev.Kind,24),handoffTrim(ev.Ref,90),
    handoffTrim(ev.Digest,64),handoffTrim(ev.Note,110))
  }
 }
}
