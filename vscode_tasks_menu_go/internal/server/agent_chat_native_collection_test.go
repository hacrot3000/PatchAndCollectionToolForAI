package server

import (
 "archive/zip"
 "bytes"
 "encoding/json"
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/patchtool"
 "bletonfc/vscode_tasks_menu/internal/session"
)

func TestAgentNativeCollectionRequestUsesOriginalSchemaAndIdentity(t *testing.T){
 identity:=patchtool.ProjectIdentity{Key:"server-patching",ID:"4fb18d97-cbc2-433b-8234-538b32ed0189"}
 request:=`{"id":"inspect-decompile","actions":[{"type":"decompile","source":"source/large.c","address":"0x21E551A","neighbors_before":2}],"limits":{"max_decompile_file_bytes":268435456}}`
 raw,err:=agentNativeCollectionRequest(request,identity,false)
 if err!=nil{t.Fatal(err)}
 var value map[string]any
 if json.Unmarshal(raw,&value)!=nil{t.Fatal("invalid packaged request")}
 project:=value["project"].(map[string]any)
 if project["key"]!=identity.Key||project["id"]!=identity.ID{t.Fatalf("native identity not injected: %+v",project)}
 actions:=value["actions"].([]any)
 if actions[0].(map[string]any)["type"]!="decompile"||actions[0].(map[string]any)["address"]!="0x21E551A"{
  t.Fatalf("native-specific decompile arguments were lost: %+v",actions)
 }
 if _,ok:=agentNativeCollectionTextRequest(request,identity);ok{
  t.Fatal("text fallback accepted COLLECT without explicit identity")
 }
 wrong:=`{"project":{"key":"foreign","id":"4fb18d97-cbc2-433b-8234-538b32ed0189"},"actions":[{"type":"search","query":"hello"}]}`
 if _,err:=agentNativeCollectionRequest(wrong,identity,false);err==nil{t.Fatal("foreign native request was accepted")}
 if _,err:=agentNativeCollectionRequest(wrong,identity,true);err==nil{t.Fatal("foreign text request was accepted")}
 native:=`{"project":{"key":"server-patching","id":"4fb18d97-cbc2-433b-8234-538b32ed0189"},"actions":[{"type":"symbol","path":"src/code.c","symbol":"main"}]}`
 if _,ok:=agentNativeCollectionTextRequest(native,identity);!ok{t.Fatal("valid native text COLLECT was rejected")}
 if _,ok:=agentNativeCollectionTextRequest("please execute "+native,identity);ok{t.Fatal("prose embedded request was accepted")}
}

func writeAgentNativeCollectionArtifacts(t *testing.T,root,stem,status string,identity patchtool.ProjectIdentity)(string,string){
 t.Helper()
 dir:=filepath.Join(root,"artifacts","patch_tool_code_collections")
 if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
 buf:=new(bytes.Buffer)
 z:=zip.NewWriter(buf)
 w,err:=z.Create("COLLECTION_MANIFEST.json");if err!=nil{t.Fatal(err)}
 manifest,_:=json.Marshal(map[string]any{"collection_status":status,"project_identity":identity})
 if _,err=w.Write(manifest);err!=nil{t.Fatal(err)}
 if err=z.Close();err!=nil{t.Fatal(err)}
 zipRel:="artifacts/patch_tool_code_collections/"+stem+".zip"
 txtRel:="artifacts/patch_tool_code_collections/"+stem+".txt"
 if err:=os.WriteFile(filepath.Join(dir,stem+".zip"),buf.Bytes(),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,stem+".txt"),[]byte("PROJECT IDENTITY\nNative COLLECT result\nSEARCH COVERAGE: PARTIAL\n"),0600);err!=nil{t.Fatal(err)}
 return zipRel,txtRel
}

func TestAgentNativeCollectionPartialArtifactSurvivesFailedWrapperRC3(t *testing.T){
 root:=t.TempDir()
 identity:=patchtool.ProjectIdentity{Key:"server-patching",ID:"4fb18d97-cbc2-433b-8234-538b32ed0189"}
 zipRel,txtRel:=writeAgentNativeCollectionArtifacts(t,root,"CODE_COLLECTION_RESULT_partial_20261123","INCOMPLETE",identity)
 rc:=3
 item:=session.ProtocolItemState{Index:1,Name:"CODE_COLLECTION_REQUEST_partial.zip",Kind:"COLLECT",Status:"FAIL",RunID:"run-1",RC:&rc}
 state:=session.ProtocolState{Items:[]session.ProtocolItemState{item},Artifacts:[]session.ProtocolArtifactState{
  {ItemName:item.Name,ItemKind:"COLLECT",Index:1,ArtifactKind:"collect_result_zip",Path:zipRel},
  {ItemName:item.Name,ItemKind:"COLLECT",Index:1,ArtifactKind:"collect_result_text",Path:txtRel},
 }}
 if _,found:=agentNativeCollectionFinalItem(state,item.Name,1);!found{t.Fatal("native rc=3 item not considered final")}
 gotZip,gotTxt:=agentNativeCollectionArtifacts(state,item)
 if gotZip!=zipRel||gotTxt!=txtRel{t.Fatalf("wrong native artifacts %q %q",gotZip,gotTxt)}
 var observed map[string]any
 if err:=json.Unmarshal([]byte(agentNativeCollectionObservation(root,identity,"native-session",item,gotZip,gotTxt)),&observed);err!=nil{t.Fatal(err)}
 if observed["status"]!="INCOMPLETE"||observed["evidence_verified"]!=true||observed["partial_evidence_available"]!=true{
  t.Fatalf("rc=3 discarded actual native evidence: %+v",observed)
 }
 if !strings.Contains(observed["text_excerpt"].(string),"SEARCH COVERAGE: PARTIAL"){t.Fatal("native TXT was not provided to Agent")}
 if observed["result_zip"]!=zipRel||observed["result_text"]!=txtRel{t.Fatal("manual ZIP/TXT companion was not retained")}
 // A different project must NEVER inherit the positive evidence status.
 other:=patchtool.ProjectIdentity{Key:"foreign",ID:identity.ID}
 var denied map[string]any
 _=json.Unmarshal([]byte(agentNativeCollectionObservation(root,other,"native-session",item,gotZip,gotTxt)),&denied)
 if denied["evidence_verified"]==true{t.Fatal("cross-project Collection evidence was trusted")}
 if denied["text_excerpt"]!=nil{t.Fatal("foreign native TXT source was leaked to Agent before identity verification")}
}

func TestAgentNativeCollectionArtifactSecurityAnd150MBSparseTXT(t *testing.T){
 root:=t.TempDir()
 identity:=patchtool.ProjectIdentity{Key:"server-patching",ID:"4fb18d97-cbc2-433b-8234-538b32ed0189"}
 _,txt:=writeAgentNativeCollectionArtifacts(t,root,"CODE_COLLECTION_RESULT_large_20261123","INCOMPLETE",identity)
 path:=filepath.Join(root,filepath.FromSlash(txt))
 file,err:=os.OpenFile(path,os.O_RDWR,0600);if err!=nil{t.Fatal(err)}
 if err=file.Truncate(150<<20);err!=nil{t.Fatal(err)}
 if _,err=file.WriteAt([]byte("TAIL DIAGNOSIS: native COLLECT preserved evidence\n"),(150<<20)-1024);err!=nil{t.Fatal(err)}
 if err=file.Close();err!=nil{t.Fatal(err)}
 excerpt,truncated,err:=agentNativeCollectionTextExcerpt(root,txt)
 if err!=nil||!truncated||len(excerpt)>agentNativeCollectionTextBudget+300{
  t.Fatalf("large native companion was not bounded: truncated=%v len=%d err=%v",truncated,len(excerpt),err)
 }
 if !strings.Contains(excerpt,"Native COLLECT result"){t.Fatal("manual result header lost")}
 for _,bad:=range []string{"../../etc/passwd","/etc/passwd","artifacts/outside.txt",
  "artifacts/patch_tool_code_collections/NOT_A_COLLECT.txt"} {
  if _,err:=agentNativeCollectionArtifactPath(root,bad,".txt");err==nil{t.Fatalf("unsafe artifact accepted: %s",bad)}
 }
}

func TestAgentNativeCollectionProductionCannotFallbackToIndependentReader(t *testing.T){
 s:=&Server{Workspace:t.TempDir()}
 identity:=patchtool.ProjectIdentity{Key:"test",ID:"00000000-0000-4000-8000-000000000001"}
 _,err:=s.agentRunNativeCollection(context.Background(),"local",[]byte(`{"actions":[{"type":"ls","path":"."}]}`),identity)
 if err==nil||!strings.Contains(err.Error(),"native Patch Tool session service unavailable"){
  t.Fatalf("missing native backend must fail closed, not invoke old reader: %v",err)
 }
}
