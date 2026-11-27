package server

import (
 "context"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

const responseRepairQuestions = `{"questions":[{"id":"notes","prompt":"Compiler flags?","type":"text","required":true,"options":[],"allow_comment":false}]}`

func TestQuestionnaireResponseInGitWorkspacePersists(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 gitQuickRun(t,s.Workspace,"init","-q")
 gitQuickRun(t,s.Workspace,"-c","user.name=Test","-c","user.email=test@example.invalid","commit","--allow-empty","-qm","baseline")
 snap,err:=agentWorkspaceSnapshotOf(context.Background(),s.Workspace)
 if err!=nil||!snap.IsRepo||snap.Head==""{t.Fatalf("missing Git fixture: %v",err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++
  return agentchat.ResponsesResult{ResponseID:"response-question",Calls:[]agentchat.FunctionCall{
   {CallID:"question-new",Name:agentchat.AskUserToolName,Arguments:responseRepairQuestions},
  }},nil
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",map[string]any{
  "message":"Ask me for compiler flags if needed","expected_revision":1,"confirm_external":true,"request_id":"git-questionnaire",
 })
 if w.Code!=200||calls!=1{t.Fatalf("questionnaire response save: %d calls=%d %s",w.Code,calls,w.Body)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Status!="waiting_tool"||x.ProviderRun.Phase!="completed"||len(x.ToolCalls)!=1||
  !agentchat.ValidAskUserRecord(x.ToolCalls[0],x.History){t.Fatal("questionnaire failed strict validation")}
 c:=x.ToolCalls[0]
 if c.ExpectedHead!=""||c.ExpectedBranch!=""||c.WorkspaceStatusSHA256!=""||c.GitIsRepo{t.Fatal("questionnaire inherited PATCH drift metadata")}
}

func TestQuestionnaireResponseCheckpointRepairBeforeAndAfterParking(t *testing.T){
 for _,parked:=range []bool{false,true}{
  name:="sending";if parked{name="already_failed"}
  t.Run(name,func(t *testing.T){
   s,store,base,candidate:=receivedResponseRecoveryFixture(t)
   candidate.Status="waiting_tool"
   candidate.History=append(candidate.History,agentchat.HistoryItem{
    Type:"function_call",Name:agentchat.AskUserToolName,CallID:"question-recover",Arguments:responseRepairQuestions,
   })
   candidate.ToolCalls=append([]agentchat.ToolCallRecord(nil),base.ToolCalls...)
   candidate.ToolCalls=append(candidate.ToolCalls,agentchat.ToolCallRecord{
    Name:agentchat.AskUserToolName,CallID:"question-recover",Status:"pending",
    ArgumentsSHA256:agentchat.BodyDigest([]byte(responseRepairQuestions)),
    ExpectedHead:strings.Repeat("a",40),ExpectedBranch:"dev",WorkspaceStatusSHA256:strings.Repeat("b",64),GitIsRepo:true,
   })
   if agentchat.ValidAskUserRecord(candidate.ToolCalls[len(candidate.ToolCalls)-1],candidate.History){t.Fatal("fixture must reproduce original invalid projection")}
   if err:=store.StageProviderResponse(candidate);err!=nil{t.Fatal(err)}
   if parked{
    base.Status="failed";markAgentProviderOutcome(&base,"unknown")
    base.AppendEvent("turn_interrupted","failed","","provider_response_save_failed")
    var err error;base,err=store.Save(base,base.Revision);if err!=nil{t.Fatal(err)}
    // A local diagnostic after parking must not invalidate the exact receipt.
    base.AppendEvent("clp_http_attempt","failed","","diagnostic")
    base,err=store.Save(base,base.Revision);if err!=nil{t.Fatal(err)}
   }
   s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
    t.Fatal("checkpoint repair must not contact provider");return agentchat.ResponsesResult{},nil
   }
   s.agentReconcileOneStartupSession(context.Background(),store,base.ID)
   got,err:=store.Get(base.ID,"local");if err!=nil{t.Fatal(err)}
   if got.Revision!=base.Revision+1||got.Status!="waiting_tool"||got.ProviderRun.Phase!="completed"||
    got.ProviderRun.ID!=base.ProviderRun.ID||len(got.History)!=len(candidate.History)||
    !agentchat.ValidAskUserRecord(got.ToolCalls[len(got.ToolCalls)-1],got.History){t.Fatal("exact saved questionnaire was not recovered")}
   s.agentReconcileOneStartupSession(context.Background(),store,base.ID)
   again,err:=store.Get(base.ID,"local");if err!=nil||again.Revision!=got.Revision{t.Fatal("duplicate recovery")}
  })
 }
}
