package server

import (
 "archive/zip"
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "os"
 "path"
 "path/filepath"
 "reflect"
 "regexp"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

const agentFailureHistoryLimit = 8 << 20
const agentFailureZIPLimit = 32 << 20
const agentFailureArchivePartsLimit = 64

var agentFailureRunID = regexp.MustCompile("^[A-Za-z0-9_.-]{1,64}$")

// The resolver emits a bounded projection, not a replacement tool receipt.
// Missing or unbound artifacts never authorize retry or imply rollback.
type agentFailureEvidence struct {
 CallID string `json:"call_id"`
 RunID string `json:"run_id"`
 PackageName string `json:"package_name"`
 PackageSHA256 string `json:"package_sha256"`
 ReceiptSHA256 string `json:"receipt_sha256"`
 NativeStatus string `json:"native_status"`
 RC *int `json:"rc,omitempty"`
 Diagnosis string `json:"diagnosis_kind,omitempty"`
 Reason string `json:"failure_reason,omitempty"`
 Stage string `json:"stage,omitempty"`
 Excerpt string `json:"diagnostic_excerpt,omitempty"`
 MutationState string `json:"mutation_state"`
 Integrity string `json:"integrity"`
 Availability string `json:"availability"`
 Predecessor string `json:"predecessor_disposition"`
 PredecessorSource string `json:"predecessor_source,omitempty"`
 ProposalKind string `json:"proposal_kind,omitempty"`
 Truncated bool `json:"truncated"`
}

type agentFailureDiagnosis struct {
 Kind string `json:"kind"`
 Message string `json:"message"`
}
type agentFailurePatchResult struct {
 SHA256 string `json:"patch_sha256"`
 Stage string `json:"stage"`
 Diagnosis agentFailureDiagnosis `json:"diagnosis"`
 Partial struct {
  Detected *bool `json:"detected"`
 } `json:"partial_modification"`
}
type agentFailureHistoryRow struct {
 Name string `json:"name"`
 Kind string `json:"kind"`
 Status string `json:"status"`
 RC *int `json:"rc"`
 Reason string `json:"failure_reason"`
 DiagnosisKind string `json:"diagnosis_kind"`
 Diagnosis agentFailureDiagnosis `json:"diagnosis"`
 PatchResult json.RawMessage `json:"patch_result"`
 Handoff string `json:"fail_handoff"`
 OutputTail string `json:"output_tail"`
}
type agentFailureHistoryReport struct {
 Format string `json:"format"`
 Version int `json:"format_version"`
 RunID string `json:"run_id"`
 FinishedAt string `json:"finished_at"`
 Project patchtool.ProjectIdentity `json:"project_identity"`
 Selected []string `json:"selected"`
 ExecutionOrder []string `json:"execution_order"`
 Results []agentFailureHistoryRow `json:"results"`
}

func agentFailureStatus(status string) bool {
 return status == "FAIL" || status == "PREFLIGHT_FAIL" || status == "INCOMPLETE"
}
func agentFailureCallCandidate(call agentchat.ToolCallRecord) bool {
 return call.Name=="taskdeck_submit_patch" && call.AgentRunID=="" && !agentDuplicateSkipResult(call.Result) &&
  call.Status=="error" && call.CallID!="" && agentFailureRunID.MatchString(call.RunID) &&
  len(call.PackageSHA256)==64 && call.PackageSize>0 && call.PackageSize<=2<<20
}
func agentFailureReceipt(call agentchat.ToolCallRecord) agentFailureEvidence {
 hash:=sha256.Sum256([]byte(call.Result))
 out:=agentFailureEvidence{CallID:call.CallID,RunID:call.RunID,PackageName:filepath.Base(call.QueuePath),
  PackageSHA256:call.PackageSHA256,ReceiptSHA256:hex.EncodeToString(hash[:]),NativeStatus:"UNKNOWN",
  MutationState:"UNKNOWN",Integrity:"receipt_only",Availability:"native_history_unavailable",Predecessor:"unknown"}
 var receipt struct {
  Status string `json:"status"`
  RunID string `json:"run_id"`
  Name string `json:"name"`
  RC *int `json:"rc"`
  Reason string `json:"failure_reason"`
  Diagnosis string `json:"diagnosis_kind"`
 }
 if json.Unmarshal([]byte(call.Result),&receipt)==nil && agentFailureStatus(receipt.Status) &&
  (receipt.RunID=="" || receipt.RunID==call.RunID) && (receipt.Name=="" || receipt.Name==out.PackageName) {
  out.NativeStatus=receipt.Status
  out.RC=receipt.RC
  out.Reason=scrubAIContext(receipt.Reason,400)
  out.Diagnosis=scrubAIContext(receipt.Diagnosis,128)
 }
 return out
}

// Only fixed native directories and a history-derived relative artifact path
// reach this helper. Reject every symlink component; use the existing pinned
// file reader to avoid following a swapped leaf or escaping the workspace.
func agentFailureReadFile(root, rel string, limit int64) ([]byte,error) {
 if rel=="" || strings.ContainsAny(rel,"\\\x00:") || path.IsAbs(rel) || path.Clean(rel)!=rel || strings.HasPrefix(rel,"../") || rel==".." {
  return nil,errors.New("unsafe evidence path")
 }
 parts:=strings.Split(rel,"/")
 current:=root
 var expected os.FileInfo
 for i,part:=range parts {
  if part==""||part=="."||part==".." {return nil,errors.New("unsafe evidence path")}
  current=filepath.Join(current,part)
  info,err:=os.Lstat(current)
  if err!=nil{return nil,err}
  if info.Mode()&os.ModeSymlink!=0 || (i<len(parts)-1&&!info.IsDir()) || (i==len(parts)-1&&!info.Mode().IsRegular()){
   return nil,errors.New("unsafe evidence file")
  }
  if i==len(parts)-1 {expected=info}
 }
 pinned,err:=openProjectPinnedFile(root,current)
 if err!=nil{return nil,err}
 defer pinned.close()
 data,actual,err:=pinned.readCurrent(limit)
 if err!=nil{return nil,err}
 if expected==nil||actual==nil||!os.SameFile(expected,actual){return nil,errors.New("evidence file changed during read")}
 return data,nil
}

func agentFailureCount(list []string,name string) int {
 count:=0
 for _,entry:=range list {if entry==name {count++}}
 return count
}

// Native History is authoritative for the run-to-artifact reference. Match
// exact project, run, selection, final status and immutable package SHA.
func (s *Server) agentFailureHistory(call agentchat.ToolCallRecord, identity patchtool.ProjectIdentity) (agentFailureHistoryRow,agentFailurePatchResult,error) {
 var empty agentFailureHistoryRow
 var patch agentFailurePatchResult
 dir:=s.Workspace
 for _,part:=range []string{"artifacts","patch_tool","history"} {
  dir=filepath.Join(dir,part)
  info,err:=os.Lstat(dir)
  if err!=nil{return empty,patch,err}
  if !info.IsDir()||info.Mode()&os.ModeSymlink!=0{return empty,patch,errors.New("unsafe native History")}
 }
 fd,err:=os.Open(dir)
 if err!=nil{return empty,patch,err}
 defer fd.Close()
 entries,err:=fd.ReadDir(1001)
 if err!=nil&&!errors.Is(err,io.EOF){return empty,patch,err}
 if len(entries)>1000{return empty,patch,errors.New("native History inspection bound exceeded")}
 name:=filepath.Base(call.QueuePath)
 found:=false
 reportSeen:=false
 var matched agentFailureHistoryRow
 for _,entry:=range entries {
  if !strings.HasSuffix(entry.Name(),"_"+call.RunID+".json"){continue}
  if reportSeen{return empty,patch,errors.New("ambiguous native History reports")}
  reportSeen=true
  raw,err:=agentFailureReadFile(s.Workspace,"artifacts/patch_tool/history/"+entry.Name(),agentFailureHistoryLimit)
  if err!=nil{return empty,patch,err}
  var report agentFailureHistoryReport
  if json.Unmarshal(raw,&report)!=nil || report.Format!="python-patch-tool-last-run" || report.Version!=2 ||
   report.RunID!=call.RunID || report.Project!=identity || len(report.Results)>256 ||
   len(report.Selected)>256 || len(report.ExecutionOrder)>256 {
   return empty,patch,errors.New("native History identity mismatch")
  }
  finished,err:=time.Parse(time.RFC3339Nano,report.FinishedAt)
  if err!=nil || (call.NativeStartedAt!=nil&&finished.Before(call.NativeStartedAt.Add(-5*time.Second))) {
   return empty,patch,errors.New("native History timestamp mismatch")
  }
  if agentFailureCount(report.Selected,name)!=1{return empty,patch,errors.New("native History selection mismatch")}
  executed:=agentFailureCount(report.ExecutionOrder,name)
  for _,row:=range report.Results {
   if row.Name!=name||row.Kind!="PATCH"{continue}
   if found || !agentFailureStatus(row.Status) || executed>1 ||
    (executed==0&&(row.Status!="PREFLIGHT_FAIL"||row.RC==nil||*row.RC!=2)) ||
    (call.SelectedIndex<1&&row.Status!="PREFLIGHT_FAIL"){
    return empty,patch,errors.New("ambiguous native failure")
   }
   if json.Unmarshal(row.PatchResult,&patch)!=nil || patch.SHA256!=call.PackageSHA256 {
    return empty,patch,errors.New("native failure package SHA mismatch")
   }
   found=true;matched=row
  }
 }
 if !found{return empty,patch,os.ErrNotExist}
 return matched,patch,nil
}

// Read exactly FAIL_SUMMARY.json, never source, executable package bytes or
// arbitrary archive entries. The native History row must point to this ZIP;
// its structured result must match the summary, not merely its filename.
func agentFailureHandoff(root string,row agentFailureHistoryRow,identity patchtool.ProjectIdentity) error {
 rel:=row.Handoff
 if rel=="" {return os.ErrNotExist}
 dir:=path.Dir(rel)
 if (dir!="artifacts/patch_tool/fail_handoffs"&&dir!="artifacts/patch_tool") ||
  !strings.HasPrefix(path.Base(rel),"FAIL_HANDOFF_")||!strings.HasSuffix(rel,".zip"){
  return errors.New("invalid handoff reference")
 }
 raw,err:=agentFailureReadFile(root,rel,agentFailureZIPLimit)
 if err!=nil{return err}
 reader,err:=zip.NewReader(bytes.NewReader(raw),int64(len(raw)))
 if err!=nil||len(reader.File)>4096{return errors.New("invalid or oversized failure archive")}
 var summary *zip.File
 seen:=map[string]bool{}
 var total uint64
 for _,member:=range reader.File {
  n:=strings.TrimSuffix(member.Name,"/")
  if n==""||path.Clean(n)!=n||path.IsAbs(n)||n==".."||strings.HasPrefix(n,"../")||strings.ContainsAny(n,"\\\x00:")||
   seen[n]||member.Mode()&os.ModeSymlink!=0||(!member.Mode().IsRegular()&&!member.FileInfo().IsDir()){
   return errors.New("unsafe failure archive member")
  }
  seen[n]=true
  if member.UncompressedSize64>256<<20-total{return errors.New("failure archive expansion bound exceeded")}
  total+=member.UncompressedSize64
  if n=="FAIL_SUMMARY.json" {summary=member}
 }
 if summary==nil||summary.UncompressedSize64>2<<20{return errors.New("missing or oversized failure summary")}
 rd,err:=summary.Open()
 if err!=nil{return err}
 defer rd.Close()
 data,err:=io.ReadAll(io.LimitReader(rd,(2<<20)+1))
 if err!=nil||len(data)>2<<20{return errors.New("unreadable failure summary")}
 var meta struct {
  Format string `json:"format"`
  Version int `json:"format_version"`
  Project patchtool.ProjectIdentity `json:"project_identity"`
  Patch string `json:"patch"`
  RC *int `json:"rc"`
  PatchResult json.RawMessage `json:"patch_result"`
 }
 if json.Unmarshal(data,&meta)!=nil||meta.Format!="python-patch-tool-fail-handoff"||meta.Version!=2||
  meta.Project!=identity||meta.Patch!=row.Name||meta.RC==nil||row.RC==nil||*meta.RC!=*row.RC{
  return errors.New("failure summary identity mismatch")
 }
 var a,b any
 if json.Unmarshal(meta.PatchResult,&a)!=nil||json.Unmarshal(row.PatchResult,&b)!=nil||!reflect.DeepEqual(a,b){
  return errors.New("failure summary result mismatch")
 }
 return nil
}

// Only a call selected from the current authenticated session/verified
// archive may be passed here. This method is read-only.
func (s *Server) agentResolveFailedPatchEvidence(store *agentchat.Store,x agentchat.Session, call agentchat.ToolCallRecord) agentFailureEvidence {
 unavailable:=agentFailureEvidence{Integrity:"unavailable",Availability:"scope_or_receipt_invalid",MutationState:"UNKNOWN",Predecessor:"unknown"}
 if s==nil||store==nil||x.Owner!="local"||s.Config.SharedServerEnabled||x.Workspace!=s.agentOwnerWorkspace()||
  !agentFailureCallCandidate(call){return unavailable}
 canonical,err:=store.Get(x.ID,x.Owner)
 if err!=nil||canonical.Workspace!=x.Workspace||canonical.ProjectID!=x.ProjectID{return unavailable}
 bound,ok:=agentFindFailureLedgerCall(store,canonical,call)
 if !ok{return unavailable}
 // Use only the canonical ledger's call and durable retirement metadata.
 call=bound
 out:=agentFailureReceipt(call)
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace)
 if err!=nil||identity.ID!=x.ProjectID||!s.agentFailureVerifyPackage(call) {
  out.Availability="project_or_package_unverified";return out
 }
 out.ProposalKind=s.agentFailurePackageKind(call,identity)
 row,patch,err:=s.agentFailureHistory(call,identity)
 if err!=nil {
  if !errors.Is(err,os.ErrNotExist){out.Availability="native_history_integrity_or_limit_refused";return out}
  // History is bounded and may legitimately age out. Independently retain
  // the confirmed canonical retirement + immutable archive observation;
  // do not reconstruct diagnosis, mutation state, or a native PASS.
  disposition,source:=s.agentFailurePredecessor(call)
  if disposition=="retired"&&(source=="durable_native_retirement_and_archive"||source=="native_registry_and_archive"){
   out.Predecessor=disposition;out.PredecessorSource=source
   out.Availability="verified_retirement_history_unavailable"
  }
  return out
 }
 // A known failure receipt can never be contradicted by a different result.
 // Older destructive size fallback used INCOMPLETE without a native run ID;
 // History may recover the actual status without changing that old receipt.
 if out.NativeStatus!="UNKNOWN"&&out.NativeStatus!=row.Status&&
  !agentFailureLegacySizeFallback(call.Result){
  out.Availability="native_history_receipt_mismatch";return out
 }
 out.Integrity="native_history_sha_bound"
 out.Predecessor,out.PredecessorSource=s.agentFailurePredecessor(call)
 out.Availability="native_history_verified_handoff_unavailable"
 out.NativeStatus=row.Status
 out.RC=row.RC
 reason:=row.Reason
 if reason==""{reason=row.Diagnosis.Message}
 if reason==""{reason=patch.Diagnosis.Message}
 if reason!=""{out.Reason=scrubAIContext(reason,600);out.Truncated=len(reason)>600}
 kind:=row.DiagnosisKind
 if kind==""{kind=row.Diagnosis.Kind}
 if kind==""{kind=patch.Diagnosis.Kind}
 if kind!=""{out.Diagnosis=scrubAIContext(kind,128)}
 out.Stage=scrubAIContext(patch.Stage,80)
 if row.OutputTail!="" {out.Excerpt=agentPatchLogExcerpt(row.OutputTail,700);out.Truncated=out.Truncated||len(row.OutputTail)>700}
 if patch.Partial.Detected!=nil {
  if *patch.Partial.Detected {out.MutationState="PARTIAL_MODIFICATION_DETECTED"} else {out.MutationState="NO_PARTIAL_MODIFICATION_REPORTED"}
 }
 if row.Status=="PREFLIGHT_FAIL"&&patch.Partial.Detected!=nil&&!*patch.Partial.Detected {out.MutationState="UNCHANGED"}
 if err:=agentFailureHandoff(s.Workspace,row,identity);err==nil {
  out.Integrity="native_history_and_handoff_sha_bound"
  out.Availability="verified_handoff_summary"
 } else if !errors.Is(err,os.ErrNotExist) {
  out.Availability="handoff_integrity_or_limit_refused"
 }
 return out
}

// Owner and project verification is performed by the Store on every segment.
// Only the active session and bounded recent immutable segments are inspected.
func agentFailureCandidates(store *agentchat.Store,x agentchat.Session) ([]agentchat.ToolCallRecord,bool) {
 calls:=append([]agentchat.ToolCallRecord(nil),x.ToolCalls...)
 truncated:=x.PatchArchiveParts>agentFailureArchivePartsLimit
 for part,n:=x.PatchArchiveParts,0;part>0&&n<agentFailureArchivePartsLimit;part,n=part-1,n+1 {
  entries,err:=store.ArchivedPatchHistory(x.ID,x.Owner,part)
  if err!=nil {truncated=true;break}
  for i:=len(entries)-1;i>=0;i--{calls=append(calls,entries[i].Call)}
 }
 return calls,truncated
}

func agentFailureLegacySizeFallback(result string) bool {
 var v struct {Status string `json:"status"`; Reason string `json:"reason"`; RunID string `json:"run_id"`}
 return json.Unmarshal([]byte(result),&v)==nil && v.Status=="INCOMPLETE" &&
  v.Reason=="result exceeded data limit" && v.RunID==""
}

// The current tool call wins. An explicit known run ID can recover an older
// failure; otherwise consider only the newest terminal Patch, not every
// unrelated historical failure. A newer success is not proof of repair, but
// it means older unrelated failures are no longer automatically injected.
func agentFailureAttempt(call agentchat.ToolCallRecord,a agentchat.ToolAttemptRecord) agentchat.ToolCallRecord {
 call.Status=a.Status;call.Result=a.Result;call.QueuePath=a.QueuePath
 call.PackageSHA256=a.PackageSHA256;call.PackageSize=a.PackageSize
 call.RunID=a.RunID;call.SelectedIndex=a.SelectedIndex
 call.NativeStartedAt=a.NativeStartedAt;call.PatchSessionID=a.PatchSessionID
 call.PromptID=a.PromptID;call.FailedQueueRetirement=a.FailedQueueRetirement
 call.Attempts=nil
 return call
}
func agentFailureSameReceipt(a,b agentchat.ToolCallRecord)bool {
 return a.CallID==b.CallID&&a.RunID==b.RunID&&a.QueuePath==b.QueuePath&&
  a.PackageSHA256==b.PackageSHA256&&a.PackageSize==b.PackageSize&&a.Result==b.Result&&
  a.Status==b.Status&&a.Name==b.Name&&a.AgentRunID==b.AgentRunID
}
func agentFindFailureLedgerCall(store *agentchat.Store,x agentchat.Session,want agentchat.ToolCallRecord)(agentchat.ToolCallRecord,bool){
 find:=func(calls []agentchat.ToolCallRecord)(agentchat.ToolCallRecord,int){
  var match agentchat.ToolCallRecord
  n:=0
  for _,call:=range calls {
   if agentFailureSameReceipt(call,want){match=call;n++}
   for _,a:=range call.Attempts {
    previous:=agentFailureAttempt(call,a)
    if agentFailureSameReceipt(previous,want){match=previous;n++}
   }
  }
  return match,n
 }
 if match,n:=find(x.ToolCalls);n!=0{return match,n==1}
 calls,_:=agentFailureCandidates(store,x)
 match,n:=find(calls[len(x.ToolCalls):])
 return match,n==1
}

func agentSelectFailureCall(calls []agentchat.ToolCallRecord,currentCall,text string) (agentchat.ToolCallRecord,bool) {
 if currentCall!="" {
  for _,call:=range calls {if call.CallID==currentCall&&agentFailureCallCandidate(call){return call,true}}
  return agentchat.ToolCallRecord{},false
 }
 for _,call:=range calls {
  if agentFailureCallCandidate(call)&&len(call.RunID)>=8&&strings.Contains(text,call.RunID){return call,true}
  for _,attempt:=range call.Attempts {
   previous:=agentFailureAttempt(call,attempt)
   if agentFailureCallCandidate(previous)&&len(previous.RunID)>=8&&strings.Contains(text,previous.RunID){return previous,true}
  }
 }
 if agentFailureRunIDInText(text){return agentchat.ToolCallRecord{},false}
 for _,call:=range calls {
  if call.Name!="taskdeck_submit_patch"||call.AgentRunID!=""{continue}
  if agentFailureCallCandidate(call){return call,true}
  if call.Status=="done"||call.Status=="error"||call.Status=="rejected"{break}
 }
 return agentchat.ToolCallRecord{},false
}

// Build a transient projection only. Do not change the canonical user
// message, tool receipt, feedback acknowledgement or prepared payload.
// The existing compactor accounts for this catalog entry in its token budget.
func (s *Server) agentFailureContextProjection(store *agentchat.Store,x agentchat.Session,currentCall,text string) agentchat.Session {
 if s==nil||store==nil||x.Owner!="local"||s.Config.SharedServerEnabled||x.Workspace!=s.agentOwnerWorkspace(){return x}
 // Collection/questionnaire feedback is not a new PATCH outcome. Preserve
 // the relevant prior failure instead of using a non-PATCH call ID as a
 // selector which can never match. Never replay its canonical tool output.
 if currentCall!=""{
  isPatch:=false
  for _,c:=range x.ToolCalls{if c.CallID==currentCall&&c.Name=="taskdeck_submit_patch"&&c.AgentRunID==""{isPatch=true;break}}
  if !isPatch{currentCall=""}
 }
 // A user may simply say "continue" after the model named the failed run.
 // Only use the last assistant message as a selector into this ledger.
 // The text grants no filesystem path or execution permission.
 lower:=strings.ToLower(strings.TrimSpace(text))
 if currentCall==""&&!agentFailureRunIDInText(text)&&(strings.HasPrefix(lower,"continue")||strings.HasPrefix(lower,"resume")||strings.HasPrefix(lower,"tiếp")||lower==""||x.Status=="failed"){
  for i:=len(x.History)-1;i>=0;i--{
   if x.History[i].Type=="assistant"{
    // A Nim gateway/Collection run ID in the last message is not a
    // PATCH selector. Only adopt a reference which actually binds to
    // this conversation's failure ledger; otherwise use the newest failure.
    runs:=agentFailureRunTextPattern.FindAllString(x.History[i].Content,8)
    if len(runs)>0{
     candidates,_:=agentFailureCandidates(store,x)
     for _,run:=range runs{
      if _,ok:=agentSelectFailureCall(candidates,"",run);ok{text+=" "+run;break}
     }
    }
    break
   }
  }
 }
 // A passing command-only regression is evidence for that invocation,
 // not a resolution of an earlier source-changing failure. Do not hide
 // its predecessor merely because the most recent Patch ran a script.
 passedCommand:=map[string]bool{}
 for _,c:=range x.ToolCalls{
  if c.Status!="done"||c.AgentRunID!=""||c.Name!="taskdeck_submit_patch"{continue}
  item,ok:=findAgentToolCall(x.History,c.CallID)
  if !ok{continue}
  args,err:=parseAgentPatchArguments(item.Arguments)
  if err==nil&&strings.TrimSpace(args.PatchManifest)!=""{passedCommand[c.CallID]=true}
 }
 if passedCommand[currentCall]{currentCall=""}
 active:=make([]agentchat.ToolCallRecord,0,len(x.ToolCalls))
 for i:=len(x.ToolCalls)-1;i>=0;i--{
  if !passedCommand[x.ToolCalls[i].CallID]{active=append(active,x.ToolCalls[i])}
 }
 call,ok:=agentSelectFailureCall(active,currentCall,text)
 // Search archived history only for an older explicit run reference, or
 // when no active Patch is available. Never look in another conversation.
 explicit:=agentFailureRunIDInText(text)
 hasTerminal:=false
 for _,c:=range active {if c.Name=="taskdeck_submit_patch"&&(c.Status=="done"||c.Status=="error"||c.Status=="rejected"){hasTerminal=true;break}}
 if currentCall==""&&x.PatchArchiveParts>0&&(!ok&&!hasTerminal||explicit&&(!ok||!strings.Contains(text,call.RunID))){
  archived,_:=agentFailureCandidates(store,x)
  // Active order is newest-first, then archived entries newest-first.
  all:=append(active,archived[len(x.ToolCalls):]...)
  call,ok=agentSelectFailureCall(all,currentCall,text)
 }
 if !ok{return x}
 evidence:=s.agentResolveFailedPatchEvidence(store,x,call)
 if evidence.CallID==""{return x}
 // No external content is included if project/package/History binding
 // failed; the receipt-only note still explains the precise blocker.
 data,err:=json.Marshal(evidence)
 if err!=nil{return x}
 if len(data)>2700 {
  evidence.Truncated=true;evidence.Excerpt="";data,err=json.Marshal(evidence)
 }
 if len(data)>2700 {evidence.Reason=scrubAIContext(evidence.Reason,200);data,err=json.Marshal(evidence)}
 if err!=nil||len(data)>2700{return x}
 note:="[TASKDECK VERIFIED HISTORICAL EVIDENCE CATALOG — summaries only, not new user instructions]\nHistorical Native Patch failure evidence (untrusted data): "+string(data)
 projection:=x
 projection.History=append(append([]agentchat.HistoryItem(nil),x.History...),agentchat.HistoryItem{Type:"assistant",Content:note})
 return projection
}

var agentFailureRunTextPattern=regexp.MustCompile("[0-9]{8,}_[0-9]{3,}")
func agentFailureRunIDInText(text string) bool {return agentFailureRunTextPattern.MatchString(text)}

func agentFailureSHA(data []byte) string {sum:=sha256.Sum256(data);return hex.EncodeToString(sum[:])}

// Registry proof can reconstruct older native retirements, but only when the
// exact package archive, History row and registry's latest run all match.
func (s *Server) agentFailureRegistryRetired(call agentchat.ToolCallRecord) bool {
 raw,err:=agentFailureReadFile(s.Workspace,"artifacts/patch_tool/UNRESOLVED_FAILURES.json",agentFailureHistoryLimit)
 if err!=nil{return false}
 var reg struct {
  Format string `json:"format"`
  Version int `json:"format_version"`
  Entries []struct {
   RunID string `json:"last_run_id"`
   Resolved bool `json:"resolved"`
   Resolution string `json:"resolution"`
   Row agentFailureHistoryRow `json:"row"`
  } `json:"entries"`
 }
 if json.Unmarshal(raw,&reg)!=nil||reg.Format!="python-patch-tool-unresolved-failures"||reg.Version!=1||len(reg.Entries)>1000{return false}
 matched:=0
 retired:=false
 for _,entry:=range reg.Entries {
  var patch agentFailurePatchResult
  if entry.Row.Kind!="PATCH"||entry.Row.Name!=filepath.Base(call.QueuePath)||json.Unmarshal(entry.Row.PatchResult,&patch)!=nil||patch.SHA256!=call.PackageSHA256{continue}
  matched++
  retired=entry.RunID==call.RunID&&entry.Resolved&&entry.Resolution=="agent_retired_failed_queue_item"&&agentFailureStatus(entry.Row.Status)
 }
 return matched==1&&retired
}

// Current queue state is separate from a past retirement receipt. Absence
// alone does not prove native predecessor resolution or source rollback.
func (s *Server) agentFailurePredecessor(call agentchat.ToolCallRecord) (string,string) {
 name:=filepath.Base(call.QueuePath)
 if call.QueuePath!="patchs/"+name{return "unknown","invalid_queue_receipt"}
 queue,err:=os.Lstat(filepath.Join(s.Workspace,"patchs"))
 if err!=nil||!queue.IsDir()||queue.Mode()&os.ModeSymlink!=0{return "unknown","unsafe_or_missing_queue_directory"}
 data,err:=agentFailureReadFile(s.Workspace,call.QueuePath,2<<20)
 if err==nil {
  if agentFailureSHA(data)!=call.PackageSHA256{return "retained","queue_name_reused_or_changed"}
  if call.FailedQueueRetirement!=nil&&call.FailedQueueRetirement.Status=="retirement_refused"{return "retirement_refused","exact_package_still_queued"}
  return "retained","exact_package_still_queued"
 }
 if !errors.Is(err,os.ErrNotExist){return "unknown","queue_integrity_unverified"}
 archive:="patchs/ignore/agent-failed-"+call.PackageSHA256+".zip"
 data,err=agentFailureReadFile(s.Workspace,archive,2<<20)
 if err==nil&&int64(len(data))==call.PackageSize&&agentFailureSHA(data)==call.PackageSHA256 {
  if receipt:=call.FailedQueueRetirement;receipt!=nil&&receipt.Status=="retired"&&receipt.RunID==call.RunID&&receipt.PackageSHA256==call.PackageSHA256&&receipt.Archive==archive {
   return "retired","durable_native_retirement_and_archive"
  }
  if s.agentFailureRegistryRetired(call){return "retired","native_registry_and_archive"}
  return "not_queued","archive_verified_registry_disposition_unconfirmed"
 }
 if err!=nil&&!errors.Is(err,os.ErrNotExist){return "unknown","retirement_archive_integrity_unverified"}
 return "not_queued","queue_absent_retirement_not_proven"
}


// Match agentVerifyFinalPackage's queue and archive receipt contract, but
// do not call patchUploadDir: its write-oriented helper may create patchs/.
func (s *Server) agentFailureVerifyPackage(call agentchat.ToolCallRecord) bool {
 name:=filepath.Base(call.QueuePath)
 digest,err:=hex.DecodeString(call.PackageSHA256)
 if err!=nil||len(digest)!=32||name=="."||name==""||call.QueuePath!="patchs/"+name||
  call.PackageSize<=0||call.PackageSize>2<<20{return false}
 for _,rel:=range []string{call.QueuePath,"patchs/ignore/agent-failed-"+call.PackageSHA256+".zip","patchs/patched/"+name}{
  data,err:=agentFailureReadFile(s.Workspace,rel,2<<20)
  if err==nil&&int64(len(data))==call.PackageSize&&agentFailureSHA(data)==call.PackageSHA256{return true}
 }
 return false
}

// Classify only the SHA-bound immutable archive, never a name or an
// assistant claim. Command-only means no PATCH payload, NOT no side effects.
func(s *Server)agentFailurePackageKind(call agentchat.ToolCallRecord,identity patchtool.ProjectIdentity)string{
 name:=filepath.Base(call.QueuePath)
 for _,rel:=range []string{call.QueuePath,"patchs/ignore/agent-failed-"+call.PackageSHA256+".zip","patchs/patched/"+name}{
  raw,err:=agentFailureReadFile(s.Workspace,rel,2<<20)
  if err!=nil||int64(len(raw))!=call.PackageSize||agentFailureSHA(raw)!=call.PackageSHA256{continue}
  zr,err:=zip.NewReader(bytes.NewReader(raw),int64(len(raw)))
  if err!=nil{return "unknown"}
  if len(zr.File)!=1{return "not_verified_command_only"}
  f:=zr.File[0]
  if f.Name!="PATCH_TOOL_MANIFEST.json"||!f.Mode().IsRegular()||f.UncompressedSize64>64<<10{return "not_verified_command_only"}
  rd,err:=f.Open();if err!=nil{return "unknown"}
  body,err:=io.ReadAll(io.LimitReader(rd,(64<<10)+1));rd.Close()
  if err!=nil||len(body)>64<<10{return "unknown"}
  m,err:=parseCommandOnlyManifest(string(body))
  if err==nil&&m.Project==identity{return "verified_command_only"}
  return "not_verified_command_only"
 }
 return "unknown"
}
