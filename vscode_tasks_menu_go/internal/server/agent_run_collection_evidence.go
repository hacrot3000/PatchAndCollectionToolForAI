package server

import (
 "encoding/json"
 "errors"
 "reflect"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentrun"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// A provider call ID is not globally unique across runs/providers. Qualified
// references are explicit owner selections, never inferred from model text.
func parseRunCollectionEvidenceRef(ref string)(runID,callID string,ok bool){
 if !strings.HasPrefix(ref,"run:")||len(ref)>256||strings.ContainsAny(ref,"\r\n\x00"){return "","",false}
 runID,callID,ok=strings.Cut(strings.TrimPrefix(ref,"run:"),"/")
 return runID,callID,ok&&safeAgentRunID.MatchString(runID)&&callID!=""&&strings.TrimSpace(callID)==callID
}

type verifiedRunCollectionEvidence struct{
 owner,workspace,project,conversation string
 proof agentchat.TaskPlanEvidence
}

func(v verifiedRunCollectionEvidence) forSession(x agentchat.Session)(agentchat.TaskPlanEvidence,error){
 if x.Owner!=v.owner||x.Workspace!=v.workspace||x.ProjectID!=v.project||x.ID!=v.conversation{
  return agentchat.TaskPlanEvidence{},errors.New("run evidence scope mismatch")
 }
 return v.proof,nil
}

// Select the exact pair from both the completed run's durable journal and
// the checksummed immutable snapshot. No assistant/user narration, historical
// foreign run, unsupported tool or duplicate call ID can serve as evidence.
func runCollectionEvidencePair(history []agentchat.HistoryItem,spec agentchat.RunSpec,callID string)([]agentchat.HistoryItem,error){
 var pair []agentchat.HistoryItem
 for _,item:=range history{
  if item.CallID!=callID{continue}
  if item.Type!="function_call"&&item.Type!="function_call_output"{continue}
  if item.OriginRunID!=spec.RunID||item.ProviderID!=spec.ProviderID||
   item.ModelID!=spec.ModelID||item.ParentTurnID!=spec.ParentTurnID{
   return nil,errors.New("Collection receipt has different run provenance")
  }
  if len(pair)==0&&item.Type=="function_call"&&item.Name=="taskdeck_collect_workspace"||
   len(pair)==1&&item.Type=="function_call_output"{
   pair=append(pair,item)
  }else{return nil,errors.New("ambiguous Collection receipt pair")}
 }
 if len(pair)!=2{return nil,errors.New("exact Collection call/output unavailable")}
 var receipt struct{Source string `json:"source"`}
 if json.Unmarshal([]byte(pair[1].Content),&receipt)!=nil||receipt.Source!="native_patch_tool_collect"{
  return nil,errors.New("Collection output is not a native artifact receipt")
 }
 return pair,nil
}

// The server obtains this proof BEFORE the plan journal CAS lock. The
// callback inside the paged writer only checks owner/project/conversation
// and returns this immutable result, avoiding reentrant Store calls. No
// run is started/resumed and no provider or native tool is contacted.
func(s *Server) resolveRunCollectionEvidence(owner,conversation,ref,note string)(verifiedRunCollectionEvidence,error){
 return s.resolveRunCollectionEvidenceForPath(owner,conversation,ref,note,"")
}

func(s *Server) resolveRunCollectionEvidenceForPath(owner,conversation,ref,note,requiredPath string)(verifiedRunCollectionEvidence,error){
 var empty verifiedRunCollectionEvidence
 runID,callID,ok:=parseRunCollectionEvidenceRef(ref)
 if !ok||len(note)>512{return empty,agentchat.ErrInvalidSession}
 workspace:=s.agentOwnerWorkspace()
 workspaceID,err:=agentrun.WorkspaceID(workspace)
 if err!=nil{return empty,err}
 identity,err:=patchtool.ReadProjectIdentity(workspace)
 if err!=nil{return empty,err}
 dir,err:=s.agentRunJournalDir(owner)
 if err!=nil{return empty,err}
 record,err:=agentrun.ReadCompletedRecord(dir,runID)
 if err!=nil{return empty,err}
 spec:=record.Spec
 if spec.ConversationID!=conversation||spec.OwnerWorkspaceID!=workspaceID{return empty,errors.New("run is outside the current conversation or workspace")}
 snapshots,err:=s.agentContextSnapshotStore(owner)
 if err!=nil{return empty,err}
 snapshot,err:=snapshots.Get(spec.ContextSnapshotID,workspaceID,conversation,identity.ID)
 if err!=nil{return empty,err}
 if err=verifyAgentRunSnapshotBinding(spec,snapshot);err!=nil{return empty,err}
 proof,err:=verifyRunCollectionProof(record,snapshot,identity.ID,callID,note,requiredPath)
 if err!=nil{return empty,err}
 proof.Ref=ref
 return verifiedRunCollectionEvidence{owner:owner,workspace:workspace,project:identity.ID,conversation:conversation,proof:proof},nil
}

func verifyRunCollectionProof(record agentrun.Record,snapshot agentrun.ContextSnapshot,projectID,callID,note,requiredPath string)(agentchat.TaskPlanEvidence,error){
 var empty agentchat.TaskPlanEvidence
 spec:=record.Spec
 if snapshot.ID!=spec.ContextSnapshotID||snapshot.ProjectID!=projectID||
  snapshot.ConversationID!=spec.ConversationID||snapshot.OwnerWorkspaceID!=spec.OwnerWorkspaceID{
  return empty,errors.New("Collection snapshot scope mismatch")
 }
 if err:=verifyAgentRunSnapshotBinding(spec,snapshot);err!=nil{return empty,err}
 pair,err:=runCollectionEvidencePair(record.History,spec,callID)
 if err!=nil{return empty,err}
 pinned,err:=runCollectionEvidencePair(snapshot.History,spec,callID)
 if err!=nil||!reflect.DeepEqual(pair,pinned){return empty,errors.New("Collection receipt differs from its immutable run snapshot")}
 session:=agentchat.Session{ProjectID:projectID,History:pair}
 if requiredPath!=""{
  return agentchat.VerifyTaskAutoCheck(session,&agentchat.TaskAutoCheck{Kind:"full_file_read",CallID:callID,Path:requiredPath})
 }
 return agentchat.VerifyTaskPlanEvidence(session,"collection",callID,note)
}
