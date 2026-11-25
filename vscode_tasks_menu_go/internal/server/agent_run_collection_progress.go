package server

import (
 "encoding/json"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
 "bletonfc/vscode_tasks_menu/internal/agentrun"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Existing Unified runs drain through their immutable snapshots. Never use
// mutable Legacy objective state to make decisions for another worktree.
type runCollectionProgress struct{
 Version int `json:"version"`
 Counters cp.CounterState `json:"counters"`
 Evidence cp.EvidenceIndex `json:"evidence"`
 Decision cp.Decision `json:"decision"`
 LastHistorySize int `json:"last_history_size"`
}

func runCollectionObjective(owner string,spec agentchat.RunSpec,inputs AgentRunInputs)(cp.ObjectiveBinding,error){
 // Neutral intent for old runs: do not reconstruct owner instructions
 // from generated user-role text or historical tool feedback.
 return cp.BindObjective(cp.ObjectiveScope{
  Owner:owner,ProjectID:inputs.ProjectID,OwnerWorkspace:spec.OwnerWorkspaceID,
  ExecutionWorkspace:spec.ExecutionWorkspaceID,ConversationID:spec.ConversationID,
  Goal:"Continue",StepID:agentchat.ProviderPayloadDigest([]byte(spec.RunID+"\x00"+spec.ParentTurnID)),Generation:1,
 })
}
func readRunCollectionProgress(owner string,spec agentchat.RunSpec,inputs AgentRunInputs)(runCollectionProgress,bool){
 if inputs.Snapshot.Policy==nil{return runCollectionProgress{},false}
 raw:=inputs.Snapshot.Policy.CollectionProgress
 binding,err:=runCollectionObjective(owner,spec,inputs)
 if err!=nil{return runCollectionProgress{},false}
 if len(raw)==0{
  c,err:=cp.NewCounterState(binding)
  e:=cp.NewEvidenceIndex(binding)
  for _,item:=range inputs.Snapshot.History{
   if item.Type=="function_call_output"&&item.OriginRunID==spec.RunID{e.Evicted=true;break}
  }
  return runCollectionProgress{Version:cp.Version,Counters:c,Evidence:e},err==nil
 }
 var g runCollectionProgress
 if len(raw)>96<<10||json.Unmarshal(raw,&g)!=nil||g.Version!=cp.Version||
  g.Counters.Version!=cp.Version||g.Counters.Objective!=binding||g.Evidence.ObjectiveKey!=binding.Key||
  len(g.Evidence.Entries)>cp.MaxEvidenceEntries||g.LastHistorySize<1||
  g.LastHistorySize>len(inputs.Snapshot.History)||g.Counters.LastSequence!=uint64(g.LastHistorySize)*2-1||
  g.Counters.Observed>g.Counters.Requested||g.Counters.NativeReads>g.Counters.Observed||
  g.Counters.Progress.DuplicateStreak>g.Counters.NativeReads{
  return runCollectionProgress{},false
 }
 return g,true
}
func runCollectionProgressUpdate(owner string,run agentrun.Record,inputs AgentRunInputs,identity patchtool.ProjectIdentity,call agentchat.HistoryItem,request []byte,receipt string,native bool)json.RawMessage{
 raw:=inputs.Snapshot.Policy.CollectionProgress
 g,ok:=readRunCollectionProgress(owner,run.Spec,inputs)
 if !ok||len(run.History)<len(inputs.Snapshot.History)||len(run.History)==0{return raw}
 // Bind the projection to the exact canonical history prefix. A changed
 // history or unknown metadata must not manufacture verified novelty.
 for i,it:=range inputs.Snapshot.History{if run.History[i]!=it{return raw}}
 // A possible intervening mutation invalidates historical source proof.
 // Never reuse evidence to skip reads, even when no mutation is observed.
 for _,it:=range run.History[g.LastHistorySize:]{
  if it.Type=="function_call"&&it.Name=="taskdeck_submit_patch"{
   g.Evidence=cp.InvalidateEvidence(g.Evidence,false)
   g.Counters.Progress.DuplicateStreak=0
  }
 }
 novelty:=cp.UnknownNovelty
 if !native{novelty=cp.ObservationError}else if scope,err:=cp.BindCollectionScope(g.Counters.Objective,identity.Key,identity.ID,request,"");err==nil{
  proof:=cp.InspectNativeEvidence(scope,identity.Key,identity.ID,request,receipt)
  g.Evidence,novelty=cp.ObserveEvidence(g.Evidence,scope,proof)
 }
 // Existing immutable snapshot is the observation commit boundary. The
 // stable sequence derives from canonical history position, not HTTP.
 seq:=uint64(len(run.History))*2
 if seq<=g.Counters.LastSequence{return raw}
 id:=agentchat.ProviderPayloadDigest([]byte(run.Spec.RunID+"\x00"+call.CallID))
 c,_,applied,err:=cp.ApplyCounterEvent(cp.Defaults(),g.Counters,cp.CounterEvent{Sequence:seq,Identity:id,Kind:cp.CollectionRequested},false)
 if err!=nil||!applied{return raw}
 c,d,applied,err:=cp.ApplyCounterEvent(cp.Defaults(),c,cp.CounterEvent{Sequence:seq+1,Identity:id,Kind:cp.CollectionObserved,Novelty:novelty,NativeRead:native},false)
 if err!=nil||!applied{return raw}
 g.Counters=c;g.Decision=d;g.LastHistorySize=len(run.History)+1
 out,err:=json.Marshal(g)
 if err!=nil||len(out)>96<<10{return raw}
 return out
}
func runCollectionProgressAdvice(owner string,spec agentchat.RunSpec,inputs AgentRunInputs)string{
 g,ok:=readRunCollectionProgress(owner,spec,inputs)
 if !ok||g.LastHistorySize!=len(inputs.Snapshot.History){return ""}
 // Advisory only: do not enable Unified creation or automatic replan.
 return collectionProgressAdvice(g.Decision)
}
