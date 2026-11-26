package server

import (
 "encoding/json"
 "sort"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentrun"
)

const agentDebugUnifiedRunLimit=8
const agentDebugUnifiedRetryLimit=16

type agentDebugUnifiedRun struct{
 RunID string `json:"run_id"`
 Status agentrun.Status `json:"status"`
 ProviderID string `json:"provider_id"`
 ModelID string `json:"model_id"`
 UpdatedAt time.Time `json:"updated_at"`
 Attempt int `json:"run_attempt"`
 Progress map[string]any `json:"collection_progress,omitempty"`
 Retries []map[string]any `json:"retry_events,omitempty"`
 RetryEventsOmitted int `json:"retry_events_omitted,omitempty"`
 ProgressUnavailable bool `json:"progress_unavailable,omitempty"`
}

// Read only existing owner-scoped journal records, never create a manager or
// schedule work. Old snapshots are allowed to be unavailable (deleted worktree,
// migration, corrupt metadata); no inferred coverage or progress is exported.
func(s *Server) agentDebugUnifiedRuns(owner string,sessions []agentchat.Session,after time.Time)(map[string][]agentDebugUnifiedRun,error){
 result:=map[string][]agentDebugUnifiedRun{}
 records,err:=s.agentRunRecords(owner)
 if err!=nil{return result,err}
 byID:=map[string]agentchat.Session{}
 for _,x:=range sessions{if x.Owner==owner{byID[x.ID]=x}}
 sort.SliceStable(records,func(i,j int)bool{return records[i].UpdatedAt.After(records[j].UpdatedAt)})
 snapshots,err:=s.agentContextSnapshotStore(owner)
 if err!=nil&&len(records)>0{return result,err}
 for _,r:=range records{
  spec:=r.Spec
  x,ok:=byID[spec.ConversationID]
  if !ok||spec.Validate()!=nil||(!after.IsZero()&&!r.UpdatedAt.After(after)){continue}
  rows:=result[x.ID]
  if len(rows)>=agentDebugUnifiedRunLimit{continue}
  row:=agentDebugUnifiedRun{RunID:spec.RunID,Status:r.Status,ProviderID:spec.ProviderID,ModelID:spec.ModelID,UpdatedAt:r.UpdatedAt,Attempt:r.Attempt}
  for _,e:=range r.Events{
   if e.Type!=agentchat.RunEventRetry||e.RunID!=spec.RunID||e.ProviderID!=spec.ProviderID||(!after.IsZero()&&!e.At.After(after)){continue}
   v:=agentSafeUnifiedRetry(e)
   if v==nil{continue}
   if len(row.Retries)==agentDebugUnifiedRetryLimit{
    row.Retries=row.Retries[1:]
    row.RetryEventsOmitted++
   }
   row.Retries=append(row.Retries,v)
  }
  // A cumulative snapshot created before Clear is not a new debug event.
  // Suppress its totals for pre-clear runs, keeping fresh retry metadata.
  if after.IsZero()||r.CreatedAt.After(after){
   snap,e:=snapshots.Get(spec.ContextSnapshotID,spec.OwnerWorkspaceID,x.ID,x.ProjectID)
   if e==nil&&verifyAgentRunSnapshotBinding(spec,snap)==nil&&len(snap.History)<=len(r.History){
    prefix:=true
    for i,it:=range snap.History{if it!=r.History[i]{prefix=false;break}}
    if prefix{
     if g,ok:=readRunCollectionProgress(owner,spec,AgentRunInputs{ProjectID:x.ProjectID,Snapshot:snap});ok&&g.LastHistorySize>0{
      c:=g.Counters
      row.Progress=map[string]any{"requested":c.Requested,"observed":c.Observed,"native_reads":c.NativeReads,"verified_new":c.NewEvidence,"verified_duplicate":c.DuplicateEvidence,"unknown":c.UnknownEvidence,"errors":c.Errors,"duplicate_streak":c.Progress.DuplicateStreak,"reads_since_progress":c.Progress.ReadsSinceProgress,"decision":g.Decision.Kind,"reason":g.Decision.Reason,"history_checkpoint":g.LastHistorySize}
     }
    }
   }
   row.ProgressUnavailable=row.Progress==nil
  }
  result[x.ID]=append(rows,row)
 }
 return result,nil
}

// Do not export the raw RunEvent payload (which can include model output or
// provider text). Only known retry enums, bounded numbers and validated hashes.
func agentSafeUnifiedRetry(e agentrun.Event)map[string]any{
 if len(e.Payload)>16<<10{return nil}
 var obj map[string]any
 if json.Unmarshal(e.Payload,&obj)!=nil{return nil}
 row:=map[string]any{"sequence":e.Sequence,"at":e.At,"attempt_id":e.AttemptID}
 for _,k:=range []string{"attempt","next_attempt","canonical_bytes","wire_template_bytes","estimated_input_tokens","output_budget","same_digest_failures","recovery_generation","instruction_bytes","input_bytes","tools_bytes","other_bytes"}{
  if v,ok:=obj[k].(float64);ok&&v>=0&&v<=1000000000&&v==float64(int64(v)){row[k]=int64(v)}
 }
 for _,k:=range []string{"payload_sha256","next_payload_sha256"}{
  if v,ok:=obj[k].(string);ok&&len(v)==64&&strings.Trim(v,"0123456789abcdef")==""{row[k]=v}
 }
 if v,ok:=obj["outcome"].(string);ok{
  switch agentchat.ProviderAttemptOutcome(v){
  case agentchat.AttemptResponse,agentchat.AttemptCancelled,agentchat.AttemptConnectTimeout,agentchat.AttemptHeaderTimeout,agentchat.AttemptRequestTimeout,agentchat.AttemptHTTP499,agentchat.AttemptContextRejected,agentchat.AttemptIncomplete,agentchat.AttemptHTTPRejected,agentchat.AttemptUnknown:row["outcome"]=v
  }
 }
 if v,ok:=obj["retry_mode"].(string);ok{
  switch agentchat.RetryMode(v){case agentchat.RetryOff,agentchat.RetryLimited,agentchat.RetryUnlimited:row["retry_mode"]=v}
 }
 return row
}
