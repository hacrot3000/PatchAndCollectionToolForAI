package remoterelay

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Real TLS/WebSocket regression: a malicious but legitimately enrolled Agent
// cannot turn one authenticated socket into an unbounded signature-verification
// workload. It is important to test AFTER the device challenge and registration,
// not just the pre-upgrade handshake counter.
func TestRelayAuthenticatedAgentSocketFrameFloodClosesConnection(t *testing.T) {
	portalPub, portalPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(base64.StdEncoding.EncodeToString(portalPub), "https://portal.example.com")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewRegistry("https://relay.example.com")
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{Tickets: verifier, Devices: registry}
	srv := httptest.NewTLSServer(handler)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, srv.URL+"/v1/device", &websocket.DialOptions{HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	kind, frame, err := conn.Read(ctx)
	var challenge struct {
		Protocol string `json:"protocol"`
		Challenge string `json:"challenge"`
	}
	if err != nil || kind != websocket.MessageText || json.Unmarshal(frame, &challenge) != nil ||
		challenge.Protocol != DeviceChallengeProtocol {
		t.Fatalf("Relay sent invalid device challenge: %s (%v)", frame, err)
	}
	devicePub, devicePriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(devicePub)
	deviceID := hex.EncodeToString(sum[:])
	now := time.Now()
	lease := makeLease(t, portalPriv, DeviceLease{
		V: 1, Iss: "https://portal.example.com", Aud: "taskdeck-relay-device",
		DeviceID: deviceID, Iat: now.Unix(), Exp: now.Unix() + 70,
	})
	proof := DeviceProof{
		DeviceID: deviceID, PublicKey: base64.StdEncoding.EncodeToString(devicePub),
		Audience: "https://relay.example.com", Challenge: challenge.Challenge,
		DeviceLease: lease, Timestamp: now.Unix(), Nonce: strings.Repeat("c", 32),
	}
	wire := strings.Join([]string{
		DeviceChallengeProtocol, "connect", deviceID, proof.Audience,
		strconv.FormatInt(proof.Timestamp, 10), proof.Nonce, proof.Challenge,
	}, "\n")
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(devicePriv, []byte(wire)))
	payload, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		t.Fatal(err)
	}
	kind, frame, err = conn.Read(ctx)
	if err != nil || kind != websocket.MessageText || string(frame) != `{"registered":true}` {
		t.Fatalf("valid signed Agent was not registered: %s (%v)", frame, err)
	}
	pulse, _ := json.Marshal(map[string]string{"type": "ping", "device_lease": lease})
	for i := 0; i < maxDeviceFrameBurst; i++ {
		if err := conn.Write(ctx, websocket.MessageText, pulse); err != nil {
			t.Fatalf("legitimate device keepalive %d rejected: %v", i, err)
		}
		kind, frame, err := conn.Read(ctx)
		if err != nil || kind != websocket.MessageText || string(frame) != "pong" {
			t.Fatalf("keepalive %d failed: %s (%v)", i, frame, err)
		}
	}
	if err := conn.Write(ctx, websocket.MessageText, pulse); err != nil {
		// A closed socket is also a correct rejection of the 13th frame.
		return
	}
	if kind, frame, err = conn.Read(ctx); err == nil {
		t.Fatalf("authenticated socket flood was still served: kind=%v frame=%q", kind, frame)
	}
}
