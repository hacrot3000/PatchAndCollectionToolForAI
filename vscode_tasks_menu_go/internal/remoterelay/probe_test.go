package remoterelay

import (
	"crypto/ed25519"
	"encoding/base64"
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
