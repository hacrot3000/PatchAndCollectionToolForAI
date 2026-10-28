package remoterelay

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

const BrowserAttestationProtocol = "taskdeck-browser-attestation-v1"

type ProbeAttestation struct {
	ProbeID string `json:"probe_id"`
	ClientNonce string `json:"client_nonce"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

type ProbeOutcome struct {
	OK bool
	Attestation ProbeAttestation
}

type ProbeRequest struct {
	ProbeID string `json:"probe_id"`
	ClientNonce string `json:"client_nonce"`
}

type probeState struct {
	id string
	clientNonce string
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
func (r *Registry) BeginProbe(deviceID, clientNonce string) (<-chan ProbeOutcome,error) {
	if !validProbeNonce(clientNonce) {
		return nil,errors.New("invalid browser challenge")
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
		result:make(chan ProbeOutcome,1)}
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
	return ProbeRequest{ProbeID:state.probe.id,ClientNonce:state.probe.clientNonce},true
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
	state.probe.signal(ProbeOutcome{OK:true,Attestation:ack})
	state.probe=nil
	return true
}
