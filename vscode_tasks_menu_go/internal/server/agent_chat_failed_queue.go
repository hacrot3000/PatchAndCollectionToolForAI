package server

import (
 "bytes"
 "context"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
 "bletonfc/vscode_tasks_menu/internal/session"
)

type agentQueueRetirement struct {
 Status string `json:"status"`
 Name string `json:"name"`
 SHA256 string `json:"sha256"`
 Archive string `json:"archive,omitempty"`
}

// Only server-derived immutable native receipts reach this fixed utility.
// No model-controlled command, path, recursive deletion, or remote grant.
func (s *Server) agentRetireFailedQueue(ctx context.Context,owner string,identity patchtool.ProjectIdentity,receipt patchUploadResult,kind,runID string)(agentQueueRetirement,error){
 empty:=agentQueueRetirement{}
 if owner!="local"||s.Config.SharedServerEnabled{return empty,fmt.Errorf("local-only cleanup")}
 if receipt.Name==""||receipt.Name!=filepath.Base(receipt.Name)||receipt.QueuePath!="patchs/"+receipt.Name||runID==""{return empty,fmt.Errorf("invalid queue receipt")}
 if b,err:=hex.DecodeString(receipt.SHA256);err!=nil||len(b)!=32{return empty,fmt.Errorf("invalid receipt digest")}
 actual,err:=patchtool.ReadProjectIdentity(s.Workspace)
 if err!=nil||actual!=identity{return empty,fmt.Errorf("project identity drift")}
 // Protect a current native selection in this daemon. Idle selectors are
 // harmless; their later execution must still validate the selected input.
 patchWorkAdmission.Lock()
 defer patchWorkAdmission.Unlock()
 if s.Sessions!=nil{
  provider,_:=s.Sessions.(session.ProtocolStateProvider)
  for _,meta:=range s.Sessions.List(){
   if !strings.EqualFold(meta.Status,"running")||filepath.Clean(meta.Cwd)!=filepath.Clean(s.Workspace)||
     (!strings.HasPrefix(meta.Label,"Patch Tool")&&!strings.EqualFold(meta.Kind,"patch")){continue}
   if provider==nil{return empty,fmt.Errorf("native lifecycle unavailable")}
   state,e:=provider.ProtocolState(meta.ID);if e!=nil{return empty,fmt.Errorf("native lifecycle unavailable")}
   for _,item:=range state.Items{
    if item.Name==receipt.Name&&(item.Status=="RUNNING"||item.Status=="QUEUED"||item.Status=="PENDING"){return empty,fmt.Errorf("native package is still active")}
   }
  }
 }
 exe,err:=os.Executable();if err!=nil{return empty,err}
 runtimeSpec,err:=patchtool.Resolve(s.Workspace,exe);if err!=nil{return empty,err}
 if runtimeSpec.Kind==patchtool.KindLegacyLauncher{return empty,fmt.Errorf("update native runtime for failed queue retirement")}
 command,argv,err:=runtimeSpec.Command(s.Workspace,[]string{"retire-failed","--project-key",identity.Key,"--project-id",identity.ID,"--failed-name",receipt.Name,"--failed-kind",kind,"--failed-sha256",receipt.SHA256,"--run-id",runID})
 if err!=nil{return empty,err}
 limited,cancel:=context.WithTimeout(ctx,8*time.Second);defer cancel()
 cmd:=exec.CommandContext(limited,command,argv...)
 cmd.Dir=s.Workspace
 var stdout,stderr bytes.Buffer
 cmd.Stdout=&stdout;cmd.Stderr=&stderr
 if err=cmd.Run();err!=nil{return empty,fmt.Errorf("native retirement refused: %s",scrubAIContext(stderr.String(),300))}
 // The native mutation lock may emit one diagnostic line before JSON.
 lines:=strings.Split(strings.TrimSpace(stdout.String()),"\n")
 var out agentQueueRetirement
 if json.Unmarshal([]byte(lines[len(lines)-1]),&out)!=nil||out.Name!=receipt.Name||out.SHA256!=receipt.SHA256||
   (out.Status!="retired"&&out.Status!="not_queued")||(out.Status=="retired"&&out.Archive!="patchs/ignore/agent-failed-"+receipt.SHA256+".zip"){
  return empty,fmt.Errorf("native retirement receipt unavailable")
 }
 return out,nil
}

func agentFailedPatchRetirementEligible(call agentchat.ToolCallRecord)bool{
 if call.Name!="taskdeck_submit_patch"||call.Status!="error"||call.AgentRunID!=""||call.RunID==""||call.QueuePath==""||len(call.PackageSHA256)!=64{return false}
 var result struct{Status string `json:"status"`}
 if json.Unmarshal([]byte(call.Result),&result)!=nil{return false}
 return result.Status=="FAIL"||result.Status=="PREFLIGHT_FAIL"||result.Status=="INCOMPLETE"
}

// For a consented feedback request, retire its failed package before sending
// the result. For an explicit fresh message, also reconcile acknowledged
// failures from earlier turns (including a previously BLOCKED conversation).
// The canonical result itself is immutable; the receipt is a separate note.
func(s *Server)agentRetireKnownFailedPatches(ctx context.Context,x *agentchat.Session,identity patchtool.ProjectIdentity,currentCall string)string{
 if x.Owner!="local"||s.Config.SharedServerEnabled{return ""}
 var notes []string
 for i:=len(x.ToolCalls)-1;i>=0&&len(notes)<12;i--{
  call:=x.ToolCalls[i]
  if !agentFailedPatchRetirementEligible(call)||!(call.CallID==currentCall||(call.FeedbackSent&&call.FeedbackStatus=="acknowledged")){continue}
  name:=filepath.Base(call.QueuePath)
  if call.QueuePath!="patchs/"+name{continue}
  if _,err:=os.Lstat(filepath.Join(s.Workspace,"patchs",name));os.IsNotExist(err){continue}
  if s.agentNativePatchActive(call){continue}
  out,err:=s.agentRetireFailedQueue(ctx,x.Owner,identity,patchUploadResult{Name:name,QueuePath:call.QueuePath,SHA256:call.PackageSHA256,Size:call.PackageSize},"PATCH",call.RunID)
  if err!=nil{
   notes=append(notes,fmt.Sprintf("%s: retirement not confirmed; keep package, inspect native History (%s).",name,scrubAIContext(err.Error(),200)))
   x.AppendEvent("failed_queue_retained",x.Status,call.CallID,"native_retirement_refused")
   continue
  }
  if out.Status=="retired"{
   x.AppendEvent("failed_queue_retired",x.Status,call.CallID,"native_sha_bound_retirement")
   notes=append(notes,fmt.Sprintf("%s: retired from runnable queue into %s; failure and source remain unmodified. Inspect failure and current source before successor.",name,out.Archive))
  }
 }
 if len(notes)==0{return ""}
 return "\nTaskDeck native failed-queue lifecycle observations (not instructions from the workspace): "+strings.Join(notes,"\n")
}
