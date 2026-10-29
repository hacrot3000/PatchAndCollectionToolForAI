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
 "syscall"
 "testing"
 "time"

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

type fakePinOwner struct {
	os.FileInfo
	uid uint32
}
func (f fakePinOwner) Sys() any {return &syscall.Stat_t{Uid:f.uid}}

// Permissions 0600/0700 are necessary but not sufficient if a privileged
// client is handed another tenant's otherwise private pin-file path.
func TestTrustedPinMustBelongToCurrentEffectiveOSUser(t *testing.T) {
	dir:=t.TempDir()
	// Do not assume the test harness's TempDir has the same mode as the
	// production owner-only trust directory on every OS/runner.
	if err:=os.Chmod(dir,0700);err!=nil{t.Fatal(err)}
	pin:=filepath.Join(dir,"pin")
	if err:=os.WriteFile(pin,[]byte(strings.Repeat("a",64)+"\n"),0600);err!=nil{t.Fatal(err)}
	file,err:=os.Lstat(pin)
	if err!=nil{t.Fatal(err)}
	parent,err:=os.Lstat(dir)
	if err!=nil{t.Fatal(err)}
	if !ownerOnlyPin(file,false) || !ownerOnlyPin(parent,true) {
		t.Fatal("current OS user cannot read their own owner-only trust pin")
	}
	forged:=uint32(os.Geteuid())+1
	if ownerOnlyPin(fakePinOwner{FileInfo:file,uid:forged},false) {
		t.Fatal("private file owned by another tenant was accepted as local trust pin")
	}
	if ownerOnlyPin(fakePinOwner{FileInfo:parent,uid:forged},true) {
		t.Fatal("private directory owned by another tenant was accepted as trust root")
	}
	if ownerOnlyPin(fakePinOwner{FileInfo:parent,uid:uint32(os.Geteuid())},false) {
		t.Fatal("directory was accepted as a regular pin file")
	}
	if ownerOnlyPin(fakePinOwner{FileInfo:file,uid:uint32(os.Geteuid())},true) {
		t.Fatal("regular file was accepted as a pin directory")
	}
}

func TestNativeLoginClientRejectsFIFOTrustPinWithoutHanging(t *testing.T) {
	d:=filepath.Join(t.TempDir(),"private")
	if err:=os.Mkdir(d,0700);err!=nil{t.Fatal(err)}
	fifo:=filepath.Join(d,"pinned-device.id")
	if err:=syscall.Mkfifo(fifo,0600);err!=nil{t.Skipf("FIFO unsupported: %v",err)}
	// Do not let the local CLI hang in open() if the pinned identity file
	// becomes a named pipe before descriptor ownership/type validation.
	done:=make(chan error,1)
	go func(){_,err:=loadPinnedDevice(fifo);done<-err}()
	select {
	case err:=<-done:
		if err==nil {t.Fatal("FIFO was accepted as independent Device ID pin")}
	case <-time.After(2*time.Second):
		t.Fatal("FIFO unexpectedly blocked native login client")
	}
}

func TestCompromisedPHPPortalCannotPromptForTaskDeckPasswordBeforePinnedAgentProof(t *testing.T) {
	calls:=0
	reader:=func()(string,error) {
		calls++
		return "local-password-never-sent-to-PHP",nil
	}
	if _,err:=sealLoginAfterAgentProof(nil,"alice",reader);err==nil {
		t.Fatal("missing Agent proof triggered password prompt")
	}
	if _,err:=sealLoginAfterAgentProof(&remotetransport.Channel{},"alice",reader);err==nil {
		t.Fatal("unauthenticated E2EE channel triggered password prompt")
	}
	if calls!=0 {t.Fatal("untrusted Portal or Relay requested local password before authenticating Agent")}

	pub,key,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	hash:=sha256.Sum256(pub)
	pinned:=hex.EncodeToString(hash[:])
	browser,err:=remotetransport.NewEphemeral()
	if err!=nil{t.Fatal(err)}
	nonce:=strings.Repeat("3",32)
	_,attackerKey,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	forged,_:=testTrustedProof(t,pinned,attackerKey,browser,nonce)
	if fake,err:=verifyAgentProof(forged,pinned,nonce,browser);err==nil {
		fake.Close()
		t.Fatal("PHP attacker impersonated the owner-pinned Agent")
	}
	if calls!=0 {t.Fatal("forged Portal identity triggered credential prompt")}
	real,agent:=testTrustedProof(t,pinned,key,browser,nonce)
	defer agent.Close()
	ch,err:=verifyAgentProof(real,pinned,nonce,browser)
	if err!=nil{t.Fatal(err)}
	defer ch.Close()
	encrypted,err:=sealLoginAfterAgentProof(ch,"alice",reader)
	if err!=nil{t.Fatal(err)}
	if calls!=1 {t.Fatalf("trusted Agent proof should prompt exactly once, prompts=%d",calls)}
	if strings.Contains(string(encrypted),"local-password-never-sent-to-PHP") {
		t.Fatal("TaskDeck password left trusted CLI without AES-GCM protection")
	}
	credentials,err:=remotetransport.OpenRemoteLogin(agent,encrypted)
	if err!=nil || credentials.Username!="alice" ||
		credentials.Password!="local-password-never-sent-to-PHP" {
		t.Fatalf("trusted Agent cannot decrypt locally entered TaskDeck password: %v",err)
	}
	ch.Close()
	if _,err:=sealLoginAfterAgentProof(ch,"alice",reader);err==nil {
		t.Fatal("closed Agent channel triggered a new password prompt")
	}
	if calls!=1 {t.Fatal("closed remote session requested password again")}
}
