package remoterelay

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

	// Fixed, separate resource budgets for unauthenticated device and
	// browser WebSocket handshakes and their subsequent connections.
	// A stalled client cannot allocate unbounded goroutines/sockets.
	slotsOnce sync.Once
	deviceSlots chan struct{}
	browserSlots chan struct{}
}

const maxDeviceSockets = 256
const maxBrowserSockets = 64

func (h *Handler) acquireSocketSlot(device bool) (func(), bool) {
	h.slotsOnce.Do(func() {
		h.deviceSlots = make(chan struct{}, maxDeviceSockets)
		h.browserSlots = make(chan struct{}, maxBrowserSockets)
	})
	slots := h.browserSlots
	if device { slots = h.deviceSlots }
	select {
	case slots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func(){ <-slots }) }, true
	default:
		return nil, false
	}
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
	if r.Method != http.MethodGet || r.Header.Get("Origin") != "" || r.URL.RawQuery != "" {
		http.Error(w, "device connector requires a non-browser WebSocket", http.StatusForbidden)
		return
	}
	releaseSlot, ok := h.acquireSocketSlot(true)
	if !ok {
		http.Error(w, "Relay device capacity reached", http.StatusServiceUnavailable)
		return
	}
	defer releaseSlot()
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
			ClientNonce string `json:"client_nonce"`
			PublicKey string `json:"public_key"`
			Signature string `json:"signature"`
			Crypto *ProbeCrypto `json:"crypto"`
			Ciphertext string `json:"ciphertext"`
			DeviceLease string `json:"device_lease"`
		}
		if json.Unmarshal(payload,&frame)!=nil {return}
		if err:=h.Tickets.VerifyDeviceLease(frame.DeviceLease,proof.DeviceID,time.Now());err!=nil{return}
		reply:=[]byte("pong")
		switch frame.Type {
		case "ping":
			if !h.Devices.Touch(proof.DeviceID,proof.Nonce,time.Now()){return}
			if request,ok:=h.Devices.TakeProbe(proof.DeviceID,proof.Nonce);ok {
				probe,_:=json.Marshal(struct {
					Type string `json:"type"`
					ProbeRequest
				}{Type:"reachability_probe",ProbeRequest:request})
				reply=probe
			} else if probeID,ciphertext,ok:=h.Devices.TakeEcho(proof.DeviceID,proof.Nonce);ok {
				opaque,_:=json.Marshal(struct{
					Type string `json:"type"`
					ProbeID string `json:"probe_id"`
					Ciphertext string `json:"ciphertext"`
				}{Type:"e2ee_echo",ProbeID:probeID,Ciphertext:ciphertext})
				reply=opaque
			}
		case "probe_ack":
			if !h.Devices.AckProbe(proof.DeviceID,proof.Nonce,ProbeAttestation{
				ProbeID:frame.ProbeID,ClientNonce:frame.ClientNonce,
				PublicKey:frame.PublicKey,Signature:frame.Signature,Crypto:frame.Crypto,
			}) && h.Devices.IsCurrentProbe(proof.DeviceID,proof.Nonce,frame.ProbeID) {
				return // Reject a forged proof against an active browser.
			}
			// The browser may have closed while Agent was signing: don't
			// disconnect the authenticated Agent for an orphaned reply.
		case "echo_ack":
			if !h.Devices.AckEcho(proof.DeviceID,proof.Nonce,frame.ProbeID,frame.Ciphertext) &&
				h.Devices.IsCurrentEcho(proof.DeviceID,proof.Nonce,frame.ProbeID) {
				return
			}
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
	releaseSlot, ok := h.acquireSocketSlot(false)
	if !ok {
		http.Error(w,"Relay browser capacity reached",http.StatusServiceUnavailable)
		return
	}
	defer releaseSlot()
	u,err:=url.Parse(h.Tickets.issuer)
	if err!=nil{return}
	conn,err:=websocket.Accept(w,r,&websocket.AcceptOptions{OriginPatterns:[]string{u.Host}})
	if err!=nil{return}
	defer conn.CloseNow()
	conn.SetReadLimit(8192)
	ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
	typ,data,err:=conn.Read(ctx)
	cancel()
	if err!=nil || typ!=websocket.MessageText {return}
	var request struct {
		Ticket string `json:"ticket"`
		ClientNonce string `json:"client_nonce"`
		BrowserKey string `json:"browser_key"`
	}
	if json.Unmarshal(data,&request)!=nil || !validProbeNonce(request.ClientNonce) {return}
	claims,err:=h.Tickets.Consume(strings.TrimSpace(request.Ticket),time.Now())
	if err!=nil {return}
	release,err:=h.Devices.Acquire(claims,time.Now())
	if err!=nil{return}
	defer release()
	pending,err:=h.Devices.BeginProbe(claims.DeviceID,request.ClientNonce,request.BrowserKey)
	if err!=nil{return}
	// One read goroutine watches the browser while Agent answers the proof.
	// It detects a closed tab promptly; early data is rejected, not queued.
	type browserFrame struct {
		kind websocket.MessageType
		body []byte
		err error
	}
	incoming:=make(chan browserFrame,1)
	stopRead:=make(chan struct{})
	defer close(stopRead)
	go func(){
		for {
			readCtx,cancel:=context.WithTimeout(r.Context(),135*time.Second)
			kind,body,readErr:=conn.Read(readCtx)
			cancel()
			select {
			case incoming<-browserFrame{kind,body,readErr}:
			case <-stopRead:
				return
			}
			if readErr!=nil{return}
		}
	}()
	var outcome ProbeOutcome
	select {
	case outcome=<-pending:
	case <-time.After(27*time.Second):
	case <-incoming:
		return
	case <-r.Context().Done():
		return
	}
	// No TaskDeck HTTP/WS content or user credentials are forwarded.
	answer,_:=json.Marshal(map[string]any{
		"ticket_valid":true,
		"agent_reachable":outcome.OK,
		"remote_control_available":false,
		"device_id":claims.DeviceID,
		"agent_proof":outcome.Attestation,
	})
	ctx,cancel=context.WithTimeout(r.Context(),5*time.Second)
	writeErr:=conn.Write(ctx,websocket.MessageText,answer)
	cancel()
	if writeErr!=nil || !outcome.OK || outcome.Attestation.Crypto==nil {return}
	// Diagnostic stream v2: at most THREE stop-and-wait AES-GCM records.
	// Only a fixed encrypted nonce challenge exists. Arbitrary HTTP, WS,
	// terminal, files, methods, paths and credentials remain blocked.
	for round:=0;round<MaxDiagnosticRounds;round++ {
		var requestEcho browserFrame
		select {
		case requestEcho=<-incoming:
		case <-time.After(15*time.Second):
			return
		case <-r.Context().Done():
			return
		}
		if requestEcho.err!=nil || requestEcho.kind!=websocket.MessageText{return}
		var echo struct {
			Type string `json:"type"`
			Ciphertext string `json:"ciphertext"`
		}
		if len(requestEcho.body)>2048 || json.Unmarshal(requestEcho.body,&echo)!=nil ||
			echo.Type!="e2ee_echo" {return}
		reply,err:=h.Devices.QueueEcho(claims.DeviceID,outcome.Attestation.ProbeID,echo.Ciphertext)
		if err!=nil{return}
		var returned string
		select {
		case returned=<-reply:
			if returned==""{return}
		case <-time.After(27*time.Second):
			return
		case <-incoming:
			// No pipelining: a browser cannot send round N+1 before
			// Agent has acknowledged round N. Fail closed.
			return
		case <-r.Context().Done():
			return
		}
		result,_:=json.Marshal(map[string]any{
			"type":"e2ee_echo_reply",
			"ciphertext":returned,
			"remote_control_available":false,
		})
		ctx,cancel=context.WithTimeout(r.Context(),5*time.Second)
		writeErr:=conn.Write(ctx,websocket.MessageText,result)
		cancel()
		if writeErr!=nil{return}
	}
	_ = conn.Close(websocket.StatusNormalClosure,"bounded encrypted diagnostics completed")
}
