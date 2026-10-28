package main

import (
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func testTrustedProof(t *testing.T,pinned string,identity ed25519.PrivateKey,
 browser *remotetransport.Ephemeral,nonce string) ([]byte,*remotetransport.Channel) {
 t.Helper()
 fingerprint:=sha256.Sum256(identity.Public().(ed25519.PublicKey))
 id:=hex.EncodeToString(fingerprint[:])
 probe:=strings.Repeat("b",32)
 hello,agent,err:=remotetransport.ClientHello(identity,id,probe,nonce,browser.Public)
 if err!=nil {t.Fatal(err)}
 ready,err:=agent.Seal([]byte("taskdeck-e2ee-ready-v1"))
 if err!=nil{t.Fatal(err)}
 obj:=map[string]interface{}{
  "ticket_valid":true,"agent_reachable":true,"remote_control_available":false,
  "device_id":pinned,
  "agent_proof":map[string]interface{}{
   "probe_id":probe,
   "crypto":map[string]string{
    "identity":base64.StdEncoding.EncodeToString(hello.Identity),
    "ephemeral":base64.StdEncoding.EncodeToString(hello.Ephemeral),
    "signature":base64.StdEncoding.EncodeToString(hello.Signature),
    "encrypted_ready":base64.StdEncoding.EncodeToString(ready),
   },
  },
 }
 raw,err:=json.Marshal(obj)
 if err!=nil{t.Fatal(err)}
 return raw,agent
}

func TestTrustedClientRejectsPortalControlledEndpointsAndUnpinnedDevice(t *testing.T) {
 device:=strings.Repeat("a",64)
 for _,tc:=range []struct{s clientSettings;good bool}{
  {clientSettings{Relay:"https://relay.example.com",Portal:"https://portal.example.com",DeviceID:device},true},
  {clientSettings{Relay:"http://relay.example.com",Portal:"https://portal.example.com",DeviceID:device},false},
  {clientSettings{Relay:"https://relay.example.com/path",Portal:"https://portal.example.com",DeviceID:device},false},
  {clientSettings{Relay:"https://relay.example.com@evil.example.com",Portal:"https://portal.example.com",DeviceID:device},false},
  {clientSettings{Relay:"https://relay.example.com",Portal:"https://portal.example.com/?inject=1",DeviceID:device},false},
  {clientSettings{Relay:"https://relay.example.com",Portal:"https://portal.example.com",DeviceID:"untrusted"},false},
 }{
  if good:=tc.s.validate()==nil;good!=tc.good{t.Fatalf("endpoint validation %#v good=%v",tc.s,good)}
 }
}

func TestTrustedClientRejectsForgedRelayOrCompromisedPortalAgentIdentity(t *testing.T) {
 pub,key,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 good:=sha256.Sum256(pub)
 pinned:=hex.EncodeToString(good[:])
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 nonce:=strings.Repeat("c",32)
 // A compromised Portal can choose its own relay response, but cannot
 // generate a signed authenticated ECDH handshake for the pinned TaskDeck.
 _,attackerKey,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 forged,_:=testTrustedProof(t,pinned,attackerKey,browser,nonce)
 if ch,err:=verifyAgentProof(forged,pinned,nonce,browser);err==nil{
  ch.Close()
  t.Fatal("untrusted Agent accepted because the Relay returned a matching device_id string")
 }
 legitimate,agent:=testTrustedProof(t,pinned,key,browser,nonce)
 defer agent.Close()
 // The PHP host can present the wrong device ID to trick the user.
 if ch,err:=verifyAgentProof(legitimate,strings.Repeat("d",64),nonce,browser);err==nil{
  ch.Close();t.Fatal("unverified Device ID accepted")
 }
 ch,err:=verifyAgentProof(legitimate,pinned,nonce,browser)
 if err!=nil{t.Fatal(err)}
 defer ch.Close()
 // A false "remote control available" must be rejected, even if the
 // same Agent otherwise signed a valid ECDH handshake.
 var injected map[string]interface{}
 if err:=json.Unmarshal(legitimate,&injected);err!=nil{t.Fatal(err)}
 injected["remote_control_available"]=true
 modified,err:=json.Marshal(injected)
 if err!=nil{t.Fatal(err)}
 if wrong,err:=verifyAgentProof(modified,pinned,nonce,browser);err==nil{
  wrong.Close();t.Fatal("Relay advertised forbidden remote control")
 }
}

func TestTrustedClientAcceptsOnlyOneEncryptedLoginBoolean(t *testing.T) {
 pub,key,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256(pub)
 device:=hex.EncodeToString(sum[:])
 nonce:=strings.Repeat("e",32)
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 proof,agent:=testTrustedProof(t,device,key,browser,nonce)
 defer agent.Close()
 ch,err:=verifyAgentProof(proof,device,nonce,browser)
 if err!=nil{t.Fatal(err)}
 defer ch.Close()
 request,err:=remotetransport.RemoteLoginRequestFrame(ch,"owner","private-local-secret")
 if err!=nil{t.Fatal(err)}
 if _,err:=agent.Open(request);err!=nil{t.Fatal(err)}
 sealed,err:=remotetransport.SealRemoteLoginResult(agent,true)
 if err!=nil{t.Fatal(err)}
 reply,err:=json.Marshal(map[string]interface{}{
  "type":"e2ee_login_result",
  "ciphertext":base64.StdEncoding.EncodeToString(sealed),
  "remote_control_available":false,
 })
 if err!=nil{t.Fatal(err)}
 ok,err:=loginReply(reply,ch)
 if err!=nil || !ok {t.Fatalf("verified local TaskDeck result rejected: %v",err)}
 if _,err:=loginReply(reply,ch);err==nil {t.Fatal("replayed login result accepted")}
 bogus,err:=json.Marshal(map[string]interface{}{
  "type":"terminal.execute","ciphertext":base64.StdEncoding.EncodeToString(sealed),
  "remote_control_available":true,
 })
 if err!=nil{t.Fatal(err)}
 if _,err:=loginReply(bogus,ch);err==nil{t.Fatal("Relay response gave remote control")}
}

func TestTrustedClientRequiresOwnerOnlyOutOfBandPin(t *testing.T) {
 dir:=t.TempDir()
 trusted:=filepath.Join(dir,"private")
 if err:=os.Mkdir(trusted,0700);err!=nil{t.Fatal(err)}
 pinned:=strings.Repeat("e",64)
 pin:=filepath.Join(trusted,"taskdeck.id")
 if err:=os.WriteFile(pin,[]byte(pinned+"\n"),0600);err!=nil{t.Fatal(err)}
 got,err:=loadPinnedDevice(pin)
 if err!=nil || got!=pinned {t.Fatalf("valid independent Device ID pin failed: %v",err)}
 if _,err:=loadPinnedDevice("relative/pin");err==nil{t.Fatal("relative pin path trusted")}
 if err:=os.Chmod(pin,0644);err!=nil{t.Fatal(err)}
 if _,err:=loadPinnedDevice(pin);err==nil{t.Fatal("world-readable pin trusted")}
 if err:=os.Chmod(pin,0600);err!=nil{t.Fatal(err)}
 if err:=os.Chmod(trusted,0755);err!=nil{t.Fatal(err)}
 if _,err:=loadPinnedDevice(pin);err==nil{t.Fatal("world-readable pin directory trusted")}
 if err:=os.Chmod(trusted,0700);err!=nil{t.Fatal(err)}
 sym:=filepath.Join(trusted,"alias.id")
 if err:=os.Symlink(pin,sym);err!=nil{t.Fatal(err)}
 if _,err:=loadPinnedDevice(sym);err==nil{t.Fatal("symlinked pin trusted")}
 if err:=os.WriteFile(pin,[]byte(strings.Repeat("f",64)+"\nextra"),0600);err!=nil{t.Fatal(err)}
 if _,err:=loadPinnedDevice(pin);err==nil{t.Fatal("additional untrusted pin data accepted")}
}
