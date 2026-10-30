package remoterelay

import (
	"crypto/ed25519"
	"encoding/base64"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
	"strings"
	"testing"
	"time"
)

func signedProbeAttestation(key ed25519.PrivateKey, deviceID string, req ProbeRequest) ProbeAttestation {
	msg:=strings.Join([]string{BrowserAttestationProtocol,deviceID,req.ProbeID,req.ClientNonce},string([]byte{10}))
	return ProbeAttestation{
		ProbeID:req.ProbeID,ClientNonce:req.ClientNonce,
		PublicKey:base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)),
		Signature:base64.StdEncoding.EncodeToString(ed25519.Sign(key,[]byte(msg))),
	}
}

func TestReachabilityProbeRequiresValidDeviceSignatureAndBrowserNonce(t *testing.T) {
	now:=time.Now()
	r,err:=NewRegistry("https://relay.example.com")
	if err!=nil{t.Fatal(err)}
	proof,private:=newAgentProofWithKey(t,now)
	if err:=r.Register(proof,now);err!=nil{t.Fatal(err)}
	challenge:=strings.Repeat("c",32)
	if _,err:=r.BeginProbe(proof.DeviceID,challenge);err==nil{t.Fatal("probe without browser reservation accepted")}
	release,err:=r.Acquire(Claims{DeviceID:proof.DeviceID},now)
	if err!=nil{t.Fatal(err)}
	if _,err:=r.BeginProbe(proof.DeviceID,"bad-nonce");err==nil{t.Fatal("invalid browser nonce accepted")}
	result,err:=r.BeginProbe(proof.DeviceID,challenge)
	if err!=nil{t.Fatal(err)}
	if _,err:=r.BeginProbe(proof.DeviceID,challenge);err==nil{t.Fatal("second probe accepted")}
	if _,ok:=r.TakeProbe(proof.DeviceID,"forged");ok{t.Fatal("foreign Agent took probe")}
	request,ok:=r.TakeProbe(proof.DeviceID,proof.Nonce)
	if !ok || request.ClientNonce!=challenge{t.Fatal("valid Agent could not take probe")}
	if _,ok:=r.TakeProbe(proof.DeviceID,proof.Nonce);ok{t.Fatal("probe redelivered")}
	ack:=signedProbeAttestation(private,proof.DeviceID,request)
	fake:=ack;fake.ProbeID="invalid"
	if r.AckProbe(proof.DeviceID,proof.Nonce,fake){t.Fatal("wrong probe ID accepted")}
	fake=ack;fake.ClientNonce=strings.Repeat("d",32)
	if r.AckProbe(proof.DeviceID,proof.Nonce,fake){t.Fatal("wrong browser nonce accepted")}
	fake=ack;fake.Signature=base64.StdEncoding.EncodeToString(make([]byte,ed25519.SignatureSize))
	if r.AckProbe(proof.DeviceID,proof.Nonce,fake){t.Fatal("unsigned ACK accepted")}
	if !r.AckProbe(proof.DeviceID,proof.Nonce,ack){t.Fatal("valid signed ACK rejected")}
	outcome:=<-result
	if !outcome.OK || outcome.Attestation.Signature!=ack.Signature {
		t.Fatal("verified proof not returned to browser")
	}
	if r.AckProbe(proof.DeviceID,proof.Nonce,ack){t.Fatal("duplicate ACK accepted")}
	release()
	again,err:=r.Acquire(Claims{DeviceID:proof.DeviceID},now)
	if err!=nil{t.Fatal(err)}
	pending,err:=r.BeginProbe(proof.DeviceID,challenge)
	if err!=nil{t.Fatal(err)}
	r.Unregister(proof.DeviceID,proof.Nonce)
	if (<-pending).OK{t.Fatal("disconnect incorrectly completed pending probe")}
	again()
}

func TestRelayRejectsForgedEncryptedAgentHandshake(t *testing.T) {
	now:=time.Now()
	registry,err:=NewRegistry("https://relay.example.com")
	if err!=nil{t.Fatal(err)}
	proof,private:=newAgentProofWithKey(t,now)
	if err:=registry.Register(proof,now);err!=nil{t.Fatal(err)}
	browser,err:=remotetransport.NewEphemeral()
	if err!=nil{t.Fatal(err)}
	release,err:=registry.Acquire(Claims{DeviceID:proof.DeviceID},now)
	if err!=nil{t.Fatal(err)}
	defer release()
	nonce:=strings.Repeat("f",32)
	_,err=registry.BeginProbe(proof.DeviceID,nonce,base64.StdEncoding.EncodeToString(browser.Public))
	if err!=nil{t.Fatal(err)}
	request,ok:=registry.TakeProbe(proof.DeviceID,proof.Nonce)
	if !ok{t.Fatal("secured browser probe was not queued")}
	hello,channel,err:=remotetransport.ClientHello(private,proof.DeviceID,request.ProbeID,nonce,browser.Public)
	if err!=nil{t.Fatal(err)}
	ready,err:=channel.Seal([]byte("taskdeck-e2ee-ready-v1"))
	if err!=nil{t.Fatal(err)}
	ack:=signedProbeAttestation(private,proof.DeviceID,request)
	ack.Crypto=&ProbeCrypto{
		Identity:base64.StdEncoding.EncodeToString(hello.Identity),
		Ephemeral:base64.StdEncoding.EncodeToString(hello.Ephemeral),
		Signature:base64.StdEncoding.EncodeToString(hello.Signature),
		EncryptedReady:base64.StdEncoding.EncodeToString(ready),
	}
	for name,mutate:=range map[string]func(*ProbeAttestation){
		"missing":func(a *ProbeAttestation){a.Crypto=nil},
		"bad signature":func(a *ProbeAttestation){
			a.Crypto.Signature=base64.StdEncoding.EncodeToString(make([]byte,64))
		},
		"wrong Agent identity":func(a *ProbeAttestation){
			a.Crypto.Identity=base64.StdEncoding.EncodeToString(make([]byte,32))
		},
	} {
		modified:=ack
		if ack.Crypto!=nil {
			copyOfCrypto:=*ack.Crypto
			modified.Crypto=&copyOfCrypto
		}
		mutate(&modified)
		if registry.AckProbe(proof.DeviceID,proof.Nonce,modified) {
			t.Fatalf("%s encrypted Agent proof accepted",name)
		}
	}
	if !registry.AckProbe(proof.DeviceID,proof.Nonce,ack) {
		t.Fatal("valid signed Agent E2EE ready response rejected")
	}
}

func TestRejectProbeLeavesAuthenticatedAgentConnectedAndRejectsStaleID(t *testing.T) {
 now:=time.Now()
 registry,err:=NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 proof,_:=newAgentProofWithKey(t,now)
 if err:=registry.Register(proof,now);err!=nil{t.Fatal(err)}
 oldRelease,err:=registry.Acquire(Claims{DeviceID:proof.DeviceID},now)
 if err!=nil{t.Fatal(err)}
 oldResult,err:=registry.BeginProbe(proof.DeviceID,strings.Repeat("d",32))
 if err!=nil{t.Fatal(err)}
 first,ok:=registry.TakeProbe(proof.DeviceID,proof.Nonce)
 if !ok{t.Fatal("old browser probe not delivered")}
 if registry.RejectProbe(proof.DeviceID,"wrong-Agent",first.ProbeID){
  t.Fatal("foreign Agent rejected probe")
 }
 if registry.RejectProbe(proof.DeviceID,proof.Nonce,"wrong-probe"){
  t.Fatal("invalid probe ID rejected current Browser")
 }
 if !registry.IsCurrentProbe(proof.DeviceID,proof.Nonce,first.ProbeID) {
  t.Fatal("invalid rejection corrupted active browser probe")
 }
 if !registry.RejectProbe(proof.DeviceID,proof.Nonce,first.ProbeID) {
  t.Fatal("matching Agent denial was not accepted")
 }
 if (<-oldResult).OK {t.Fatal("denied Agent grant returned a successful proof")}
 if registry.IsCurrentProbe(proof.DeviceID,proof.Nonce,first.ProbeID) {
  t.Fatal("rejected probe remained active")
 }
 if !registry.Online(proof.DeviceID,time.Now()) {
  t.Fatal("rejected handshake unregistered Agent")
 }
 oldRelease()
 newRelease,err:=registry.Acquire(Claims{DeviceID:proof.DeviceID},now)
 if err!=nil{t.Fatal(err)}
 defer newRelease()
 newResult,err:=registry.BeginProbe(proof.DeviceID,strings.Repeat("e",32))
 if err!=nil{t.Fatal(err)}
 second,ok:=registry.TakeProbe(proof.DeviceID,proof.Nonce)
 if !ok || second.ProbeID==first.ProbeID {t.Fatal("new browser probe unavailable")}
 if registry.RejectProbe(proof.DeviceID,proof.Nonce,first.ProbeID) {
  t.Fatal("late old rejection cancelled newer browser")
 }
 if !registry.IsCurrentProbe(proof.DeviceID,proof.Nonce,second.ProbeID) {
  t.Fatal("new browser probe lost after stale rejection")
 }
 if !registry.RejectProbe(proof.DeviceID,proof.Nonce,second.ProbeID) {
  t.Fatal("new browser probe could not be independently rejected")
 }
 if (<-newResult).OK{t.Fatal("rejected new browser was marked successful")}
}
