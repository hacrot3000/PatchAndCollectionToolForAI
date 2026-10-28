package remoterelay

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

const BrowserAttestationProtocol = "taskdeck-browser-attestation-v1"

type ProbeCrypto struct {
	Identity string `json:"identity"`
	Ephemeral string `json:"ephemeral"`
	Signature string `json:"signature"`
	EncryptedReady string `json:"encrypted_ready"`
}

type ProbeAttestation struct {
	ProbeID string `json:"probe_id"`
	ClientNonce string `json:"client_nonce"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
	Crypto *ProbeCrypto `json:"crypto,omitempty"`
}

type ProbeOutcome struct {
	OK bool
	Attestation ProbeAttestation
}

type ProbeRequest struct {
	ProbeID string `json:"probe_id"`
	ClientNonce string `json:"client_nonce"`
	BrowserKey string `json:"browser_key,omitempty"`
}

type probeState struct {
	id string
	clientNonce string
	browserKey string
	sent bool
	completed bool
	result chan ProbeOutcome
}

func (p *probeState) signal(outcome ProbeOutcome) {
	if p.completed { return }
	p.completed=true
	p.result <- outcome
	close(p.result)
}

func validProbeNonce(nonce string) bool {
	return jtiPattern.MatchString(nonce)
}

// BeginProbe binds an authenticated Portal browser request's fresh random
// nonce to the exact device session; neither Relay nor Agent can choose it.
func (r *Registry) BeginProbe(deviceID, clientNonce string, browserPublic ...string) (<-chan ProbeOutcome,error) {
	if !validProbeNonce(clientNonce) || len(browserPublic)>1 {
		return nil,errors.New("invalid browser challenge")
	}
	var browserKey string
	if len(browserPublic)==1 {
		browserKey=browserPublic[0]
		key,err:=base64.StdEncoding.Strict().DecodeString(browserKey)
		if err!=nil || remotetransport.VerifyPublicKey(key)!=nil {
			return nil,errors.New("invalid browser ephemeral public key")
		}
	}
	var random [16]byte
	if _,err:=rand.Read(random[:]);err!=nil{return nil,err}
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[deviceID]
	if !ok || !state.busy || state.probe!=nil {
		return nil,errors.New("probe requires an exclusive authorized browser session")
	}
	p:=&probeState{id:hex.EncodeToString(random[:]),clientNonce:clientNonce,
		browserKey:browserKey,result:make(chan ProbeOutcome,1)}
	state.probe=p
	return p.result,nil
}

func (r *Registry) TakeProbe(deviceID,connectionNonce string) (ProbeRequest,bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[deviceID]
	if !ok || state.nonce!=connectionNonce || state.probe==nil || state.probe.sent {
		return ProbeRequest{},false
	}
	state.probe.sent=true
	return ProbeRequest{ProbeID:state.probe.id,ClientNonce:state.probe.clientNonce,
		BrowserKey:state.probe.browserKey},true
}

// IsCurrentProbe distinguishes a forged reply for the current probe from
// a delayed, now-orphaned ACK belonging to an older browser reservation.
func (r *Registry) IsCurrentProbe(deviceID,connectionNonce,probeID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[deviceID]
	return ok && state.nonce==connectionNonce && state.probe!=nil &&
		state.probe.id==probeID && state.probe.sent
}

func (r *Registry) AckProbe(deviceID,connectionNonce string,ack ProbeAttestation) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[deviceID]
	if !ok || state.nonce!=connectionNonce || state.probe==nil ||
		!state.probe.sent || state.probe.id!=ack.ProbeID ||
		state.probe.clientNonce!=ack.ClientNonce {
		return false
	}
	public,err:=base64.StdEncoding.Strict().DecodeString(ack.PublicKey)
	if err!=nil || len(public)!=ed25519.PublicKeySize { return false }
	hash:=sha256.Sum256(public)
	if hex.EncodeToString(hash[:])!=deviceID {return false}
	sig,err:=base64.StdEncoding.Strict().DecodeString(ack.Signature)
	if err!=nil || len(sig)!=ed25519.SignatureSize{return false}
	message:=strings.Join([]string{
		BrowserAttestationProtocol,deviceID,ack.ProbeID,ack.ClientNonce,
	},string([]byte{10}))
	if !ed25519.Verify(public,[]byte(message),sig) {return false}
	if state.probe.browserKey!="" {
		if ack.Crypto==nil{return false}
		browserPublic,err:=base64.StdEncoding.Strict().DecodeString(state.probe.browserKey)
		if err!=nil{return false}
		identity,err:=base64.StdEncoding.Strict().DecodeString(ack.Crypto.Identity)
		if err!=nil{return false}
		ephemeral,err:=base64.StdEncoding.Strict().DecodeString(ack.Crypto.Ephemeral)
		if err!=nil{return false}
		cryptoSignature,err:=base64.StdEncoding.Strict().DecodeString(ack.Crypto.Signature)
		if err!=nil{return false}
		ready,err:=base64.StdEncoding.Strict().DecodeString(ack.Crypto.EncryptedReady)
		if err!=nil || len(ready)<25 || len(ready)>256{return false}
		if err:=remotetransport.VerifyHello(deviceID,ack.ProbeID,ack.ClientNonce,browserPublic,
			remotetransport.Hello{Identity:identity,Ephemeral:ephemeral,Signature:cryptoSignature});err!=nil{return false}
	}
	if state.probe.browserKey!="" {
		if state.echo!=nil {return false}
		state.echo=&encryptedEcho{
			probeID:ack.ProbeID,seq:state.sessionSeq,reply:make(chan string,1),
		}
	}
	state.probe.signal(ProbeOutcome{OK:true,Attestation:ack})
	state.probe=nil
	return true
}
