package server

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestDurableNativeTaskReceiptSurvivesSessionEvictionAndRejectsForgedHistory(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("native Task PTY fixture uses /bin/sh")}
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 planPath:="/api/agent/sessions/"+id+"/plan"
 created:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,planPath,map[string]any{
  "action":"create","expected_revision":x.Revision,"goal":"Record a verifiable native Task",
  "steps":[]map[string]string{{"id":"test","title":"Run test Task","acceptance":"Verify exact native Task command and exit status"}},
 })
 if created.Code!=200{t.Fatalf("cannot create plan: %d %s",created.Code,created.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 manager:=session.NewManager(1<<20);s.Sessions=manager
 defer manager.Shutdown(time.Second)
 launch:=func(code int)session.Metadata{
  t.Helper()
  script:="exit 0"
  if code!=0{script="exit 7"}
  meta,err:=manager.Start(tasks.Execution{TaskID:42,Label:"Test Task",Command:"/bin/sh",
   Args:[]string{"-c",script},Cwd:s.Workspace,Env:os.Environ(),Preview:"/bin/sh -c "+script})
  if err!=nil{t.Fatal(err)}
  return waitTaskRunSession(t,manager,meta.ID)
 }
 record:=func(meta session.Metadata,log string)taskRunRecord{
  t.Helper()
  body,_:=json.Marshal(taskRunRecordRequest{SessionID:meta.ID,Log:log})
  req:=httptest.NewRequest(http.MethodPost,"/api/task-runs",bytes.NewReader(body))
  req.Header.Set("Content-Type","application/json")
  rr:=httptest.NewRecorder();s.Handler().ServeHTTP(rr,req)
  if rr.Code!=http.StatusCreated{t.Fatalf("cannot record Task history: %d %s",rr.Code,rr.Body.String())}
  var out taskRunRecord
  if err:=json.Unmarshal(rr.Body.Bytes(),&out);err!=nil{t.Fatal(err)}
  return out
 }
 bad:=launch(7)
 badHistory:=record(bad,"client supplied: PASS")
 if badHistory.VerifiedReceipt{t.Fatal("failed Task earned signed success proof from fake browser log")}
 if err:=manager.Remove(bad.ID);err!=nil{t.Fatal(err)}
 if _,err:=s.verifyNativeTaskRunEvidence(x,bad.ID,42,"");err==nil{
  t.Fatal("history row or spoofed browser PASS was treated as native success")
 }
 good:=launch(0)
 saved:=record(good,"client supplied log may contain anything")
 if !saved.VerifiedReceipt{t.Fatal("successful finished Task did not issue private completion receipt")}
 dir,err:=s.taskRunReceiptDirectory(false);if err!=nil{t.Fatal(err)}
 file,err:=nativeTaskRunReceiptFile(dir,good.ID);if err!=nil{t.Fatal(err)}
 if strings.HasPrefix(file,s.Workspace+string(filepath.Separator)){t.Fatal("receipt was persisted in untrusted project history")}
 if err:=manager.Remove(good.ID);err!=nil{t.Fatal(err)}
 if _,ok:=manager.Metadata(good.ID);ok{t.Fatal("native Task was not evicted")}
 // Legacy JSON metadata and browser logs lack a host-private signed receipt.
 if _,err:=s.verifyNativeTaskRunEvidence(x,"invented-session-id",42,"");err==nil{
  t.Fatal("missing native Task session accepted from history")
 }
 // A wrong Task ID must fail even for an authentic signed receipt.
 if _,err:=s.verifyNativeTaskRunEvidence(x,good.ID,41,"");err==nil{
  t.Fatal("valid receipt accepted for another Task")
 }
 // Modify the private receipt's signed metadata, retaining the old MAC.
 raw,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
 var signed signedTaskRunReceipt
 if err:=json.Unmarshal(raw,&signed);err!=nil{t.Fatal(err)}
 signed.Payload.Metadata.TaskID=99
 changed,err:=json.Marshal(signed);if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(file,changed,0600);err!=nil{t.Fatal(err)}
 if _,err:=s.verifyNativeTaskRunEvidence(x,good.ID,42,"");err==nil{
  t.Fatal("tampered signed Task metadata was accepted")
 }
 if err:=os.WriteFile(file,raw,0600);err!=nil{t.Fatal(err)}
 // The same successful Task remains verifiable after native eviction.
 // No browser-provided log, task-history status or rerun is needed.
 verified,err:=s.verifyNativeTaskRunEvidence(x,good.ID,42,"Reviewed actual task command")
 if err!=nil||verified.Kind!="task_run"||len(verified.Digest)!=64{
  t.Fatalf("signed receipt not accepted after eviction: %+v %v",verified,err)
 }
 y,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 done:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,planPath,map[string]any{
  "action":"set_step","expected_revision":y.Revision,"step_id":"test","status":"done",
  "evidence_kind":"task_run","evidence_ref":good.ID,"evidence_task_id":42,
  "evidence_note":"Reviewed exact unit test command",
 })
 if done.Code!=200{t.Fatalf("signed Task receipt refused by plan API: %d %s",done.Code,done.Body.String())}
 reopened,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 later,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
 if later.TaskPlan.State=="completed"||later.TaskPlan.Steps[0].Status!="done"||
  later.TaskPlan.Steps[0].Evidence[0].Ref!=good.ID{
  t.Fatalf("retained native receipt not adopted without completing overall plan: %+v",later.TaskPlan)
 }
}

func TestSignedNativeTaskReceiptProjectKeyAndOwnerTamperingFailClosed(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("native POSIX Task fixture")}
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=agentchat.NewTaskPlan("Verify Task against project",[]agentchat.TaskPlanStep{
  {ID:"run",Title:"Execute tests",Acceptance:"Exactly this Task run exited zero"},
 })
 if err!=nil{t.Fatal(err)}
 meta:=session.Metadata{
  ID:"native-signed-01",TaskID:12,Kind:tasks.SessionKindTask,Label:"Tests",CommandPreview:"go test ./...",
  CommandSHA256:strings.Repeat("a",64),Cwd:s.Workspace,Status:"exited",
  StartedAt:time.Now().UTC().Format(time.RFC3339),EndedAt:time.Now().UTC().Format(time.RFC3339),
 }
 zero:=0;meta.ExitCode=&zero
 if err:=s.recordNativeTaskRunReceipt(meta);err!=nil{t.Fatal(err)}
 if _,err:=s.verifiedNativeTaskRunReceipt(meta.ID,"not-the-project");err==nil{
  t.Fatal("signed Task receipt crossed project ID boundary")
 }
 dir,err:=s.taskRunReceiptDirectory(false);if err!=nil{t.Fatal(err)}
 keyFile:=filepath.Join(dir,"receipt.key")
 key,err:=os.ReadFile(keyFile);if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(keyFile,[]byte("invalid key"),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.verifiedNativeTaskRunReceipt(meta.ID,x.ProjectID);err==nil{
  t.Fatal("corrupted private signing key was accepted")
 }
 if err:=os.WriteFile(keyFile,key,0600);err!=nil{t.Fatal(err)}
 if _,err:=s.verifiedNativeTaskRunReceipt(meta.ID,x.ProjectID);err!=nil{
  t.Fatalf("authentic retained receipt lost after restoration: %v",err)
 }
 if _,err:=verifyNativeTaskRunMetadata(s.Workspace,true,x,meta,meta.TaskID,"");err==nil{
  t.Fatal("unsigned Task owner mismatch accepted as shared evidence")
 }
}
