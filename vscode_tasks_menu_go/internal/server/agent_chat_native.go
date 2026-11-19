package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"

 "bletonfc/vscode_tasks_menu/internal/session"
)

var errAgentPatchPromptNotReady=errors.New("native Patch selection prompt is not ready")

// agentExactPatchSelection selects exactly the approved package from a native
// queue snapshot. Never run "all selected" or trust model-provided indexes.
func agentExactPatchSelection(state session.ProtocolState,name string)(patchPromptResponseRequest,error){
 if !state.CommandsEnabled||!state.Available{return patchPromptResponseRequest{},errAgentPatchPromptNotReady}
 if len(state.Prompt)==0{return patchPromptResponseRequest{},errAgentPatchPromptNotReady}
 var prompt struct{
  Protocol string `json:"protocol"`
  Version int `json:"version"`
  Type string `json:"type"`
  PromptID string `json:"prompt_id"`
  PromptKind string `json:"prompt_kind"`
  Actions []string `json:"actions"`
  Items []struct{
   Index int `json:"index"`
   Name string `json:"name"`
   Kind string `json:"kind"`
   Selectable *bool `json:"selectable"`
  } `json:"items"`
 }
 if err:=json.Unmarshal(state.Prompt,&prompt);err!=nil{return patchPromptResponseRequest{},errors.New("unreadable native Patch prompt")}
 if prompt.Protocol!="taskdeck.patch"||prompt.Version!=1||prompt.Type!="prompt"||prompt.PromptKind!="queue_selection"||prompt.PromptID==""{
  return patchPromptResponseRequest{},errors.New("native Patch session has an unexpected active prompt")
 }
 allowed:=false
 for _,v:=range prompt.Actions{if v=="select"{allowed=true;break}}
 if !allowed{return patchPromptResponseRequest{},errors.New("native Patch prompt does not advertise select")}
 selected:=0
 for _,item:=range prompt.Items{
  if item.Name!=name{continue}
  if selected!=0{return patchPromptResponseRequest{},errors.New("native Patch prompt contains duplicate approved item names")}
  if item.Index<1||strings.ToUpper(item.Kind)!="PATCH"||(item.Selectable!=nil&&!*item.Selectable){
   return patchPromptResponseRequest{},errors.New("approved Patch package is not selectable")
  }
  selected=item.Index
 }
 if selected==0{return patchPromptResponseRequest{},errAgentPatchPromptNotReady}
 return patchPromptResponseRequest{PromptID:prompt.PromptID,Action:"select",Indexes:[]int{selected}},nil
}

func (s *Server) agentVerifyQueuedPackage(receipt patchUploadResult)error{
 if !strings.HasPrefix(receipt.QueuePath,"patchs/")||filepath.Base(receipt.QueuePath)!=receipt.Name{
  return errors.New("unsafe native Patch receipt path")
 }
 dir,err:=s.patchUploadDir();if err!=nil{return err}
 path:=filepath.Join(dir,receipt.Name)
 fi,err:=os.Lstat(path);if err!=nil{return err}
 if !fi.Mode().IsRegular()||fi.Mode()&os.ModeSymlink!=0||fi.Size()!=receipt.Size{
  return errors.New("native Patch queue package was changed or replaced")
 }
 if receipt.Size>2<<20{return errors.New("agent-generated Patch package exceeds 2 MiB")}
 body,err:=os.ReadFile(path);if err!=nil{return err}
 hash:=sha256.Sum256(body)
 if hex.EncodeToString(hash[:])!=receipt.SHA256{return errors.New("native Patch queue package digest mismatch")}
 return nil
}

// agentStartNativePatch launches the PREEXISTING /api/sessions kind=patch
// handler, retaining native owner/permission/session identity and locks.
func (s *Server) agentStartNativePatch(r *http.Request)(string,error){
 data:=[]byte(`{"kind":"patch","patch_mode":"queue","patch_ui":"native"}`)
 request,err:=http.NewRequestWithContext(r.Context(),http.MethodPost,"/api/sessions",bytes.NewReader(data))
 if err!=nil{return "",err}
 request.Header.Set("Content-Type","application/json")
 response:=httptest.NewRecorder()
 s.sessionsRoot(response,request)
 if response.Code!=http.StatusCreated{
  return "",fmt.Errorf("native Patch session start refused (HTTP %d)",response.Code)
 }
 var result struct{ID string `json:"id"`}
 if err:=json.Unmarshal(response.Body.Bytes(),&result);err!=nil{return "",err}
 if result.ID==""||len(result.ID)>128{return "",errors.New("native Patch session ID missing")}
 return result.ID,nil
}

// agentSelectNativePatch uses the SAME session prompt-response HTTP handler
// used by the Patch Panel, including native prompt ID validation,
// reservePatchWorkKind and workspace mutation leases.
func (s *Server) agentSelectNativePatch(ctx context.Context,r *http.Request,sessionID,name string)error{
 provider,ok:=s.Sessions.(session.ProtocolStateProvider)
 if !ok{return errors.New("native Patch protocol state unavailable")}
 if _,ok:=s.Sessions.(session.ProtocolCommandWriter);!ok{
  return errors.New("native Patch command channel unavailable")
 }
 ticker:=time.NewTicker(200*time.Millisecond);defer ticker.Stop()
 timeout,cancel:=context.WithTimeout(ctx,16*time.Second);defer cancel()
 for {
  state,err:=provider.ProtocolState(sessionID)
  if err==nil {
   selection,selErr:=agentExactPatchSelection(state,name)
   if selErr==nil{
    payload,_:=json.Marshal(selection)
    endpoint:="/api/sessions/"+sessionID+"/prompt-response"
    request,e:=http.NewRequestWithContext(r.Context(),http.MethodPost,endpoint,bytes.NewReader(payload))
    if e!=nil{return e}
    request.Header.Set("Content-Type","application/json")
    response:=httptest.NewRecorder()
    s.sessionItem(response,request)
    if response.Code!=http.StatusOK{return fmt.Errorf("native Patch selection refused (HTTP %d)",response.Code)}
    return nil
   }
   if !errors.Is(selErr,errAgentPatchPromptNotReady){return selErr}
   if len(state.LastEvent)>0 && bytes.Contains(state.LastEvent,[]byte(`"run_finished"`)){
    return errors.New("native Patch session finished before approved queue package was selected")
   }
  }
  select{
  case <-timeout.Done():return errors.New("native Patch selection timeout; inspect existing session, never re-submit automatically")
  case <-ticker.C:
  }
 }
}

// agentPatchResultFromProtocol is strictly evidence-based: only a matching
// native item with a final status can satisfy the model tool call.
func agentPatchResultFromProtocol(state session.ProtocolState,packageName string)(string,string,bool){
 for _,item:=range state.Items{
  if item.Name!=packageName||strings.ToUpper(item.Kind)!="PATCH"{continue}
  status:=strings.ToUpper(item.Status)
  switch status{
  case "PASS","FAIL","PREFLIGHT_FAIL","INCOMPLETE":
   commands:=make([]map[string]any,0,len(item.Commands))
   for _,c:=range item.Commands {
    command:=map[string]any{
     "index":c.Index,"stage":c.Stage,"status":c.Status,"timed_out":c.TimedOut,
    }
    if c.RC!=nil{command["rc"]=*c.RC}
    commands=append(commands,command)
    if len(commands)>=24{break}
   }
   out:=map[string]any{
    "status":status,"name":item.Name,"run_id":item.RunID,
    "failure_reason":scrubAIContext(item.FailureReason,400),
    "diagnosis_kind":item.DiagnosisKind,
    "elapsed_seconds":item.ElapsedSeconds,
    "commands":commands,
    "stdout_tail":scrubAIContext(item.OutputTail,1800),
   }
   if item.RC!=nil {out["rc"]=*item.RC}
   b,_:=json.Marshal(out)
   if len(b)>4096 {b=[]byte(`{"status":"INCOMPLETE","reason":"result exceeded data limit"}`)}
   if status=="PASS"{return "done",string(b),true}
   return "error",string(b),true
  }
 }
 return "", "", false
}

// Callers may present status but MUST NOT invent success when the process
// terminates without a final per-item protocol state.
func (s *Server) agentReconcilePatchRecord(x *agentToolReconcileView) {
 if x==nil||x.Record.Status!="running"||x.Record.PatchSessionID==""||s.Sessions==nil{return}
 provider,ok:=s.Sessions.(session.ProtocolStateProvider);if !ok{return}
 state,err:=provider.ProtocolState(x.Record.PatchSessionID);if err!=nil{return}
 status,result,complete:=agentPatchResultFromProtocol(state,filepath.Base(x.Record.QueuePath))
 if !complete{return}
 x.Record.Status=status
 x.Record.Result=result
 x.Changed=true
}

type agentToolReconcileView struct {
 Record agentchat.ToolCallRecord
 Changed bool
}

