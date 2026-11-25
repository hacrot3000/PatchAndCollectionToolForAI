package server

import (
 "bytes"
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func(s *Server)autoResumeRecoveredCollectionOne(ctx context.Context,store *agentchat.Store,id string)bool{
 if ctx.Err()!=nil||s.Config.SharedServerEnabled||s.agentConversationRequiresRunContinuation("local",id){return false}
 release,ok:=lockAgentTurn(s.Workspace,"local",id)
 if !ok{return false}
 x,err:=store.Get(id,"local")
 if err!=nil||x.Workspace!=s.Workspace||!agentRecoveredCollectionResumeCandidate(x){release();return false}
 runID:=x.ProviderRun.ID
 // Record the single recovery attempt BEFORE releasing the lease. The
 // normal message API rechecks the exact revision, owner and workspace.
 // Crash/admission failure here leaves the explicit Resume action available.
 x.AppendEvent("collection_resume_attempted",x.Status,x.ProviderRun.CallID,runID)
 saved,err:=store.Save(x,x.Revision)
 release()
 if err!=nil{return false}
 msg:="TaskDeck user-authorized recovery of the ORIGINAL unfinished conversation after a verified local Collection validation failure. This is a new continuation, not replay of provider run "+runID+". Preserve all native Patch receipts and plan acceptance. Correct the rejected request using the current native COLLECT contract, choose a narrower action, and never assume the rejected request produced evidence."
 body,err:=json.Marshal(map[string]any{"message":msg,"confirm_external":true,"expected_revision":saved.Revision,"request_id":"collect-resume-"+id+"-"+runID})
 if err!=nil{return true}
 req:=httptest.NewRequest(http.MethodPost,"/api/agent/sessions/"+id+"/messages",bytes.NewReader(body)).WithContext(collectionContinuationContext(ctx))
 req.Header.Set("Content-Type","application/json")
 w:=httptest.NewRecorder()
 s.agentChatMessageAPI(w,req,id)
 return true
}
