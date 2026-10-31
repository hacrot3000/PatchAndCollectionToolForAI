package remoterelay

import (
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "errors"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// INTERNAL, UNMOUNTED staging component. This Relay never sees application
// plaintext, local TaskDeck principals, passwords, URLs or command arguments.
// Nothing in HTTP, browserSocket, deviceSocket or PHP routes into this mailbox.
const MaxAppPingRounds=6
const appMailboxLifetime=5*time.Minute
const appPingReplyDeadline=20*time.Second
// Application ciphertext is opaque to Relay. The 6-round, 5-minute and
// single-outstanding-request limits are unchanged. Both endpoints apply
// stricter, typed plaintext validation after AEAD authentication.
const maxAppPingCiphertext=remotetransport.MaxEncryptedAppFrame
const MaxEncodedAppCiphertext=((maxAppPingCiphertext+2)/3)*4
const appReadyProofDomain="taskdeck-e2ee-app-ready-v1"
var ErrAppMailboxUnavailable=errors.New("verified E2EE application mailbox unavailable")

type pendingAppPing struct {
 relayID string
 ciphertext string
 deadline time.Time
 delivered bool
 reply chan string
 timer *time.Timer
}
type opaqueAppMailbox struct {
 probeID string
 seq uint64
 expiresAt time.Time
 rounds int
 fileTransferOnly bool
 pending *pendingAppPing
 timer *time.Timer
}
func appReadyWire(deviceID,agentNonce,probeID string) []byte {
 return []byte(strings.Join([]string{appReadyProofDomain,deviceID,agentNonce,probeID},"\n"))
}
func validAppPingCiphertext(encoded string) bool {
 if len(encoded)<36 || len(encoded)>MaxEncodedAppCiphertext {return false}
 raw,err:=base64.StdEncoding.Strict().DecodeString(encoded)
 return err==nil && len(raw)>=25 && len(raw)<=maxAppPingCiphertext &&
  base64.StdEncoding.EncodeToString(raw)==encoded
}

// ActivateAppPing is reserved for a signed, authenticated Agent device socket
// AFTER a separate host-local approved client key + password login created
// a live in-process TaskDeck session. A signature only authenticates which
// Agent asserted readiness: it is NOT by itself TaskDeck authorization.
// Application actions must still be opened and checked by TaskDeck local
// E2EE+RBAC+live Portal grant. No external handler invokes this method yet.
func (r *Registry) ActivateAppPing(deviceID,agentNonce,probeID,publicKey,signature string) error {
 if r==nil || !validProbeNonce(agentNonce) || !validProbeNonce(probeID) ||
  len(publicKey)!=base64.StdEncoding.EncodedLen(ed25519.PublicKeySize) ||
  len(signature)!=base64.StdEncoding.EncodedLen(ed25519.SignatureSize) {
  // Bound untrusted encoded inputs BEFORE allocating or verifying signatures.
  return ErrAppMailboxUnavailable
 }
 pub,err:=base64.StdEncoding.Strict().DecodeString(publicKey)
 if err!=nil || len(pub)!=ed25519.PublicKeySize{return ErrAppMailboxUnavailable}
 sum:=sha256.Sum256(pub)
 if hex.EncodeToString(sum[:])!=deviceID{return ErrAppMailboxUnavailable}
 sig,err:=base64.StdEncoding.Strict().DecodeString(signature)
 if err!=nil || len(sig)!=ed25519.SignatureSize ||
  !ed25519.Verify(pub,appReadyWire(deviceID,agentNonce,probeID),sig) {
  return ErrAppMailboxUnavailable
 }
 r.mu.Lock()
 defer r.mu.Unlock()
 st,ok:=r.devices[deviceID]
 if !ok || !st.busy || st.nonce!=agentNonce || st.sessionSeq==0 ||
  st.lastLoginProbeID!=probeID || st.app!=nil ||
  st.probe!=nil || st.login!=nil || st.echo!=nil {
  return ErrAppMailboxUnavailable
 }
 app:=&opaqueAppMailbox{probeID:probeID,seq:st.sessionSeq,
  expiresAt:time.Now().Add(appMailboxLifetime)}
 st.app=app
 // Even an idle Agent/Browser reservation must release its opaque
 // mailbox without requiring another malicious Relay frame or request.
 // The pointer and generation checks prevent stale timers from closing
 // a replacement login or another authenticated device connection.
 app.timer=time.AfterFunc(appMailboxLifetime,func(){
  r.mu.Lock()
  defer r.mu.Unlock()
  current,exists:=r.devices[deviceID]
  if exists && current==st && current.nonce==agentNonce &&
   current.app==app && current.sessionSeq==app.seq {
   r.clearAppMailbox(current)
  }
 })
 // Consume the one-use authenticated login slot. Even after quota
 // exhaustion or explicit Close, replaying the signed ready proof cannot
 // reopen a fresh mailbox without a NEW TaskDeck password login.
 st.lastLoginProbeID=""
 return nil
}

// QueueAppPing only accepts a bounded, opaque AEAD record. One request is
// outstanding; its Relay nonce cannot be chosen by Browser, PHP or Agent.
// The opaque Relay may increase its six-round limit ONLY on fresh, signed
// Agent evidence issued after an independently authorized FilesRead chunk.
// Neither a PHP ticket nor the browser can make this statement.
func (r *Registry) AuthorizeScopedFileTransfer(deviceID,agentNonce,probeID,publicKey,signature string) error {
 denied:=ErrAppMailboxUnavailable
 if r==nil||!validProbeNonce(agentNonce)||!validProbeNonce(probeID)||
  len(publicKey)!=base64.StdEncoding.EncodedLen(ed25519.PublicKeySize)||
  len(signature)!=base64.StdEncoding.EncodedLen(ed25519.SignatureSize){return denied}
 pub,err:=base64.StdEncoding.Strict().DecodeString(publicKey)
 if err!=nil||len(pub)!=ed25519.PublicKeySize{return denied}
 hash:=sha256.Sum256(pub)
 if hex.EncodeToString(hash[:])!=deviceID{return denied}
 sig,err:=base64.StdEncoding.Strict().DecodeString(signature)
 if err!=nil||len(sig)!=ed25519.SignatureSize{return denied}
 proof:=[]byte(strings.Join([]string{
  remotetransport.ScopedFileTransferProofDomain,deviceID,agentNonce,probeID,
 },"\n"))
 if !ed25519.Verify(pub,proof,sig){return denied}
 r.mu.Lock()
 defer r.mu.Unlock()
 st,ok:=r.devices[deviceID]
 if !ok||!st.busy||st.nonce!=agentNonce||st.app==nil||
  st.app.seq!=st.sessionSeq||st.app.probeID!=probeID||
  st.app.fileTransferOnly||st.app.pending!=nil||
  st.app.rounds<1||st.app.rounds>=appRoundLimit(st.app)||
  !time.Now().Before(st.app.expiresAt){return denied}
 st.app.fileTransferOnly=true
 return nil
}
func appRoundLimit(app *opaqueAppMailbox)int {
 if app!=nil&&app.fileTransferOnly{return remotetransport.MaxScopedFileTransferRounds}
 return MaxAppPingRounds
}

func (r *Registry) QueueAppPing(deviceID,probeID,ciphertext string)(string,<-chan string,error){
 return r.queueAppPingWithTimeout(deviceID,probeID,ciphertext,appPingReplyDeadline)
}

// The unexported bounded timeout variation exists solely for deterministic
// expiry tests; the production admission path always uses the 20s maximum.
func (r *Registry) queueAppPingWithTimeout(deviceID,probeID,ciphertext string,
 timeout time.Duration)(string,<-chan string,error){
 if r==nil || timeout<=0 || timeout>appPingReplyDeadline ||
  !validAppPingCiphertext(ciphertext){return "",nil,ErrAppMailboxUnavailable}
 var random [16]byte
 if _,err:=rand.Read(random[:]);err!=nil{return "",nil,ErrAppMailboxUnavailable}
 id:=hex.EncodeToString(random[:])
 r.mu.Lock()
 defer r.mu.Unlock()
 st,ok:=r.devices[deviceID]
 if !ok || !st.busy || st.app==nil || st.app.seq!=st.sessionSeq ||
  st.app.probeID!=probeID || st.app.pending!=nil ||
  st.app.rounds>=appRoundLimit(st.app) {
  return "",nil,ErrAppMailboxUnavailable
 }
 if !time.Now().Before(st.app.expiresAt){
  r.clearAppMailbox(st)
  return "",nil,ErrAppMailboxUnavailable
 }
 app:=st.app
 p:=&pendingAppPing{relayID:id,ciphertext:ciphertext,
  deadline:time.Now().Add(timeout),reply:make(chan string,1)}
 app.pending=p
 // A compromised PHP/Relay can simply never send a valid ACK. Without
 // independent expiry, the waiting native caller remains blocked forever
 // unless some unrelated request happens to revisit this mailbox.
 // Auto-expiry is fail-closed: it retires the whole app mailbox and signals
 // its pending caller, never restores quota or accepts a late Agent ACK.
 p.timer=time.AfterFunc(timeout,func(){
  r.mu.Lock()
  defer r.mu.Unlock()
  current,exists:=r.devices[deviceID]
  if exists && current==st && current.sessionSeq==app.seq &&
   current.app==app && app.pending==p {
   r.clearAppMailbox(current)
  }
 })
 return id,p.reply,nil
}

func (r *Registry) TakeAppPing(deviceID,agentNonce string)(probeID,relayID,ciphertext string,ok bool){
 if r==nil{return "","","",false}
 r.mu.Lock()
 defer r.mu.Unlock()
 st,exists:=r.devices[deviceID]
 if !exists || !st.busy || st.nonce!=agentNonce || st.app==nil ||
  st.app.seq!=st.sessionSeq || st.app.pending==nil ||
  st.app.pending.delivered {return "","","",false}
 if !time.Now().Before(st.app.expiresAt) ||
  !time.Now().Before(st.app.pending.deadline) {
  r.clearAppMailbox(st)
  return "","","",false
 }
 p:=st.app.pending
 p.delivered=true
 return st.app.probeID,p.relayID,p.ciphertext,true
}

func (r *Registry) AckAppPing(deviceID,agentNonce,probeID,relayID,ciphertext string)bool{
 if r==nil || !validAppPingCiphertext(ciphertext){return false}
 r.mu.Lock()
 defer r.mu.Unlock()
 st,ok:=r.devices[deviceID]
 if !ok || !st.busy || st.nonce!=agentNonce || st.app==nil ||
  st.app.seq!=st.sessionSeq || st.app.probeID!=probeID ||
  st.app.pending==nil || !st.app.pending.delivered ||
  st.app.pending.relayID!=relayID {return false}
 if !time.Now().Before(st.app.expiresAt) ||
  !time.Now().Before(st.app.pending.deadline) {
  r.clearAppMailbox(st)
  return false
 }
 p:=st.app.pending
 if p.timer!=nil {p.timer.Stop()}
 p.reply<-ciphertext
 close(p.reply)
 st.app.pending=nil
 st.app.rounds++
 if st.app.rounds>=appRoundLimit(st.app) {r.clearAppMailbox(st)}
 return true
}

func (r *Registry) CloseAppPing(deviceID,agentNonce,probeID string)bool{
 if r==nil{return false}
 r.mu.Lock()
 defer r.mu.Unlock()
 st,ok:=r.devices[deviceID]
 if !ok || !st.busy || st.nonce!=agentNonce || st.app==nil ||
  st.app.seq!=st.sessionSeq || st.app.probeID!=probeID {return false}
 r.clearAppMailbox(st)
 return true
}
func (r *Registry) clearAppMailbox(st *deviceState){
 if st==nil || st.app==nil{return}
 if st.app.timer!=nil {st.app.timer.Stop()}
 if st.app.pending!=nil{
  if st.app.pending.timer!=nil {st.app.pending.timer.Stop()}
  close(st.app.pending.reply)
  st.app.pending=nil
 }
 st.app=nil
}

    
// Readiness is never inferred from a PHP ticket or login ACK. Only the
// registered Agent's signed app_ready opens this exact bounded mailbox.
func (r *Registry) AppPingReady(deviceID,probeID string)bool{
 if r==nil{return false}
 r.mu.Lock()
 defer r.mu.Unlock()
 st,ok:=r.devices[deviceID]
 return ok && st.busy && st.app!=nil && st.app.seq==st.sessionSeq &&
  st.app.probeID==probeID && st.app.pending==nil &&
  st.app.rounds<appRoundLimit(st.app) && time.Now().Before(st.app.expiresAt)
}
