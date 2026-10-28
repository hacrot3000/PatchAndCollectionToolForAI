package remoterelay

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest" 
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRelayHandshakeRateLimitSurvivesImmediateDisconnect(t *testing.T) {
	h:=&Handler{}
	now:=time.Unix(1700000000,0)
	peer:="192.0.2.19:10000"
	for i:=0;i<maxBrowserHandshakesPerSource;i++ {
		// Simulate a new connection and immediate close: active socket
		// counters never increment, but the source's attempt count must.
		if !h.admitHandshake(false,peer,now) {
			t.Fatalf("short-lived browser handshake %d rejected",i)
		}
	}
	if h.admitHandshake(false,"[::ffff:192.0.2.19]:30000",now) {
		t.Fatal("IPv4-mapped IPv6 bypassed per-source WebSocket churn limit")
	}
	if !h.admitHandshake(false,"198.51.100.8:443",now) {
		t.Fatal("browser at an unrelated TCP peer lost its handshake budget")
	}
	if !h.admitHandshake(true,peer,now) {
		t.Fatal("device and browser should have separate handshake budgets")
	}
	if h.admitHandshake(false,peer,now.Add(-time.Second)) {
		t.Fatal("clock rollback replenished browser handshake admission")
	}
	if h.admitHandshake(false,peer,now.Add(59*time.Second)) {
		t.Fatal("rate window refilled early")
	}
	if !h.admitHandshake(false,peer,now.Add(time.Minute)) {
		t.Fatal("full 60-second window did not reopen")
	}
}

func TestRelayGlobalHandshakeBudgetAcrossDifferentIPs(t *testing.T) {
	h:=&Handler{}
	now:=time.Unix(1700000000,0)
	for i:=0;i<maxBrowserHandshakesGlobal;i++ {
		// Use 60 different IPv4 sources to stay below the local cap.
		peer:=fmt.Sprintf("203.0.113.%d:12000",i%60+1)
		if !h.admitHandshake(false,peer,now) {
			t.Fatalf("Relay rejected global budget before capacity at %d",i)
		}
	}
	if h.admitHandshake(false,"198.51.100.220:20000",now) {
		t.Fatal("rotating source IPs bypassed global browser handshake cap")
	}
	if !h.admitHandshake(false,"198.51.100.220:20000",now.Add(time.Minute)) {
		t.Fatal("global rate limit did not reset after its time window")
	}
}

func TestRelayHandshakeRateConcurrentAdmissionAndBoundedMemory(t *testing.T) {
	h:=&Handler{}
	now:=time.Unix(1700000000,0)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i:=0;i<250;i++ {
		wg.Add(1)
		go func(){
			defer wg.Done()
			if h.admitHandshake(false,"198.51.100.27:45555",now) {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load()!=maxBrowserHandshakesPerSource {
		t.Fatalf("concurrent handshake bypassed source quota: %d",accepted.Load())
	}
	// The 4096-entry cardinality bound applies even when a malformed or
	// overloaded peer cycles thousands of actual TCP addresses.
	var budget handshakeBudget
	for i:=0;i<maxHandshakeSources+500;i++ {
		host:=fmt.Sprintf("10.%d.%d.%d:4400",(i>>16)&255,(i>>8)&255,i&255)
		budget.admit(host,now,1,10000)
	}
	budget.mu.Lock()
	size:=len(budget.sources)
	budget.mu.Unlock()
	if size>maxHandshakeSources {t.Fatalf("handshake source map grew to %d",size)}
	if !budget.admit("192.0.2.123:5540",now.Add(time.Minute),1,10000) {
		t.Fatal("expired source entries were not pruned to admit a new client")
	}
}

func TestRelayRealHTTPHandshakeFloodRejectedBeforeSocketAllocation(t *testing.T) {
	pub,_,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	verify,err:=NewVerifier(base64.StdEncoding.EncodeToString(pub),"https://portal.example.com")
	if err!=nil{t.Fatal(err)}
	registry,err:=NewRegistry("https://relay.example.com")
	if err!=nil{t.Fatal(err)}
	h:=&Handler{Tickets:verify,Devices:registry}
	for i:=0;i<maxBrowserHandshakesPerSource;i++ {
		req:=httptest.NewRequest(http.MethodGet,
			"https://relay.example.com/v1/browser",nil)
		req.RemoteAddr="192.0.2.45:1111"
		req.Header.Set("Origin","https://portal.example.com")
		req.Header.Set("X-Forwarded-For","198.51.100.220")
		resp:=httptest.NewRecorder()
		h.ServeHTTP(resp,req)
		// The request is deliberately not a WebSocket upgrade. Rate control
		// is evaluated first but websocket.Accept still rejects the upgrade.
		if resp.Code==http.StatusTooManyRequests {
			t.Fatalf("handshake rate limit fired early at %d",i)
		}
	}
	req:=httptest.NewRequest(http.MethodGet,
		"https://relay.example.com/v1/browser",nil)
	req.RemoteAddr="[::ffff:192.0.2.45]:1000"
	req.Header.Set("Origin","https://portal.example.com")
	req.Header.Set("X-Forwarded-For","198.51.100.220")
	w:=httptest.NewRecorder()
	h.ServeHTTP(w,req)
	if w.Code!=http.StatusTooManyRequests {
		t.Fatalf("spoofed XFF or IPv4-mapped source evaded HTTP 429: %d",w.Code)
	}
	if len(h.browserSlots)!=0 {
		t.Fatal("429 rejection allocated an active Relay browser socket")
	}
	// An invalid origin must fail preflight without creating source state.
	bad:=httptest.NewRequest(http.MethodGet,
		"https://relay.example.com/v1/browser",nil)
	bad.RemoteAddr="203.0.113.15:12000"
	bad.Header.Set("Origin","https://attacker.example")
	h.ServeHTTP(httptest.NewRecorder(),bad)
	h.browserHandshake.mu.Lock()
	_,exists:=h.browserHandshake.sources["203.0.113.15"]
	h.browserHandshake.mu.Unlock()
	if exists {t.Fatal("forbidden origin created a handshake budget identity")} 
}
