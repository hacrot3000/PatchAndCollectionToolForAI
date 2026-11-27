package agentchat

import (
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "regexp"
 "strings"
 "unicode/utf8"
)
const AskUserToolName="taskdeck_ask_user"
const AskUserInstructions="When information or a decision only the user can supply is necessary, call taskdeck_ask_user with 1-8 questions in the user's language. Each question has a unique id, prompt, type (single_choice, multiple_choice or text), required, options (empty for text), and allow_comment (false for text). Choice options have unique id and label. Ask independent questions together. Do not ask the user to manually execute an eligible project-local script when the native command-only PATCH lane is available; use its normal approval workflow instead. Wait for actual user answers; never infer them. Never request passwords, tokens, private keys or other secrets. Cancellation is not approval. This tool is not native COLLECT or Patch approval."
var askUserID=regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$")
type AskUserOption struct{ID string `json:"id"`;Label string `json:"label"`}
type AskUserQuestion struct{
 ID string `json:"id"`
 Prompt string `json:"prompt"`
 Type string `json:"type"`
 Required bool `json:"required"`
 Options []AskUserOption `json:"options"`
 AllowComment bool `json:"allow_comment"`
}
type AskUserRequest struct{Questions []AskUserQuestion `json:"questions"`}
type AskUserAnswer struct{
 QuestionID string `json:"question_id"`
 Selected []string `json:"selected"`
 Text string `json:"text"`
}
type AskUserResult struct{
 Status string `json:"status"`
 Answers []AskUserAnswer `json:"answers"`
}
func askUserDecode(raw string,dst any)error{
 if !utf8.ValidString(raw)||strings.ContainsRune(raw,0){return errors.New("invalid UTF-8 questionnaire")}
 d:=json.NewDecoder(strings.NewReader(raw));d.DisallowUnknownFields()
 if d.Decode(dst)!=nil{return errors.New("invalid questionnaire JSON or fields")}
 var extra any
 if d.Decode(&extra)!=io.EOF{return errors.New("exactly one JSON object required")}
 return nil
}
func askUserText(s string,max int)bool{
 return utf8.ValidString(s)&&!strings.ContainsRune(s,0)&&len(s)<=max&&strings.TrimSpace(s)!=""
}
func ParseAskUser(raw string)(AskUserRequest,error){
 var req AskUserRequest
 if len(raw)>24<<10{return req,errors.New("questionnaire exceeds 24 KiB")}
 if err:=askUserDecode(raw,&req);err!=nil{return req,err}
 if len(req.Questions)<1||len(req.Questions)>8{return req,errors.New("provide 1-8 questions")}
 seen:=map[string]bool{}
 for i,q:=range req.Questions{
  if !askUserID.MatchString(q.ID)||seen[q.ID]||!askUserText(q.Prompt,1000){return req,fmt.Errorf("invalid question %d id or prompt",i+1)}
  seen[q.ID]=true
  switch q.Type{
  case "text":
   if len(q.Options)!=0||q.AllowComment{return req,fmt.Errorf("text question %d cannot have options or allow_comment",i+1)}
  case "single_choice","multiple_choice":
   if len(q.Options)<2||len(q.Options)>8{return req,fmt.Errorf("question %d needs 2-8 options",i+1)}
   ids:=map[string]bool{}
   for _,o:=range q.Options{
    if !askUserID.MatchString(o.ID)||ids[o.ID]||!askUserText(o.Label,400){return req,fmt.Errorf("invalid option in question %d",i+1)}
    ids[o.ID]=true
   }
  default:return req,fmt.Errorf("invalid type for question %d",i+1)
  }
 }
 return req,nil
}
func EncodeAskUserResult(req AskUserRequest,answers []AskUserAnswer,cancelled bool)(string,error){
 result:=AskUserResult{Status:"ANSWERED",Answers:[]AskUserAnswer{}}
 if cancelled{
  if len(answers)!=0{return "",errors.New("cancelled questionnaire must have no answers")}
  result.Status="CANCELLED"
 }else{
  byID:=map[string]AskUserAnswer{}
  for _,a:=range answers{
   if _,ok:=byID[a.QuestionID];ok{return "",errors.New("duplicate answer")}
   byID[a.QuestionID]=a
  }
  for i,q:=range req.Questions{
   a:=byID[q.ID];delete(byID,q.ID);a.QuestionID=q.ID
   if len(a.Text)>1200||!utf8.ValidString(a.Text)||strings.ContainsRune(a.Text,0){return "",fmt.Errorf("question %d text exceeds 1200 UTF-8 bytes or is invalid",i+1)}
   selected:=map[string]bool{}
   for _,id:=range a.Selected{
    if selected[id]{return "",fmt.Errorf("duplicate choice in question %d",i+1)}
    selected[id]=true
   }
   if q.Type=="text"{
    if len(selected)!=0||q.Required&&strings.TrimSpace(a.Text)==""{return "",fmt.Errorf("question %d requires valid text input",i+1)}
   }else{
    if !q.AllowComment&&a.Text!=""||q.Type=="single_choice"&&len(selected)>1||q.Required&&len(selected)==0{return "",fmt.Errorf("invalid or missing selection in question %d",i+1)}
   }
   a.Selected=[]string{}
   for _,o:=range q.Options{if selected[o.ID]{a.Selected=append(a.Selected,o.ID);delete(selected,o.ID)}}
   if len(selected)!=0{return "",fmt.Errorf("unknown option in question %d",i+1)}
   result.Answers=append(result.Answers,a)
  }
  if len(byID)!=0{return "",errors.New("unknown question id")}
 }
 b,err:=json.Marshal(result)
 if err!=nil||len(b)>4096{return "",errors.New("answers exceed the 4096-byte result limit; shorten descriptions")}
 return string(b),nil
}
func ValidAskUserRecord(call ToolCallRecord,history []HistoryItem)bool{
 if call.Name!=AskUserToolName||call.AgentRunID!=""||call.TextFallback||
  call.QueuePath!=""||call.PackageSHA256!=""||call.PackageSize!=0||call.PatchSessionID!=""||
  call.NativeStartedAt!=nil||call.AutoRunOnce||call.SendAfterRetry||call.PromptID!=""||call.SelectedIndex!=0||call.RunID!=""||
  call.ExpectedHead!=""||call.ExpectedBranch!=""||call.WorkspaceStatusSHA256!=""||call.GitIsRepo||
  call.ApprovalExpiresAt!=nil||call.ManualRetryOnly||len(call.Attempts)!=0||call.FailedQueueRetirement!=nil{return false}
 var source *HistoryItem
 for i:=range history{it:=&history[i];if it.Type=="function_call"&&it.CallID==call.CallID{
  if source!=nil||it.Name!=AskUserToolName||it.OriginRunID!=""||BodyDigest([]byte(it.Arguments))!=call.ArgumentsSHA256{return false};source=it
 }}
 if source==nil{return false}
 req,err:=ParseAskUser(source.Arguments);if err!=nil{return false}
 if call.Status=="pending"{return call.Result==""&&!call.FeedbackSent&&call.FeedbackStatus==""}
 if call.Status!="done"{return false}
 var result AskUserResult
 if askUserDecode(call.Result,&result)!=nil||(result.Status!="ANSWERED"&&result.Status!="CANCELLED"){return false}
 encoded,err:=EncodeAskUserResult(req,result.Answers,result.Status=="CANCELLED")
 return err==nil&&encoded==call.Result
}
func AskUserToolSchema()map[string]any{
 str:=map[string]any{"type":"string"}
 option:=map[string]any{"type":"object","additionalProperties":false,"required":[]string{"id","label"},"properties":map[string]any{"id":str,"label":str}}
 question:=map[string]any{"type":"object","additionalProperties":false,"required":[]string{"id","prompt","type","required","options","allow_comment"},"properties":map[string]any{
 "id":str,"prompt":str,"type":map[string]any{"type":"string","enum":[]string{"single_choice","multiple_choice","text"}},"required":map[string]any{"type":"boolean"},"options":map[string]any{"type":"array","items":option,"maxItems":8},"allow_comment":map[string]any{"type":"boolean"}}}
 return map[string]any{"type":"function","name":AskUserToolName,"description":AskUserInstructions,"strict":false,"parameters":map[string]any{"type":"object","additionalProperties":false,"required":[]string{"questions"},"properties":map[string]any{"questions":map[string]any{"type":"array","minItems":1,"maxItems":8,"items":question}}}}
}
