package server

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

func agentPendingCallFixture(t *testing.T, valid bool)(*Server,string,string){
 t.Helper()
 s,id:=newAgentConversationFixture(t,"stateless")
 identity,err:=patchtool.EnsureProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
 code:="# TASKDECK_PROJECT_KEY: "+identity.Key+"\n# TASKDECK_PROJECT_ID: "+identity.ID+"\nprint('test patch')\n"
 if !valid {code="# TASKDECK_PROJECT_KEY: invalid\n# TASKDECK_PROJECT_ID: invalid\nprint('test patch')\n"}
 args,_:=json.Marshal(map[string]string{"name":"agent-check","patch_python":code})
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error){
  var req map[string]any
  if err:=json.Unmarshal(payload,&req);err!=nil{t.Fatal(err)}
  if _,ok:=req["tools"];!ok{t.Fatal("tool schema was not advertised")}
  return agentchat.ResponsesResult{ResponseID:"resp_pending",Calls:[]agentchat.FunctionCall{{
   CallID:"call_agent_001",Name:"taskdeck_submit_patch",Arguments:string(args),
  }}},nil
 }
 first:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"Propose patch","confirm_external":true,"expected_revision":1})
 if first.Code!=200{t.Fatalf("provider call failed: %d %s",first.Code,first.Body)}
 return s,id,code
}
func TestAgentToolApprovalQueuesUsingNativePatchToolOnce(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,true)
 store,e:=s.agentChatStore();if e!=nil{t.Fatal(e)}
 before,e:=store.Get(id,"local");if e!=nil{t.Fatal(e)}
 if len(before.ToolCalls)!=1||before.ToolCalls[0].Status!="pending"{t.Fatalf("tool intent not persisted: %+v",before.ToolCalls)}
 path:="/api/agent/sessions/"+id+"/tools/call_agent_001/decision"
 missing:=invokeAI(t,s.agentChatSessionItemAPI,"POST",path,map[string]any{
  "action":"approve","expected_revision":before.Revision,
 })
 if missing.Code!=403{t.Fatalf("missing explicit execution approval accepted: %d",missing.Code)}
 approved:=invokeAI(t,s.agentChatSessionItemAPI,"POST",path,map[string]any{
  "action":"approve","confirm_execution":true,"expected_revision":before.Revision,
 })
 if approved.Code!=200{t.Fatalf("native queue submission failed: %d %s",approved.Code,approved.Body)}
 after,e:=store.Get(id,"local");if e!=nil{t.Fatal(e)}
 if after.ToolCalls[0].Status!="queued"||after.ToolCalls[0].QueuePath==""{
  t.Fatalf("missing native queue receipt: %+v",after.ToolCalls)
 }
 queueFile:=filepath.Join(s.Workspace,filepath.FromSlash(after.ToolCalls[0].QueuePath))
 info,e:=os.Stat(queueFile);if e!=nil||!info.Mode().IsRegular(){t.Fatalf("native Patch queue package missing: %v",e)}
 again:=invokeAI(t,s.agentChatSessionItemAPI,"POST",path,map[string]any{
  "action":"approve","confirm_execution":true,"expected_revision":after.Revision,
 })
 if again.Code!=409{t.Fatalf("duplicate patch approval accepted: %d %s",again.Code,again.Body)}
 files,e:=os.ReadDir(filepath.Join(s.Workspace,"patchs"));if e!=nil{t.Fatal(e)}
 if len(files)!=1{t.Fatalf("repeat generated additional patches: %d",len(files))}
}
func TestAgentToolRejectDoesNotQueueAndIsImmutable(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,false)
 before,e:=s.agentChatStore();if e!=nil{t.Fatal(e)}
 x,e:=before.Get(id,"local");if e!=nil{t.Fatal(e)}
 path:="/api/agent/sessions/"+id+"/tools/call_agent_001/decision"
 result:=invokeAI(t,s.agentChatSessionItemAPI,"POST",path,map[string]any{"action":"reject","expected_revision":x.Revision})
 if result.Code!=200{t.Fatalf("reject failed: %d %s",result.Code,result.Body)}
 x,e=before.Get(id,"local");if e!=nil{t.Fatal(e)}
 if x.ToolCalls[0].Status!="rejected"||x.Status!="tool_rejected"{t.Fatalf("rejection not persisted %+v",x)}
 if _,err:=os.Stat(filepath.Join(s.Workspace,"patchs"));!os.IsNotExist(err){t.Fatalf("reject touched queue unexpectedly: %v",err)}
 repeat:=invokeAI(t,s.agentChatSessionItemAPI,"POST",path,map[string]any{"action":"approve","confirm_execution":true,"expected_revision":x.Revision})
 if repeat.Code!=409{t.Fatalf("rejected patch accepted later: %d",repeat.Code)}
}
func TestAgentToolInvalidNativeProjectIdentityRefusedBeforeQueue(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,false)
 store,e:=s.agentChatStore();if e!=nil{t.Fatal(e)}
 x,e:=store.Get(id,"local");if e!=nil{t.Fatal(e)}
 result:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
  map[string]any{"action":"approve","confirm_execution":true,"expected_revision":x.Revision})
 if result.Code!=400{t.Fatalf("invalid patch identity accepted: %d %s",result.Code,result.Body)}
 now,e:=store.Get(id,"local");if e!=nil{t.Fatal(e)}
 if now.Revision!=x.Revision||now.ToolCalls[0].Status!="pending"{t.Fatalf("bad patch changed queue or journal: %+v",now.ToolCalls)}
}
func TestAgentToolPermissionMappedAndBoundedArguments(t *testing.T){
 r:=httptest.NewRequest(http.MethodPost,"/api/agent/sessions/id/tools/call/decision",nil)
 permissions:=sharedRoutePermissions(r)
 if len(permissions)!=2||permissions[0]!=identity.PermissionSettingsRead||permissions[1]!=identity.PermissionPatchRun{
  t.Fatalf("Agent tool bypasses patch.run permission: %+v",permissions)
 }
 for _,raw:=range []string{
  "{}",
  `{"name":"patch","patch_python":"test","unknown":"field"}`,
  `{"name":"patch","patch_python":""}`,
  `{"name":"","patch_python":"print(1)"}`,
  strings.Repeat("a",512<<10+1),
 }{
  if _,e:=parseAgentPatchArguments(raw);e==nil{t.Fatalf("invalid arguments accepted (len=%d): %s",len(raw),raw[:func()int{if len(raw)<150{return len(raw)};return 150}()])}
 }
}
