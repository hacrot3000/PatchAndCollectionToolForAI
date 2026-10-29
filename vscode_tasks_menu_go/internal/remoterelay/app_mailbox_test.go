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
  base64.StdEncoding.EncodeToString(make([]byte,600))}{
  if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,v);err==nil{
   t.Fatal("invalid opaque frame accepted")
  }
 }
}
