package remoteportal

import (
 "context"
 "errors"
 "fmt"
 "strings"
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

func TestAgentLoginProbeChecksGrantAndQuotaBeforeTaskDeckCallback(t *testing.T) {
 agent,signer:=newAgentLoginFixture(t)
 ch:=testBoundChannelWithPrivate(t,agent.identity.private)
 agent.liveGrantCheck=func(_ context.Context,_ *AgentGrant)error{return nil}
 // Each expensive TaskDeck password attempt requires its OWN signed
 // Portal ticket; a single ticket never supplies repeated guesses.
 var invoked atomic.Int32
 if err:=agent.ConfigureEncryptedLoginVerifier(func(
  _ context.Context,channel *remotetransport.Channel,grant *AgentGrant,_ []byte)([]byte,error){
  invoked.Add(1)
  if _,err:=grant.TakeForLogin(channel,time.Now());err!=nil{return nil,err}
  return remotetransport.SealRemoteLoginResult(channel,true)
 });err!=nil{t.Fatal(err)}
 var fifth *AgentGrant
 for i:=0;i<agentLoginProbeBurst;i++ {
  grant:=signedLoginGrant(t,agent,signer,ch,fmt.Sprintf("%032x",i+1))
  if _,err:=agent.verifyEncryptedLogin(context.Background(),ch,grant,[]byte("opaque"));err!=nil{
   t.Fatalf("allowed distinct signed ticket %d denied: %v",i,err)
  }
  fifth=grant
 }
 if _,err:=agent.verifyEncryptedLogin(context.Background(),ch,fifth,[]byte("another"));
  !errors.Is(err,remotetransport.ErrRemoteActionDenied) {
  t.Fatalf("Agent allowed second use of fifth signed ticket: %v",err)
 }
 sixth:=signedLoginGrant(t,agent,signer,ch,strings.Repeat("e",32))
 if _,err:=agent.verifyEncryptedLogin(context.Background(),ch,sixth,[]byte("opaque"));
  !errors.Is(err,remotetransport.ErrRemoteActionDenied) {
  t.Fatalf("Agent quota did not deny excess distinct signed ticket: %v",err)
 }
 if invoked.Load()!=agentLoginProbeBurst {
  t.Fatalf("TaskDeck callback saw more than allowed CPU work: %d",invoked.Load())
 }
 other:=testBoundChannelWithPrivate(t,agent.identity.private)
 foreign:=signedLoginGrant(t,agent,signer,other,strings.Repeat("f",32))
 if _,err:=agent.verifyEncryptedLogin(context.Background(),ch,foreign,[]byte("opaque"));
  !errors.Is(err,remotetransport.ErrRemoteActionDenied) {
  t.Fatal("Portal grant can be substituted into another E2EE channel")
 }
 foreign.Close()
 if _,err:=agent.verifyEncryptedLogin(context.Background(),other,foreign,[]byte("opaque"));
  !errors.Is(err,remotetransport.ErrRemoteActionDenied) {
  t.Fatal("closed Portal grant accepted")
 }
}
