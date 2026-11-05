package remoterelay

import (
 "crypto/ed25519"
 "encoding/base64"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func signedScopedTransfer(f *appMailboxFixture)string{
 wire:=strings.Join([]string{
  remotetransport.ScopedFileTransferProofDomain,
  f.device.DeviceID,f.device.Nonce,f.probeID,
 },"\n")
 return base64.StdEncoding.EncodeToString(ed25519.Sign(f.key,[]byte(wire)))
}

func TestScopedFileTransferCannotUpgradeBeforeLocalResultOrWithForgedProof(t *testing.T){
 f:=newAppMailboxFixture(t)
 f.activate(t)
 p:=f.device
 valid:=signedScopedTransfer(f)
 if err:=f.r.AuthorizeScopedFileTransfer(p.DeviceID,p.Nonce,f.probeID,
  p.PublicKey,valid);err==nil{
  t.Fatal("Agent signed proof before any authenticated result raised quota")
 }
 id,reply,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher)
 if err!=nil{t.Fatal(err)}
 if _,_,_,ok:=f.r.TakeAppPing(p.DeviceID,p.Nonce);!ok{t.Fatal("missing pending")}
 if err:=f.r.AuthorizeScopedFileTransfer(p.DeviceID,p.Nonce,f.probeID,
  p.PublicKey,valid);err==nil{
  t.Fatal("pending action was treated as successful TaskDeck authorization")
 }
 if !f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher){t.Fatal("failed ACK")}
 <-reply
 for name,c:=range map[string]struct{device,nonce,probe,pub,sig string}{
  "wrong nonce":{p.DeviceID,strings.Repeat("e",32),f.probeID,p.PublicKey,valid},
  "wrong probe":{p.DeviceID,p.Nonce,strings.Repeat("d",32),p.PublicKey,valid},
  "wrong public key":{p.DeviceID,p.Nonce,f.probeID,base64.StdEncoding.EncodeToString(make([]byte,32)),valid},
  "fake signature":{p.DeviceID,p.Nonce,f.probeID,p.PublicKey,base64.StdEncoding.EncodeToString(make([]byte,64))},
  "unknown device":{strings.Repeat("f",64),p.Nonce,f.probeID,p.PublicKey,valid},
 }{
  t.Run(name,func(t *testing.T){
   if err:=f.r.AuthorizeScopedFileTransfer(c.device,c.nonce,c.probe,c.pub,c.sig);err==nil{
    t.Fatal("forged file-transfer proof expanded opaque Relay quota")
   }
  })
 }
 if err:=f.r.AuthorizeScopedFileTransfer(p.DeviceID,p.Nonce,f.probeID,
  p.PublicKey,valid);err!=nil{t.Fatal(err)}
 if err:=f.r.AuthorizeScopedFileTransfer(p.DeviceID,p.Nonce,f.probeID,
  p.PublicKey,valid);err==nil{t.Fatal("duplicate signed transfer proof accepted")}
 for i:=1;i<remotetransport.MaxScopedFileTransferRounds;i++{
  id,done,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher)
  if err!=nil{t.Fatalf("scoped round %d denied: %v",i,err)}
  if _,_,_,ok:=f.r.TakeAppPing(p.DeviceID,p.Nonce);!ok{t.Fatal("missing scoped request")}
  if !f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher){t.Fatal("ack refused")}
  if _,ok:=<-done;!ok{t.Fatal("lost result")}
 }
 if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher);err==nil{
  t.Fatal("file-only quota exceeded 64 round bound")
 }
 if err:=f.r.AuthorizeScopedFileTransfer(p.DeviceID,p.Nonce,f.probeID,
  p.PublicKey,valid);err==nil{t.Fatal("expired quota revived with replayed signature")}
}

func TestScopedFileTransferTimeoutAndLocalDisconnectCannotRevive(t *testing.T){
 f:=newAppMailboxFixture(t)
 f.activate(t)
 p:=f.device
 id,done,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher)
 if err!=nil{t.Fatal(err)}
 if _,_,_,ok:=f.r.TakeAppPing(p.DeviceID,p.Nonce);!ok{t.Fatal("missing")}
 if !f.r.AckAppPing(p.DeviceID,p.Nonce,f.probeID,id,f.cipher){t.Fatal("ack refused")}
 <-done
 f.r.mu.Lock()
 f.r.devices[p.DeviceID].app.expiresAt=time.Now().Add(-time.Second)
 f.r.mu.Unlock()
 if err:=f.r.AuthorizeScopedFileTransfer(p.DeviceID,p.Nonce,f.probeID,
  p.PublicKey,signedScopedTransfer(f));err==nil{
  t.Fatal("expired scoped-file proof expanded session")
 }
 if _,_,err:=f.r.QueueAppPing(p.DeviceID,f.probeID,f.cipher);err==nil{
  t.Fatal("expired scoped transfer accepted ciphertext")
 }
}
