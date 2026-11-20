package server

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/patchtool"
 "bletonfc/vscode_tasks_menu/internal/session"
)

func TestAgentSyncAdoptsArchivedNativePatchHistory(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil { t.Fatal(err) }
 x,err:=store.Get(id,"local")
 if err!=nil { t.Fatal(err) }
 x.ToolCalls[0].Status="running"
 x.ToolCalls[0].PatchSessionID="expired-native-patch-session"
 x.ToolCalls[0].QueuePath="patchs/PATCH_CLEAR_TEXT_Create-root-helloworld-php_20261120T041232_7db6ac85.zip"
 x.ToolCalls[0].SelectedIndex=1
 x.ToolCalls[0].PromptID="native-prompt"
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace)
 if err!=nil { t.Fatal(err) }
 packageBytes:=[]byte("exact approved archived patch package")
 sha:=sha256.Sum256(packageBytes)
 x.ToolCalls[0].PackageSHA256=hex.EncodeToString(sha[:])
 x.ToolCalls[0].PackageSize=int64(len(packageBytes))
 x,err=store.Save(x,x.Revision)
 if err!=nil { t.Fatal(err) }
 archive:=filepath.Join(s.Workspace,"patchs","patched")
 if err:=os.MkdirAll(archive,0700); err!=nil { t.Fatal(err) }
 name:=filepath.Base(x.ToolCalls[0].QueuePath)
 if err:=os.WriteFile(filepath.Join(archive,name),packageBytes,0600); err!=nil { t.Fatal(err) }
 history:=filepath.Join(s.Workspace,"artifacts","patch_tool","history")
 if err:=os.MkdirAll(history,0700); err!=nil { t.Fatal(err) }
 report:=map[string]any{
  "format":"python-patch-tool-last-run", "format_version":2,
  "run_id":"1795147952886097_4107775",
  "project_identity":identity,
  "selected":[]string{name},
  "execution_order":[]string{name},
  "results":[]map[string]any{{"name":name,"kind":"PATCH","status":"PASS","rc":0}},
 }
 payload,err:=json.Marshal(report)
 if err!=nil { t.Fatal(err) }
 if err:=os.WriteFile(filepath.Join(history,"20261120_native.json"),payload,0600); err!=nil { t.Fatal(err) }
 // Native terminal has exited; its in-memory protocol state is no longer
 // available. The authenticated History report must be enough to reconcile.
 s.Sessions=&fakeAgentPatchProtocol{expectedID:"different-expired-session",state:session.ProtocolState{}}
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/sync",map[string]any{})
 if w.Code!=200 { t.Fatalf("history sync failed: %d %s",w.Code,w.Body) }
 y,err:=store.Get(id,"local")
 if err!=nil { t.Fatal(err) }
 if y.ToolCalls[0].Status!="done" || !strings.Contains(y.ToolCalls[0].Result,`"PASS"`) ||
   y.ToolCalls[0].RunID!="1795147952886097_4107775" {
  t.Fatalf("native PASS not recovered from exact archive and history: %+v",y.ToolCalls[0])
 }
 if y.Status!="waiting_tool" { t.Fatalf("verified result must remain ready for AI feedback: %s",y.Status) }
}

func TestAgentSyncRejectsHistoryWithWrongArchivedDigest(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,_:=s.agentChatStore()
 x,err:=store.Get(id,"local")
 if err!=nil { t.Fatal(err) }
 x.ToolCalls[0].Status="running"
 x.ToolCalls[0].PatchSessionID="live-session"
 x.ToolCalls[0].QueuePath="patchs/approved.zip"
 x.ToolCalls[0].SelectedIndex=1
 x.ToolCalls[0].PackageSize=5
 x.ToolCalls[0].PackageSHA256=strings.Repeat("a",64)
 x,err=store.Save(x,x.Revision)
 if err!=nil { t.Fatal(err) }
 dir:=filepath.Join(s.Workspace,"patchs","patched")
 if err:=os.MkdirAll(dir,0700); err!=nil { t.Fatal(err) }
 if err:=os.WriteFile(filepath.Join(dir,"approved.zip"),[]byte("wrong"),0600); err!=nil { t.Fatal(err) }
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace)
 if err!=nil { t.Fatal(err) }
 path:=filepath.Join(s.Workspace,"artifacts","patch_tool","history")
 if err:=os.MkdirAll(path,0700); err!=nil { t.Fatal(err) }
 b,_:=json.Marshal(map[string]any{
  "format":"python-patch-tool-last-run","format_version":2,"run_id":"run-unsafe",
  "project_identity":identity,"selected":[]string{"approved.zip"},
  "execution_order":[]string{"approved.zip"},
  "results":[]map[string]any{{"name":"approved.zip","kind":"PATCH","status":"PASS","rc":0}},
 })
 if err:=os.WriteFile(filepath.Join(path,"fake.json"),b,0600); err!=nil { t.Fatal(err) }
 s.Sessions=&fakeAgentPatchProtocol{expectedID:"live-session",state:session.ProtocolState{Available:true}}
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/sync",map[string]any{})
 if w.Code!=200 { t.Fatalf("unexpected sync response: %d %s",w.Code,w.Body) }
 y,err:=store.Get(id,"local")
 if err!=nil { t.Fatal(err) }
 if y.ToolCalls[0].Status!="running" || y.Revision!=x.Revision {
  t.Fatal("incorrect archive digest caused false success")
 }
}
