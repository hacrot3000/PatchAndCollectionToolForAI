package server

import (
 "context"
 "encoding/json"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentPatchRejectedProposalRecordsActionableReasonWithoutExecution(t *testing.T){
 for _,tc:=range []struct{
  name string
  calls []agentchat.FunctionCall
  want string
 }{
  {"malformed_patch_args",[]agentchat.FunctionCall{{
   CallID:"call_invalid",Name:"taskdeck_submit_patch",
   Arguments:`{"patch_text":"wrong field, no patch_python"}`,
  }},"invalid_patch_arguments"},
  {"unsupported_function",[]agentchat.FunctionCall{{
   CallID:"call_unsupported",Name:"dangerous_unknown_function",
   Arguments:`{"anything":true}`,
  }},"unsupported_function"},
  {"multiple_patch_proposals",[]agentchat.FunctionCall{{
   CallID:"call_a",Name:"taskdeck_submit_patch",Arguments:`{"name":"a","patch_python":"x"}`,
  },{
   CallID:"call_b",Name:"taskdeck_submit_patch",Arguments:`{"name":"b","patch_python":"y"}`,
  }},"multiple_tool_calls"},
 }{
  t.Run(tc.name,func(t *testing.T){
   s,id:=newAgentConversationFixture(t,"stateless")
   s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
    return agentchat.ResponsesResult{ResponseID:"resp-rejected",Calls:tc.calls},nil
   }
   response:=invokeAI(t,s.agentChatSessionItemAPI,"POST",
    "/api/agent/sessions/"+id+"/messages",
    map[string]any{"message":"Draft the Patch","confirm_external":true,"expected_revision":1})
   if response.Code!=200||!strings.Contains(response.Body.String(),`"tool_rejected"`){
    t.Fatalf("provider tool rejection not saved: %d %s",response.Code,response.Body)
   }
   store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
   x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
   if x.Status!="tool_rejected" {t.Fatalf("wrong rejection status: %s",x.Status)}
   if tc.name=="malformed_patch_args" {
    if len(x.ToolCalls)!=1||x.ToolCalls[0].Status!="rejected"||
       x.ToolCalls[0].CallID!="call_invalid"||x.ToolCalls[0].QueuePath!=""||
       x.ToolCalls[0].RunID!=""||x.ToolCalls[0].FeedbackSent {
     t.Fatalf("malformed proposal was not recorded as an unexecuted feedback result: %+v",x.ToolCalls)
    }
    var feedback struct {
     Status string `json:"status"`
     ReasonCode string `json:"reason_code"`
     PatchQueued bool `json:"patch_queued"`
     PatchExecuted bool `json:"patch_executed"`
     Retryable bool `json:"retryable_by_ai"`
    }
    if json.Unmarshal([]byte(x.ToolCalls[0].Result),&feedback)!=nil||
       feedback.Status!="REJECTED"||feedback.ReasonCode!="INVALID_PATCH_ARGUMENTS"||
       feedback.PatchQueued||feedback.PatchExecuted||!feedback.Retryable {
     t.Fatalf("invalid structured rejection feedback: %s",x.ToolCalls[0].Result)
    }
    if _,ok:=agentAutoFeedbackCandidate(x);ok{t.Fatal("no-consent rejection scheduled")}
   }else if len(x.ToolCalls)!=0 {
    t.Fatalf("unsupported or multiple calls became native execution intents: %+v",x.ToolCalls)
   }
   rejected:=0
   for _,event:=range x.Events{
    if event.Kind!="tool_proposal_rejected"{continue}
    if !strings.Contains(event.Detail,tc.want)||event.Status!="tool_rejected"{
     t.Fatalf("non-actionable or unsafe diagnostic: %+v",event)
    }
    rejected++
   }
   if rejected!=len(tc.calls){t.Fatalf("missing per-call durable reasons: got %d",rejected)}
   session:=invokeAI(t,s.agentChatSessionItemAPI,"GET",
    "/api/agent/sessions/"+id,nil)
   if session.Code!=200{t.Fatalf("owner cannot inspect rejected tool: %d",session.Code)}
   var view struct{Session agentchat.Session `json:"session"`}
   if err=json.Unmarshal(session.Body.Bytes(),&view);err!=nil{t.Fatal(err)}
   if len(view.Session.ToolCalls)!=len(x.ToolCalls)||view.Session.Revision!=x.Revision{
    t.Fatal("read-only diagnostic changed journal or created an approval")
   }
  })
 }
}
