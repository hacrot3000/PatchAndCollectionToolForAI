package remoterelay

import (
	"encoding/base64"
	"errors"
)

// EncryptedEcho is a single, one-shot, opaque diagnostic mailbox. The Relay
// never decrypts ciphertext and has no route to a TaskDeck HTTP handler.
// There is one echo request per authenticated browser reservation.
// MaxDiagnosticRounds bounds the diagnostic stream; it is NOT a TaskDeck API.
const MaxDiagnosticRounds = 3

type encryptedEcho struct {
	probeID string
	seq uint64
	payload string
	delivered bool
	rounds int
 // True only for a single encrypted pre-login client-key check.
 keyCheck bool
	reply chan string
}

func validEchoCiphertext(encoded string) bool {
	if len(encoded)<36 || len(encoded)>1500 { return false }
	raw,err:=base64.StdEncoding.Strict().DecodeString(encoded)
	return err==nil && len(raw)>=25 && len(raw)<=1024
}

func (r *Registry) QueueEcho(deviceID,probeID,ciphertext string) (<-chan string,error) {
	if !validEchoCiphertext(ciphertext) {
		return nil,errors.New("invalid encrypted echo frame")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[deviceID]
	if !ok || !state.busy || state.echo==nil ||
		state.echo.probeID!=probeID || state.echo.seq!=state.sessionSeq ||
		state.echo.payload!="" || state.echo.rounds>=MaxDiagnosticRounds {
		return nil,errors.New("no authorized encrypted diagnostic session")
	}
	state.echo.payload=ciphertext
	state.echo.delivered=false
	state.echo.reply=make(chan string,1)
	return state.echo.reply,nil
}

// QueueClientKeyCheck uses the existing opaque E2EE mailbox exactly once.
// The Relay cannot read the public key or Agent's encrypted approval status.
func (r *Registry) QueueClientKeyCheck(deviceID,probeID,ciphertext string)(<-chan string,error){
 if !validEchoCiphertext(ciphertext){return nil,errors.New("invalid encrypted key-check size")}
 r.mu.Lock();defer r.mu.Unlock()
 state,ok:=r.devices[deviceID]
 if !ok||!state.busy||state.echo==nil||state.login!=nil||
  state.echo.probeID!=probeID||state.echo.seq!=state.sessionSeq||
  state.echo.rounds!=0||state.echo.payload!=""||state.echo.delivered||state.echo.keyCheck {
  return nil,errors.New("no fresh client-key check window")
 }
 state.echo.keyCheck=true
 state.echo.payload=ciphertext
 state.echo.reply=make(chan string,1)
 return state.echo.reply,nil
}

func (r *Registry) TakeEcho(deviceID,connectionNonce string) (probeID,ciphertext string,ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state,exists:=r.devices[deviceID]
	if !exists || state.nonce!=connectionNonce || state.echo==nil ||
		!state.busy || state.echo.seq!=state.sessionSeq ||
		state.echo.payload=="" || state.echo.delivered {
		return "","",false
	}
	state.echo.delivered=true
	return state.echo.probeID,state.echo.payload,true
}

func (r *Registry) HasActiveEcho(deviceID,connectionNonce string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[deviceID]
	return ok && state.nonce==connectionNonce && state.echo!=nil
}

// IsCurrentEcho ensures a late ACK for browser A does not disconnect Agent
// after browser B has been granted a different probe/session ID.
func (r *Registry) IsCurrentEcho(deviceID,connectionNonce,probeID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[deviceID]
	return ok && state.nonce==connectionNonce && state.echo!=nil &&
		state.echo.probeID==probeID && state.echo.delivered &&
		state.echo.seq==state.sessionSeq
}

func (r *Registry) AckEcho(deviceID,connectionNonce,probeID,ciphertext string) bool {
	if !validEchoCiphertext(ciphertext) { return false }
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[deviceID]
	if !ok || state.nonce!=connectionNonce || !state.busy ||
		state.echo==nil || !state.echo.delivered ||
		state.echo.probeID!=probeID || state.echo.seq!=state.sessionSeq {
		return false
	}
	state.echo.reply<-ciphertext
	close(state.echo.reply)
	state.echo.reply=nil
	state.echo.payload=""
	state.echo.delivered=false
	state.echo.rounds++
	if state.echo.rounds>=MaxDiagnosticRounds {
		state.echo=nil
	}
	return true
}

// RejectEcho cancels only the exact Browser's currently delivered echo.
// A tampered or malformed encrypted diagnostic must never force the
// authenticated Agent presence connection offline. Late rejections cannot
// terminate a different (newer) Browser's reservation.
func (r *Registry) RejectEcho(deviceID,connectionNonce,probeID string) bool {
 r.mu.Lock()
 defer r.mu.Unlock()
 state,ok:=r.devices[deviceID]
 if !ok || state.nonce!=connectionNonce || !state.busy ||
  state.echo==nil || !state.echo.delivered ||
  state.echo.probeID!=probeID || state.echo.seq!=state.sessionSeq {
  return false
 }
 if state.echo.reply!=nil {close(state.echo.reply)}
 state.echo=nil
 return true
}

func (r *Registry) clearEcho(state *deviceState) {
	if state.echo!=nil {
		if state.echo.reply!=nil {
			close(state.echo.reply)
		}
		state.echo=nil
	}
}
