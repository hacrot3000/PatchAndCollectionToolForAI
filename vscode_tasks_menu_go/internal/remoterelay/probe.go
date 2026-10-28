package remoterelay

import (
 "crypto/rand"
 "encoding/hex"
 "errors"
)

type probeState struct {
 id string
 sent bool
 completed bool
 result chan bool
}
func (p *probeState) signal(ok bool) {
 if p.completed{return}
 p.completed=true
 p.result<-ok
 close(p.result)
}

// BeginProbe starts one bounded, read-only reachability check for the device
// currently reserved by a verified Portal ticket. It grants no TaskDeck access.
func (r *Registry) BeginProbe(deviceID string) (<-chan bool,error) {
 var nonce [16]byte
 if _,err:=rand.Read(nonce[:]);err!=nil{return nil,err}
 r.mu.Lock()
 defer r.mu.Unlock()
 state,ok:=r.devices[deviceID]
 if !ok || !state.busy || state.probe!=nil {
  return nil,errors.New("probe requires an exclusive authorized browser session")
 }
 p:=&probeState{id:hex.EncodeToString(nonce[:]),result:make(chan bool,1)}
 state.probe=p
 return p.result,nil
}

// TakeProbe is only called for an authenticated, currently leased Agent socket.
func (r *Registry) TakeProbe(deviceID,connectionNonce string) (string,bool) {
 r.mu.Lock()
 defer r.mu.Unlock()
 state,ok:=r.devices[deviceID]
 if !ok || state.nonce!=connectionNonce || state.probe==nil || state.probe.sent {
  return "",false
 }
 state.probe.sent=true
 return state.probe.id,true
}

func (r *Registry) AckProbe(deviceID,connectionNonce,probeID string) bool {
 r.mu.Lock()
 defer r.mu.Unlock()
 state,ok:=r.devices[deviceID]
 if !ok || state.nonce!=connectionNonce || state.probe==nil ||
  !state.probe.sent || state.probe.id!=probeID {return false}
 state.probe.signal(true)
 state.probe=nil
 return true
}
