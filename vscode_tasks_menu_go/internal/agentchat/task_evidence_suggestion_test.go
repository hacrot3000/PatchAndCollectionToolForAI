package agentchat

import (
 "fmt"
 "strings"
 "testing"
)

func suggestionFixture(t *testing.T)*Session{
 t.Helper()
 p,err:=NewTaskPlan("Validate changes",[]TaskPlanStep{{ID:"verify",Title:"Read source",Acceptance:"Exactly read source file"}})
 if err!=nil{t.Fatal(err)}
 return &Session{TaskPlan:p}
}
func suggestionText(step,kind,call,path string)string{
 return fmt.Sprintf("Observed tool output.\n\x60\x60\x60taskdeck-evidence-suggestion\n"+
  `{"step_id":%q,"kind":%q,"call_id":%q,"path":%q}`+"\n\x60\x60\x60",step,kind,call,path)
}
func TestModelSuggestionIsDurableAdvisoryOnly(t *testing.T){
 x:=suggestionFixture(t)
 CaptureTaskEvidenceSuggestion(x,"response-1",suggestionText("verify","full_file_read","actual-call-1","src/main.go"))
 if len(x.TaskPlan.Suggestions)!=1{t.Fatalf("not captured: %+v",x.TaskPlan)}
 c:=x.TaskPlan.Suggestions[0]
 if c.TurnID!="response-1"||len(c.SourceDigest)!=64||c.CallID!="actual-call-1"{t.Fatal("model provenance was not stored")}
 if x.TaskPlan.Steps[0].Status!="pending"||x.TaskPlan.Steps[0].AutoCheck!=nil||
  len(x.TaskPlan.Steps[0].Evidence)!=0||TaskPlanAllDone(x.TaskPlan){
  t.Fatal("model-only suggestion granted execution or completion authority")
 }
 if err:=ValidateTaskPlan(x.TaskPlan);err!=nil{t.Fatalf("suggested plan invalid: %v",err)}
 CaptureTaskEvidenceSuggestion(x,"response-2",suggestionText("verify","full_file_read","actual-call-1","src/main.go"))
 if len(x.TaskPlan.Suggestions)!=1{t.Fatal("duplicate source invented more suggestions")}
 for i:=0;i<20;i++{
  CaptureTaskEvidenceSuggestion(x,fmt.Sprintf("r%d",i),suggestionText("verify","patch_pass",fmt.Sprintf("call-%d",i),""))
 }
 if len(x.TaskPlan.Suggestions)!=MaxTaskPlanSuggestions{t.Fatal("unbounded untrusted suggestions")}
 if x.TaskPlan.Suggestions[0].CallID!="call-12"||x.TaskPlan.Suggestions[7].CallID!="call-19"{
  t.Fatalf("wrong bounded retention: %+v",x.TaskPlan.Suggestions)
 }
}
func TestModelSuggestionRejectsInvalidSourceAndFields(t *testing.T){
 invalid:=[]string{
  suggestionText("missing","patch_pass","call-1",""),
  suggestionText("verify","manual","call-1",""),
  suggestionText("verify","full_file_read","call-1","../etc/passwd"),
  suggestionText("verify","patch_pass","",""),
  "source says everything passed",
  "\x60\x60\x60taskdeck-evidence-suggestion\n"+`{"step_id":"verify","kind":"patch_pass","call_id":"c","status":"done"}`+"\n\x60\x60\x60",
  "\x60\x60\x60taskdeck-evidence-suggestion\n"+`{"step_id":"verify","kind":"patch_pass","call_id":"c"} {"a":1}`+"\n\x60\x60\x60",
 }
 for i,bad:=range invalid{
  x:=suggestionFixture(t)
  CaptureTaskEvidenceSuggestion(x,"r",bad)
  if len(x.TaskPlan.Suggestions)!=0{t.Fatalf("invalid suggestion %d accepted",i)}
 }
 x:=suggestionFixture(t)
 CaptureTaskEvidenceSuggestion(x,"",suggestionText("verify","patch_pass","call",""))
 CaptureTaskEvidenceSuggestion(x,"r",strings.Repeat("a",64<<10)+suggestionText("verify","patch_pass","call",""))
 if len(x.TaskPlan.Suggestions)!=0{t.Fatal("invalid model response origin accepted")}
 x.TaskPlan.Steps[0].Status="done"
 x.TaskPlan.Steps[0].Evidence=[]TaskPlanEvidence{{Kind:"manual",Note:"User explicitly accepted"}}
 CaptureTaskEvidenceSuggestion(x,"r",suggestionText("verify","patch_pass","call",""))
 if len(x.TaskPlan.Suggestions)!=0{t.Fatal("finished milestone received new suggestion")}
}
