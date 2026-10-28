package remoteportal

import (
 "context"
 "errors"
 "path/filepath"
 "sync"
 "sync/atomic"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestAgentLoginBudgetCapsBurstAndRefillsPrecisely(t *testing.T){
 var limiter agentLoginBudget
 now:=time.Now()
 for i:=0;i<agentLoginProbeBurst;i++ {
  if !limiter.allow(now){t.Fatalf("login %d of initial burst was incorrectly denied",i)}
 }
 if limiter.allow(now) {t.Fatal("Agent accepted login attempt beyond burst")}
 if limiter.allow(now.Add(-time.Minute)){t.Fatal("system clock rollback replenished login budget")}
 if limiter.allow(now.Add(11*time.Second)){t.Fatal("login budget refilled prematurely")}
 if !limiter.allow(now.Add(12*time.Second)){t.Fatal("login budget did not refill at 12 seconds")}
 if limiter.allow(now.Add(12*time.Second)){t.Fatal("refill granted multiple login attempts")}
 if !limiter.allow(now.Add(24*time.Second)){t.Fatal("second refill was denied")}
 if limiter.allow(now.Add(24*time.Second)){t.Fatal("second refill counted twice")}
 refill:=now.Add(3*time.Minute)
 for i:=0;i<agentLoginProbeBurst;i++ {
  if !limiter.allow(refill){t.Fatalf("full refill missed attempt %d",i)}
 }
 if limiter.allow(refill) {t.Fatal("refill exceeded the fixed burst maximum")}
}

func TestAgentLoginBudgetConcurrentAttemptRace(t *testing.T){
 var limiter agentLoginBudget
 when:=time.Now()
 var accepted atomic.Int32
 var wg sync.WaitGroup
 for i:=0;i<100;i++ {
  wg.Add(1)
  go func(){
   defer wg.Done()
   if limiter.allow(when) {accepted.Add(1)}
  }()
 }
 wg.Wait()
 if accepted.Load()!=agentLoginProbeBurst {
  t.Fatalf("concurrent login bypassed Agent quota: %d",accepted.Load())
 }
}

func TestAgentLoginProbeChecksGrantAndQuotaBeforeTaskDeckCallback(t *testing.T){
 ch:=testBoundChannel(t)
 agent,err:=NewAgent("https://portal.example.com",
  filepath.Join(t.TempDir(),"private","seed"),nil)
 if err!=nil{t.Fatal(err)}
 agent.liveGrantCheck=func(_ context.Context,_ *AgentGrant)error{return nil}
 // The device budget is persisted; this test still exercises the
 // absolute per-Agent bound, without depending on the wall clock.
 var invoked atomic.Int32
 if err:=agent.ConfigureEncryptedLoginVerifier(func(
  _ context.Context,_ *remotetransport.Channel,_ *AgentGrant,_ []byte)([]byte,error){
  invoked.Add(1)
  return []byte("encrypted-test-only"),nil
 });err!=nil{t.Fatal(err)}
 grant:=&AgentGrant{channel:ch,role:remotetransport.RoleViewer,
  subject:"21",ticketJTI:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",authEpoch:1,
  expiresAt:time.Now().Add(time.Minute)}
 for i:=0;i<agentLoginProbeBurst;i++ {
  if _,err:=agent.verifyEncryptedLogin(context.Background(),ch,grant,[]byte("opaque"));err!=nil{
   t.Fatalf("allowed local test callback denied %d: %v",i,err)
  }
 }
 if _,err:=agent.verifyEncryptedLogin(context.Background(),ch,grant,[]byte("opaque"));
  !errors.Is(err,remotetransport.ErrRemoteActionDenied) {
  t.Fatalf("Agent quota did not deny excess login: %v",err)
 }
 if invoked.Load()!=agentLoginProbeBurst {
  t.Fatalf("TaskDeck callback saw more than allowed CPU work: %d",invoked.Load())
 }
 other:=testBoundChannel(t)
 if _,err:=agent.verifyEncryptedLogin(context.Background(),other,grant,[]byte("opaque"));
  !errors.Is(err,remotetransport.ErrRemoteActionDenied) {
  t.Fatal("Portal grant can be substituted into another E2EE channel")
 }
 grant.Close()
 if _,err:=agent.verifyEncryptedLogin(context.Background(),ch,grant,[]byte("opaque"));
  !errors.Is(err,remotetransport.ErrRemoteActionDenied) {
  t.Fatal("closed Portal grant accepted")
 }
}
