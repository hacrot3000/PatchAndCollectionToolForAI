//go:build linux || darwin

package agentchat

import (
 "bufio"
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

const (
 postCommitCrashChildEnv = "TASKDECK_AGENT_TEST_POST_COMMIT_CRASH"
 postCommitCrashMarker = "TASKDECK_STATUS_JOURNAL_COMMITTED_CLEANUP_NOT_RUN"
)

// The child enters the production single-segment status mutation and pauses
// INSIDE Store.withLock after the canonical journal's atomic rename. A real
// parent-initiated OS kill then leaves the previous immutable segment orphaned.
func taskPlanPostCommitCrashChild() {
 dir := os.Getenv("TASKDECK_AGENT_TEST_POST_COMMIT_DIR")
 id := os.Getenv("TASKDECK_AGENT_TEST_POST_COMMIT_SESSION")
 store,err:=NewStore(dir)
 if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(2)}
 before,err:=store.Get(id,"crash-owner")
 if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(2)}
 store.afterSegmentStatusJournalCommit=func(){
  fmt.Fprintln(os.Stdout,postCommitCrashMarker)
  time.Sleep(30*time.Second) // Must be terminated by the parent.
  os.Exit(3)
 }
 _,err=store.SetSegmentTaskPlanStepStatus(
  id,"crash-owner",before.Revision,"step-0000","blocked",0,24)
 fmt.Fprintln(os.Stderr,"unexpected return from post-commit barrier:",err)
 os.Exit(4)
}

func TestTaskPlanForcedCrashAfterJournalCommitBeforeSegmentCleanup(t *testing.T) {
 if os.Getenv(postCommitCrashChildEnv)=="1"{
  taskPlanPostCommitCrashChild()
  return
 }
 dir:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(dir)
 if err!=nil{t.Fatal(err)}
 original,oldRefs:=testRecoveryPlan(t,store,"crash-owner","/workspace-A","project-A")
 other,otherRefs:=testRecoveryPlan(t,store,"unrelated-owner","/workspace-B","project-B")
 if len(oldRefs)!=5||len(otherRefs)!=5{t.Fatal("expected five immutable pages in each plan")}
 if original.TaskPlan.Steps[0].Status!="done"||
  original.TaskPlan.Steps[0].Evidence[0].Note!="Keep this owner-approved evidence"{
  t.Fatal("test did not start with accepted evidence")
 }
 beforeRaw,err:=os.ReadFile(store.file(original.ID))
 if err!=nil{t.Fatal(err)}

 ctx,cancel:=context.WithTimeout(context.Background(),12*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(ctx,os.Args[0],
  "-test.run=^TestTaskPlanForcedCrashAfterJournalCommitBeforeSegmentCleanup$")
 cmd.Env=append(os.Environ(),
  postCommitCrashChildEnv+"=1",
  "TASKDECK_AGENT_TEST_POST_COMMIT_DIR="+dir,
  "TASKDECK_AGENT_TEST_POST_COMMIT_SESSION="+original.ID)
 var stderr bytes.Buffer
 cmd.Stderr=&stderr
 out,err:=cmd.StdoutPipe()
 if err!=nil{t.Fatal(err)}
 if err=cmd.Start();err!=nil{t.Fatal(err)}
 defer func(){
  if cmd.ProcessState==nil{
   _=cmd.Process.Kill()
   _=cmd.Wait()
  }
 }()
 scanner:=bufio.NewScanner(out)
 observed:=false
 for scanner.Scan(){
  if strings.TrimSpace(scanner.Text())==postCommitCrashMarker{
   observed=true
   break
  }
 }
 if !observed{
  t.Fatalf("child never reached committed-journal barrier: scan=%v stderr=%s",
   scanner.Err(),stderr.String())
 }

 // Read only filesystem bytes while the child deliberately holds withLock:
 // Store.Get would block on the OS journal lock and invalidate this test.
 committedRaw,err:=os.ReadFile(store.file(original.ID))
 if err!=nil{t.Fatal(err)}
 if bytes.Equal(committedRaw,beforeRaw){
  t.Fatal("post-commit barrier fired before authoritative journal changed")
 }
 var manifest Session
 if err=json.Unmarshal(committedRaw,&manifest);err!=nil{t.Fatal(err)}
 if manifest.Revision!=original.Revision+1||
  len(manifest.TaskPlanSegments)!=len(oldRefs){
  t.Fatalf("new canonical manifest did not commit exactly once: revision=%d segments=%d",
   manifest.Revision,len(manifest.TaskPlanSegments))
 }
 freshRef:=manifest.TaskPlanSegments[0]
 if freshRef.Index!=oldRefs[0].Index||
  freshRef.Revision!=manifest.Revision||freshRef.SHA256==oldRefs[0].SHA256{
  t.Fatal("new canonical manifest does not reference a changed immutable page")
 }
 for i:=1;i<len(oldRefs);i++{
  if manifest.TaskPlanSegments[i]!=oldRefs[i]{
   t.Fatalf("unchanged page %d was rewritten",i+1)
  }
 }
 for _,path:=range []string{
  store.taskPlanSegmentPath(original.ID,oldRefs[0]),
  store.taskPlanSegmentPath(original.ID,freshRef),
 }{
  if _,err=os.Stat(path);err!=nil{
   t.Fatalf("expected old and new pages to coexist before cleanup: %s: %v",path,err)
  }
 }

 if err=cmd.Process.Kill();err!=nil{t.Fatal(err)}
 if err=cmd.Wait();err==nil||ctx.Err()!=nil{
  t.Fatalf("child did not reach an intentional forced process kill: wait=%v ctx=%v stderr=%s",
   err,ctx.Err(),stderr.String())
 }
 reopened,err:=NewProjectStore(dir,original.Workspace,original.ProjectID)
 if err!=nil{t.Fatal(err)}
 survived,err:=reopened.Get(original.ID,"crash-owner")
 if err!=nil{t.Fatal(err)}
 if survived.Revision!=original.Revision+1||
  survived.TaskPlan.Steps[0].Status!="blocked"||
  survived.TaskPlan.Steps[0].Evidence[0].Note!="Keep this owner-approved evidence"||
  survived.TaskPlan.Steps[129].Status!="pending"||
  len(survived.TaskPlanSegments)!=len(oldRefs){
  t.Fatal("committed status/accepted proof was lost or duplicated after OS kill")
 }
 for i,ref:=range survived.TaskPlanSegments{
  if i==0&&ref!=freshRef||i>0&&ref!=oldRefs[i]{
   t.Fatalf("reloaded segment %d mismatched the canonical manifest",i+1)
  }
 }
 // Recovery must remove the OBSOLETE page while keeping the newly committed
 // page and ALL pages belonging to an unrelated owner/project.
 if err=reopened.EnsureTaskPlanOrphanRecovery();err!=nil{t.Fatal(err)}
 if _,err=os.Stat(store.taskPlanSegmentPath(original.ID,oldRefs[0]));!os.IsNotExist(err){
  t.Fatalf("post-commit stale segment survived recovery: %v",err)
 }
 for _,ref:=range survived.TaskPlanSegments{
  if _,err=os.Stat(store.taskPlanSegmentPath(original.ID,ref));err!=nil{
   t.Fatalf("recovery deleted a committed page: %v",err)
  }
 }
 for _,ref:=range otherRefs{
  if _,err=os.Stat(store.taskPlanSegmentPath(other.ID,ref));err!=nil{
   t.Fatalf("recovery deleted another owner's committed page: %v",err)
  }
 }
 verified,err:=store.Get(other.ID,"unrelated-owner")
 if err!=nil||verified.Revision!=other.Revision||
  verified.TaskPlan.Steps[0].Evidence[0].Note!="Keep this owner-approved evidence"{
  t.Fatalf("unrelated owner evidence lost: %v",err)
 }
 checked,err:=reopened.Get(original.ID,"crash-owner")
 if err!=nil||checked.Revision!=survived.Revision{
  t.Fatalf("recovered committed plan cannot be loaded: %v",err)
 }
}
