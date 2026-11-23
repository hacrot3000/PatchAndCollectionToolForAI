//go:build linux || darwin

package server

import (
 "bufio"
 "bytes"
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

const (
 providerSIGKILLChildFlag="TASKDECK_TEST_PROVIDER_SIGKILL_CHILD"
 providerSIGKILLDirFlag="TASKDECK_TEST_PROVIDER_SIGKILL_DIR"
 providerSIGKILLIDFlag="TASKDECK_TEST_PROVIDER_SIGKILL_ID"
 providerSIGKILLMarker="TASKDECK_PROVIDER_BARRIER_DURABLE_AND_NETWORK_NOT_STARTED"
 providerSIGKILLPayload=`{"model":"gpt-4.1-mini","input":[{"role":"user","content":"Synthetic crash-safety objective"}]}`
)

// This helper is a separate REAL OS process. The parent terminates it
// immediately after the marked persistence boundary and BEFORE any provider
// network call. It never creates a provider client or executes a native tool.
func agentProviderSIGKILLChild() {
 dir:=os.Getenv(providerSIGKILLDirFlag)
 id:=os.Getenv(providerSIGKILLIDFlag)
 stage:=os.Getenv(providerSIGKILLChildFlag)
 store,err:=agentchat.NewStore(dir)
 if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(2)}
 x,err:=store.Get(id,"local")
 if err!=nil||x.ProviderRun==nil||x.ProviderRun.Phase!="prepared"||
  x.Status!="requesting_ai"||x.ProviderRun.PayloadSHA256!=agentchat.ProviderPayloadDigest([]byte(providerSIGKILLPayload)){
  fmt.Fprintln(os.Stderr,"unexpected original PREPARED journal",err)
  os.Exit(3)
 }
 switch stage{
 case "prepared":
  // The parent already synced the exact PREPARED payload and journal.
  // PREPARED is the only restart state eligible to resume the same bytes.
 case "sending_no_attempt":
  // The real markAgentProviderSending first commits this exact change
  // and only then deletes the PREPARED sidecar. Kill in that gap to
  // prove an orphan PREPARED payload cannot authorize a replay.
  x.ProviderRun.Phase="sending"
  x.ProviderRun.UpdatedAt=time.Now().UTC()
  x.AppendEvent("provider_egress",x.Status,x.ProviderRun.CallID,x.ProviderRun.Kind)
  if _,err=store.Save(x,x.Revision);err!=nil{
   fmt.Fprintln(os.Stderr,"cannot commit SENDING before kill:",err);os.Exit(4)
  }
 case "sending_attempt":
  if err=markAgentProviderSending(store,&x);err!=nil{
   fmt.Fprintln(os.Stderr,"cannot commit SENDING:",err);os.Exit(5)
  }
  if _,err=store.RecordProviderAttempt(
   id,"local",x.ProviderRun.ID,1,[]byte(providerSIGKILLPayload),true,
  );err!=nil{
   fmt.Fprintln(os.Stderr,"cannot durably record attempt before kill:",err);os.Exit(6)
  }
 default:
  fmt.Fprintln(os.Stderr,"unknown crash boundary:",stage);os.Exit(7)
 }
 fmt.Fprintln(os.Stdout,providerSIGKILLMarker)
 // Parent must SIGKILL this process. If it returns normally, the test
 // must fail rather than falsely attributing process death to an OS kill.
 time.Sleep(30*time.Second)
 os.Exit(8)
}

// TestAgentProviderRealSIGKILLAtEgressBoundaries distinguishes a provably
// UNSENT PREPARED request from SENDING/UNKNOWN at TWO actual kill points.
// Synthetic status mutation alone cannot exercise the cross-process flock
// release, persistent filesystem namespace or crash/restart coordinator.
func TestAgentProviderRealSIGKILLAtEgressBoundaries(t *testing.T){
 if os.Getenv(providerSIGKILLChildFlag)!=""{
  agentProviderSIGKILLChild()
  return
 }
 for _,phase:=range []string{"prepared","sending_no_attempt","sending_attempt"}{
  t.Run(phase,func(t *testing.T){
   s,id:=newAgentConversationFixture(t,"stateless")
   store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
   x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
   provider:=agentProviderFixture()
   originalRun:=""
   x.History=append(x.History,agentchat.HistoryItem{Type:"user",
    Content:"Synthetic crash-safety objective"})
   x.Status="requesting_ai"
   x.RetryMode=agentchat.RetryUnlimited
   x.AppendEvent("turn_started","requesting_ai","","")
   payload:=[]byte(providerSIGKILLPayload)
   if err=prepareAgentProviderRun(store,&x,"message","",
    provider,provider.Models[0],payload);err!=nil{t.Fatal(err)}
   x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
   originalRun=x.ProviderRun.ID
   privateFile,err:=aiPrivatePath(s.Workspace);if err!=nil{t.Fatal(err)}
   dir:=filepath.Join(filepath.Dir(privateFile),"agent-sessions-"+aiWorkspaceHash(s.Workspace))

   ctx,cancel:=context.WithTimeout(context.Background(),12*time.Second)
   defer cancel()
   cmd:=exec.CommandContext(ctx,os.Args[0],
    "-test.run=^TestAgentProviderRealSIGKILLAtEgressBoundaries$")
   cmd.Env=append(os.Environ(),
    providerSIGKILLChildFlag+"="+phase,
    providerSIGKILLDirFlag+"="+dir,
    providerSIGKILLIDFlag+"="+id)
   var stderr bytes.Buffer
   cmd.Stderr=&stderr
   out,err:=cmd.StdoutPipe();if err!=nil{t.Fatal(err)}
   if err=cmd.Start();err!=nil{t.Fatal(err)}
   defer func(){
    if cmd.ProcessState==nil{
     _=cmd.Process.Kill()
     _=cmd.Wait()
    }
   }()
   scan:=bufio.NewScanner(out)
   marker:=false
   for scan.Scan(){
    if strings.TrimSpace(scan.Text())==providerSIGKILLMarker{
     marker=true;break
    }
   }
   if !marker{
    t.Fatalf("child never reached durable boundary: %v stderr=%s",
     scan.Err(),stderr.String())
   }
   // Deliberately force-stop the OS process rather than cancelling a Go
   // context. The child holds no right to turn pre-send evidence into ACK.
   if err=cmd.Process.Kill();err!=nil{t.Fatal(err)}
   if err=cmd.Wait();err==nil||ctx.Err()!=nil{
    t.Fatalf("child was not killed at its durable barrier: wait=%v ctx=%v stderr=%s",
     err,ctx.Err(),stderr.String())
   }
   reopened,err:=agentchat.NewStore(dir);if err!=nil{t.Fatal(err)}
   before,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
   if before.ProviderRun==nil||before.ProviderRun.ID!=originalRun||
    before.ProviderRun.PayloadSHA256!=agentchat.ProviderPayloadDigest(payload)||
    before.Status!="requesting_ai"||before.RetryMode!=agentchat.RetryUnlimited{
    t.Fatal("cross-process restart lost exact run, payload digest or retry preference")
   }
   expectedPhase:="sending"
   if phase=="prepared"{expectedPhase="prepared"}
   if before.ProviderRun.Phase!=expectedPhase{
    t.Fatalf("crashed journal unexpectedly reached %q",before.ProviderRun.Phase)
   }
   audit,err:=reopened.ProviderAttempts(id,"local");if err!=nil{t.Fatal(err)}
   if phase=="sending_attempt"{
    if audit==nil||audit.Total!=1||len(audit.Attempts)!=1||
     audit.RunID!=originalRun||!audit.Attempts[0].HeaderConfigured||
     audit.Attempts[0].PayloadSHA256!=agentchat.ProviderPayloadDigest(payload){
     t.Fatalf("recorded pre-network audit missing after real SIGKILL: %+v",audit)
    }
   }else if audit!=nil{
    t.Fatalf("pre-audit killed process manufactured a request receipt: %+v",audit)
   }
   if phase=="sending_no_attempt"{
    // This is the critical orphan-sidecar scenario: the original PREPARED
    // payload bytes may remain on disk, but MUST NOT be eligible for replay.
    _,_,e:=reopened.PreparedProviderPayload(id,"local")
    if !errors.Is(e,agentchat.ErrInvalidSession){
     t.Fatalf("stale PREPARED sidecar bypassed SENDING boundary: %v",e)
    }
   }

   calls:=0
   s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,key string,body []byte)(
    agentchat.ResponsesResult,error,
   ){
    calls++
    if key!="TEST-KEY"||!bytes.Equal(body,payload){
     t.Fatal("recovered PREPARED provider request changed its exact bytes")
    }
    return agentchat.ResponsesResult{ResponseID:"only-verified-prepared-recovery",
     Text:"Synthetic response; no native tool call"},nil
   }
   // A fresh-daemon startup reconciliation must force SENDING => UNKNOWN,
   // regardless of whether the audit was already durably written.
   s.agentReconcilePendingOnStartup(context.Background())
   current,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
   if phase!="prepared"{
    if current.Status!="failed"||current.ProviderRun.Phase!="unknown"{
     t.Fatalf("restarted ambiguous run not fenced: %+v",current.ProviderRun)
    }
    if agentAutoFeedbackNeedsWork(current){
     t.Fatal("ambiguous provider run was scheduled for replay")
    }
    review:=agentchat.ProviderEgressReviewForSession(current)
    if review==nil||review.AutomaticReplayAllowed||!review.RequiresReconciliation{
     t.Fatal("local audit falsely acknowledged ambiguous provider")
    }
   }else if current.ProviderRun.Phase!="prepared"{
    t.Fatalf("provably unsent PREPARED request was changed: %+v",current.ProviderRun)
   }
   // Exercise the actual Agent scheduler, not a fabricated replay stub.
   s.processAgentAutoFeedbackSession(context.Background(),store,id)
   s.processAgentAutoFeedbackSession(context.Background(),store,id)
   after,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
   if phase=="prepared"{
    if calls!=1||after.ProviderRun.Phase!="completed"||
     after.Status!="completed"{
     t.Fatalf("PREPARED exact-once resumption failed: calls=%d phase=%+v",calls,after.ProviderRun)
    }
   }else{
    if calls!=0||after.ProviderRun.Phase!="unknown"||
     after.Status!="failed"||after.ProviderRun.ID!=originalRun{
     t.Fatalf("ambiguous provider was silently replayed: calls=%d phase=%+v",calls,after.ProviderRun)
    }
    if _,e:=reopened.RecordProviderAttempt(id,"local",originalRun,2,payload,true);
     !errors.Is(e,agentchat.ErrInvalidSession){
     t.Fatalf("after restart, UNKNOWN admitted a second attempt: %v",e)
    }
   }
   retained,err:=reopened.ProviderAttempts(id,"local");if err!=nil{t.Fatal(err)}
   if phase=="sending_attempt"{
    if retained==nil||retained.Total!=1||retained.RunID!=originalRun{
     t.Fatal("restart reconciliation erased or replaced prior pre-network audit")
    }
   }else if phase=="prepared"{
    // PREPARED legitimately resumed its exact payload ONCE through the
    // native provider retry wrapper; that authorized send must create
    // exactly one new pre-network audit (not zero and not two).
    if retained==nil||retained.Total!=1||len(retained.Attempts)!=1||
     retained.RunID!=originalRun||
     retained.Attempts[0].PayloadSHA256!=agentchat.ProviderPayloadDigest(payload){
     t.Fatalf("legitimate PREPARED resumption lacks exact one-attempt audit: %+v",retained)
    }
   }else if retained!=nil{
    t.Fatal("SENDING before first audit acquired a synthetic receipt after restart")
   }
  })
 }
}
