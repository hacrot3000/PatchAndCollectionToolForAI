package remoterelay

import (
 "encoding/base64"
 "errors"
)

// encryptedLogin is a one-shot opaque mailbox, independent of diagnostic
// echo. Relay never parses TaskDeck credentials or decrypts application data.
type encryptedLogin struct {
 probeID string
 seq uint64
 payload string
 delivered bool
 reply chan string
}

func validLoginCiphertext(encoded string) bool {
 if len(encoded)<36 || len(encoded)>9000{return false}
 raw,err:=base64.StdEncoding.Strict().DecodeString(encoded)
 return err==nil && len(raw)>=25 && len(raw)<=9+6144+16
}

// QueueLogin may replace a *fresh*, unused diagnostic echo slot only.
// No pipelining, login-after-echo, second password guess or arbitrary command.
func (r *Registry) QueueLogin(deviceID,probeID,ciphertext string) (<-chan string,error) {
 if !validLoginCiphertext(ciphertext){return nil,errors.New("invalid encrypted login length")}
 r.mu.Lock()
 defer r.mu.Unlock()
 state,ok:=r.devices[deviceID]
 if !ok || !state.busy || state.login!=nil || state.echo==nil ||
  state.echo.seq!=state.sessionSeq || state.echo.probeID!=probeID ||
  (state.echo.rounds!=0 && !(state.echo.rounds==1 && state.echo.keyCheck)) ||
  state.echo.payload!="" || state.echo.delivered {
  return nil,errors.New("no fresh authorized login session")
 }
 state.echo=nil
 state.login=&encryptedLogin{probeID:probeID,seq:state.sessionSeq,payload:ciphertext,
  reply:make(chan string,1)}
 return state.login.reply,nil
}

func (r *Registry) TakeLogin(deviceID,nonce string) (probeID,ciphertext string,ok bool){
 r.mu.Lock()
 defer r.mu.Unlock()
 state,exists:=r.devices[deviceID]
 if !exists || state.nonce!=nonce || !state.busy || state.login==nil ||
  state.login.seq!=state.sessionSeq || state.login.delivered{return "","",false}
 state.login.delivered=true
 return state.login.probeID,state.login.payload,true
}

func (r *Registry) AckLogin(deviceID,nonce,probeID,ciphertext string) bool {
 if !validLoginCiphertext(ciphertext){return false}
 r.mu.Lock()
 defer r.mu.Unlock()
 state,ok:=r.devices[deviceID]
 if !ok || state.nonce!=nonce || !state.busy || state.login==nil ||
  !state.login.delivered || state.login.seq!=state.sessionSeq ||
  state.login.probeID!=probeID{return false}
 state.login.reply<-ciphertext
 close(state.login.reply)
 state.lastLoginProbeID=probeID
 state.login=nil
 return true
}

// RejectLogin cancels only the matching in-flight Browser login. The Agent
// device/presence connection stays authenticated and online. This is used
// for invalid ciphertext, a disabled local login callback, revoked grant,
// or any other fail-closed TaskDeck authentication path.
func (r *Registry) RejectLogin(deviceID,nonce,probeID string) bool {
 r.mu.Lock()
 defer r.mu.Unlock()
 state,ok:=r.devices[deviceID]
 if !ok || state.nonce!=nonce || !state.busy || state.login==nil ||
  !state.login.delivered || state.login.seq!=state.sessionSeq ||
  state.login.probeID!=probeID {return false}
 close(state.login.reply)
 state.login=nil
 return true
}

func (r *Registry) IsCurrentLogin(deviceID,nonce,probeID string) bool {
 r.mu.Lock()
 defer r.mu.Unlock()
 state,ok:=r.devices[deviceID]
 return ok && state.nonce==nonce && state.login!=nil &&
  state.login.seq==state.sessionSeq && state.login.probeID==probeID &&
  state.login.delivered
}

func (r *Registry) clearLogin(state *deviceState) {
 if state.login!=nil{
  close(state.login.reply)
  state.login=nil
 }
}
