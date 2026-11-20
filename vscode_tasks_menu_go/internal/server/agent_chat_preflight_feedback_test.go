package server

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
 "bletonfc/vscode_tasks_menu/internal/session"
)

// This is a real native resource-preflight result: the package was SELECTED
// but nothing appears in execution_order because no source code was run.
// Native Python stores the cause under diagnosis.kind/message, not top-level
// diagnosis_kind/failure_reason.
func installResourcePreflightReport(t *testing.T, s *Server, name string, bytes []byte) {
 t.Helper()
 dir,err:=s.patchUploadDir()
 if err!=nil {t.Fatal(err)}
 if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,name),bytes,0600);err!=nil {t.Fatal(err)}
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace)
 if err!=nil {t.Fatal(err)}
 history:=filepath.Join(s.Workspace,"artifacts","patch_tool","history")
 if err:=os.MkdirAll(history,0700);err!=nil {t.Fatal(err)}
 report:=map[string]any{
  "format":"python-patch-tool-last-run","format_version":2,
  "project_identity":identity,"run_id":"preflight-disk-space-001",
  "status":"FAIL","exit_code":2,
  "selected":[]string{name},"execution_order":[]string{},
  "results":[]map[string]any{{
   "name":name,"kind":"PATCH","status":"PREFLIGHT_FAIL","rc":2,
   "diagnosis":map[string]any{"kind":"insufficient_disk_space","message":"resource preflight found insufficient project/temp free space"},
   "output_tail":"BATCH PREFLIGHT FAIL — project unchanged | insufficient_disk_space",
  }},
 }
 body,err:=json.Marshal(report)
 if err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(history,"resource-preflight.json"),body,0600);err!=nil {t.Fatal(err)}
}

func TestAgentPreflightDiskFailureReconcilesAndAutoSendsToAIOnce(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 name:="PATCH_CLEAR_TEXT_Vietnamese-diacritics-and-phpinfo-output_20261120T072133_0a2531d7.zip"
 packageBytes:=[]byte("immutable native package rejected before source writes")
 digest:=sha256.Sum256(packageBytes)
 x.ToolCalls[0].Status="unknown" // legacy failed before queue selection
 x.ToolCalls[0].PatchSessionID="expired-preflight-terminal"
 x.ToolCalls[0].QueuePath="patchs/"+name
 x.ToolCalls[0].PackageSHA256=hex.EncodeToString(digest[:])
 x.ToolCalls[0].PackageSize=int64(len(packageBytes))
 x.ToolCalls[0].SelectedIndex=0
 x.ToolCalls[0].PromptID=""
 x.Status="tool_unknown"
 x.AutoPatch=true // Auto apply must NOT retry a failed preflight
 x.AutoCollection=new(bool);*x.AutoCollection=true
 x.AutoFeedback=true
 x.AutoFeedbackPersistent=true
 x,err=store.Save(x,x.Revision)
 if err!=nil {t.Fatal(err)}
 installResourcePreflightReport(t,s,name,packageBytes)

 providerCalls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  providerCalls++
  if !strings.Contains(string(payload),"PREFLIGHT_FAIL") ||
    !strings.Contains(string(payload),"insufficient_disk_space") ||
    !strings.Contains(string(payload),"project_unchanged") ||
    !strings.Contains(string(payload),"resource preflight found insufficient project/temp free space") {
   t.Fatalf("missing verified failure evidence delivered to AI: %s",payload)
  }
  return agentchat.ResponsesResult{ResponseID:"preflight-ai-reply",Text:"Patch chưa được áp dụng vì thiếu dung lượng ổ đĩa cho dự án hoặc thư mục tạm. Hãy giải phóng dung lượng rồi yêu cầu thử lại."},nil
 }
 s.runAgentAutoFeedback(context.Background())
 y,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if providerCalls!=1 || y.Status!="completed" ||
   !y.ToolCalls[0].FeedbackSent || y.ToolCalls[0].FeedbackStatus!="acknowledged" ||
   y.ToolCalls[0].Status!="error" || !strings.Contains(y.ToolCalls[0].Result,`"rc":2`) ||
   !strings.Contains(y.ToolCalls[0].Result,"insufficient_disk_space") {
  t.Fatalf("verified native preflight error was not sent once: %+v calls=%d",y,providerCalls)
 }
 if !strings.Contains(y.History[len(y.History)-1].Content,"thiếu dung lượng") {
  t.Fatal("AI response after failed Patch was not stored")
 }
 s.runAgentAutoFeedback(context.Background())
 if providerCalls!=1 {t.Fatalf("preflight result was sent twice: %d",providerCalls)}
 got,err:=os.ReadFile(filepath.Join(s.Workspace,"patchs",name))
 if err!=nil || string(got)!=string(packageBytes) {t.Fatal("failed preflight was automatically requeued or modified")}
}

func TestAgentPreselectionLiveProtocolPreflightBecomesError(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 name:="PATCH_FAILED_before_selection.zip"
 pkg:=[]byte("immutable-approved-package")
 digest:=sha256.Sum256(pkg)
 dir,err:=s.patchUploadDir()
 if err!=nil {t.Fatal(err)}
 if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,name),pkg,0600);err!=nil {t.Fatal(err)}
 x.ToolCalls[0].Status="dispatching"
 x.ToolCalls[0].QueuePath="patchs/"+name
 x.ToolCalls[0].PackageSHA256=hex.EncodeToString(digest[:])
 x.ToolCalls[0].PackageSize=int64(len(pkg))
 x.ToolCalls[0].PatchSessionID="native-preflight-live"
 x.ToolCalls[0].SelectedIndex=0
 x.Status="tool_unknown"
 x,err=store.Save(x,x.Revision)
 if err!=nil {t.Fatal(err)}
 rc:=2
 s.Sessions=&fakeAgentPatchProtocol{
  expectedID:"native-preflight-live",
  state:session.ProtocolState{Available:true,Items:[]session.ProtocolItemState{{
   Index:1,Name:name,Kind:"PATCH",Status:"PREFLIGHT_FAIL",RunID:"native-preflight-run",
   RC:&rc,DiagnosisKind:"insufficient_disk_space",FailureReason:"resource preflight found insufficient project/temp free space",
  }}},
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/sync",map[string]any{})
 if w.Code!=200 {t.Fatalf("preflight sync failed: %d %s",w.Code,w.Body)}
 y,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if y.ToolCalls[0].Status!="error" || y.Status!="waiting_tool" || y.ToolCalls[0].RunID!="native-preflight-run" {
  t.Fatalf("live preselection error was not made feedback-ready: %+v",y)
 }
 if !strings.Contains(y.ToolCalls[0].Result,`"project_unchanged":true`) {
  t.Fatal("preflight evidence did not identify unchanged project")
 }
}

func TestAgentPreflightRejectsAmbiguousOrAlteredPackage(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 name:="PATCH_resource_failure.zip"
 pkg:=[]byte("different bytes")
 x.ToolCalls[0].Status="unknown"
 x.ToolCalls[0].QueuePath="patchs/"+name
 x.ToolCalls[0].PackageSHA256=strings.Repeat("a",64)
 x.ToolCalls[0].PackageSize=int64(len(pkg))
 x.ToolCalls[0].PatchSessionID="native-absent"
 x.Status="tool_unknown"
 x.AutoFeedback=true
 x.AutoFeedbackPersistent=true
 x,err=store.Save(x,x.Revision)
 if err!=nil {t.Fatal(err)}
 installResourcePreflightReport(t,s,name,pkg)
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++
  return agentchat.ResponsesResult{},nil
 }
 s.runAgentAutoFeedback(context.Background())
 y,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if calls!=0 || y.ToolCalls[0].Status!="unknown" || y.ToolCalls[0].FeedbackSent {
  t.Fatal("unverified/altered Patch result was sent to AI")
 }
}

func TestAgentApproveRefreshSafeAfterAutomationRevisionChanges(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 previousRevision:=x.Revision
 x.AutoCollection=new(bool);*x.AutoCollection=true
 x,err=store.Save(x,x.Revision)
 if err!=nil {t.Fatal(err)}
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,
  "/api/agent/sessions/"+id+"/tools/"+x.ToolCalls[0].CallID+"/decision",
  map[string]any{"action":"approve","confirm_execution":true,"expected_revision":previousRevision})
 if w.Code!=200 {
  t.Fatalf("explicit approval rejected only due to benign Automation revision bump: %d %s",w.Code,w.Body)
 }
 y,err:=store.Get(id,"local")
 if err!=nil || y.ToolCalls[0].Status!="queued" {t.Fatalf("native approval did not queue correct patch: %+v %v",y,err)}
}


func TestAgentCollectionReadFailureIsReturnedToAIWithoutASecondApproval(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls==1 {
   return agentchat.ResponsesResult{ResponseID:"read-missing",Calls:[]agentchat.FunctionCall{{
    CallID:"collect-file-missing",Name:"taskdeck_collect_workspace",
    Arguments:`{"actions":[{"type":"read","path":"not-created.php"}]}`,
   }}},nil
  }
  if calls!=2 {t.Fatalf("unexpected tool retry: %d",calls)}
  var req struct {Input []map[string]any `json:"input"`}
  if err:=json.Unmarshal(payload,&req);err!=nil {t.Fatal(err)}
  found:=false
  for _,item:=range req.Input {
   if item["type"]=="function_call_output" && item["call_id"]=="collect-file-missing" {
    if strings.Contains(item["output"].(string),"not-created.php") &&
     strings.Contains(item["output"].(string),`"error"`) {found=true}
   }
  }
  if !found {
   t.Fatalf("read-only Collection failure was not returned to model: %s",payload)
  }
  return agentchat.ResponsesResult{ResponseID:"diagnosed-missing-file",
   Text:"Không tìm thấy file not-created.php trong project. Hãy kiểm tra đường dẫn trước khi sửa."},nil
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,
  "/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"read not-created.php","confirm_external":true,"expected_revision":uint64(1)})
 if w.Code!=200 || calls!=2 {t.Fatalf("Collection failure did not reach AI: %d %s (calls=%d)",w.Code,w.Body,calls)}
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if x.Status!="completed" || len(x.ToolCalls)!=0 || !strings.Contains(x.History[len(x.History)-1].Content,"Không tìm thấy") {
  t.Fatalf("AI did not respond to Collection failure: %+v",x)
 }
}
