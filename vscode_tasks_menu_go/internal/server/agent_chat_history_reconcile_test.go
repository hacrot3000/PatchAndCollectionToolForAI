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
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"

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


func TestAgentHistoryRecoversExactDuplicateSkipNotEarlierPass(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,true)
 store,_:=s.agentChatStore()
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
 name:="PATCH_CLEAR_TEXT_new-regression.zip"
 prior:="PATCH_CLEAR_TEXT_old-regression.zip"
 raw:=[]byte("same approved command-only ZIP bytes")
 digest:=sha256.Sum256(raw)
 call:=x.ToolCalls[0]
 now:=time.Now().UTC()
 call.Status="running";call.PatchSessionID="old-native-session";call.PromptID="p";call.SelectedIndex=1
 call.QueuePath="patchs/"+name;call.PackageSize=int64(len(raw));call.PackageSHA256=hex.EncodeToString(digest[:]);call.NativeStartedAt=&now
 ignore:="patchs/ignore/2026-11-25-"+name
 for _,rel:=range []string{ignore,"patchs/patched/"+prior}{
  p:=filepath.Join(s.Workspace,filepath.FromSlash(rel));if err=os.MkdirAll(filepath.Dir(p),0700);err!=nil{t.Fatal(err)}
  if err=os.WriteFile(p,raw,0600);err!=nil{t.Fatal(err)}
 }
 history:=filepath.Join(s.Workspace,"artifacts","patch_tool","history")
 if err=os.MkdirAll(history,0700);err!=nil{t.Fatal(err)}
 write:=func(file,run,selected string,executed []string,row map[string]any,skips []any){
  b,e:=json.Marshal(map[string]any{"format":"python-patch-tool-last-run","format_version":2,"run_id":run,
   "project_identity":identity,"finished_at":now.Add(time.Second).Format(time.RFC3339Nano),"selected":[]string{selected},"execution_order":executed,
   "results":[]any{row},"local_history_skipped":skips});if e!=nil{t.Fatal(e)}
  if e=os.WriteFile(filepath.Join(history,file),b,0600);e!=nil{t.Fatal(e)}
 }
 write("old.json","old-run",prior,[]string{prior},map[string]any{"name":prior,"kind":"PATCH","status":"PASS","rc":0},nil)
 if _,_,_,ok:=s.agentFinalResultFromHistory(call);ok{t.Fatal("previous same-hash PASS adopted")}
 skip:=map[string]any{"name":name,"kind":"PATCH","status":"SKIPPED_DUPLICATE_LOCAL","rc":0,"ignore_path":ignore}
 metadata:=[]any{map[string]any{"name":name,"sha256":call.PackageSHA256,"ignore_path":ignore}}
 write("skip.json","new-run",name,[]string{},skip,metadata)
 status,result,run,ok:=s.agentFinalResultFromHistory(call)
 if !ok||status!="error"||run!="new-run"||!agentDuplicateSkipResult(result)||!strings.Contains(result,`"patch_executed":false`){t.Fatalf("duplicate receipt: %s %s %s %v",status,result,run,ok)}
 view:=agentToolReconcileView{Record:call};s.agentReconcilePatchRecord(&view)
 if !view.Changed||view.Record.Status!="error"||agentFailedPatchRetirementEligible(view.Record){t.Fatal("skip not reconciled safely")}
 x.ToolCalls[0]=call;x.Status="waiting_tool";x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 // Check explicit and background sync use the same read-only proof.
 s.Sessions=&fakeAgentPatchProtocol{expectedID:"other"}
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/sync",map[string]any{})
 if w.Code!=200{t.Fatal(w.Body.String())}
 fresh,err:=store.Get(id,"local");if err!=nil||fresh.ToolCalls[0].Status!="error"||fresh.ToolCalls[0].FeedbackSent{t.Fatalf("sync: %+v %v",fresh.ToolCalls,err)}
 // Reject path substitution and changed bytes even with a matching History row.
 if s.agentVerifyDuplicateIgnore(call,"patchs/ignore/../"+name){t.Fatal("traversal accepted")}
 if err=os.WriteFile(filepath.Join(s.Workspace,filepath.FromSlash(ignore)),[]byte(strings.Repeat("x",len(raw))),0600);err!=nil{t.Fatal(err)}
 if _,_,_,ok=s.agentFinalResultFromHistory(call);ok{t.Fatal("changed ignore archive accepted")}
}

type exitedAgentNativeFixture struct{fakeAgentPatchProtocol}
func(f *exitedAgentNativeFixture) Metadata(id string)(session.Metadata,bool){
 if id!=f.expectedID{return session.Metadata{},false}
 return session.Metadata{ID:id,Status:"exited"},true
}

func TestAgentExitedNativeWithoutReceiptDoesNotRemainRunning(t *testing.T){
 s:=&Server{Workspace:t.TempDir(),Sessions:&exitedAgentNativeFixture{fakeAgentPatchProtocol{expectedID:"exited"}}}
 view:=agentToolReconcileView{Record:agentchat.ToolCallRecord{Status:"running",PatchSessionID:"exited"}}
 s.agentReconcilePatchRecord(&view)
 if !view.Changed||view.Record.Status!="unknown"||view.Record.FeedbackSent{t.Fatalf("unsafe stale status: %+v",view)}
}
