package remoterelay

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// Handler is a security-gated rendezvous skeleton. It authenticates device
// and browser handshakes but DOES NOT forward any TaskDeck bytes or commands.
// Remote execution is intentionally unavailable until end-to-end authentication
// and scoped stream multiplexing pass their security release gates.
type Handler struct {
	Tickets *Verifier
	Devices *Registry
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if h.Tickets == nil || h.Devices == nil {
		http.Error(w, "relay unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.URL.Path {
	case "/healthz":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"ok\":true,\"remote_control_available\":false}"))
	case "/v1/device":
		h.deviceSocket(w,r)
	case "/v1/browser":
		h.browserSocket(w,r)
	default:
		http.NotFound(w,r)
	}
}
func (h *Handler) deviceSocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.Header.Get("Origin") != "" {
		http.Error(w, "device connector requires a non-browser WebSocket", http.StatusForbidden)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil { return }
	defer conn.CloseNow()
	conn.SetReadLimit(4096)
	// A fresh server-generated nonce binds the Agent signature to THIS socket.
	// The old client-selected nonce alone was replayable after Relay restart.
	var challengeBytes [DeviceChallengeSize]byte
	if _, err := rand.Read(challengeBytes[:]); err != nil { return }
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	hello, err := json.Marshal(struct {
		Protocol string `json:"protocol"`
		Challenge string `json:"challenge"`
	}{DeviceChallengeProtocol, challenge})
	if err != nil { return }
	ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
	err=conn.Write(ctx,websocket.MessageText,hello)
	cancel()
	if err!=nil{return}
	ctx,cancel=context.WithTimeout(context.Background(),10*time.Second)
	typ,data,err:=conn.Read(ctx)
	cancel()
	if err!=nil || typ!=websocket.MessageText { return }
	var proof DeviceProof
	if err:=json.Unmarshal(data,&proof);err!=nil {return}
	if proof.Challenge != challenge { return }
	if err:=h.Tickets.VerifyDeviceLease(proof.DeviceLease,proof.DeviceID,time.Now());err!=nil {return}
	if err:=h.Devices.Register(proof,time.Now());err!=nil {return}
	defer h.Devices.Unregister(proof.DeviceID,proof.Nonce)
	ctx,cancel=context.WithTimeout(context.Background(),5*time.Second)
	err=conn.Write(ctx,websocket.MessageText,[]byte("{\"registered\":true}"))
	cancel()
	if err!=nil {return}
	for {
		ctx,cancel=context.WithTimeout(context.Background(),45*time.Second)
		typ,payload,readErr:=conn.Read(ctx)
		cancel()
		if readErr!=nil || typ!=websocket.MessageText {return}
		var frame struct {
			Type string `json:"type"`
			ProbeID string `json:"probe_id"`
			DeviceLease string `json:"device_lease"`
		}
		if json.Unmarshal(payload,&frame)!=nil {return}
		if err:=h.Tickets.VerifyDeviceLease(frame.DeviceLease,proof.DeviceID,time.Now());err!=nil{return}
		reply:=[]byte("pong")
		switch frame.Type {
		case "ping":
			if !h.Devices.Touch(proof.DeviceID,proof.Nonce,time.Now()){return}
			if probeID,ok:=h.Devices.TakeProbe(proof.DeviceID,proof.Nonce);ok {
				probe,_:=json.Marshal(struct {
					Type string `json:"type"`
					ProbeID string `json:"probe_id"`
				}{Type:"reachability_probe",ProbeID:probeID})
				reply=probe
			}
		case "probe_ack":
			if !h.Devices.AckProbe(proof.DeviceID,proof.Nonce,frame.ProbeID){return}
		default:
			return
		}
		ctx,cancel=context.WithTimeout(context.Background(),5*time.Second)
		err=conn.Write(ctx,websocket.MessageText,reply)
		cancel()
		if err!=nil{return}
	}
}
func (h *Handler) browserSocket(w http.ResponseWriter, r *http.Request) {
	// No wildcard origins, no browser credential or ticket in query strings.
	if r.Method!=http.MethodGet || r.URL.RawQuery!="" ||
		r.Header.Get("Origin")!=h.Tickets.issuer {
		http.Error(w,"forbidden browser origin",http.StatusForbidden)
		return
	}
	u,err:=url.Parse(h.Tickets.issuer)
	if err!=nil{return}
	conn,err:=websocket.Accept(w,r,&websocket.AcceptOptions{OriginPatterns:[]string{u.Host}})
	if err!=nil{return}
	defer conn.CloseNow()
	conn.SetReadLimit(4096)
	ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
	typ,data,err:=conn.Read(ctx)
	cancel()
	if err!=nil || typ!=websocket.MessageText {return}
	claims,err:=h.Tickets.Consume(strings.TrimSpace(string(data)),time.Now())
	if err!=nil {return}
	release,err:=h.Devices.Acquire(claims,time.Now())
	if err!=nil{return}
	defer release()
	pending,err:=h.Devices.BeginProbe(claims.DeviceID)
	if err!=nil{return}
	confirmed:=false
	select {
	case confirmed=<-pending:
	case <-time.After(27*time.Second):
	case <-r.Context().Done():
		return
	}
	// The Agent has only answered an authenticated reachability probe.
	// No HTTP/WS payload or TaskDeck credential is forwarded.
	answer,_:=json.Marshal(map[string]any{
		"ticket_valid":true,
		"agent_reachable":confirmed,
		"remote_control_available":false,
	})
	ctx,cancel=context.WithTimeout(context.Background(),5*time.Second)
	_ = conn.Write(ctx,websocket.MessageText,answer)
	cancel()
	_ = conn.Close(websocket.StatusNormalClosure,"remote transport pending security validation")
}
