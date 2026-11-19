package server

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/http/httptest"
 "strings"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

type agentPatchArguments struct {
 Name string `json:"name"`
 PatchPython string `json:"patch_python"`
}

func parseAgentPatchArguments(raw string)(agentPatchArguments,error){
 if len(raw)==0||len(raw)>512<<10||!utf8.ValidString(raw){return agentPatchArguments{},errors.New("invalid or oversized patch tool arguments")}
 dec:=json.NewDecoder(strings.NewReader(raw));dec.DisallowUnknownFields()
 var args agentPatchArguments
 if err:=dec.Decode(&args);err!=nil{return agentPatchArguments{},errors.New("invalid patch function arguments")}
 var trailing any
 if err:=dec.Decode(&trailing);!errors.Is(err,io.EOF){return agentPatchArguments{},errors.New("patch function must contain exactly one JSON object")}
 args.Name=strings.TrimSpace(args.Name)
 if args.Name==""||len(args.Name)>80||len(args.PatchPython)==0||len(args.PatchPython)>480<<10||strings.ContainsRune(args.PatchPython,0){
  return agentPatchArguments{},errors.New("invalid patch name or Python body size")
 }
 return args,nil
}

func findAgentToolCall(history []agentchat.HistoryItem,callID string)(agentchat.HistoryItem,bool){
 for i:=len(history)-1;i>=0;i--{
  if history[i].Type=="function_call"&&history[i].CallID==callID&&history[i].Name=="taskdeck_submit_patch"{
   return history[i],true
  }
 }
 return agentchat.HistoryItem{},false
}

// agentQueueViaNativePaste calls the pre-existing validated Patch Paste queue
// entrypoint in-process. It DOES NOT execute Python or bypass native Patch
// schema / project-key checks. The caller must separately authorize patch.run.
func (s *Server) agentQueueViaNativePaste(r *http.Request,args agentPatchArguments)(patchUploadResult,error){
 input,err:=json.Marshal(patchPasteRequest{Kind:"patch-python",Name:args.Name,Text:args.PatchPython})
 if err!=nil{return patchUploadResult{},err}
 req,err:=http.NewRequestWithContext(r.Context(),http.MethodPost,"/api/patch/paste",bytes.NewReader(input))
 if err!=nil{return patchUploadResult{},err}
 req.Header.Set("Content-Type","application/json")
 // Preserve verified auth/owner and browser origin context for native lease
 // and mutation audit. The handler is reused, never reimplemented.
 req=req.WithContext(r.Context())
 rec:=httptest.NewRecorder()
 s.patchPaste(rec,req)
 if rec.Code!=http.StatusCreated{
  return patchUploadResult{},fmt.Errorf("native Patch queue refused package: HTTP %d (%s)",rec.Code,strings.TrimSpace(rec.Body.String()))
 }
 var upload patchUploadResult
 if err:=json.Unmarshal(rec.Body.Bytes(),&upload);err!=nil{return patchUploadResult{},err}
 if upload.Name==""||upload.QueuePath==""||len(upload.SHA256)!=64 {
  return patchUploadResult{},errors.New("native Patch queue returned incomplete receipt")
 }
 return upload,nil
}

func (s *Server) agentChatToolDecisionAPI(w http.ResponseWriter,r *http.Request,sessionID,callID string){
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",405);return}
 if len(callID)==0||len(callID)>256||strings.Contains(callID,"/"){http.Error(w,"invalid tool call id",400);return}
 if !s.requireSharedActionPermission(w,r,identity.PermissionPatchRun,"agent.patch_decision",sessionID){return}
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 var request struct{
  Action string `json:"action"`
  ExpectedRevision uint64 `json:"expected_revision"`
  ConfirmExecution bool `json:"confirm_execution"`
 }
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if dec.Decode(&request)!=nil{http.Error(w,"invalid agent tool decision",400);return}
 var trailing any
 if err:=dec.Decode(&trailing);!errors.Is(err,io.EOF){http.Error(w,"exactly one tool decision required",400);return}
 if request.Action!="approve"&&request.Action!="reject"{http.Error(w,"action must be approve or reject",400);return}
 if request.Action=="approve"&&!request.ConfirmExecution{http.Error(w,"explicit patch execution approval is required",403);return}
 owner:=aiPlanOwner(r)
 release,ok:=lockAgentTurn(s.Workspace,owner,sessionID)
 if !ok{http.Error(w,"Agent session is busy",409);return}
 defer release()
 store,err:=s.agentChatStore()
 if err!=nil{http.Error(w,"agent journal unavailable",500);return}
 x,err:=store.Get(sessionID,owner)
 if errors.Is(err,agentchat.ErrNotFound){http.Error(w,"Agent session not found",404);return}
 if err!=nil{http.Error(w,"agent journal unavailable",500);return}
 if x.Workspace!=s.Workspace||x.Revision!=request.ExpectedRevision||x.Status!="waiting_tool"{
  http.Error(w,"Agent session revision/status mismatch",409);return
 }
 identity,err:=patchtool.EnsureProjectIdentity(s.Workspace)
 if err!=nil||identity.ID!=x.ProjectID{http.Error(w,"project identity changed",409);return}
 idx:=-1
 for i,item:=range x.ToolCalls{if item.CallID==callID{idx=i;break}}
 if idx<0||x.ToolCalls[idx].Status!="pending"{http.Error(w,"pending approved tool call not found",409);return}
 call,found:=findAgentToolCall(x.History,callID)
 if !found{http.Error(w,"original model call missing",409);return}
 arguments,err:=parseAgentPatchArguments(call.Arguments)
 if err!=nil{http.Error(w,"invalid original tool arguments",400);return}
 // Native validation verifies BOTH identity headers against this workspace.
 // Do it before mutating the durable journal.
 if _,err=pastedPythonPackage(arguments.PatchPython,identity,patchPasteSlug(arguments.Name));err!=nil {
  http.Error(w,"Patch Tool contract validation failed: "+err.Error(),400);return
 }
 if request.Action=="reject"{
  x.ToolCalls[idx].Status="rejected"
  x.ToolCalls[idx].Result="User rejected patch execution; no patch was queued or executed."
  x.Status="tool_rejected"
  x,err=store.Save(x,x.Revision)
  if err!=nil{http.Error(w,"cannot persist tool rejection",409);return}
  w.Header().Set("Cache-Control","no-store")
  writeJSON(w,200,map[string]any{"session_id":sessionID,"status":"rejected","revision":x.Revision,"execution_enabled":false});return
 }
 // Commit a non-retryable QUEUING marker BEFORE the file-side effect.
 // A crash after this point is explicitly UNKNOWN; no duplicate package or
 // execution is permitted through the same call_id.
 x.ToolCalls[idx].Status="queuing"
 x.Status="tool_queuing"
 x,err=store.Save(x,x.Revision)
 if err!=nil{http.Error(w,"cannot record tool idempotency marker",409);return}
 receipt,err:=s.agentQueueViaNativePaste(r,arguments)
 if err!=nil{
  x.ToolCalls[idx].Status="unknown"
  x.ToolCalls[idx].Result="Patch queue handoff failed/ambiguous; inspect Patch Tool queue before retry."
  x.Status="tool_unknown"
  _,_=store.Save(x,x.Revision)
  http.Error(w,"native Patch queue handoff failed; call is marked UNKNOWN and will not be retried automatically",502);return
 }
 // The hash/filename receipt is verified against the native queue result.
 if len(receipt.SHA256)!=64{http.Error(w,"invalid native Patch queue receipt",502);return}
 x.ToolCalls[idx].Status="queued"
 x.ToolCalls[idx].QueuePath=receipt.QueuePath
 x.ToolCalls[idx].PackageSHA256=receipt.SHA256
 x.Status="waiting_tool"
 x,err=store.Save(x,x.Revision)
 if err!=nil{http.Error(w,"Patch was queued but journal update failed; inspect queue, do not retry",409);return}
 // This milestone queues an approved patch in the real native Patch Tool.
 // Execution dispatch and function_call_output are separate protocol stages.
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,200,map[string]any{
  "session_id":sessionID,"call_id":callID,"status":"queued",
  "queue_path":receipt.QueuePath,"sha256":receipt.SHA256,
  "revision":x.Revision,"executed":false,"execution_enabled":false,
 })
}

// Stable digest used by tests and future ledger reconciliation; never expose
// the raw patch body in a notification or log.
func agentPatchPayloadSHA256(raw string)string{
 digest:=sha256.Sum256([]byte(raw))
 return hex.EncodeToString(digest[:])
}
