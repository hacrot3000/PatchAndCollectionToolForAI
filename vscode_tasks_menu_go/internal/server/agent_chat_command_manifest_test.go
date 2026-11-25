package server

import (
 "archive/zip"
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "context"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func testAgentCommandManifest(identity patchtool.ProjectIdentity,argv []string)string{
 b,_:=json.Marshal(map[string]any{
  "schema_version":1,"project":identity,"patch":map[string]string{"id":"regression-existing-script"},
  "post_patch":map[string]any{"commands":[]any{map[string]any{"argv":argv,"cwd":".","timeout_seconds":300}},
   "run_when_no_changes":true,"no_change_reason":"Run the existing regression without changing project source."},
 })
 return string(b)
}

func TestAgentCommandManifestNativePasteAndTextFallback(t *testing.T){
 s:=&Server{Workspace:t.TempDir()}
 identity,err:=patchtool.EnsureProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
 raw:=testAgentCommandManifest(identity,[]string{"bash","scripts/regression.sh"})
 files:=openQueuedPaste(t,s.Workspace,postPatchPaste(t,s,"patch-manifest","regression",raw))
 if len(files)!=1{t.Fatalf("unexpected payload: %+v",files)}
 m,err:=parseCommandOnlyManifest(string(files["PATCH_TOOL_MANIFEST.json"]))
 if err!=nil||m.Project!=identity||m.PostPatch.SafetyProfile!="legacy_strict"||m.PostPatch.Commands[0].Argv[1]!="scripts/regression.sh"{t.Fatalf("wrong manifest: %+v %v",m,err)}
 env,_:=json.Marshal(map[string]any{"type":"taskdeck_patch_proposal","project":identity,"name":"regression","patch_python":"","patch_manifest":raw})
 args,err:=parseAgentTextPatchProposal(string(env),identity)
 if err!=nil||args.PatchManifest!=raw||args.PatchPython!=""{t.Fatalf("text fallback failed: %v",err)}
}

func TestAgentCommandManifestRejectsUnsafeShape(t *testing.T){
 identity:=patchtool.ProjectIdentity{Key:"project",ID:"project-id"}
 raw:=testAgentCommandManifest(identity,[]string{"bash","scripts/regression.sh"})
 for _,argv:=range [][]string{{"sudo","true"},{"bash","-c","id"},{"python3","-m","module"},{"/bin/bash","scripts/regression.sh"},{"bash","../outside.sh"},{"bash","/tmp/outside.sh"},{"unknown-command"}}{
  if _,err:=pastedCommandOnlyPackage(testAgentCommandManifest(identity,argv),identity);err==nil{t.Fatalf("unsafe argv admitted: %v",argv)}
 }
 for _,extra:=range []string{`"payload":{}`,`"git":{"push":true}`,`"batch":{"previous_failure":{"action":"delete","reason":"ignore"}}`,`"remote_ssh":{}`,`"on_failure":{}`,`"validation":{}`}{
  bad:=strings.TrimSuffix(raw,"}")+","+extra+"}"
  if _,err:=pastedCommandOnlyPackage(bad,identity);err==nil{t.Fatalf("unexpected field admitted: %s",extra)}
 }
 for _,bad:=range []string{strings.Replace(raw,`"cwd":"."`,`"cwd":"scripts"`,1),strings.Replace(raw,`"run_when_no_changes":true`,`"run_when_no_changes":false`,1),strings.Replace(raw,`"post_patch":{`,`"post_patch":{"safety_profile":"current",`,1),raw+" {}"}{
  if _,err:=pastedCommandOnlyPackage(bad,identity);err==nil{t.Fatal("invalid command manifest accepted")}
 }
 if _,err:=pastedCommandOnlyPackage(raw,patchtool.ProjectIdentity{Key:"other",ID:identity.ID});err==nil{t.Fatal("foreign project accepted")}
 both,_:=json.Marshal(agentPatchArguments{Name:"both",PatchPython:"print(1)",PatchManifest:raw})
 if _,err:=parseAgentPatchArguments(string(both));err==nil{t.Fatal("mixed payload accepted")}
 old:=`{"name":"legacy","patch_python":"print(1)"}`
 if _,err:=parseAgentPatchArguments(old);err!=nil{t.Fatalf("old Python proposal rejected: %v",err)}
}

func TestAgentCommandManifestApprovalQueuesExactlyOnce(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 identity,err:=patchtool.EnsureProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
 if err=os.MkdirAll(filepath.Join(s.Workspace,"scripts"),0700);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(filepath.Join(s.Workspace,"scripts","regression.sh"),[]byte("#!/bin/sh\nexit 0\n"),0600);err!=nil{t.Fatal(err)}
 raw:=testAgentCommandManifest(identity,[]string{"bash","scripts/regression.sh"})
 args,_:=json.Marshal(agentPatchArguments{Name:"run regression",PatchManifest:raw})
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  return agentchat.ResponsesResult{ResponseID:"manifest-proposal",Calls:[]agentchat.FunctionCall{{CallID:"regression-call",Name:"taskdeck_submit_patch",Arguments:string(args)}}},nil
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",map[string]any{"message":"Run existing regression","confirm_external":true,"expected_revision":1})
 if w.Code!=200{t.Fatalf("proposal: %d %s",w.Code,w.Body)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if len(x.ToolCalls)!=1||x.ToolCalls[0].Status!="pending"{t.Fatal("proposal not pending")}
 route:=fmt.Sprintf("/api/agent/sessions/%s/tools/regression-call/decision",id)
 w=invokeAI(t,s.agentChatSessionItemAPI,"POST",route,map[string]any{"action":"approve","expected_revision":x.Revision})
 if w.Code!=403{t.Fatalf("approval without consent: %d",w.Code)}
 w=invokeAI(t,s.agentChatSessionItemAPI,"POST",route,map[string]any{"action":"approve","confirm_execution":true,"expected_revision":x.Revision})
 if w.Code!=200{t.Fatalf("queue: %d %s",w.Code,w.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 call:=x.ToolCalls[0]
 if call.Status!="queued"||call.QueuePath==""||len(call.PackageSHA256)!=64{t.Fatalf("invalid receipt: %+v",call)}
 z,err:=zip.OpenReader(filepath.Join(s.Workspace,filepath.FromSlash(call.QueuePath)));if err!=nil{t.Fatal(err)}
 defer z.Close()
 if len(z.File)!=1||z.File[0].Name!="PATCH_TOOL_MANIFEST.json"{t.Fatal("not a manifest-only ZIP")}
 w=invokeAI(t,s.agentChatSessionItemAPI,"POST",route,map[string]any{"action":"approve","confirm_execution":true,"expected_revision":x.Revision})
 if w.Code!=409{t.Fatal("duplicate approval was admitted")}
}
