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

// INTERNAL, UNMOUNTED staging component. This Relay never sees application
// plaintext, local TaskDeck principals, passwords, URLs or command arguments.
// Nothing in HTTP, browserSocket, deviceSocket or PHP routes into this mailbox.
const MaxAppPingRounds=4
const maxAppPingCiphertext=512+9+16
const appReadyProofDomain="taskdeck-e2ee-app-ready-v1"
var ErrAppMailboxUnavailable=errors.New("verified E2EE application mailbox unavailable")

type pendingAppPing struct {
 relayID string
 ciphertext string
 delivered bool
 reply chan string
}
type opaqueAppMailbox struct {
 probeID string
 seq uint64
 rounds int
 pending *pendingAppPing
}
func appReadyWire(deviceID,agentNonce,probeID string) []byte {
 return []byte(strings.Join([]string{appReadyProofDomain,deviceID,agentNonce,probeID},"\n"))
}
func validAppPingCiphertext(encoded string) bool {
 if len(encoded)<36 || len(encoded)>716 {return false}
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
 if r==nil || !validProbeNonce(agentNonce) || !validProbeNonce(probeID) {
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
 st.app=&opaqueAppMailbox{probeID:probeID,seq:st.sessionSeq}
 // Consume the one-use authenticated login slot. Even after quota
 // exhaustion or explicit Close, replaying the signed ready proof cannot
 // reopen a fresh mailbox without a NEW TaskDeck password login.
 st.lastLoginProbeID=""
 return nil
}

// QueueAppPing only accepts a bounded, opaque AEAD record. One request is
// outstanding; its Relay nonce cannot be chosen by Browser, PHP or Agent.
func (r *Registry) QueueAppPing(deviceID,probeID,ciphertext string)(string,<-chan string,error){
 if r==nil || !validAppPingCiphertext(ciphertext){return "",nil,ErrAppMailboxUnavailable}
 var random [16]byte
 if _,err:=rand.Read(random[:]);err!=nil{return "",nil,ErrAppMailboxUnavailable}
 id:=hex.EncodeToString(random[:])
 r.mu.Lock()
 defer r.mu.Unlock()
 st,ok:=r.devices[deviceID]
 if !ok || !st.busy || st.app==nil || st.app.seq!=st.sessionSeq ||
  st.app.probeID!=probeID || st.app.pending!=nil ||
  st.app.rounds>=MaxAppPingRounds {
  return "",nil,ErrAppMailboxUnavailable
 }
 p:=&pendingAppPing{relayID:id,ciphertext:ciphertext,reply:make(chan string,1)}
 st.app.pending=p
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
 p:=st.app.pending
 p.reply<-ciphertext
 close(p.reply)
 st.app.pending=nil
 st.app.rounds++
 if st.app.rounds>=MaxAppPingRounds {st.app=nil}
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
 if st.app.pending!=nil{
  close(st.app.pending.reply)
  st.app.pending=nil
 }
 st.app=nil
}
