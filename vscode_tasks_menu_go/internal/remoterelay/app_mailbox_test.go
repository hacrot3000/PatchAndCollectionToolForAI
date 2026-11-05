package remoterelay

import (
 "crypto/ed25519"
 "encoding/base64"
 "strings"
 "sync"
 "testing"
 "time"
)

type appMailboxFixture struct {
 r *Registry
 device DeviceProof
 key ed25519.PrivateKey
 probeID string
 cipher string
 release func()
}

func newAppMailboxFixture(t *testing.T) *appMailboxFixture{
 t.Helper()
 now:=time.Now()
 r,err:=NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 proof,key:=newAgentProofWithKey(t,now)
 if err:=r.Register(proof,now);err!=nil{t.Fatal(err)}
 release,err:=r.Acquire(Claims{DeviceID:proof.DeviceID},now)
 if err!=nil{t.Fatal(err)}
 t.Cleanup(release)
 const probe="cccccccccccccccccccccccccccccccc"
 r.mu.Lock()
 // Simulate the ACK of the one-shot encrypted login. The ACK alone is
 // intentionally not enough: signed Agent app-ready proof is still required.
 r.devices[proof.DeviceID].lastLoginProbeID=probe
 r.mu.Unlock()
 return &appMailboxFixture{r:r,device:proof,key:key,probeID:probe,
  cipher:base64.StdEncoding.EncodeToString(make([]byte,90)),release:release}
}
func (f *appMailboxFixture) signedProof()string{
 return base64.StdEncoding.EncodeToString(ed25519.Sign(f.key,
  appReadyWire(f.device.DeviceID,f.device.Nonce,f.probeID)))
}
func (f *appMailboxFixture) activate(t *testing.T){
 t.Helper()
 if err:=f.r.ActivateAppPing(f.device.DeviceID,f.device.Nonce,
   f.probeID,f.device.PublicKey,f.signedProof());err!=nil{t.Fatal(err)}
}

func TestAppMailboxAgentSignatureAndLoginBindingRequired(t *testing.T){
 f:=newAppMailboxFixture(t)
 p:=f.device
 if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher);err==nil {
  t.Fatal("login ACK without signed Agent proof opened mailbox")
 }
 valid:=f.signedProof()
 for name,c:=range map[string]struct{device,nonce,probe,pub,sig string}{
  "wrong nonce":{p.DeviceID,strings.Repeat("d",32),f.probeID,p.PublicKey,valid},
  "wrong probe":{p.DeviceID,p.Nonce,strings.Repeat("e",32),p.PublicKey,valid},
  "forged signature":{p.DeviceID,p.Nonce,f.probeID,p.PublicKey,base64.StdEncoding.EncodeToString(make([]byte,64))},
  "different device":{strings.Repeat("f",64),p.Nonce,f.probeID,p.PublicKey,valid},
  "missing key":{p.DeviceID,p.Nonce,f.probeID,"",valid},
  "oversize public key":{p.DeviceID,p.Nonce,f.probeID,strings.Repeat("A",100000),valid},
  "oversize signature":{p.DeviceID,p.Nonce,f.probeID,p.PublicKey,strings.Repeat("A",100000)},
 }{
  t.Run(name,func(t *testing.T){
   if err:=f.r.ActivateAppPing(c.device,c.nonce,c.probe,c.pub,c.sig);err==nil{
    t.Fatal("forged app readiness accepted")
   }
  })
 }
 f.activate(t)
 if err:=f.r.ActivateAppPing(p.DeviceID,p.Nonce,f.probeID,p.PublicKey,valid);err==nil{
  t.Fatal("duplicate Agent ready activated twice")
 }
}

func TestAppMailboxOpaqueStopAndWaitAndQuota(t *testing.T){
 f:=newAppMailboxFixture(t)
 f.activate(t)
 p:=f.device
 for round:=0;round<MaxAppPingRounds;round++{
  id,reply,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher)
  if err!=nil{t.Fatal(err)}
  if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher);err==nil{
   t.Fatal("second outstanding request admitted")
  }
  if _,_,_,ok:=f.r.TakeAppPing(p.DeviceID,"wrong-agent-nonce");ok{
   t.Fatal("wrong Agent socket received ciphertext")
  }
  probe,gotID,payload,ok:=f.r.TakeAppPing(p.DeviceID,p.Nonce)
  if !ok || probe!=f.probeID || gotID!=id || payload!=f.cipher{
   t.Fatal("different opaque packet delivered")
  }
  if _,_,_,ok:=f.r.TakeAppPing(p.DeviceID,p.Nonce);ok{
   t.Fatal("same packet taken twice")
  }
  if f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,"wrong-id",f.cipher){
   t.Fatal("mismatched response ID accepted")
  }
  if !f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher){
   t.Fatal("verified Agent response denied")
  }
  if result,ok:=<-reply;!ok || result!=f.cipher {
   t.Fatal("Relay modified ciphertext or lost response")
  }
  if f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher){
   t.Fatal("replayed acknowledgement accepted")
  }
 }
 if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher);err==nil {
  t.Fatal("per-session quota bypassed")
 }
 if err:=f.r.ActivateAppPing(p.DeviceID,p.Nonce,f.probeID,
  p.PublicKey,f.signedProof());err==nil{
  t.Fatal("signed readiness replay bypassed exhausted quota")
 }
}

func TestAppMailboxReleaseCancelsPendingAndOldAck(t *testing.T){
 f:=newAppMailboxFixture(t)
 f.activate(t)
 p:=f.device
 id,reply,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher)
 if err!=nil{t.Fatal(err)}
 if _,_,_,ok:=f.r.TakeAppPing(p.DeviceID,p.Nonce);!ok{t.Fatal("missing pending")}
 f.release()
 if _,ok:=<-reply;ok{t.Fatal("released session delivered success")}
 if f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher){
  t.Fatal("late ACK revived old session")
 }
 next,err:=f.r.Acquire(Claims{DeviceID:p.DeviceID},time.Now())
 if err!=nil{t.Fatal(err)}
 defer next()
 if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher);err==nil{
  t.Fatal("next browser inherited prior action mailbox")
 }
}

func TestAppMailboxCloseAndAckAreSerialized(t *testing.T){
 f:=newAppMailboxFixture(t)
 f.activate(t)
 p:=f.device
 id,reply,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher)
 if err!=nil{t.Fatal(err)}
 if _,_,_,ok:=f.r.TakeAppPing(p.DeviceID,p.Nonce);!ok{t.Fatal("no pending")}
 var wg sync.WaitGroup
 wg.Add(2)
 go func(){defer wg.Done();f.r.CloseAppPing(p.DeviceID,p.Nonce,f.probeID)}()
 go func(){defer wg.Done();f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher)}()
 wg.Wait()
 <-reply // could be acknowledged or canceled; no double-close/panic
 if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher);err==nil{
  t.Fatal("closed mailboxes accepted packets")
 }
}

func TestAppMailboxRejectsOversizedAndMalformedCiphertext(t *testing.T){
 f:=newAppMailboxFixture(t)
 f.activate(t)
 p:=f.device
 for _,v:=range []string{"","not base64",base64.StdEncoding.EncodeToString(make([]byte,24)),
  // Largest supported ciphertext is 6,169 bytes; exceeding it must fail.
  base64.StdEncoding.EncodeToString(make([]byte,maxAppPingCiphertext+1))}{
  if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,v);err==nil{
   t.Fatal("invalid opaque frame accepted")
  }
 }
}

func TestAppMailboxTTLPreventsZombiePendingRequest(t *testing.T){
 f:=newAppMailboxFixture(t)
 f.activate(t)
 p:=f.device
 id,reply,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher)
 if err!=nil{t.Fatal(err)}
 if _,_,_,ok:=f.r.TakeAppPing(p.DeviceID,p.Nonce);!ok{
  t.Fatal("fresh request not delivered")
 }
 f.r.mu.Lock()
 f.r.devices[p.DeviceID].app.expiresAt=time.Now().Add(-time.Second)
 f.r.mu.Unlock()
 if f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher){
  t.Fatal("expired mailbox accepted late Agent ACK")
 }
 if _,ok:=<-reply;ok {t.Fatal("expired pending reply returned success")}
 if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher);err==nil{
  t.Fatal("TTL expiry did not retire app mailbox")
 }
 if err:=f.r.ActivateAppPing(p.DeviceID,p.Nonce,f.probeID,p.PublicKey,
  f.signedProof());err==nil{
  t.Fatal("expired app mailbox restarted using old signed admission")
 }
}

func TestAppMailboxPendingAckMustArriveBeforeDeadline(t *testing.T){
 f:=newAppMailboxFixture(t)
 f.activate(t)
 p:=f.device
 id,reply,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher)
 if err!=nil{t.Fatal(err)}
 if _,_,_,ok:=f.r.TakeAppPing(p.DeviceID,p.Nonce);!ok{t.Fatal("Agent did not receive packet")}
 f.r.mu.Lock()
 f.r.devices[p.DeviceID].app.pending.deadline=time.Now().Add(-time.Second)
 f.r.mu.Unlock()
 if f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher){
  t.Fatal("late opaque reply bypassed per-request deadline")
 }
 if _,ok:=<-reply;ok{t.Fatal("expired request was delivered successfully")}
 if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher);err==nil{
  t.Fatal("expired request left mailbox reusable")
 }
}

// The PHP/Relay compromise case includes NEVER sending an ACK after a
// client request. Expiry must close the pending channel independently;
// neither another browser action nor another Agent heartbeat is required.
func TestCompromisedRelaySilentAppACKCannotHoldPendingForever(t *testing.T){
 f:=newAppMailboxFixture(t)
 f.activate(t)
 p:=f.device
 id,reply,err:=f.r.queueAppPingWithTimeout(
  p.DeviceID,f.probeID,f.cipher,25*time.Millisecond)
 if err!=nil{t.Fatal(err)}
 probe,gotID,_,ok:=f.r.TakeAppPing(p.DeviceID,p.Nonce)
 if !ok || probe!=f.probeID || gotID!=id {
  t.Fatal("request not delivered before the timeout")
 }
 select {
 case _,open:=<-reply:
  if open{t.Fatal("silent compromised Relay obtained successful response")}
 case <-time.After(2*time.Second):
  t.Fatal("silent compromised Relay held app response channel past deadline")
 }
 if f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher){
  t.Fatal("late forged ACK revived expired app mailbox")
 }
 if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher);err==nil{
  t.Fatal("expired mailbox granted another app action without a new login")
 }
}

// A valid ACK and an auto-expiry timer can win in either order. No timer
// may close a new mailbox, double-close a reply or admit a stale frame.
func TestCompromisedRelayAppACKExpiryRaceIsSingleTerminalOutcome(t *testing.T){
 for round:=0;round<24;round++{
  f:=newAppMailboxFixture(t)
  f.activate(t)
  p:=f.device
  id,reply,err:=f.r.queueAppPingWithTimeout(
   p.DeviceID,f.probeID,f.cipher,5*time.Millisecond)
  if err!=nil{t.Fatal(err)}
  if _,_,_,ok:=f.r.TakeAppPing(p.DeviceID,p.Nonce);!ok {
   t.Fatal("Agent failed to take fresh request")
  }
  done:=make(chan struct{})
  go func(){
   _=f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher)
   close(done)
  }()
  select{
  case <-reply:
  case <-time.After(2*time.Second):
   t.Fatal("ACK/timeout race deadlocked")
  }
  <-done
  if f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher){
   t.Fatal("duplicate ACK accepted after race")
  }
 }
}
