package server

import (
 "context"
 "crypto/sha256"
 "encoding/json"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "runtime"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
 sessionpkg "bletonfc/vscode_tasks_menu/internal/session"
)

// No replacement reader: exercise the production Agent queue and direct
// native Python COLLECT worker with a real large source file. The provider
// boundary below is inspected locally, not sent to an external endpoint.
func TestAgentCLP20NativeTargeted150MiBSourceReceipt(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 s.AgentCollectionCall=nil // real native worker, not protocol fixture
 _,testFile,_,ok:=runtime.Caller(0);if !ok{t.Fatal("cannot locate runtime")}
 entry:=filepath.Clean(filepath.Join(filepath.Dir(testFile),"..","..","..","python_patch_entry.py"))
 if _,err:=os.Stat(entry);err!=nil{t.Fatal(err)}
 python,err:=exec.LookPath("python3");if err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_PATCH_RUNTIME",entry);t.Setenv("TASKDECK_PATCH_PYTHON",python)
 native:=sessionpkg.NewManager(1<<20)
 s.Sessions=native
 t.Cleanup(func(){native.Shutdown(2*time.Second)})
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
 const marker="CLP20_TARGET_NEAR_END_91cf"
 const size=150<<20
 path:=filepath.Join(s.Workspace,"large_source.c")
 f,err:=os.Create(path);if err!=nil{t.Fatal(err)}
 hash:=sha256.New();sourceWriter:=io.MultiWriter(f,hash)
 // Fixed 128-byte lines, not NUL-filled sparse bytes: the search backend
 // must actually inspect text beyond its default 64 MiB search limit.
 row:=strings.Repeat(" ",124)+"//x\n"
 block:=strings.Repeat(row,(1<<20)/len(row))
 for n:=0;n<149;n++{if _,err=io.WriteString(sourceWriter,block);err!=nil{f.Close();t.Fatal(err)}}
 tail:=fmt.Sprintf("int %s(void) { return 17; }\n",marker)
 remaining:=size-149*len(block)-len(tail)
 if _,err=io.WriteString(sourceWriter,strings.Repeat(row,remaining/len(row))+strings.Repeat(" ",remaining%len(row))+tail);err!=nil{f.Close();t.Fatal(err)}
 if err=f.Close();err!=nil{t.Fatal(err)}
 before:=fmt.Sprintf("%x",hash.Sum(nil))
 req,err:=agentNativeCollectionRequest(fmt.Sprintf(`{"id":"clp20-target","actions":[{"type":"search","paths":["large_source.c"],"query":%q,"context_lines":0,"max_matches":2,"report_coverage":true,"must_find":true}],"limits":{"max_search_file_bytes":200000000}}`,marker),identity,false)
 if err!=nil{t.Fatal(err)}
 // Exercise the whole real native read -> provider UNKNOWN -> restart path.
 // The provider alone is simulated; no second local collector is installed.
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 initial,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 initial.RetryMode=agentchat.RetryOff
 initial.AutoContinueText=false
 initial,err=store.Save(initial,initial.Revision);if err!=nil{t.Fatal(err)}
 calls:=0
 receipt:=""
 readReceipt:=func(payload []byte)string{
  t.Helper()
  var wire struct{Input []map[string]any}
  if json.Unmarshal(payload,&wire)!=nil{t.Fatal("invalid provider payload")}
  got:="";n:=0
  for _,it:=range wire.Input{if it["type"]=="function_call_output"&&it["call_id"]=="large-read"{n++;got,_=it["output"].(string)}}
  if n!=1||got==""{t.Fatal("native receipt missing or duplicated")}
  return got
 }
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  switch calls{
  case 1:return agentchat.ResponsesResult{ResponseID:"before-read",Calls:[]agentchat.FunctionCall{{CallID:"large-read",Name:"taskdeck_collect_workspace",Arguments:string(req)}}},nil
  case 2:receipt=readReceipt(payload);return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"RESPONSE_HEADER_TIMEOUT",Retryable:true,Ambiguous:true}
  default:t.Fatal("unexpected provider replay");return agentchat.ResponsesResult{},nil
  }
 }
 endpoint:="/api/agent/sessions/"+id+"/messages"
 w:=invokeAI(t,s.agentChatSessionItemAPI,"POST",endpoint,map[string]any{"message":"Inspect the exact target without collecting the whole source","confirm_external":true,"expected_revision":initial.Revision})
 if w.Code<400||calls!=2||receipt==""{t.Fatalf("expected failed continuation after actual native read: HTTP %d calls %d %s",w.Code,calls,w.Body)}
 saved,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if saved.Status!="failed"||saved.ProviderRun==nil||saved.ProviderRun.Kind!="collection"||saved.ProviderRun.Phase!="unknown"||saved.ProviderRun.CallID!="large-read"||saved.History[len(saved.History)-1].Content!=receipt{t.Fatal("actual native receipt not durable at UNKNOWN checkpoint")}
 var obs struct{Status string;EvidenceVerified bool `json:"evidence_verified"`;Text string `json:"text_excerpt"`;Zip string `json:"result_zip"`;TXT string `json:"result_text"`;RunID string `json:"run_id"`}
 if err=json.Unmarshal([]byte(receipt),&obs);err!=nil{t.Fatal(err)}
 if obs.Status!="PASS"||!obs.EvidenceVerified||obs.RunID==""||obs.Zip==""||obs.TXT==""||!strings.Contains(obs.Text,marker)||len(receipt)>256<<10{t.Fatalf("native targeted receipt not verified/bounded: status=%s verified=%t bytes=%d run=%s excerpt=%s",obs.Status,obs.EvidenceVerified,len(receipt),obs.RunID,obs.Text)}
 if _,err=agentNativeCollectionArtifactPath(s.Workspace,obs.Zip,".zip");err!=nil{t.Fatal(err)}
 if _,err=agentNativeCollectionArtifactPath(s.Workspace,obs.TXT,".txt");err!=nil{t.Fatal(err)}
 source,err:=os.Open(path);if err!=nil{t.Fatal(err)}
 hash.Reset();n,err:=io.Copy(hash,source);source.Close()
 if err!=nil||n!=size||fmt.Sprintf("%x",hash.Sum(nil))!=before{t.Fatal("native COLLECT modified source")}
 x:=agentchat.Session{History:[]agentchat.HistoryItem{{Type:"user",Content:"Inspect the exact target without collecting the whole source"},{Type:"function_call",CallID:"large-read",Name:"taskdeck_collect_workspace",Arguments:string(req)}}}
 result:=agentchat.HistoryItem{Type:"function_call_output",CallID:"large-read",Content:receipt}
 for _,api:=range []string{"responses","chat_completions"}{
  cfg:=agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"Current native policy",Next:[]agentchat.HistoryItem{result},EnableCollectionTool:true,MaxOutputTokens:8192}
  provider:=agentchat.ProviderConfig{APIType:api}
  model:=agentchat.ModelConfig{MaxInputTokens:65536}
  body,e:=agentBuildContextRequest(x,model,provider,cfg);if e!=nil{t.Fatal(e)}
  var wire struct{Input []map[string]any}
  if json.Unmarshal(body,&wire)!=nil{t.Fatal("bad canonical payload")}
  count:=0
  for _,it:=range wire.Input{if it["type"]=="function_call_output"&&it["call_id"]=="large-read"{count++;if it["output"]!=receipt{t.Fatal("native receipt changed")}}}
  if count!=1{t.Fatal("native receipt omitted/duplicated")}
  if api=="chat_completions"{
   converted,e:=agentchat.BuildChatCompletionsRequest(body);if e!=nil{t.Fatal(e)}
   var target struct{Messages []map[string]any}
   if json.Unmarshal(converted,&target)!=nil{t.Fatal("bad target format")}
   count=0;for _,it:=range target.Messages{if it["tool_call_id"]=="large-read"{count++;if it["content"]!=receipt{t.Fatal("target wire changed native receipt")}}}
   if count!=1{t.Fatal("target wire lost receipt")}
  }
  cfg.Next[0].Content=strings.Repeat("x",(512<<10)+1)
  if bad,e:=agentBuildContextRequest(x,model,provider,cfg);e==nil||len(bad)!=0{t.Fatal("mandatory overflow silently accepted")}
 }
 artifactsBefore,err:=filepath.Glob(filepath.Join(s.Workspace,"artifacts","patch_tool_code_collections","CODE_COLLECTION_RESULT_*.zip"));if err!=nil||len(artifactsBefore)!=1{t.Fatalf("expected one native artifact, got %v %v",artifactsBefore,err)}
 restarted:=&Server{Workspace:s.Workspace,Config:s.Config,ConnectionSecrets:s.ConnectionSecrets,AgentChatPack:s.AgentChatPack,Sessions:native}
 restarted.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls!=3||readReceipt(payload)!=receipt||!strings.Contains(string(payload),"NEW continuation"){t.Fatal("restart did not use exact saved native receipt as a new continuation")}
  return agentchat.ResponsesResult{Text:"Targeted observation verified; no further read required.",ResponseID:"after-resume"},nil
 }
 reopened,err:=restarted.agentChatStore();if err!=nil{t.Fatal(err)}
 restarted.agentReconcilePendingOnStartup(context.Background())
 restarted.processAgentAutoFeedbackSession(context.Background(),reopened,id)
 if calls!=2{t.Fatal("startup replayed UNKNOWN provider request")}
 resume:=map[string]any{"message":"Resume original task","confirm_external":true,"expected_revision":saved.Revision,"request_id":"native-large-resume","resume_collection_run_id":saved.ProviderRun.ID}
 w=invokeAI(t,restarted.agentChatSessionItemAPI,"POST",endpoint,resume)
 if w.Code!=200||calls!=3{t.Fatalf("resume: HTTP %d calls=%d %s",w.Code,calls,w.Body)}
 final,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
 if len(final.History)<len(saved.History)||!reflect.DeepEqual(final.History[:len(saved.History)],saved.History)||len(final.CollectionResumeHistory)!=1||final.CollectionResumeHistory[0].ReceiptSHA256!=agentchat.ProviderPayloadDigest([]byte(receipt)){t.Fatal("resume changed canonical receipt or lost UNKNOWN audit")}
 artifactsAfter,err:=filepath.Glob(filepath.Join(s.Workspace,"artifacts","patch_tool_code_collections","CODE_COLLECTION_RESULT_*.zip"));if err!=nil||!reflect.DeepEqual(artifactsAfter,artifactsBefore){t.Fatal("native read rerun on resume")}
 w=invokeAI(t,restarted.agentChatSessionItemAPI,"POST",endpoint,resume)
 if w.Code!=200||calls!=3{t.Fatal("duplicate resume egress")}
 t.Logf("real native source=%d bytes, run=%s, receipt=%d bytes, original SHA256 unchanged; UNKNOWN restart/resume exactly once",n,obs.RunID,len(receipt))
}


 // Exercise real native line proofs after external writes between provider turns.
 // No replacement collector, no external provider, no implicit Patch approval.
func TestAgentCLP20NativeRangeExternalMutationInvalidatesCoverage(t *testing.T) {
 s, id := newAgentConversationFixture(t, "stateless")
 s.AgentCollectionCall = nil
 _, testFile, _, ok := runtime.Caller(0)
 if !ok { t.Fatal("cannot locate runtime") }
 entry := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", "..", "python_patch_entry.py"))
 if _, err := os.Stat(entry); err != nil { t.Fatal(err) }
 python, err := exec.LookPath("python3")
 if err != nil { t.Fatal(err) }
 t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
 t.Setenv("TASKDECK_PATCH_PYTHON", python)
 native := sessionpkg.NewManager(1 << 20)
 s.Sessions = native
 t.Cleanup(func() { native.Shutdown(2*time.Second) })
 path := filepath.Join(s.Workspace, "clp20_mutation.go")
 const a = "package fixture\n// CLP20_REVISION_A\n"
 const b = "package fixture\n// CLP20_REVISION_B\n"
 if err := os.WriteFile(path, []byte(a), 0600); err != nil { t.Fatal(err) }
 expected := []string{a,a,b,b,a}
 revisions := []string{}
 receipts := map[string]string{}
 runs := map[string]bool{}
 calls := 0
 s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, payload []byte) (agentchat.ResponsesResult,error) {
  calls++
  if calls > 1 {
   var wire struct { Input []map[string]any }
   if err := json.Unmarshal(payload, &wire); err != nil { t.Fatal(err) }
   key := fmt.Sprintf("mutation-read-%d", calls-1)
   receipt := ""
   n := 0
   for _, item := range wire.Input {
    if item["type"] == "function_call_output" && item["call_id"] == key {
     n++
     receipt, _ = item["output"].(string)
    }
   }
   if n != 1 || receipt == "" { t.Fatal("missing or duplicate current receipt") }
   receipts[key] = receipt
   var o struct {
    Status string
    EvidenceVerified bool `json:"evidence_verified"`
    RunID string `json:"run_id"`
    Text string `json:"text_excerpt"`
    Lines []struct {
     Path string `json:"path"`
     Revision string `json:"source_revision_sha256"`
     First uint64 `json:"start_line"`
     Last uint64 `json:"end_line"`
    } `json:"native_line_reads"`
   }
   if err := json.Unmarshal([]byte(receipt), &o); err != nil { t.Fatal(err) }
   if o.Status != "PASS" || !o.EvidenceVerified || o.RunID == "" || runs[o.RunID] ||
    len(o.Lines) != 1 || o.Lines[0].Path != "clp20_mutation.go" || o.Lines[0].First != 1 ||
    o.Lines[0].Last != 2 || len(o.Lines[0].Revision) != 64 {
    t.Fatalf("expected verified native line proof: %+v", o)
   }
   runs[o.RunID] = true
   revisions = append(revisions, o.Lines[0].Revision)
   current, err := os.ReadFile(path)
   if err != nil || string(current) != expected[calls-2] { t.Fatal("native collection modified source") }
   marker := "CLP20_REVISION_A"
   if expected[calls-2] == b { marker = "CLP20_REVISION_B" }
   if !strings.Contains(o.Text, marker) { t.Fatal("native observation stale after external edit") }
  }
  if calls == 6 { return agentchat.ResponsesResult{ResponseID:"done", Text:"Verified the changing source."},nil }
  if calls > 6 { t.Fatalf("unexpected provider call %d", calls) }
  // Explicit fixture mutation between real native reads; this is NOT Patch.
  if calls == 3 || calls == 5 {
   if err := os.WriteFile(path, []byte(expected[calls-1]), 0600); err != nil { t.Fatal(err) }
  }
  args := fmt.Sprintf(`{"id":"mutation-%d","actions":[{"type":"range","path":"clp20_mutation.go","start_line":1,"end_line":2}]}`,calls)
  return agentchat.ResponsesResult{ResponseID:fmt.Sprintf("reply-%d",calls),Calls:[]agentchat.FunctionCall{{CallID:fmt.Sprintf("mutation-read-%d",calls),Name:"taskdeck_collect_workspace",Arguments:args}}},nil
 }
 w := invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",map[string]any{"message":"Audit the file read-only","confirm_external":true,"expected_revision":1})
 if w.Code != 200 || calls != 6 || len(runs) != 5 { t.Fatalf("HTTP=%d calls=%d native runs=%d body=%s",w.Code,calls,len(runs),w.Body) }
 if revisions[0] != revisions[1] || revisions[2] != revisions[3] || revisions[0] == revisions[2] || revisions[0] != revisions[4] {
  t.Fatalf("source revision mismatch: %v",revisions)
 }
 store, err := s.agentChatStore()
 if err != nil { t.Fatal(err) }
 saved, err := store.Get(id,"local")
 if err != nil { t.Fatal(err) }
 g, ok := saved.ReadCollectionGuard()
 if !ok || g.Counters.NativeReads != 5 || g.Counters.NewEvidence != 3 || g.Counters.DuplicateEvidence != 2 || g.Counters.UnknownEvidence != 0 || len(saved.ToolCalls) != 0 {
  t.Fatalf("external mutation incorrectly classified: %+v",g)
 }
 n := 0
 for _, item := range saved.History {
  if item.Type == "function_call_output" {
   n++
   if item.Content != receipts[item.CallID] { t.Fatal("native receipt changed in journal") }
  }
 }
 if n != 5 { t.Fatalf("canonical receipt count %d",n) }
 artifacts, err := filepath.Glob(filepath.Join(s.Workspace,"artifacts","patch_tool_code_collections","CODE_COLLECTION_RESULT_*.zip"))
 if err != nil || len(artifacts) != 5 { t.Fatalf("native artifact count %d: %v",len(artifacts),err) }
}
