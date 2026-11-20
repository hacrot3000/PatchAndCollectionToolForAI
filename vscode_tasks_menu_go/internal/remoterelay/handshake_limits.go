package remoterelay

import (
	"net"
	"sync"
	"time"
)

// TCP-peer handshake rate limits are separate from active WebSocket slots.
// A peer can repeatedly connect and disconnect without ever exhausting its
// concurrent socket budget. Count admitted upgrades BEFORE websocket.Accept.
// These limits are process-local: a multi-replica deployment must add an
// edge/proxy rate limiter with a shared source-of-truth.
const (
	relayHandshakeWindow = time.Minute
	maxBrowserHandshakesPerSource = 40
	maxDeviceHandshakesPerSource = 180
	maxBrowserHandshakesGlobal = 300
	maxDeviceHandshakesGlobal = 2400
	maxHandshakeSources = 4096
)

type handshakeCounter struct {
	start time.Time
	count int
}

type handshakeBudget struct {
	mu sync.Mutex
	sources map[string]handshakeCounter
	global handshakeCounter
	requests uint64
}

// admit is a fixed-window CPU/handshake admission guard. The direct TCP peer
// address is trusted, NEVER X-Forwarded-For. Invalid addresses, overflowing
// cardinality, future/rolled-back clocks and full budgets fail closed.
// Denied requests do not change any existing per-IP or global counters.
func (b *handshakeBudget) admit(remote string, now time.Time, perSource, total int) bool {
	host,_,err:=net.SplitHostPort(remote)
	if err!=nil{return false}
	ip:=net.ParseIP(host)
	if ip==nil{return false}
	source:=ip.String() // canonical IPv4-mapped IPv6 form
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sources==nil{b.sources=make(map[string]handshakeCounter)}
	// At most 4096 IP entries per class; expired entries are removed
	// periodically AND on full-map admission. A source flood cannot grow
	// this map indefinitely, including under repeated failed requests.
	b.requests++
	if b.requests%128==0 || len(b.sources)>=maxHandshakeSources {
		for k,v:=range b.sources {
			if !now.Before(v.start) && now.Sub(v.start)>=relayHandshakeWindow {
				delete(b.sources,k)
			}
		}
	}
	g:=b.global
	if !g.start.IsZero() {
		if now.Before(g.start){return false}
		if now.Sub(g.start)>=relayHandshakeWindow {g=handshakeCounter{}}
	}
	s:=b.sources[source]
	if !s.start.IsZero() {
		if now.Before(s.start){return false}
		if now.Sub(s.start)>=relayHandshakeWindow {s=handshakeCounter{}}
	}
	if g.count>=total || s.count>=perSource {return false}
	if s.count==0 && len(b.sources)>=maxHandshakeSources {
		if _,exists:=b.sources[source];!exists{return false}
	}
	if g.count==0 {g.start=now}
	if s.count==0 {s.start=now}
	g.count++
	s.count++
	b.global=g
	b.sources[source]=s
	return true
}

func (h *Handler) admitHandshake(device bool, remote string, now time.Time) bool {
	if device {
		return h.deviceHandshake.admit(remote,now,
			maxDeviceHandshakesPerSource,maxDeviceHandshakesGlobal)
	}
	return h.browserHandshake.admit(remote,now,
		maxBrowserHandshakesPerSource,maxBrowserHandshakesGlobal)
}
