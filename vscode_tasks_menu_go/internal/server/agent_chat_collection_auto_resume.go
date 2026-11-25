package server

import (
 "bytes"
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strconv"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func(s *Server)autoResumeRecoveredCollectionOne(ctx context.Context,store *agentchat.Store,id string)bool{
 if ctx.Err()!=nil||s.Config.SharedServerEnabled||s.agentConversationRequiresRunContinuation("local",id){return false}
 release,ok:=lockAgentTurn(s.Workspace,"local",id)
 if !ok{return false}
 x,err:=store.Get(id,"local")
 if err!=nil||x.Workspace!=s.Workspace{release();return false}
 if agentUnknownCollectionResumeCandidate(x){
  // No pre-admission "attempted" tombstone: if the daemon dies before
  // PREPARED, the next scheduler may still admit this exact NEW request.
  release()
  return s.autoResumeUnknownCollection(ctx,store,x)
 }
 if !agentRecoveredCollectionResumeCandidate(x){release();return false}
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

// Private in-process provenance, never a wire field or header.
type collectionAutoResumeContextKey struct{}

func collectionAutoResumeRunID(ctx context.Context)string{
 id,_:=ctx.Value(collectionAutoResumeContextKey{}).(string)
 return id
}

func(s *Server)autoResumeUnknownCollection(ctx context.Context,store *agentchat.Store,x agentchat.Session)bool{
 if ctx.Err()!=nil{return false}
 run:=*x.ProviderRun
 body,err:=json.Marshal(map[string]any{
  "message":"Resume the original unfinished task after an interrupted read-only Collection delivery.",
  "confirm_external":true,"expected_revision":x.Revision,
  "request_id":"collect-auto-resume-"+x.ID+"-"+run.ID,
  "resume_collection_run_id":run.ID,
 })
 if err!=nil{return false}
 req:=httptest.NewRequest(http.MethodPost,"/api/agent/sessions/"+x.ID+"/messages",bytes.NewReader(body)).
  WithContext(context.WithValue(ctx,collectionAutoResumeContextKey{},run.ID))
 req.Header.Set("Content-Type","application/json")
 w:=httptest.NewRecorder()
 s.agentChatMessageForOwner(w,req,x.ID,x.Owner)
 // A successful admission changes the run ID atomically. A failed
 // admission can be retried after backoff without replaying an admitted run.
 if w.Code!=http.StatusOK&&ctx.Err()==nil{
  release,ok:=lockAgentTurn(s.Workspace,x.Owner,x.ID)
  if ok{
   defer release()
   current,e:=store.Get(x.ID,x.Owner)
   if e==nil&&current.Workspace==s.Workspace&&current.ProviderRun!=nil&&
    current.ProviderRun.ID==run.ID&&current.ProviderRun.Phase=="unknown"&&current.Status=="failed"&&
    agentchat.CollectionAutoResumeAllowed(current){
    current.AppendEvent("collection_auto_resume_deferred",current.Status,run.CallID,run.ID)
    if _,e=store.Save(current,current.Revision);e==nil{
     s.recordAgentDebugEvent(x.Owner,x.ID,run.CallID,"collection","collection.auto_resume_deferred","HTTP_"+strconv.Itoa(w.Code),"New Collection continuation admission deferred; old provider request was not replayed")
    }
   }
  }
 }
 return true
}
