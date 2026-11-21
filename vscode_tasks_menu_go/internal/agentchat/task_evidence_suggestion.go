package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "io"
 "strings"
 "time"
)

const taskEvidenceSuggestionFence="\x60\x60\x60taskdeck-evidence-suggestion\n"

// TaskEvidenceSuggestion is an ADVISORY pointer authored by an AI model.
// It has no trusted receipt status and no effect on AutoCheck/approval.
// A complete response digest and model turn identify where it was suggested.
type TaskEvidenceSuggestion struct {
 StepID string `json:"step_id"`
 Kind string `json:"kind"`
 CallID string `json:"call_id"`
 Path string `json:"path,omitempty"`
 TurnID string `json:"turn_id"`
 SourceDigest string `json:"source_digest"`
 At time.Time `json:"at"`
}

// CaptureTaskEvidenceSuggestion must only be called with the actual provider
// response in appendAgentProviderResult, never with user/Collection/source
// content. A fenced model claim is not an execution receipt or user consent.
func CaptureTaskEvidenceSuggestion(x *Session, turnID, modelText string) {
 if x==nil||x.TaskPlan==nil||x.TaskPlan.State!="active"||
  len(turnID)==0||len(turnID)>256||len(modelText)>64<<10{return}
 first:=strings.Index(modelText,taskEvidenceSuggestionFence)
 if first<0{return}
 tail:=modelText[first+len(taskEvidenceSuggestionFence):]
 end:=strings.Index(tail,"\n\x60\x60\x60")
 if end<0||end>2048{return}
 var candidate struct{
  StepID string `json:"step_id"`
  Kind string `json:"kind"`
  CallID string `json:"call_id"`
  Path string `json:"path"`
 }
 decoder:=json.NewDecoder(strings.NewReader(strings.TrimSpace(tail[:end])))
 decoder.DisallowUnknownFields()
 if decoder.Decode(&candidate)!=nil{return}
 var extra any
 if decoder.Decode(&extra)!=io.EOF{return}
 rule:=&TaskAutoCheck{Kind:candidate.Kind,CallID:candidate.CallID,Path:candidate.Path}
 if !taskPlanStepIDPattern.MatchString(candidate.StepID)||ValidateTaskAutoCheck(rule)!=nil{return}
 eligible:=false
 for _,step:=range x.TaskPlan.Steps{
  if step.ID==candidate.StepID && step.Status!="done"{eligible=true;break}
 }
 if !eligible{return}
 for _,prior:=range x.TaskPlan.Suggestions{
  if prior.StepID==candidate.StepID&&prior.Kind==candidate.Kind&&
   prior.CallID==candidate.CallID&&prior.Path==candidate.Path{return}
 }
 sum:=sha256.Sum256([]byte(modelText))
 proposal:=TaskEvidenceSuggestion{StepID:candidate.StepID,Kind:candidate.Kind,
  CallID:candidate.CallID,Path:candidate.Path,TurnID:turnID,
  SourceDigest:hex.EncodeToString(sum[:]),At:time.Now().UTC()}
 x.TaskPlan.Suggestions=append(x.TaskPlan.Suggestions,proposal)
 if len(x.TaskPlan.Suggestions)>MaxTaskPlanSuggestions{
  x.TaskPlan.Suggestions=x.TaskPlan.Suggestions[len(x.TaskPlan.Suggestions)-MaxTaskPlanSuggestions:]
 }
 x.TaskPlan.UpdatedAt=proposal.At
}
