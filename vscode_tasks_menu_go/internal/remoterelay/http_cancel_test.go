package remoterelay

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestBrowserDisconnectReleasesAgentReservationWithoutTimeout(t *testing.T) {
	now:=time.Now()
	pub, private, err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil {t.Fatal(err)}
	verifier,err:=NewVerifier(base64.StdEncoding.EncodeToString(pub),"https://portal.example.com")
	if err!=nil {t.Fatal(err)}
	registry,err:=NewRegistry("https://relay.example.com")
	if err!=nil {t.Fatal(err)}
	proof:=newAgentProof(t,now)
	if err:=registry.Register(proof,now);err!=nil {t.Fatal(err)}
	handler:=&Handler{Tickets:verifier,Devices:registry}
	srv:=httptest.NewTLSServer(handler)
	defer srv.Close()
	headers:=http.Header{}
	headers.Set("Origin","https://portal.example.com")
	client,_,err:=websocket.Dial(context.Background(),srv.URL+"/v1/browser",
		&websocket.DialOptions{HTTPClient:srv.Client(),HTTPHeader:headers})
	if err!=nil {t.Fatal(err)}
	defer client.CloseNow()
	claims:=baselineTicketClaims(now)
	claims.DeviceID=proof.DeviceID
	claims.JTI=strings.Repeat("9",32)
	payload,err:=json.Marshal(map[string]string{
		"ticket":tokenFor(t,private,claims),"client_nonce":strings.Repeat("d",32),
	})
	if err!=nil{t.Fatal(err)}
	if err:=client.Write(context.Background(),websocket.MessageText,payload);err!=nil {
		t.Fatal(err)
	}
	deadline:=time.Now().Add(3*time.Second)
	observedBusy:=false
	for time.Now().Before(deadline) {
		registry.mu.Lock()
		state:=registry.devices[proof.DeviceID]
		observedBusy=state!=nil && state.busy && state.probe!=nil
		registry.mu.Unlock()
		if observedBusy {break}
		time.Sleep(5*time.Millisecond)
	}
	if !observedBusy {t.Fatal("browser session never reserved live Agent")}
	// There is intentionally no active Agent socket to answer probes. The
	// browser disconnect must end the session promptly, not after 27s.
	if err:=client.CloseNow();err!=nil {t.Fatal(err)}
	cleanupDeadline:=time.Now().Add(2*time.Second)
	for time.Now().Before(cleanupDeadline) {
		registry.mu.Lock()
		state:=registry.devices[proof.DeviceID]
		free:=state!=nil && !state.busy && state.probe==nil
		registry.mu.Unlock()
		if free {return}
		time.Sleep(5*time.Millisecond)
	}
	t.Fatal("browser disconnect held Relay Agent reservation")
}
