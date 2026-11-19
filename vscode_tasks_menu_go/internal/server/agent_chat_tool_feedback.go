package server

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// agentChatToolContinueAPI submits an authenticated, evidence-backed
// function_call_output using the SAME call_id returned by the provider.
// It does not rerun, replay or requeue any patch; feedback is once-only.
func (s *Server) agentChatToolContinueAPI(w http.ResponseWriter,r *http.Request,id,callID string){
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",405);return}
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 var req struct{
  ExpectedRevision uint64 `json:"expected_revision"`
  ConfirmExternal bool `json:"confirm_external"`
 }
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if dec.Decode(&req)!=nil{http.Error(w,"invalid tool continuation request",400);return}
 var extra any
 if err:=dec.Decode(&extra);!errors.Is(err,io.EOF){http.Error(w,"exactly one continuation object required",400);return}
 if !req.ConfirmExternal{http.Error(w,"explicit external-provider sharing consent required",403);return}
 owner:=aiPlanOwner(r)
 release,ok:=lockAgentTurn(s.Workspace,owner,id)
 if !ok{http.Error(w,"Agent session busy",409);return}
 defer release()
 store,err:=s.agentChatStore();if err!=nil{http.Error(w,"agent journal unavailable",500);return}
 x,err:=store.Get(id,owner)
 if errors.Is(err,agentchat.ErrNotFound){http.Error(w,"Agent session not found",404);return}
 if err!=nil{http.Error(w,"agent journal unavailable",500);return}
 if x.Revision!=req.ExpectedRevision||x.Workspace!=s.Workspace{
  http.Error(w,"Agent session revision/workspace mismatch",409);return
 }
 if x.Status!="waiting_tool"&&x.Status!="tool_rejected"{http.Error(w,"Agent session not awaiting a final tool result",409);return}
 idx:=-1
 for i,item:=range x.ToolCalls{if item.CallID==callID{idx=i;break}}
 if idx<0||x.ToolCalls[idx].FeedbackSent{http.Error(w,"tool result not pending",409);return}
 call:=x.ToolCalls[idx]
 if call.Status!="done"&&call.Status!="error"&&call.Status!="rejected"{
  http.Error(w,"Patch result has no verified final status; do not send success prematurely",409);return
 }
 if _,found:=findAgentToolCall(x.History,callID);!found{
  http.Error(w,"source tool call not in transcript",409);return
 }
 identity,err:=patchtool.EnsureProjectIdentity(s.Workspace)
 if err!=nil||identity.ID!=x.ProjectID{http.Error(w,"project identity drift",409);return}
 registry,err:=s.agentChatProviders();if err!=nil{http.Error(w,"provider store unavailable",500);return}
 p,err:=registry.Get(owner,x.ProviderID)
 if err!=nil{http.Error(w,"provider no longer configured",409);return}
 model,ok:=p.FindModel(x.ModelID)
 if !ok{http.Error(w,"model no longer configured",409);return}
 if s.ConnectionSecrets==nil{http.Error(w,"secret store unavailable",503);return}
 secret,err:=s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace,owner,p.ID))
 if err!=nil||len(secret)==0{http.Error(w,"provider credential unavailable",503);return}
 releaseQuota,allowed:=s.reserveAIRequest(aiWorkspaceHash(s.Workspace)+":"+owner,20)
 if !allowed{http.Error(w,"AI request rate/concurrency limit exceeded",429);return}
 defer releaseQuota()
 if len(x.History)>250{http.Error(w,"agent context limit reached",413);return}
 output:=strings.TrimSpace(call.Result)
 if output==""{http.Error(w,"tool result is empty",409);return}
 if len(output)>4000{http.Error(w,"tool result exceeds limit",413);return}
 next:=agentchat.HistoryItem{Type:"function_call_output",CallID:callID,Content:output}
 limit:=4096
 if model.MaxOutputTokens>0&&model.MaxOutputTokens<limit{limit=model.MaxOutputTokens}
 payload,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{
  Model:x.ModelID,Instructions:agentChatInstructions(x.ProjectID,model.ToolCalling),
  Mode:x.Mode,PreviousResponseID:x.PreviousResponseID,ConversationID:x.ConversationID,
  History:x.History,Next:[]agentchat.HistoryItem{next},
  MaxOutputTokens:limit,ReasoningEffort:model.DefaultReasoningEffort,
  EnablePatchTool:model.ToolCalling,
 })
 if err!=nil{http.Error(w,"cannot format tool-result model request",413);return}
 // Persist output and mark delivered BEFORE crossing the network; a timeout
 // cannot result in accidental duplicate tool output or patch replay.
 x.History=append(x.History,next)
 x.ToolCalls[idx].FeedbackSent=true
 x.Status="requesting_ai"
 x,err=store.Save(x,x.Revision)
 if err!=nil{http.Error(w,"cannot persist once-only tool output",409);return}
 ctx,cancel:=context.WithTimeout(r.Context(),60*time.Second);defer cancel()
 result,err:=s.agentChatCall(ctx,p,string(secret),payload)
 if err!=nil{
  x.Status="failed"
  _,_=store.Save(x,x.Revision)
  http.Error(w,"provider continuation failed; tool result was saved and will not be resent automatically",502);return
 }
 appendAgentProviderResult(&x,result,model.ToolCalling)
 x,err=store.Save(x,x.Revision)
 if err!=nil{http.Error(w,"provider responded but journal write failed; inspect history before retry",409);return}
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,200,map[string]any{
  "session_id":id,"revision":x.Revision,"status":x.Status,
  "answer":result.Text,"tool_calls":result.Calls,
  "response_id":result.ResponseID,"input_tokens":result.InputTokens,
  "output_tokens":result.OutputTokens,"execution_enabled":false,
 })
}
