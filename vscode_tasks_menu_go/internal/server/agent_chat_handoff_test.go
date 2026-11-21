package server

import (
 "encoding/json"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentHandoffOwnerScopedAndLatestAfterProviderStep(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"Study the repository and complete all fixes"})
 x.History=append(x.History,agentchat.HistoryItem{Type:"assistant",Content:"Read main.go; plan to test first."})
 x.Status="completed"
 x.AppendEvent("turn_finished","completed","","")
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/handoff"
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,path,nil)
 if w.Code!=http.StatusOK{t.Fatalf("handoff GET: %d %s",w.Code,w.Body)}
 var got struct{Revision uint64 `json:"revision"`;Handoff string `json:"handoff"`}
 if err:=json.Unmarshal(w.Body.Bytes(),&got);err!=nil{t.Fatal(err)}
 if got.Revision!=x.Revision||!strings.Contains(got.Handoff,"Study the repository")||
  !strings.Contains(got.Handoff,"turn_finished"){t.Fatalf("stale or incomplete handoff: %+v",got)}
 if strings.Contains(got.Handoff,"API_KEY="){t.Fatal("handoff exposed credentials")}
 markdown:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,path+"?format=markdown",nil)
 if markdown.Code!=http.StatusOK||markdown.Body.String()!=got.Handoff{
  t.Fatalf("Markdown handoff differs from canonical data: %d",markdown.Code)
 }
 if ct:=markdown.Header().Get("Content-Type");!strings.HasPrefix(ct,"text/markdown"){t.Fatal("wrong content type: "+ct)}
 denied:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,nil)
 if denied.Code!=http.StatusMethodNotAllowed{t.Fatal("handoff endpoint accepted mutation")}
 missing:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,"/api/agent/sessions/does-not-exist/handoff",nil)
 if missing.Code!=http.StatusNotFound{t.Fatal("missing session handoff exposed data")}
}
func TestAgentContextRebaseKeepsPendingPatchToolPair(t *testing.T){
 x:=agentchat.Session{Mode:agentchat.PreviousResponse,PreviousResponseID:"old-provider-ref"}
 x.History=[]agentchat.HistoryItem{
  {Type:"user",Content:"Fix broken PHP script"},
  {Type:"function_call",Name:"taskdeck_submit_patch",CallID:"pending-1",Arguments:`{"name":"Fix PHP","patch_python":"print(1)"}`},
 }
 if !agentContextNeedsRebase(x){t.Fatal("pending provider tool call did not trigger safe rebase")}
 mode:=agentContextRequestMode(x,"responses",true)
 if mode!=agentchat.Stateless{t.Fatal("uncompact stateful chain was reused")}
 agentClearProviderRefsForStateless(&x,mode)
 if x.PreviousResponseID!=""||x.ConversationID!=""{t.Fatal("stale provider refs survived rebase")}
 next:=agentchat.HistoryItem{Type:"function_call_output",CallID:"pending-1",Content:`{"status":"PASS","rc":0}`}
 visible:=agentContextForRequest(x,agentchat.ModelConfig{MaxInputTokens:32000},
  "Follow TaskDeck rules",[]agentchat.HistoryItem{next})
 payload,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{
  Model:"model",Instructions:"Follow TaskDeck rules",Mode:mode,
  History:visible,Next:[]agentchat.HistoryItem{next},EnableCollectionTool:true,EnablePatchTool:true,
 })
 if err!=nil{t.Fatal(err)}
 raw:=string(payload)
 if strings.Contains(raw,"previous_response_id")||strings.Contains(raw,`"conversation"`) {
  t.Fatalf("rebase retained provider-side conversation: %s",raw)
 }
 if !strings.Contains(raw,"pending-1")||!strings.Contains(raw,"function_call_output")||
  !strings.Contains(raw,"taskdeck_submit_patch"){t.Fatal("pending call and exact result pair omitted during rebase")}
}
