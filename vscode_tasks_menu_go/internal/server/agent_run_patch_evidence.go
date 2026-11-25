package server

import (
 "errors"
 "reflect"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentrun"
)

// Match the exact Patch call/output, including parent-turn provenance, in the
// immutable provider input. A model message containing PASS is never a receipt.
func runPatchEvidencePair(history []agentchat.HistoryItem,spec agentchat.RunSpec,callID string)([]agentchat.HistoryItem,error){
 var pair []agentchat.HistoryItem
 for _,item:=range history{
  if item.CallID!=callID||(item.Type!="function_call"&&item.Type!="function_call_output"){continue}
  if item.OriginRunID!=spec.RunID||item.ProviderID!=spec.ProviderID||item.ModelID!=spec.ModelID||item.ParentTurnID!=spec.ParentTurnID{
   return nil,errors.New("Patch receipt has different run provenance")
  }
  if len(pair)==0&&item.Type=="function_call"&&item.Name=="taskdeck_submit_patch"||
   len(pair)==1&&item.Type=="function_call_output"{
   pair=append(pair,item)
  }else{return nil,errors.New("ambiguous Patch receipt pair")}
 }
 if len(pair)!=2{return nil,errors.New("exact Patch call/output unavailable")}
 return pair,nil
}

// Require both the durable native ToolCallRecord and the same immutable
// call/output pair in the run journal and checksummed context snapshot.
func verifyRunPatchProof(x agentchat.Session,record agentrun.Record,snapshot agentrun.ContextSnapshot,callID string)(agentchat.TaskPlanEvidence,error){
 var empty agentchat.TaskPlanEvidence
 spec:=record.Spec
 if x.ID!=spec.ConversationID||x.ProjectID==""||snapshot.ProjectID!=x.ProjectID||
  snapshot.ID!=spec.ContextSnapshotID||snapshot.ConversationID!=spec.ConversationID||
  snapshot.OwnerWorkspaceID!=spec.OwnerWorkspaceID{
  return empty,errors.New("Patch receipt scope mismatch")
 }
 if err:=verifyAgentRunSnapshotBinding(spec,snapshot);err!=nil{return empty,err}
 pair,err:=runPatchEvidencePair(record.History,spec,callID)
 if err!=nil{return empty,err}
 pinned,err:=runPatchEvidencePair(snapshot.History,spec,callID)
 if err!=nil||!reflect.DeepEqual(pair,pinned){return empty,errors.New("Patch receipt differs from its immutable snapshot")}
 call,err:=canonicalRunToolCall(x.ToolCalls,callID,spec.RunID)
 if err!=nil{return empty,err}
 if call.ArgumentsSHA256!=agentPatchPayloadSHA256(pair[0].Arguments)||call.Result!=pair[1].Content{
  return empty,errors.New("Patch receipt does not match canonical native execution")
 }
 return agentchat.VerifyRunNativePatchEvidence(spec,call,x.ProjectID)
}

// Historical rule registration uses the same owner-scoped durable run reader,
// never a live manager or a provider call. Missing archived native receipts
// fail closed until a dedicated archive projection is implemented.
func(s *Server) resolveRunPatchAutoCheck(x agentchat.Session,ref string)(agentchat.TaskPlanEvidence,error){
 var empty agentchat.TaskPlanEvidence
 runID,callID,ok:=parseRunCollectionEvidenceRef(ref)
 if !ok{return empty,agentchat.ErrInvalidSession}
 workspace:=s.agentOwnerWorkspace()
 workspaceID,err:=agentrun.WorkspaceID(workspace)
 if err!=nil||x.Workspace!=workspace{return empty,errors.New("Patch evidence workspace mismatch")}
 dir,err:=s.agentRunJournalDir(x.Owner)
 if err!=nil{return empty,err}
 record,err:=agentrun.ReadCompletedRecord(dir,runID)
 if err!=nil{return empty,err}
 if record.Spec.OwnerWorkspaceID!=workspaceID||record.Spec.ConversationID!=x.ID{return empty,errors.New("Patch evidence run scope mismatch")}
 snapshots,err:=s.agentContextSnapshotStore(x.Owner)
 if err!=nil{return empty,err}
 snapshot,err:=snapshots.Get(record.Spec.ContextSnapshotID,workspaceID,x.ID,x.ProjectID)
 if err!=nil{return empty,err}
 return verifyRunPatchProof(x,record,snapshot,callID)
}
