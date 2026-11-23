package server

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentNativeCollectPythonPreflightRejectsThreeInvalidRequestsWithoutQueue(t *testing.T){
 if _,err:=exec.LookPath("python3");err!=nil{t.Skip("python3 unavailable")}
 bundle,err:=filepath.Abs(filepath.Join("..","..","..","python_patch_entry.py"))
 if err!=nil{t.Fatal(err)}
 if _,err=os.Stat(bundle);err!=nil{t.Skip("bundled Python Patch Tool source unavailable")}
 t.Setenv("TASKDECK_PATCH_RUNTIME",bundle)
 root:=t.TempDir()
 identity:=patchtool.ProjectIdentity{Key:"game_automation_client",ID:"6159c0dc-b25e-4cf8-bfa3-1539965d3b71"}
 config,_:=json.Marshal(map[string]any{"project":identity})
 if err=os.WriteFile(filepath.Join(root,".python_patch_tool.json"),config,0600);err!=nil{t.Fatal(err)}
 s:=&Server{Workspace:root}
 cases:=[]struct{name string; actions string; failing bool}{
  {"unsupported read",`[{"type":"read","path":"README.md"}]`,true},
  {"git missing operations",`[{"type":"git","operations":[]}]`,true},
  {"pack empty paths",`[{"type":"pack","paths":[]}]`,true},
  {"valid overview",`[{"type":"overview","path":"."}]`,false},
 }
 for _,c:=range cases{
  t.Run(c.name,func(t *testing.T){
   raw:=[]byte(fmt.Sprintf(`{"project":{"key":%q,"id":%q},"actions":%s}`,identity.Key,identity.ID,c.actions))
   err:=s.agentPrevalidateNativeCollect(context.Background(),raw)
   if c.failing {
    var schema *agentNativeCollectValidationError
    if !errors.As(err,&schema){t.Fatalf("must be native schema failure: %v",err)}
   } else if err!=nil {t.Fatalf("valid request rejected: %v",err)}
   packages,_:=filepath.Glob(filepath.Join(root,"patchs","*.zip"))
   if len(packages)!=0{t.Fatalf("Agent preflight created invalid queue ZIP: %v",packages)}
  })
 }
}

func TestAgentNativeCollectPythonPreflightRejectsForeignProject(t *testing.T){
 if _,err:=exec.LookPath("python3");err!=nil{t.Skip("python3 unavailable")}
 bundle,err:=filepath.Abs(filepath.Join("..","..","..","python_patch_entry.py"))
 if err!=nil{t.Fatal(err)}
 if _,err=os.Stat(bundle);err!=nil{t.Skip("bundled runtime not available")}
 t.Setenv("TASKDECK_PATCH_RUNTIME",bundle)
 root:=t.TempDir()
 if err=os.WriteFile(filepath.Join(root,".python_patch_tool.json"),
  []byte(`{"project":{"key":"project-A","id":"6159c0dc-b25e-4cf8-bfa3-1539965d3b71"}}`),0600);err!=nil{t.Fatal(err)}
 s:=&Server{Workspace:root}
 request:=[]byte(`{"project":{"key":"foreign","id":"6159c0dc-b25e-4cf8-bfa3-1539965d3b71"},"actions":[{"type":"overview"}]}`)
 err=s.agentPrevalidateNativeCollect(context.Background(),request)
 var schema *agentNativeCollectValidationError
 if !errors.As(err,&schema)||!strings.Contains(schema.Detail,"project.key/project.id"){
  t.Fatalf("foreign workspace preflight must fail closed: %v",err)
 }
}

func TestAgentNativeCollectUsesManualParallelCollectWorkerSpec(t *testing.T){
 bundle,err:=filepath.Abs(filepath.Join("..","..","..","python_patch_entry.py"))
 if err!=nil{t.Fatal(err)}
 if _,err=os.Stat(bundle);err!=nil{t.Skip("bundled runtime not available")}
 t.Setenv("TASKDECK_PATCH_RUNTIME",bundle)
 root:=t.TempDir()
 spec,err:=patchCollectExecution(root,"CODE_COLLECTION_REQUEST_CLEAR_TEXT_agent-collection_valid.zip")
 if err!=nil{t.Fatal(err)}
 joined:=fmt.Sprint(spec.Args)
 if !strings.Contains(joined,"collect")||!strings.Contains(joined,"request")||
  !strings.Contains(joined,"CODE_COLLECTION_REQUEST_CLEAR_TEXT_agent-collection_valid.zip")||
  !spec.ProtocolEvents||spec.ProtocolCommands {
  t.Fatalf("Agent must use native manual Parallel COLLECT process, not custom reader: %+v",spec)
 }
}
