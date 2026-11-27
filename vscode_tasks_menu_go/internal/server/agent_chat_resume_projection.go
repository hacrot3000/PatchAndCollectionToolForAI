package server

import (
 "strings"
 "bytes"
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strconv"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

const agentCollectionResumeInstruction="Resume the original unfinished task from the canonical conversation and current verified native receipts. This is a NEW continuation after an unconfirmed read-only Collection provider response, not a replay. Do not re-execute prior Patches or resend acknowledged results. Inspect only the specific missing scope using the current native COLLECT contract; never claim the prior provider received the result. Preserve the existing task plan and approval rules."

// The latest saved observation is mandatory context for explicit resume,
// not a replay of its old provider request or a new native execution.
// Move its exact pair into Next on a private projection. The only new
// canonical item is still the actual new owner message.
func agentCollectionResumeProjection(x agentchat.Session,history []agentchat.HistoryItem,next agentchat.HistoryItem)([]agentchat.HistoryItem,[]agentchat.HistoryItem,bool){
 digest,ok:=agentchat.CollectionResumeReceipt(x)
 if !ok||len(x.History)==0||len(history)<len(x.History){return nil,nil,false}
 for i,v:=range x.History{if v!=history[i]{return nil,nil,false}}
 last:=len(x.History)-1
 if agentchat.ProviderPayloadDigest([]byte(history[last].Content))!=digest{return nil,nil,false}
 first:=-1
 if x.ProviderRun.CallID!=""{
  for i,v:=range x.History[:last]{
   if v.CallID!=x.ProviderRun.CallID{continue}
   if v.Type!="function_call"||v.Name!="taskdeck_collect_workspace"||first>=0{return nil,nil,false}
   first=i
  }
 }else if agentchat.CollectionTextObservation(x.History,last){
  first=last
  if strings.HasPrefix(history[last].Content,agentchat.NativeCollectionTextObservationPrefix){first=last-1}
 }
 if first<0{return nil,nil,false}
 visible:=make([]agentchat.HistoryItem,0,len(history))
 for i,v:=range history{if i!=first&&i!=last{visible=append(visible,v)}}
 mandatory:=[]agentchat.HistoryItem{history[first]}
 if first!=last{mandatory=append(mandatory,history[last])}
 return visible,append(mandatory,next),true
}

// Structured context rejection may compact optional history but must not
// remove this exact receipt from a resume request. Also keep the model's
// request paired with a current modern text fallback observation.
func agentMandatoryRetrySuffix(items []agentchat.HistoryItem)int{
 cut:=len(items)-1
 if cut<0{return 0}
 if items[cut].Type=="function_call_output"{
  for cut>0&&items[cut-1].Type=="function_call_output"{cut--}
  return cut
 }
 if agentchat.CollectionTextObservation(items,cut)&&strings.HasPrefix(items[cut].Content,agentchat.NativeCollectionTextObservationPrefix){return cut-1}
 if items[cut].Type=="user"&&items[cut].Content==agentPatchFeedbackResumeInstruction&&cut>=2&&
  items[cut-1].Type=="function_call_output"&&items[cut-2].Type=="function_call"&&items[cut-2].Name=="taskdeck_submit_patch"&&
  items[cut-2].CallID==items[cut-1].CallID{return cut-2}
 if items[cut].Type=="user"&&items[cut].Content==agentCollectionResumeInstruction&&cut>0{
  prior:=cut-1
  if items[prior].Type=="function_call_output"&&prior>0&&items[prior-1].Type=="function_call"&&items[prior-1].Name=="taskdeck_collect_workspace"&&items[prior-1].CallID==items[prior].CallID{return prior-1}
  if agentchat.CollectionTextObservation(items,prior){
   if strings.HasPrefix(items[prior].Content,agentchat.NativeCollectionTextObservationPrefix){return prior-1}
   return prior
  }
 }
 return cut
}

const agentPatchFeedbackResumeInstruction="TaskDeck user-authorized NEW continuation of the original unfinished task after the provider returned an unusable response to verified native PATCH feedback. The exact completed Patch receipt immediately preceding this message is historical local evidence. The previous provider acknowledgement remains UNKNOWN. Do not execute or propose that same Patch again, do not claim provider acknowledgement, and do not resend the old feedback request. Review the original user goal and continue the next feasible remaining step, using fresh read-only Collection only when necessary. Keep all existing approval and project boundaries."

type patchFeedbackAutoResumeKey struct{}

// Narrow legacy recovery: only a journal-proven single completed PATCH with
// exact canonical call/output, current UNKNOWN parse-failure egress and
// explicit persistent automation consent. Never recover a generic transport
// ambiguity, cancelled turn, unresolved mutation or a Unified run.
func agentPatchFeedbackResumeReceipt(x agentchat.Session)(int,bool){
 r:=x.ProviderRun
 if x.Owner!="local"||x.Archived||x.Status!="failed"||x.PendingUserTurn!=nil||
  !x.AutoContinueText||x.TaskPlan!=nil||!x.AutoFeedback||!x.AutoFeedbackPersistent||
  x.RetryMode!=agentchat.RetryUnlimited||agentchat.HasUnresolvedWork(x)||
  r==nil||r.Kind!="feedback"||r.Phase!="unknown"||r.UsageRecorded||r.CallID==""||
  len(r.PayloadSHA256)!=64||r.ProviderFingerprint==""||len(x.History)<2{return -1,false}
 if x.Continuation!=nil&&x.Continuation.NoProgressTurns>=agentchat.MaxContinuationNoWorkTurns{return -1,false}
 _,seqText,ok:=strings.Cut(r.ID,"-e")
 if !ok{return -1,false}
 seq,err:=strconv.ParseUint(seqText,10,64)
 if err!=nil||seq==0||seq==^uint64(0){return -1,false}
 egress,failed:=false,false
 for _,e:=range x.Events{
  if e.Kind=="patch_feedback_resume_refused"&&e.Detail==r.ID{return -1,false}
  if e.Seq==seq&&e.Kind=="provider_egress"&&e.CallID==r.CallID&&e.Detail=="feedback"{egress=true}
  if e.Seq==seq+1&&e.Kind=="feedback_unknown"&&e.CallID==r.CallID&&e.Status=="failed"&&
   (e.Detail=="provider_acknowledgement_unconfirmed: JSON_PARSE_FAILED"||e.Detail=="provider_acknowledgement_unconfirmed: RESPONSE_EMPTY"){failed=true}
 }
 if !egress||!failed{return -1,false}
 last:=len(x.History)-1
 out:=x.History[last]
 if out.Type!="function_call_output"||out.CallID!=r.CallID||out.OriginRunID!=""||out.Content==""{return -1,false}
 var receipt struct{Status string `json:"status"`;RC *int `json:"rc"`}
 if json.Unmarshal([]byte(out.Content),&receipt)!=nil||receipt.Status!="PASS"||receipt.RC==nil||*receipt.RC!=0{return -1,false}
 var exact *agentchat.ToolCallRecord
 for i:=range x.ToolCalls{
  call:=&x.ToolCalls[i]
  if call.CallID==r.CallID{
   if exact!=nil||call.Name!="taskdeck_submit_patch"||call.AgentRunID!=""||call.TextFallback||
    call.Status!="done"||!call.FeedbackSent||call.FeedbackStatus!="unknown"||
    call.RunID==""||call.PatchSessionID==""||call.PromptID==""||call.SelectedIndex<1||
    len(call.PackageSHA256)!=64||call.PackageSize<=0||strings.TrimSpace(call.Result)!=out.Content{return -1,false}
   exact=call
  }else if !call.FeedbackSent||call.FeedbackStatus!="acknowledged"{return -1,false}
 }
 if exact==nil{return -1,false}
 source:=-1
 for i,item:=range x.History[:last]{
  if item.CallID!=r.CallID{continue}
  if source>=0||item.Type!="function_call"||item.Name!=exact.Name||item.OriginRunID!=""||
   len(exact.ArgumentsSHA256)!=64||agentPatchPayloadSHA256(item.Arguments)!=exact.ArgumentsSHA256{return -1,false}
  source=i
 }
 return source,source>=0
}
func agentPatchFeedbackAutoResumeReady(x agentchat.Session)bool{
 if _,ok:=agentPatchFeedbackResumeReceipt(x);!ok{return false}
 for i:=len(x.Events)-1;i>=0;i--{
  e:=x.Events[i]
  if e.Kind=="patch_feedback_resume_deferred"&&e.Detail==x.ProviderRun.ID{
   return !time.Now().UTC().Before(e.At.Add(30*time.Second))
  }
 }
 return true
}

func agentPatchFeedbackResumeProjection(x agentchat.Session,history []agentchat.HistoryItem,next agentchat.HistoryItem)([]agentchat.HistoryItem,[]agentchat.HistoryItem,bool){
 source,ok:=agentPatchFeedbackResumeReceipt(x)
 if !ok||len(history)<len(x.History){return nil,nil,false}
 for i,item:=range x.History{if history[i]!=item{return nil,nil,false}}
 last:=len(x.History)-1
 visible:=make([]agentchat.HistoryItem,0,len(history)-2)
 for i,item:=range history{if i!=source&&i!=last{visible=append(visible,item)}}
 return visible,[]agentchat.HistoryItem{history[source],history[last],next},true
}

func(s *Server)autoResumePatchFeedbackOne(ctx context.Context,store *agentchat.Store,id string)bool{
 if ctx.Err()!=nil||s.Config.SharedServerEnabled||s.agentConversationRequiresRunContinuation("local",id){return false}
 x,err:=store.Get(id,"local")
 if err!=nil||x.Workspace!=s.Workspace||!agentPatchFeedbackAutoResumeReady(x){return false}
 run:=*x.ProviderRun
 body,err:=json.Marshal(map[string]any{"message":agentPatchFeedbackResumeInstruction,"confirm_external":true,
  "expected_revision":x.Revision,"request_id":"patch-feedback-resume-"+id+"-"+run.ID})
 if err!=nil{return false}
 req:=httptest.NewRequest(http.MethodPost,"/api/agent/sessions/"+id+"/messages",bytes.NewReader(body)).
  WithContext(context.WithValue(collectionContinuationContext(ctx),patchFeedbackAutoResumeKey{},run.ID))
 req.Header.Set("Content-Type","application/json")
 w:=httptest.NewRecorder()
 s.agentChatMessageForOwner(w,req,id,x.Owner)
 if w.Code!=http.StatusOK&&ctx.Err()==nil{
  release,ok:=lockAgentTurn(s.Workspace,x.Owner,id)
  if ok{
   defer release()
   current,e:=store.Get(id,x.Owner)
   // Only annotate the same unchanged pre-admission revision. Once the
   // new turn is admitted, its own retry/receipt journal owns recovery.
   if e==nil&&current.Revision==x.Revision&&current.ProviderRun!=nil&&current.ProviderRun.ID==run.ID{
    kind:="patch_feedback_resume_refused"
    if w.Code==429||w.Code==503||w.Code==500{kind="patch_feedback_resume_deferred"}
    current.AppendEvent(kind,current.Status,run.CallID,run.ID)
    if _,e=store.Save(current,current.Revision);e==nil{
     s.recordAgentDebugEvent(x.Owner,id,run.CallID,"feedback",kind,"HTTP_"+strconv.Itoa(w.Code),"New continuation admission did not succeed; original PATCH receipt and UNKNOWN delivery retained.")
    }
   }
  }
 }
 return true
}
