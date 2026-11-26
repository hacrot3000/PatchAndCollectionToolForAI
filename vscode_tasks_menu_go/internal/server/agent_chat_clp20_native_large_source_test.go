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
 s,_:=newAgentConversationFixture(t,"stateless")
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
 hash:=sha256.New();w:=io.MultiWriter(f,hash)
 // Fixed 128-byte lines, not NUL-filled sparse bytes: the search backend
 // must actually inspect text beyond its default 64 MiB search limit.
 row:=strings.Repeat(" ",124)+"//x\n"
 block:=strings.Repeat(row,(1<<20)/len(row))
 for n:=0;n<149;n++{if _,err=io.WriteString(w,block);err!=nil{f.Close();t.Fatal(err)}}
 tail:=fmt.Sprintf("int %s(void) { return 17; }\n",marker)
 remaining:=size-149*len(block)-len(tail)
 if _,err=io.WriteString(w,strings.Repeat(row,remaining/len(row))+strings.Repeat(" ",remaining%len(row))+tail);err!=nil{f.Close();t.Fatal(err)}
 if err=f.Close();err!=nil{t.Fatal(err)}
 before:=fmt.Sprintf("%x",hash.Sum(nil))
 req,err:=agentNativeCollectionRequest(fmt.Sprintf(`{"id":"clp20-target","actions":[{"type":"search","paths":["large_source.c"],"query":%q,"context_lines":0,"max_matches":2,"report_coverage":true,"must_find":true}],"limits":{"max_search_file_bytes":200000000}}`,marker),identity,false)
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 receipt,err:=s.agentRunNativeCollection(ctx,"local",req,identity);if err!=nil{t.Fatal(err)}
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
 t.Logf("real native source=%d bytes, run=%s, receipt=%d bytes, original SHA256 unchanged",n,obs.RunID,len(receipt))
}
