//go:build linux || darwin

package server

import (
 "bufio"
 "bytes"
 "context"
 "fmt"
 "os"
 "os/exec"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

const (
 startupLiveWorkerEnv="TASKDECK_TEST_STARTUP_OS_LOCK_CHILD"
 startupLiveWorkerWorkspace="TASKDECK_TEST_STARTUP_OS_LOCK_WORKSPACE"
 startupLiveWorkerSession="TASKDECK_TEST_STARTUP_OS_LOCK_SESSION"
 startupLiveWorkerReady="TASKDECK_TEST_STARTUP_LIVE_WORKER_HELD"
)

func TestStartupReconcileRealCrossProcessTurnLease(t *testing.T){
 if os.Getenv(startupLiveWorkerEnv)=="1"{
  unlock,err:=acquireAgentTurnOSLock(os.Getenv(startupLiveWorkerWorkspace),os.Getenv(startupLiveWorkerSession))
  if err!=nil{fmt.Fprintln(os.Stderr,"could not hold cross-process lease:",err);os.Exit(2)}
  defer unlock()
  fmt.Println(startupLiveWorkerReady)
  time.Sleep(30*time.Second)
  os.Exit(3) // never succeed by voluntarily exiting
 }
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.RetryMode=agentchat.RetryUnlimited
 x.ProviderRun=&agentchat.ProviderRun{
  ID:"r-cross-daemon-e1",Kind:"message",Phase:"sending",
  PayloadSHA256:agentchat.ProviderPayloadDigest([]byte("cross process provider request")),
  CreatedAt:now,UpdatedAt:now,
 }
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 // Another abandoned session must not be starved by the live lease on id.
 other,err:=agentchat.NewSession("local",s.Workspace,x.ProjectID,x.ProviderID,x.ModelID,x.Mode)
 if err!=nil{t.Fatal(err)}
 other.Status="requesting_ai"
 other.ProviderRun=&agentchat.ProviderRun{
  ID:"r-abandoned-e1",Kind:"message",Phase:"sending",
  PayloadSHA256:agentchat.ProviderPayloadDigest([]byte("abandoned second session")),
  CreatedAt:now,UpdatedAt:now,
 }
 other,err=store.Save(other,0);if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),12*time.Second)
 defer cancel()
 child:=exec.CommandContext(ctx,os.Args[0],
  "-test.run=^TestStartupReconcileRealCrossProcessTurnLease$")
 child.Env=append(os.Environ(),startupLiveWorkerEnv+"=1",
  startupLiveWorkerWorkspace+"="+s.Workspace,
  startupLiveWorkerSession+"="+id)
 var stderr bytes.Buffer
 child.Stderr=&stderr
 stdout,err:=child.StdoutPipe();if err!=nil{t.Fatal(err)}
 if err=child.Start();err!=nil{t.Fatal(err)}
 defer func(){
  if child.ProcessState==nil{
   _=child.Process.Kill()
   _=child.Wait()
  }
 }()
 scanner:=bufio.NewScanner(stdout)
 held:=false
 for scanner.Scan(){
  if strings.TrimSpace(scanner.Text())==startupLiveWorkerReady{
   held=true;break
  }
 }
 if !held{t.Fatalf("separate process never held OS session lease: %v %s",scanner.Err(),stderr.String())}
 providerCalls:=0
 s.AgentChatCall=func(context.Context,agentchat.ProviderConfig,string,[]byte)(agentchat.ResponsesResult,error){
  providerCalls++
  return agentchat.ResponsesResult{},nil
 }
 s.agentReconcilePendingOnStartup(context.Background())
 live,err:=store.Get(id,"local")
 if err!=nil||live.Revision!=x.Revision||live.Status!="requesting_ai"||
  live.ProviderRun.Phase!="sending"{
  t.Fatalf("startup daemon overwrote other daemon live turn: %+v %v",live,err)
 }
 abandoned,err:=store.Get(other.ID,"local")
 if err!=nil||abandoned.Status!="failed"||abandoned.ProviderRun.Phase!="unknown"||
  abandoned.Revision!=other.Revision+1{
  t.Fatalf("independent abandoned session was blocked by another live lease: %+v %v",abandoned,err)
 }
 if providerCalls!=0{t.Fatal("startup reconciliation invoked provider instead of only updating journal")}
 if err=child.Process.Kill();err!=nil{t.Fatal(err)}
 if err=child.Wait();err==nil||ctx.Err()!=nil{
  t.Fatalf("child did not die under real SIGKILL: %v ctx=%v stderr=%s",err,ctx.Err(),stderr.String())
 }
 s.agentReconcilePendingOnStartup(context.Background())
 unknown,err:=store.Get(id,"local")
 if err!=nil||unknown.Status!="failed"||unknown.ProviderRun.Phase!="unknown"||
  unknown.Revision!=x.Revision+1{
  t.Fatalf("after OS lock release, crashed run not marked UNKNOWN: %+v %v",unknown,err)
 }
 if providerCalls!=0{t.Fatal("SENDING run was replayed after process crash")}
 review:=agentchat.ProviderEgressReviewForSession(unknown)
 if review==nil||review.AutomaticReplayAllowed||!review.RequiresReconciliation{
  t.Fatal("crashed run permitted forbidden provider replay")
 }
}
