package remoterelay

import (
	"errors"
	"sync"
	"time"
)

// Registry stores ONLY ephemeral authenticated device presence and session
// occupancy in memory. Neither IP addresses nor TaskDeck credentials are
// persisted, and an unauthenticated client cannot select another project.
type Registry struct {
	mu sync.Mutex
	audience string
	devices map[string]*deviceState
	nonces map[string]int64
}

type deviceState struct {
	nonce string
	lastSeen int64
	busy bool
	// Incremented per browser reservation; prevents a stale cleanup callback
	// from releasing a newer session on the same connected Agent.
	sessionSeq uint64
	probe *probeState
	echo *encryptedEcho
	login *encryptedLogin
}

// NewRegistry accepts the exact public Relay HTTPS origin used in
// device-connect signatures, never a TaskDeck-local URL.
func NewRegistry(audience string) (*Registry, error) {
	if !validIssuer(audience) {
		return nil, errors.New("invalid Relay audience")
	}
	return &Registry{audience: audience, devices: make(map[string]*deviceState),
		nonces: make(map[string]int64)}, nil
}

// Register refuses simultaneous device connections for the same ID.
// Reconnecting must explicitly release the previous socket first.
func (r *Registry) Register(proof DeviceProof, now time.Time) error {
	if err:=VerifyDeviceProof(proof,r.audience,now);err!=nil {return err}
	r.mu.Lock()
	defer r.mu.Unlock()
	sec:=now.Unix()
	for k,expires:=range r.nonces {if expires<=sec {delete(r.nonces,k)}}
	key:=proof.DeviceID+":"+proof.Nonce
	if _,used:=r.nonces[key];used{return errors.New("device proof replay")}
	if _,connected:=r.devices[proof.DeviceID];connected{return errors.New("device already connected")}
	if len(r.nonces)>=20000||len(r.devices)>=10000 {
		return errors.New("relay registry capacity reached")
	}
	r.nonces[key]=sec+120
	r.devices[proof.DeviceID]=&deviceState{nonce:proof.Nonce,lastSeen:sec}
	return nil
}

func (r *Registry) Online(deviceID string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[deviceID]
	return ok && now.Unix()-state.lastSeen<=60
}

// Touch must be invoked only on an already authenticated agent connection,
// identified by its original nonce. An arbitrary socket cannot mark a device online.
func (r *Registry) Touch(deviceID, connectionNonce string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[deviceID]
	if !ok || state.nonce!=connectionNonce{return false}
	state.lastSeen=now.Unix()
	return true
}

// Acquire is intentionally exclusive until a versioned multiplex transport
// exists. The returned release closure is bound to one Device ID and nonce.
func (r *Registry) Acquire(claims Claims, now time.Time) (func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[claims.DeviceID]
	if !ok||now.Unix()-state.lastSeen>60{return nil,errors.New("target device offline")}
	if state.busy{return nil,errors.New("device already has an active session")}
	state.busy=true
	state.sessionSeq++
	connectionNonce:=state.nonce
	sessionSeq:=state.sessionSeq
	var released sync.Once
	return func(){
		released.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if current,exists:=r.devices[claims.DeviceID];exists &&
				current.nonce==connectionNonce && current.busy &&
				current.sessionSeq==sessionSeq {
				current.busy=false
				r.clearEcho(current)
				r.clearLogin(current)
				if current.probe!=nil {
					current.probe.signal(ProbeOutcome{})
					current.probe=nil
				}
			}
		})
	},nil
}

// Unregister is idempotent and rejects stale disconnect callbacks.
func (r *Registry) Unregister(deviceID, connectionNonce string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state,ok:=r.devices[deviceID]
	if ok&&state.nonce==connectionNonce{
		r.clearEcho(state)
		r.clearLogin(state)
		if state.probe!=nil {state.probe.signal(ProbeOutcome{})}
		delete(r.devices,deviceID)
	}
}
