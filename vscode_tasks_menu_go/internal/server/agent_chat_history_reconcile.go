package server

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// The native runner moves successful packages from patchs/ to patchs/patched/.
// Dispatch MUST validate the queue copy, but completion may validate either
// exact location. A ZIP alone never proves that a patch passed.
func (s *Server) agentVerifyFinalPackage(record agentchat.ToolCallRecord) error {
 name := filepath.Base(record.QueuePath)
 if record.QueuePath != "patchs/"+name || name == "." || name == "" ||
    record.PackageSize <= 0 || record.PackageSize > 2<<20 || len(record.PackageSHA256) != 64 {
  return errors.New("invalid approved package receipt")
 }
 receipt := patchUploadResult{Name:name,QueuePath:record.QueuePath,Size:record.PackageSize,SHA256:record.PackageSHA256}
 if s.agentVerifyQueuedPackage(receipt) == nil { return nil }
 root := s.Workspace
 for _, dir := range []string{"patchs","patched"} {
  root = filepath.Join(root,dir)
  fi,err:=os.Lstat(root)
  if err!=nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink!=0 { return errors.New("missing or unsafe native Patch archive") }
 }
 path:=filepath.Join(root,name)
 fi,err:=os.Lstat(path)
 if err!=nil || !fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink!=0 || fi.Size()!=record.PackageSize {
  return errors.New("approved Patch archive missing or changed")
 }
 data,err:=os.ReadFile(path)
 if err!=nil || int64(len(data))!=record.PackageSize { return errors.New("could not read archived Patch") }
 digest:=sha256.Sum256(data)
 if hex.EncodeToString(digest[:])!=record.PackageSHA256 { return errors.New("archived Patch SHA-256 mismatch") }
 return nil
}

// Adopt a finished, project-bound native History record when the original
// terminal's in-memory ProtocolState has expired. Require exactly one matching
// selected/executed package AND the approved immutable ZIP bytes. This is a
// read-only recovery path; never start or repeat any Patch action.
func (s *Server) agentFinalResultFromHistory(call agentchat.ToolCallRecord) (string,string,string,bool) {
 return s.agentHistoryResultFromHistory(call,false)
}

// agentLatestResultFromHistory is only used for an explicit user choice.
func (s *Server) agentLatestResultFromHistory(call agentchat.ToolCallRecord) (string,string,string,bool) {
 return s.agentHistoryResultFromHistory(call,true)
}

func (s *Server) agentHistoryResultFromHistory(call agentchat.ToolCallRecord,latest bool) (string,string,string,bool) {
 if (call.Status!="running" && call.Status!="unknown" && call.Status!="dispatching") || s.agentVerifyFinalPackage(call)!=nil {
  return "","","",false
 }
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace)
 if err!=nil { return "","","",false }
 path:=s.Workspace
 for _,part:=range []string{"artifacts","patch_tool","history"} {
  path=filepath.Join(path,part)
  fi,err:=os.Lstat(path)
  if err!=nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink!=0 { return "","","",false }
 }
 entries,err:=os.ReadDir(path)
 if err!=nil || len(entries)>500 { return "","","",false }
 name:=filepath.Base(call.QueuePath)
 type row struct {
  Name string `json:"name"`
  Kind string `json:"kind"`
  Status string `json:"status"`
  RC *int `json:"rc"`
  FailureReason string `json:"failure_reason"`
  DiagnosisKind string `json:"diagnosis_kind"`
  Diagnosis struct {Kind string `json:"kind"`;Message string `json:"message"`} `json:"diagnosis"`
  OutputTail string `json:"output_tail"`
 }
 type match struct{status,result,run string; finished time.Time}
 matches:=make([]match,0,2)
 for _,entry:=range entries {
  if entry.IsDir() || entry.Type()&os.ModeSymlink!=0 || !strings.HasSuffix(entry.Name(),".json") { continue }
  fi,err:=entry.Info()
  if err!=nil || !fi.Mode().IsRegular() || fi.Size()==0 || fi.Size()>1<<20 { continue }
  raw,err:=os.ReadFile(filepath.Join(path,entry.Name()))
  if err!=nil { continue }
  var report struct {
   Format string `json:"format"`
   FormatVersion int `json:"format_version"`
   RunID string `json:"run_id"`
   FinishedAt string `json:"finished_at"`
   ProjectIdentity patchtool.ProjectIdentity `json:"project_identity"`
   Selected []string `json:"selected"`
   ExecutionOrder []string `json:"execution_order"`
   Results []row `json:"results"`
  }
  if json.Unmarshal(raw,&report)!=nil || report.Format!="python-patch-tool-last-run" ||
    report.FormatVersion!=2 || report.RunID=="" || len(report.RunID)>128 ||
    report.ProjectIdentity!=identity || len(report.Results)==0 || len(report.Results)>256 ||
    len(report.Selected)>256 || len(report.ExecutionOrder)>256 { continue }
  if call.RunID!="" && call.RunID!=report.RunID {continue}
  finished,stampErr:=time.Parse(time.RFC3339Nano,report.FinishedAt)
  if call.NativeStartedAt!=nil {
   if stampErr!=nil && call.RunID=="" {continue}
   if stampErr==nil && finished.Before(call.NativeStartedAt.Add(-5*time.Second)){continue}
  }
  selected,executed:=0,0
  for _,n:=range report.Selected { if n==name { selected++ } }
  for _,n:=range report.ExecutionOrder { if n==name { executed++ } }
  if selected!=1 || executed>1 {continue}
  count:=0
  var accepted match
  for _,item:=range report.Results {
   if item.Name!=name || strings.ToUpper(item.Kind)!="PATCH" { continue }
   status:=strings.ToUpper(item.Status)
   if executed==0 && status!="PREFLIGHT_FAIL" {continue}
   if call.SelectedIndex<1 && status!="PREFLIGHT_FAIL" {continue}
   if status!="PASS" && status!="FAIL" && status!="INCOMPLETE" && status!="PREFLIGHT_FAIL" { continue }
   if executed==0 && (item.RC==nil || *item.RC!=2) {continue}
   count++
   if count!=1{break}
   diag:=item.DiagnosisKind
   if diag=="" {diag=item.Diagnosis.Kind}
   reason:=item.FailureReason
   if reason=="" {reason=item.Diagnosis.Message}
   evidence:=map[string]any{
    "status":status,"name":name,"run_id":report.RunID,
    "source":"native_patch_history","item_index":call.SelectedIndex,
    "diagnosis_kind":scrubAIContext(diag,128),
    "failure_reason":scrubAIContext(reason,400),
    "stdout_tail":agentPatchLogExcerpt(item.OutputTail,1400),
     "test_verification":"Patch PASS / rc=0 does not establish that nested tests passed; inspect stdout_tail for exit codes and warnings.",
    "project_unchanged":status=="PREFLIGHT_FAIL",
    "automatic_retry":false,
   }
   if item.RC!=nil { evidence["rc"]=*item.RC }
   b,err:=json.Marshal(evidence)
   if err!=nil { return "","","",false }
   accepted=match{status:"error",result:string(b),run:report.RunID,finished:finished}
   if status=="PASS" {accepted.status="done"}
  }
  if count==1{matches=append(matches,accepted)}
 }
 if len(matches)==0{return "","","",false}
 if len(matches)==1{a:=matches[0];return a.status,a.result,a.run,true}
 if !latest{return "","","",false}
 // Missing/equal timestamps remain ambiguous: never infer chronology from
 // directory ordering or filename and never execute a package here.
 newest:=-1
 for i,a:=range matches {
  if a.finished.IsZero(){return "","","",false}
  if newest<0 || a.finished.After(matches[newest].finished){newest=i}
 }
 for i,a:=range matches {
  if i!=newest && a.finished.Equal(matches[newest].finished){return "","","",false}
 }
 a:=matches[newest]
 return a.status,a.result,a.run,true
}
