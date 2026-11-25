package server

import (
 "strconv"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Recover only an old journal that proves this exact provider run returned
// before LOCAL identity validation failed. UsageRecorded is set only after
// successful provider results. The event sequence binds it to the current run.
// No uploaded report, pre-egress attempt ID or timestamp grants recovery.
func agentRecoverAcknowledgedCollection(x *agentchat.Session) bool {
 if x==nil||x.Owner!="local"||x.Archived||x.Status!="failed"||x.ProviderRun==nil {return false}
 run:=x.ProviderRun
 if run.Kind!="collection"||run.Phase!="unknown"||!run.UsageRecorded||len(run.PayloadSHA256)!=64 {return false}
 prefix,seqText,ok:=strings.Cut(run.ID,"-e")
 if !ok||!strings.HasPrefix(prefix,"r"){return false}
 if _,err:=strconv.ParseUint(strings.TrimPrefix(prefix,"r"),10,64);err!=nil{return false}
 seq,err:=strconv.ParseUint(seqText,10,64)
 if err!=nil||seq==0||seq>^uint64(0)-2{return false}
 if x.EventSeq!=seq+2||len(x.Events)<3{return false}
 n:=len(x.Events)
 egress,rejected,failed:=x.Events[n-3],x.Events[n-2],x.Events[n-1]
 if egress.Seq!=seq||egress.Kind!="provider_egress"||egress.CallID!=run.CallID||egress.Detail!="collection"||
  rejected.Seq!=seq+1||rejected.Kind!="collect_request_rejected"||rejected.Status!="failed"||
  !(strings.HasPrefix(rejected.Detail,"project identity does not match this TaskDeck workspace")||strings.HasPrefix(rejected.Detail,"invalid project identity:")||strings.HasPrefix(rejected.Detail,"missing project.key/project.id")||strings.HasPrefix(rejected.Detail,"text COLLECT must include exact project.key and project.id"))||
  failed.Seq!=seq+2||failed.Kind!="collect_failed"||failed.Status!="failed"||failed.Detail!="collection_continuation" {return false}
 // Preserve task status and history. This is not recovery of the missing
 // response body and does not authorize replay of the old provider request.
 markAgentProviderOutcome(x,"acknowledged")
 x.PreviousResponseID="";x.ConversationID=""
 x.AppendEvent("provider_ack_recovered","failed",run.CallID,"verified local response accounting and exact Collection rejection; do not replay original request")
 return true
}


// A response checkpoint is an exact current-run marker in canonical local
// history. If interrupted after acknowledgement, never re-run native work:
// close the pending call with an explicitly unverified observation instead.
func agentCloseInterruptedCollection(x *agentchat.Session)bool{
 if x==nil||x.ProviderRun==nil||x.ProviderRun.Phase!="acknowledged"{return false}
 var event *agentchat.SessionEvent
 for i:=len(x.Events)-1;i>=0;i--{
  e:=&x.Events[i]
  if e.Kind=="collect_response_checkpoint"&&e.Detail==x.ProviderRun.ID{event=e;break}
 }
 if event==nil{return false}
 const diagnostic=`{"status":"FAILED","error":{"code":"COLLECTION_LOCAL_INTERRUPTED","message":"The acknowledged native Collection response was interrupted during local processing. No verified evidence is available for this call. Inspect native history before any potentially mutating operation; do not replay the old request.","retry_allowed":false}}`
 if event.CallID!=""{
  found:=false
  for _,it:=range x.History{
   if it.CallID!=event.CallID{continue}
   if it.Type=="function_call_output"{x.PreviousResponseID="";x.ConversationID="";return false}
   if it.Type=="function_call"&&it.Name=="taskdeck_collect_workspace"{found=true}
  }
  if !found{return false}
  x.History=append(x.History,agentchat.HistoryItem{Type:"function_call_output",CallID:event.CallID,Content:diagnostic})
 }else{
  if len(x.History)==0||x.History[len(x.History)-1].Type!="assistant"{x.PreviousResponseID="";x.ConversationID="";return false}
  x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"TaskDeck local Collection checkpoint: "+diagnostic})
 }
 x.PreviousResponseID="";x.ConversationID=""
 x.AppendEvent("collect_checkpoint_closed",x.Status,event.CallID,x.ProviderRun.ID)
 return true
}
