package agentchat
import (
 "strings"
 "testing"
)
const questionFixture=`{"questions":[{"id":"platform","prompt":"Which platform?","type":"single_choice","required":true,"options":[{"id":"ios","label":"iOS"},{"id":"android","label":"Android"}],"allow_comment":true},{"id":"notes","prompt":"Details?","type":"text","required":false,"options":[],"allow_comment":false}]}`
func TestAskUserValidation(t *testing.T){
 q,e:=ParseAskUser(questionFixture);if e!=nil{t.Fatal(e)}
 result,e:=EncodeAskUserResult(q,[]AskUserAnswer{{QuestionID:"platform",Selected:[]string{"ios"},Text:"Tiếng Việt"},{QuestionID:"notes",Text:"More details"}},false)
 if e!=nil||!strings.Contains(result,"Tiếng Việt"){t.Fatalf("answer lost: %s %v",result,e)}
 for _,answers:=range [][]AskUserAnswer{
 nil,{{QuestionID:"platform",Selected:[]string{"ios","android"}}},{{QuestionID:"platform",Selected:[]string{"unknown"}}},
 {{QuestionID:"platform",Selected:[]string{"ios"}},{QuestionID:"notes",Text:strings.Repeat("x",1201)}},
 }{if _,e:=EncodeAskUserResult(q,answers,false);e==nil{t.Fatal("invalid answer accepted")}}
 if _,e:=EncodeAskUserResult(q,nil,true);e!=nil{t.Fatal(e)}
 h:=[]HistoryItem{{Type:"function_call",CallID:"q",Name:AskUserToolName,Arguments:questionFixture}}
 c:=ToolCallRecord{CallID:"q",Name:AskUserToolName,ArgumentsSHA256:BodyDigest([]byte(questionFixture)),Status:"pending"}
 if !ValidAskUserRecord(c,h){t.Fatal("valid pending questionnaire rejected")}
 c.Status="done";c.Result=result
 if !ValidAskUserRecord(c,h){t.Fatal("valid answer rejected")}
 c.QueuePath="patchs/unsafe.zip";if ValidAskUserRecord(c,h){t.Fatal("native execution fields accepted")}
}
func TestAskUserAdvertisedOnlyWhenEnabled(t *testing.T){
 for _,enabled:=range []bool{false,true}{
  b,e:=BuildResponsesRequest(RequestConfig{Model:"m",Mode:Stateless,Instructions:"test",Next:[]HistoryItem{{Type:"user",Content:"help"}},EnableAskUserTool:enabled})
  if e!=nil||strings.Contains(string(b),AskUserToolName)!=enabled{t.Fatalf("tool advertisement %s %v",b,e)}
 }
}
