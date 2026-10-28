package remoterelay

import (
	"encoding/base64"
	"errors"
)

// EncryptedEcho is a single, one-shot, opaque diagnostic mailbox. The Relay
// never decrypts ciphertext and has no route to a TaskDeck HTTP handler.
// There is one echo request per authenticated browser reservation.
type encryptedEcho struct {
	probeID string
	seq uint64
	payload string
	delivered bool
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
		state.echo.payload!="" {
		return nil,errors.New("no authorized encrypted diagnostic session")
	}
	state.echo.payload=ciphertext
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
	state.echo=nil
	return true
}

func (r *Registry) clearEcho(state *deviceState) {
	if state.echo!=nil {
		close(state.echo.reply)
		state.echo=nil
	}
}
