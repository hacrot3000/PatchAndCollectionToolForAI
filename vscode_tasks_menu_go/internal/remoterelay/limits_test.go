package remoterelay

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRelaySeparatesAndBoundsSocketBudgets(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatal(err) }
	verifier, err := NewVerifier(base64.StdEncoding.EncodeToString(public), "https://portal.example.com")
	if err != nil { t.Fatal(err) }
	registry, err := NewRegistry("https://relay.example.com")
	if err != nil { t.Fatal(err) }
	h := &Handler{Tickets: verifier, Devices: registry}
	var devices, browsers []func()
	for i:=0;i<maxDeviceSockets;i++ {
		release,ok:=h.acquireSocketSlot(true)
		if !ok { t.Fatalf("device slot %d denied", i) }
		devices=append(devices,release)
	}
	if release,ok:=h.acquireSocketSlot(true);ok {
		release();t.Fatal("unbounded device sockets allowed")
	}
	for i:=0;i<maxBrowserSockets;i++ {
		release,ok:=h.acquireSocketSlot(false)
		if !ok { t.Fatalf("browser slot %d denied", i) }
		browsers=append(browsers,release)
	}
	if release,ok:=h.acquireSocketSlot(false);ok {
		release();t.Fatal("unbounded browser sockets allowed")
	}
	deviceRequest:=httptest.NewRequest(http.MethodGet,"https://relay.example.com/v1/device",nil)
	deviceResponse:=httptest.NewRecorder()
	h.ServeHTTP(deviceResponse,deviceRequest)
	if deviceResponse.Code!=http.StatusServiceUnavailable {
		t.Fatalf("device capacity rejection was %d",deviceResponse.Code)
	}
	browserRequest:=httptest.NewRequest(http.MethodGet,"https://relay.example.com/v1/browser",nil)
	browserRequest.Header.Set("Origin","https://portal.example.com")
	browserResponse:=httptest.NewRecorder()
	h.ServeHTTP(browserResponse,browserRequest)
	if browserResponse.Code!=http.StatusServiceUnavailable {
		t.Fatalf("browser capacity rejection was %d",browserResponse.Code)
	}
	// Duplicate cleanup must never panic or steal a slot belonging to a
	// different connection.
	browsers[0]()
	browsers[0]()
	release,ok:=h.acquireSocketSlot(false)
	if !ok { t.Fatal("browser capacity not returned after disconnect") }
	release()
	for _,f:=range browsers[1:] {f()}
	for _,f:=range devices {f()}
	if len(h.browserSlots)!=0 || len(h.deviceSlots)!=0 {
		t.Fatal("connection capacity leaked")
	}
}

func TestRelayRejectsBrowserOriginAndDeviceQueryBeforeUpgrade(t *testing.T) {
	public,_,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	verifier,err:=NewVerifier(base64.StdEncoding.EncodeToString(public),"https://portal.example.com")
	if err!=nil{t.Fatal(err)}
	registry,err:=NewRegistry("https://relay.example.com")
	if err!=nil{t.Fatal(err)}
	h:=&Handler{Tickets:verifier,Devices:registry}
	for _,test:=range []struct{path,origin string}{
		{"/v1/device?ticket=leak",""},
		{"/v1/device","https://evil.example.com"},
		{"/v1/browser","https://evil.example.com"},
		{"/v1/browser?ticket=leak","https://portal.example.com"},
	}{
		r:=httptest.NewRequest(http.MethodGet,"https://relay.example.com"+test.path,nil)
		if test.origin!="" {r.Header.Set("Origin",test.origin)}
		w:=httptest.NewRecorder()
		h.ServeHTTP(w,r)
		if w.Code!=http.StatusForbidden {t.Errorf("%s origin %s allowed: %d",test.path,test.origin,w.Code)}
	}
}
