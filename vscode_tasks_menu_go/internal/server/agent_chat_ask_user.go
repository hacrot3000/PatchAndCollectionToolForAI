package server

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Persist actual owner input before provider egress; no native operation.
func(s *Server) agentChatAnswerUserAPI(w http.ResponseWriter,r *http.Request,id,callID string){
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",405);return}
 owner:=aiPlanOwner(r)
 if owner!="local"||s.Config.SharedServerEnabled{http.Error(w,"interactive questions require a local conversation",403);return}
 r.Body=http.MaxBytesReader(w,r.Body,24<<10)
 var req struct{
  ConfirmExternal bool `json:"confirm_external"`
  ArgumentsSHA256 string `json:"arguments_sha256"`
  Cancelled bool `json:"cancelled"`
  Answers []agentchat.AskUserAnswer `json:"answers"`
 }
 d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
 if d.Decode(&req)!=nil{http.Error(w,"invalid questionnaire answer",400);return}
 var extra any;if !errors.Is(d.Decode(&extra),io.EOF){http.Error(w,"exactly one answer object required",400);return}
 if !req.ConfirmExternal{http.Error(w,"explicit external-provider sharing consent required",403);return}
 release,ok:=lockAgentTurn(s.agentOwnerWorkspace(),owner,id)
 if !ok{http.Error(w,"Agent session busy",409);return};defer release()
 store,err:=s.agentChatStore();if err!=nil{http.Error(w,"agent journal unavailable",500);return}
 x,err:=store.Get(id,owner)
 if err!=nil{http.Error(w,"conversation unavailable",404);return}
 if x.Archived||x.Workspace!=s.agentOwnerWorkspace()||x.Status!="waiting_tool"||
  x.ProviderRun==nil||x.ProviderRun.Phase!="completed"||!agentFeedbackCallIsCurrent(x,callID){
  http.Error(w,"questionnaire is no longer pending; refresh conversation",409);return
 }
 identity,err:=patchtool.EnsureProjectIdentity(s.Workspace)
 if err!=nil||identity.ID!=x.ProjectID{http.Error(w,"project identity changed",409);return}
 idx:=-1
 for i,c:=range x.ToolCalls{if c.CallID==callID{idx=i;break}}
 if idx<0||x.ToolCalls[idx].Name!=agentchat.AskUserToolName||x.ToolCalls[idx].Status!="pending"||
  x.ToolCalls[idx].ArgumentsSHA256!=req.ArgumentsSHA256||!agentchat.ValidAskUserRecord(x.ToolCalls[idx],x.History){
  http.Error(w,"questionnaire changed or already answered; refresh conversation",409);return
 }
 source:=x.History[len(x.History)-1]
 if source.Type!="function_call"||source.CallID!=callID{http.Error(w,"questionnaire binding changed",409);return}
 questions,err:=agentchat.ParseAskUser(source.Arguments)
 if err!=nil{http.Error(w,"invalid saved questionnaire",409);return}
 result,err:=agentchat.EncodeAskUserResult(questions,req.Answers,req.Cancelled)
 if err!=nil{http.Error(w,err.Error(),400);return}
 x.ToolCalls[idx].Status="done";x.ToolCalls[idx].Result=result
 x.AppendEvent("user_questions_answered","waiting_tool",callID,"")
 x,err=store.Save(x,x.Revision)
 if err!=nil{http.Error(w,"cannot save user answers; no answer was sent",409);return}
 idx=-1;for i,c:=range x.ToolCalls{if c.CallID==callID{idx=i;break}}
 if idx<0{http.Error(w,"saved questionnaire missing; no answer was sent",409);return}
 res,err:=s.agentDeliverToolResult(s.agentServerContext(),store,x,idx,true)
 if err!=nil{writeAgentContinueError(w,err);return}
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,200,map[string]any{"session_id":id,"revision":res.Session.Revision,"status":res.Session.Status,"answer":res.Result.Text,"tool_calls":res.Result.Calls})
}
