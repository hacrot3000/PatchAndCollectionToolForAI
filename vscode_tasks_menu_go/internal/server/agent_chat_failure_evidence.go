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
 MutationState string `json:"mutation_state"`
 Integrity string `json:"integrity"`
 Availability string `json:"availability"`
 Predecessor string `json:"predecessor_disposition"`
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
 return call.Name=="taskdeck_submit_patch" && call.AgentRunID=="" &&
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
 for i,part:=range parts {
  if part==""||part=="."||part==".." {return nil,errors.New("unsafe evidence path")}
  current=filepath.Join(current,part)
  info,err:=os.Lstat(current)
  if err!=nil{return nil,err}
  if info.Mode()&os.ModeSymlink!=0 || (i<len(parts)-1&&!info.IsDir()) || (i==len(parts)-1&&!info.Mode().IsRegular()){
   return nil,errors.New("unsafe evidence file")
  }
 }
 pinned,err:=openProjectPinnedFile(root,current)
 if err!=nil{return nil,err}
 defer pinned.close()
 data,_,err:=pinned.readCurrent(limit)
 return data,err
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
 var matched agentFailureHistoryRow
 for _,entry:=range entries {
  if !strings.HasSuffix(entry.Name(),"_"+call.RunID+".json"){continue}
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
func (s *Server) agentResolveFailedPatchEvidence(x agentchat.Session, call agentchat.ToolCallRecord) agentFailureEvidence {
 out:=agentFailureReceipt(call)
 if s==nil||x.Owner!="local"||s.Config.SharedServerEnabled||x.Workspace!=s.agentOwnerWorkspace()||
  !agentFailureCallCandidate(call){
  out.Integrity="unavailable";out.Availability="scope_or_receipt_invalid";return out
 }
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace)
 if err!=nil||identity.ID!=x.ProjectID||s.agentVerifyFinalPackage(call)!=nil {
  out.Availability="project_or_package_unverified";return out
 }
 row,patch,err:=s.agentFailureHistory(call,identity)
 if err!=nil {
  if !errors.Is(err,os.ErrNotExist){out.Availability="native_history_integrity_or_limit_refused"}
  return out
 }
 // A known failure receipt can never be contradicted by a different result.
 // Older destructive size fallback used INCOMPLETE without a native run ID;
 // History may recover the actual status without changing that old receipt.
 if out.NativeStatus!="UNKNOWN"&&out.NativeStatus!=row.Status&&
  !strings.Contains(call.Result,"result exceeded data limit"){
  out.Availability="native_history_receipt_mismatch";return out
 }
 out.Integrity="native_history_sha_bound"
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
