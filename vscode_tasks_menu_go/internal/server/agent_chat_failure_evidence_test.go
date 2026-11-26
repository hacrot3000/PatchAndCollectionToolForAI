package server

import (
 "archive/zip"
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Native failure fixture deliberately models an old acknowledged result
// whose model-visible receipt was reduced to the destructive size fallback.
type clp10FailureFixture struct {
 s *Server
 store *agentchat.Store
 x agentchat.Session
 call agentchat.ToolCallRecord
 report,row,summary,patch map[string]any
 reportPath,handoffPath,registryPath,archivePath string
 code string
}

func writeCLP10JSON(t *testing.T,file string,v any) {
 t.Helper()
 b,err:=json.Marshal(v);if err!=nil{t.Fatal(err)}
 if err=os.MkdirAll(filepath.Dir(file),0700);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(file,b,0600);err!=nil{t.Fatal(err)}
}
func writeCLP10Handoff(t *testing.T,file string,summary map[string]any,extraName string) {
 t.Helper()
 if err:=os.MkdirAll(filepath.Dir(file),0700);err!=nil{t.Fatal(err)}
 f,err:=os.Create(file);if err!=nil{t.Fatal(err)}
 zw:=zip.NewWriter(f)
 w,err:=zw.Create("FAIL_SUMMARY.json");if err!=nil{t.Fatal(err)}
 b,err:=json.Marshal(summary);if err!=nil{t.Fatal(err)}
 if _,err=w.Write(b);err!=nil{t.Fatal(err)}
 if extraName!=""{w,err=zw.Create(extraName);if err!=nil{t.Fatal(err)};if _,err=w.Write([]byte("must not be exposed"));err!=nil{t.Fatal(err)}}
 if err=zw.Close();err!=nil{t.Fatal(err)}
 if err=f.Close();err!=nil{t.Fatal(err)}
}
func newCLP10FailureFixture(t *testing.T) clp10FailureFixture {
 t.Helper()
 s,id,code:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
 call:=&x.ToolCalls[0]
 pkg:=[]byte("approved immutable PATCH bytes")
 call.Status="error";call.RunID="1795598230725789_1225262"
 call.QueuePath="patchs/PATCH_CLEAR_TEXT_tracker.zip"
 call.PackageSHA256=agentFailureSHA(pkg);call.PackageSize=int64(len(pkg))
 call.SelectedIndex=1
 call.Result=`{"status":"INCOMPLETE","reason":"result exceeded data limit"}`
 call.FeedbackSent=true;call.FeedbackStatus="acknowledged"
 x.History=append(x.History,agentchat.HistoryItem{Type:"function_call_output",CallID:call.CallID,Content:call.Result},
  agentchat.HistoryItem{Type:"assistant",Content:"BLOCKED: inspect FAIL_HANDOFF for run "+call.RunID})
 x.Status="failed"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 final:=x.ToolCalls[0]
 name:=filepath.Base(final.QueuePath)
 dir:=filepath.Join(s.Workspace,"artifacts","patch_tool")
 archivePath:=filepath.Join(s.Workspace,"patchs","ignore","agent-failed-"+final.PackageSHA256+".zip")
 if err=os.MkdirAll(filepath.Dir(archivePath),0700);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(archivePath,pkg,0600);err!=nil{t.Fatal(err)}
 relativeHandoff:="artifacts/patch_tool/fail_handoffs/FAIL_HANDOFF_tracker.zip"
 handoffPath:=filepath.Join(s.Workspace,filepath.FromSlash(relativeHandoff))
 patch:=map[string]any{
  "patch_sha256":final.PackageSHA256,"stage":"apply",
  "diagnosis":map[string]any{"kind":"source_drift","message":"tracker anchor mismatch"},
  "partial_modification":map[string]any{"detected":true},
 }
 row:=map[string]any{"name":name,"kind":"PATCH","status":"FAIL","rc":5,
  "patch_result":patch,"fail_handoff":relativeHandoff,"output_tail":"Traceback: tracker anchor mismatch"}
 report:=map[string]any{"format":"python-patch-tool-last-run","format_version":2,
  "project_identity":identity,"run_id":final.RunID,"finished_at":time.Now().UTC().Format(time.RFC3339Nano),
  "selected":[]string{name},"execution_order":[]string{name},"results":[]any{row}}
 summary:=map[string]any{"format":"python-patch-tool-fail-handoff","format_version":2,
  "project_identity":identity,"patch":name,"rc":5,"patch_result":patch}
 reportPath:=filepath.Join(dir,"history","20261125_"+final.RunID+".json")
 registryPath:=filepath.Join(dir,"UNRESOLVED_FAILURES.json")
 writeCLP10JSON(t,reportPath,report)
 writeCLP10Handoff(t,handoffPath,summary,"")
 writeCLP10JSON(t,registryPath,map[string]any{"format":"python-patch-tool-unresolved-failures","format_version":1,
  "entries":[]any{map[string]any{"last_run_id":final.RunID,"resolved":true,"resolution":"agent_retired_failed_queue_item","row":row}}})
 return clp10FailureFixture{s:s,store:store,x:x,call:final,report:report,row:row,summary:summary,patch:patch,
  reportPath:reportPath,handoffPath:handoffPath,registryPath:registryPath,archivePath:archivePath,code:code}
}

func TestAgentCLP10HistoricalFailureRecoversExactRunAndPartialMutation(t *testing.T) {
 f:=newCLP10FailureFixture(t)
 evidence:=f.s.agentResolveFailedPatchEvidence(f.store,f.x,f.call)
 if evidence.NativeStatus!="FAIL"||evidence.RunID!=f.call.RunID||evidence.CallID!=f.call.CallID||
  evidence.Integrity!="native_history_and_handoff_sha_bound"||evidence.Availability!="verified_handoff_summary"||
  evidence.Reason!="tracker anchor mismatch"||evidence.MutationState!="PARTIAL_MODIFICATION_DETECTED"||
  evidence.Predecessor!="retired"||evidence.RC==nil||*evidence.RC!=5 {
  t.Fatalf("unexpected recovered evidence: %+v",evidence)
 }
 projected:=f.s.agentFailureContextProjection(f.store,f.x,"","continue")
 visible:=agentContextForRequest(projected,agentchat.ModelConfig{MaxInputTokens:32768},"",nil)
 rendered:=fmt.Sprint(visible)
 for _,want:=range []string{f.call.RunID,"tracker anchor mismatch","PARTIAL_MODIFICATION_DETECTED","native_registry_and_archive"}{
  if !strings.Contains(rendered,want){t.Fatalf("missing %s in projected context",want)}
 }
 for _,item:=range visible{if item.Type=="function_call_output"&&item.CallID==f.call.CallID{t.Fatal("old receipt replayed")}}
 fresh,err:=f.store.Get(f.x.ID,"local");if err!=nil{t.Fatal(err)}
 if fresh.Revision!=f.x.Revision||fresh.ToolCalls[0].Result!=f.call.Result||len(fresh.History)!=len(f.x.History){t.Fatal("read-only projection mutated journal")}
}

func TestAgentCLP10HistoricalFailureRejectsUnsafeEvidence(t *testing.T) {
 for _,tc:=range []string{"missing","wrong_project","wrong_run","wrong_sha","duplicate_history","wrong_summary","path_traversal","zip_traversal","symlink","oversize"}{
  t.Run(tc,func(t *testing.T){
   f:=newCLP10FailureFixture(t)
   switch tc {
   case "missing":if err:=os.Remove(f.handoffPath);err!=nil{t.Fatal(err)}
   case "wrong_project":f.report["project_identity"]=map[string]string{"id":"other-project","key":"other"};writeCLP10JSON(t,f.reportPath,f.report)
   case "wrong_run":f.report["run_id"]="other-run";writeCLP10JSON(t,f.reportPath,f.report)
   case "wrong_sha":f.patch["patch_sha256"]=strings.Repeat("a",64);writeCLP10JSON(t,f.reportPath,f.report)
   case "duplicate_history":writeCLP10JSON(t,filepath.Join(filepath.Dir(f.reportPath),"second_"+f.call.RunID+".json"),f.report)
   case "wrong_summary":f.summary["project_identity"]=map[string]string{"id":"other-project","key":"other"};writeCLP10Handoff(t,f.handoffPath,f.summary,"")
   case "path_traversal":f.row["fail_handoff"]="artifacts/patch_tool/fail_handoffs/../../secret.zip";writeCLP10JSON(t,f.reportPath,f.report)
   case "zip_traversal":writeCLP10Handoff(t,f.handoffPath,f.summary,"../secret")
   case "symlink":
    target:=f.handoffPath+".original"
    if err:=os.Rename(f.handoffPath,target);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(target,f.handoffPath);err!=nil{t.Skipf("symlink unavailable: %v",err)}
   case "oversize":if err:=os.Truncate(f.handoffPath,agentFailureZIPLimit+1);err!=nil{t.Fatal(err)}
   }
   e:=f.s.agentResolveFailedPatchEvidence(f.store,f.x,f.call)
   if e.Availability=="verified_handoff_summary"||e.Integrity=="native_history_and_handoff_sha_bound"{t.Fatalf("unverified archive accepted: %+v",e)}
   switch tc {
   case "wrong_project","wrong_run","wrong_sha","duplicate_history":
    if e.Integrity!="receipt_only"||e.Reason!=""||e.Predecessor!="unknown"{t.Fatalf("invalid native History exposed: %+v",e)}
   case "missing":
    if e.Availability!="native_history_verified_handoff_unavailable"||e.Reason!="tracker anchor mismatch"{t.Fatalf("missing artifact not distinguished: %+v",e)}
   default:
    if e.Availability!="handoff_integrity_or_limit_refused"||e.Integrity!="native_history_sha_bound"{t.Fatalf("invalid ZIP not distinguished: %+v",e)}
   }
  })
 }
}

func TestAgentCLP10HistoricalFailureAfterArchiveRestartAndOwnerIsolation(t *testing.T){
 f:=newCLP10FailureFixture(t)
 x:=f.x
 // Preserve a confirmed receipt in the same archived ToolCallRecord.
 agentRecordFailedRetirement(&x.ToolCalls[0],"retired","patchs/ignore/agent-failed-"+f.call.PackageSHA256+".zip")
 for i:=0;i<25;i++{
  id:=fmt.Sprintf("later-patch-%d",i)
  result:=`{"status":"PASS","rc":0}`
  x.ToolCalls=append(x.ToolCalls,agentchat.ToolCallRecord{CallID:id,Name:"taskdeck_submit_patch",Status:"done",Result:result,FeedbackSent:true,FeedbackStatus:"acknowledged"})
  x.History=append(x.History,agentchat.HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_submit_patch",Arguments:`{"name":"later"}`},
   agentchat.HistoryItem{Type:"function_call_output",CallID:id,Content:result})
 }
 x.History=append(x.History,agentchat.HistoryItem{Type:"assistant",Content:"Blocked on historical failure "+f.call.RunID})
 x,err:=f.store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if x.PatchArchiveParts==0{t.Fatal("fixture did not archive failure")}
 if err=os.Remove(f.registryPath);err!=nil{t.Fatal(err)}
 restarted:=&Server{Workspace:f.s.Workspace,Config:f.s.Config,ConnectionSecrets:f.s.ConnectionSecrets}
 store,err:=restarted.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 evidence:=restarted.agentResolveFailedPatchEvidence(store,x,f.call)
 if evidence.Predecessor!="retired"||evidence.PredecessorSource!="durable_native_retirement_and_archive"||evidence.Reason!="tracker anchor mismatch"{t.Fatalf("durable retirement lost: %+v",evidence)}
 p:=restarted.agentFailureContextProjection(store,x,"","continue")
 if !strings.Contains(fmt.Sprint(p.History),"durable_native_retirement_and_archive"){t.Fatal("archived failure not projected on continuation")}
 other,err:=agentchat.NewSession("local",x.Workspace,x.ProjectID,x.ProviderID,x.ModelID,x.Mode);if err!=nil{t.Fatal(err)}
 other,err=store.Save(other,0);if err!=nil{t.Fatal(err)}
 foreign:=restarted.agentResolveFailedPatchEvidence(store,other,f.call)
 if foreign.CallID!=""||foreign.Reason!=""||foreign.Integrity!="unavailable"{t.Fatalf("cross-conversation failure disclosure: %+v",foreign)}
 x.Owner="other-owner"
 foreign=restarted.agentResolveFailedPatchEvidence(store,x,f.call)
 if foreign.CallID!=""||foreign.Reason!=""{t.Fatalf("cross-owner failure disclosure: %+v",foreign)}
}

func TestAgentCLP10ContinueOldBlockedConversationProposesSuccessorWithoutReplay(t *testing.T){
 f:=newCLP10FailureFixture(t)
 requests:=0
 args,err:=json.Marshal(map[string]string{"name":"corrected tracker","patch_python":f.code});if err!=nil{t.Fatal(err)}
 f.s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  requests++
  var req struct{Input []map[string]any `json:"input"`}
  if err:=json.Unmarshal(payload,&req);err!=nil{t.Fatal(err)}
  if !strings.Contains(string(payload),"tracker anchor mismatch")||!strings.Contains(string(payload),f.call.RunID){t.Fatal("provider did not receive recovered failure diagnostics")}
  for _,item:=range req.Input{
   if (item["type"]=="function_call_output"||item["type"]=="function_call")&&item["call_id"]==f.call.CallID{t.Fatal("acknowledged failed exchange replayed")}
  }
  return agentchat.ResponsesResult{ResponseID:"corrected-proposal",Calls:[]agentchat.FunctionCall{{CallID:"corrected-tracker",Name:"taskdeck_submit_patch",Arguments:string(args)}}},nil
 }
 w:=invokeAI(t,f.s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+f.x.ID+"/messages",
  map[string]any{"message":"continue","confirm_external":true,"expected_revision":f.x.Revision,"request_id":"clp10-continue"})
 if w.Code!=200{t.Fatalf("continuation failed: %d %s",w.Code,w.Body)}
 x,err:=f.store.Get(f.x.ID,"local");if err!=nil{t.Fatal(err)}
 if requests!=1||len(x.ToolCalls)!=2||x.ToolCalls[0].Result!=f.call.Result||x.ToolCalls[0].FeedbackStatus!="acknowledged"||
  x.ToolCalls[1].CallID!="corrected-tracker"||x.ToolCalls[1].Status!="pending"{t.Fatalf("old receipt changed or successor unavailable: requests=%d calls=%+v",requests,x.ToolCalls)}
}


func TestAgentCommandOnlyPassDoesNotHideEarlierAtomicFailure(t *testing.T){
 f:=newCLP10FailureFixture(t)
 identity,err:=patchtool.ReadProjectIdentity(f.s.Workspace);if err!=nil{t.Fatal(err)}
 manifest:=testAgentCommandManifest(identity,[]string{"bash","scripts/regression.sh"})
 args,err:=json.Marshal(agentPatchArguments{Name:"regression",PatchManifest:manifest});if err!=nil{t.Fatal(err)}
 x:=f.x
 x.History=append(x.History,agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"regression-pass",Arguments:string(args)})
 x.ToolCalls=append(x.ToolCalls,agentchat.ToolCallRecord{CallID:"regression-pass",Name:"taskdeck_submit_patch",Status:"done",Result:`{"status":"PASS","rc":0}`})
 // The current success is about to be delivered; the projection must
 // still include the exact prior failed run, without duplicating receipts.
 x,err=f.store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 p:=f.s.agentFailureContextProjection(f.store,x,"regression-pass","")
 visible:=fmt.Sprint(p.History)
 if !strings.Contains(visible,f.call.RunID)||!strings.Contains(visible,"PARTIAL_MODIFICATION_DETECTED")||!strings.Contains(visible,"predecessor_disposition"){t.Fatal("regression hid prior failure evidence")}
 if len(p.History)!=len(x.History)+1{t.Fatal("expected one read-only diagnostic projection")}
 fresh,err:=f.store.Get(x.ID,x.Owner);if err!=nil{t.Fatal(err)}
 if len(fresh.History)!=len(x.History)||fresh.ToolCalls[0].Result!=f.call.Result{t.Fatal("projection changed canonical receipt")}
}
