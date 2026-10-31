package remoterelay

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
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
 // Browser file:// has an opaque Origin ("null"), which IS NOT identity:
 // a hostile PHP page can also open sandboxed opaque-origin frames.
 // This flag is an operator-controlled opt-in to diagnostics only.
 AllowOfflineFileClient bool
 // Opt-in to encrypted LOGIN-ONLY (boolean response) on offline route.
 // Never mounts remote application actions or HTTP/WS TaskDeck endpoints.
 AllowOfflineLoginProbe bool
    // Disabled by default. Only an E2EE, TaskDeck-local authenticated
    // session.ping route; no PTY, file, HTTP or arbitrary commands.
    AllowAppPing bool
	// Test-only static diagnostic stages, without identities or payloads.
	BrowserHandshakeTrace func(string)

	// Fixed, separate resource budgets for unauthenticated device and
	// browser WebSocket handshakes and their subsequent connections.
	// A stalled client cannot allocate unbounded goroutines/sockets.
	slotsOnce sync.Once
	deviceSlots chan struct{}
	browserSlots chan struct{}

	// Active socket fairness by direct TCP peer. Clients cannot forge this
	// source using X-Forwarded-For. Ephemeral counters are deleted when each
	// socket leaves, and the global budgets bound their cardinality.
	sourceMu sync.Mutex
	sourceDevices map[string]int
	sourceBrowsers map[string]int

	// Handshake churn is separately bounded even when clients immediately
	// disconnect and free their concurrent socket slots.
	deviceHandshake handshakeBudget
	browserHandshake handshakeBudget
}

const maxDeviceSockets = 256
const maxBrowserSockets = 64
const maxDeviceSocketsPerSource = 64
const maxBrowserSocketsPerSource = 8

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

// Limit one peer from monopolizing every global Relay socket. Many TaskDeck
// agents may share a NAT, so the device cap is deliberately higher than
// the browser cap. Only the direct TCP source is trusted; incoming XFF
// headers are never used for rate-limit identity. No address is persisted.
func (h *Handler) acquireSourceSocketSlot(device bool, remote string) (func(), bool) {
	host,_,err:=net.SplitHostPort(remote)
	if err!=nil{return nil,false}
	ip:=net.ParseIP(host)
	if ip==nil{return nil,false}
	source:=ip.String() // canonical IPv6 and IPv4-mapped IPv6
	releaseGlobal,ok:=h.acquireSocketSlot(device)
	if !ok{return nil,false}
	h.sourceMu.Lock()
	var slots map[string]int
	max:=maxBrowserSocketsPerSource
	if device {
		if h.sourceDevices==nil {h.sourceDevices=make(map[string]int)}
		slots=h.sourceDevices
		max=maxDeviceSocketsPerSource
	} else {
		if h.sourceBrowsers==nil {h.sourceBrowsers=make(map[string]int)}
		slots=h.sourceBrowsers
	}
	if slots[source]>=max {
		h.sourceMu.Unlock()
		releaseGlobal()
		return nil,false
	}
	slots[source]++
	h.sourceMu.Unlock()
	var once sync.Once
	return func(){
		once.Do(func(){
			h.sourceMu.Lock()
			slots[source]--
			if slots[source]==0 {delete(slots,source)}
			h.sourceMu.Unlock()
			releaseGlobal()
		})
	},true
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
 case "/v1/offline":
  if !h.AllowOfflineFileClient {
   http.Error(w,"offline file client is disabled",http.StatusForbidden)
   return
  }
  // No application dispatcher, TaskDeck HTTP tunnel or access token is
  // exposed on this route. All later stages require Agent-owned proof.
  h.browserSocketWithOrigin(w,r,"null",h.AllowOfflineLoginProbe)
	default:
		http.NotFound(w,r)
	}
}
func (h *Handler) deviceSocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.Header.Get("Origin") != "" || r.URL.RawQuery != "" {
		http.Error(w, "device connector requires a non-browser WebSocket", http.StatusForbidden)
		return
	}
	if !h.admitHandshake(true,r.RemoteAddr,time.Now()) {
		http.Error(w, "Relay device handshake rate exceeded", http.StatusTooManyRequests)
		return
	}
	releaseSlot, ok := h.acquireSourceSocketSlot(true,r.RemoteAddr)
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
	proof,err:=decodeDeviceConnectProof(data)
	if err!=nil {return}
	if proof.Challenge != challenge { return }
	if err:=h.Tickets.VerifyDeviceLease(proof.DeviceLease,proof.DeviceID,time.Now());err!=nil {return}
	if err:=h.Devices.Register(proof,time.Now());err!=nil {return}
	defer h.Devices.Unregister(proof.DeviceID,proof.Nonce)
	ctx,cancel=context.WithTimeout(context.Background(),5*time.Second)
	err=conn.Write(ctx,websocket.MessageText,[]byte("{\"registered\":true}"))
	cancel()
	if err!=nil {return}
	// A signed Agent must not be able to flood one permanent socket with
	// lease-verification/registry work while bypassing handshake rate limits.
	// Each authenticated connection has an independent bounded token bucket.
	frameBudget := newDeviceFrameBudget(time.Now())
	for {
		ctx,cancel=context.WithTimeout(context.Background(),45*time.Second)
		typ,payload,readErr:=conn.Read(ctx)
		cancel()
		if readErr!=nil || typ!=websocket.MessageText {return}
		// Admit BEFORE JSON decoding or Ed25519 lease verification.
		// An exhausted or rolled-back bucket closes only this Agent socket.
		if !frameBudget.allow(time.Now()) {return}
		frame,err:=decodeAgentRelayFrame(payload)
		if err!=nil {return}
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

			} else if probeID,ciphertext,appMode,ok:=h.Devices.TakeLoginMode(proof.DeviceID,proof.Nonce);ok {
				opaque,_:=json.Marshal(struct{
					Type string `json:"type"`
					ProbeID string `json:"probe_id"`
					Ciphertext string `json:"ciphertext"`
				}{Type:func()string{if appMode{return "e2ee_app_login"};return "e2ee_login"}(),ProbeID:probeID,Ciphertext:ciphertext})
				reply=opaque
			} else if probeID,ciphertext,ok:=h.Devices.TakeEcho(proof.DeviceID,proof.Nonce);ok {
    operation:="e2ee_echo"
    if h.Devices.IsPendingClientKeyCheck(proof.DeviceID,proof.Nonce,probeID){
     operation="e2ee_client_key_check"
    }
				opaque,_:=json.Marshal(struct{
					Type string `json:"type"`
					ProbeID string `json:"probe_id"`
					Ciphertext string `json:"ciphertext"`
				}{Type:operation,ProbeID:probeID,Ciphertext:ciphertext})
				reply=opaque
            } else if appProbe, appRelayID, encrypted, ok:=h.Devices.TakeAppPing(proof.DeviceID,proof.Nonce);ok {
                // Opaque E2EE ciphertext ONLY. Neither PHP nor Relay may
                // supply a TaskDeck command, local principal or password.
                outbound,_:=json.Marshal(struct{
                    Type string `json:"type"`
                    ProbeID string `json:"probe_id"`
                    RelayID string `json:"relay_id"`
                    Ciphertext string `json:"ciphertext"`
                }{Type:"app_ping",ProbeID:appProbe,RelayID:appRelayID,Ciphertext:encrypted})
                reply=outbound
			}
		case "probe_rejected":
			// Authentication failures invalidate only this browser's
			// rendezvous; the authenticated Agent remains connected.
			// An old probe ID cannot terminate a replacement session.
			h.Devices.RejectProbe(proof.DeviceID,proof.Nonce,frame.ProbeID)
		case "probe_ack":
			if !h.Devices.AckProbe(proof.DeviceID,proof.Nonce,ProbeAttestation{
				ProbeID:frame.ProbeID,ClientNonce:frame.ClientNonce,
				PublicKey:frame.PublicKey,Signature:frame.Signature,Crypto:frame.Crypto,
			}) && h.Devices.IsCurrentProbe(proof.DeviceID,proof.Nonce,frame.ProbeID) {
				return // Reject a forged proof against an active browser.
			}
			// The browser may have closed while Agent was signing: don't
			// disconnect the authenticated Agent for an orphaned reply.
		case "login_ack":
			if !h.Devices.AckLogin(proof.DeviceID,proof.Nonce,frame.ProbeID,frame.Ciphertext) &&
				h.Devices.IsCurrentLogin(proof.DeviceID,proof.Nonce,frame.ProbeID) {
				return
			}
		case "login_rejected":
			// Invalid encrypted credentials or a disabled local callback
			// must cancel only this browser login, not Agent's presence.
			// Stale rejections cannot cancel a newer browser reservation.
			h.Devices.RejectLogin(proof.DeviceID,proof.Nonce,frame.ProbeID)
		case "echo_rejected":
			// Browser supplied malformed or unauthentic ciphertext.
			// Cancel only the matching session, not Agent presence.
			h.Devices.RejectEcho(proof.DeviceID,proof.Nonce,frame.ProbeID)
		case "echo_ack":
			if !h.Devices.AckEcho(proof.DeviceID,proof.Nonce,frame.ProbeID,frame.Ciphertext) &&
				h.Devices.IsCurrentEcho(proof.DeviceID,proof.Nonce,frame.ProbeID) {
				return
			}
        case "app_ready":
            // PHP has no method to create a local TaskDeck principal.
            // This is only an Agent-signed, one-use mailbox transition.
            if err:=h.Devices.ActivateAppPing(proof.DeviceID,proof.Nonce,
                frame.ProbeID,frame.PublicKey,frame.Signature);err!=nil {return}
        case "app_ping_ack":
            // An ACK never authenticates a TaskDeck operation: only the
            // host process may decrypt/authorize and seal the response.
            if !h.Devices.AckAppPing(proof.DeviceID,proof.Nonce,frame.ProbeID,
                frame.RelayID,frame.Ciphertext){return}
        case "app_ping_close":
            if !h.Devices.CloseAppPing(proof.DeviceID,proof.Nonce,frame.ProbeID){return}
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
 h.browserSocketWithOrigin(w,r,h.Tickets.issuer,true)
}

// The browser's Origin header is ONLY a defense against accidental browser
// cross-site WebSocket use. It is not a cryptographic authentication factor.
// Both file:// and sandboxed attacker content can send Origin: null.
func (h *Handler) browserSocketWithOrigin(w http.ResponseWriter, r *http.Request, expectedOrigin string, allowLogin bool) {
	if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("entry")}
	// No wildcard origins, no browser credential or ticket in query strings.
	if r.Method!=http.MethodGet || r.URL.RawQuery!="" ||
		r.Header.Get("Origin")!=expectedOrigin {
		if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("origin_denied")}
		http.Error(w,"forbidden browser origin",http.StatusForbidden)
		return
	}
	if !h.admitHandshake(false,r.RemoteAddr,time.Now()) {
		if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("rate_denied")}
		http.Error(w,"Relay browser handshake rate exceeded",http.StatusTooManyRequests)
		return
	}
	releaseSlot, ok := h.acquireSourceSocketSlot(false,r.RemoteAddr)
	if !ok {
		if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("slots_denied")}
		http.Error(w,"Relay browser capacity reached",http.StatusServiceUnavailable)
		return
	}
	defer releaseSlot()
	var options websocket.AcceptOptions
 if expectedOrigin!="null" {
  u,err:=url.Parse(expectedOrigin)
  if err!=nil{return}
  options.OriginPatterns=[]string{u.Host}
 } else {
  // coder/websocket checks the normal Origin policy on upgrade. Exact
  // "null" is validated above, so explicitly permit that opaque origin
  // for this bounded opt-in diagnostic route ONLY.
  options.InsecureSkipVerify=true
 }
	conn,err:=websocket.Accept(w,r,&options)
	if err!=nil{return}
	defer conn.CloseNow()
	conn.SetReadLimit(12288)
	ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
	typ,data,err:=conn.Read(ctx)
	cancel()
	if err!=nil || typ!=websocket.MessageText {if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("read_denied")};return}
	if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("read_ok")}
	request,err:=decodeBrowserHandshake(data)
	if err!=nil{if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("decode_denied")};return}
	if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("decode_ok")}
	// Reject a substituted Browser nonce/key BEFORE consuming the one-time
	// JTI; an invalid E2EE envelope must never burn a legitimate ticket.
	claims,err:=h.Tickets.ConsumeForBrowser(strings.TrimSpace(request.Ticket),
		request.ClientNonce,request.BrowserKey,time.Now())
	if err!=nil {if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("ticket_denied")};return}
	if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("ticket_ok")}
	release,err:=h.Devices.Acquire(claims,time.Now())
	if err!=nil{if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("acquire_denied")};return}
	if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("acquire_ok")}
	defer release()
	pending,err:=h.Devices.BeginProbe(claims.DeviceID,request.ClientNonce,request.BrowserKey,request.Ticket)
	if err!=nil{if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("begin_denied")};return}
	if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("begin_ok")}
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
		if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("probe_result")}
	case <-time.After(27*time.Second):
		if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("probe_timeout")}
	case <-incoming:
		if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("incoming_before_proof")}
		return
	case <-r.Context().Done():
		if h.BrowserHandshakeTrace!=nil {h.BrowserHandshakeTrace("request_context_done")}
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
	if writeErr!=nil {return}
	// A denied probe is not application authority. Flush the explicit
	// agent_reachable=false response before closing; CloseNow immediately
	// after Write can drop the only frame, producing a misleading EOF.
	if !outcome.OK || outcome.Attestation.Crypto==nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "agent proof unavailable")
		return
	}
	var incomingEcho browserFrame
	// Relay may carry a login ONLY after the exact one-shot encrypted key
	// preflight completed. The Relay does not learn the approval boolean;
	// Agent independently verifies it before any password verification.
	hadKeyCheck:=false
	// Exactly one encrypted login may replace the first diagnostic record.
	// Relay never decrypts credentials. It cannot provide cookies, tokens or
	// TaskDeck API access even when Agent reports authentication success.
	select {
	case first:=<-incoming:
		if first.err!=nil || first.kind!=websocket.MessageText || len(first.body)>10000{return}
		envelope,err:=decodeBrowserEncryptedFrame(first.body)
		if err!=nil{return}
		if envelope.Type=="e2ee_client_key_check" {
   // Only a signed Agent E2EE response can mark the client as approved.
   // PHP and Relay do not see public key, username, signature or status.
   if len(first.body)>2048{return}
   keyReply,keyErr:=h.Devices.QueueClientKeyCheck(claims.DeviceID,
    outcome.Attestation.ProbeID,envelope.Ciphertext)
   if keyErr!=nil{return}
   var sealedStatus string
   select {
   case sealedStatus=<-keyReply:
    if sealedStatus==""{return}
   case <-time.After(27*time.Second):return
   case <-incoming:return // no pipelined login before approval
   case <-r.Context().Done():return
   }
   hadKeyCheck=true
   statusWrapper,_:=json.Marshal(map[string]any{
    "type":"e2ee_client_key_check_result",
    "ciphertext":sealedStatus,"remote_control_available":false,
   })
   replyCtx,stop:=context.WithTimeout(r.Context(),5*time.Second)
   err:=conn.Write(replyCtx,websocket.MessageText,statusWrapper)
   stop()
   if err!=nil{return}
   // Only this one extra protected preflight may precede one login.
   var second browserFrame
   select{
   case second=<-incoming:
   case <-time.After(30*time.Second):return
   case <-r.Context().Done():return
   }
   if second.err!=nil||second.kind!=websocket.MessageText||len(second.body)>10000{return}
   envelope,err=decodeBrowserEncryptedFrame(second.body)
   if err!=nil{return}
  }
  if envelope.Type!="e2ee_app_login" &&
        !allowedBrowserProtectedTransitionForRoute(hadKeyCheck,envelope.Type,allowLogin){return}
        if envelope.Type=="e2ee_app_login" && (!hadKeyCheck || !allowLogin || !h.AllowAppPing){return}
        // Diagnostics remain on their existing echo branch. Never turn an
        // allowed echo into a login or accidentally disconnect the Agent.
        if envelope.Type=="e2ee_app_login" || envelope.Type=="e2ee_login" {
            var reply <-chan string
            var err error
            if envelope.Type=="e2ee_app_login" {
                reply,err=h.Devices.QueueAppLogin(claims.DeviceID,outcome.Attestation.ProbeID,envelope.Ciphertext)
            } else {
                reply,err=h.Devices.QueueLogin(claims.DeviceID,outcome.Attestation.ProbeID,envelope.Ciphertext)
            }
			if err!=nil{return}
			var ciphertext string
			select{
			case ciphertext=<-reply:
				if ciphertext==""{return}
			case <-time.After(32*time.Second):return
			case <-incoming:return
			case <-r.Context().Done():return
			}
			answer,_:=json.Marshal(map[string]any{
				"type":"e2ee_login_result","ciphertext":ciphertext,
				"remote_control_available":false,
			})
			ctx,cancel=context.WithTimeout(r.Context(),5*time.Second)
            writeErr:=conn.Write(ctx,websocket.MessageText,answer)
            cancel()
            if writeErr!=nil{return}
            if envelope.Type=="e2ee_app_login" {
                // No generic command access: bounded AEAD session.ping only.
                for count:=0;count<MaxAppPingRounds;count++{
                    var request browserFrame
                    select{
                    case request=<-incoming:
                    case <-time.After(25*time.Second):return
                    case <-r.Context().Done():return
                    }
                    if request.err!=nil || request.kind!=websocket.MessageText{return}
                    frame,parseErr:=decodeBrowserEncryptedFrame(request.body)
                    if parseErr!=nil||frame.Type!="e2ee_app_ping"{return}
                    readyCtx,stop:=context.WithTimeout(r.Context(),8*time.Second)
                    for !h.Devices.AppPingReady(claims.DeviceID,outcome.Attestation.ProbeID){
                        select{
                        case <-time.After(50*time.Millisecond):
                        case <-readyCtx.Done():stop();return
                        case <-incoming:stop();return
                        }
                    }
                    stop()
                    _,queued,queueErr:=h.Devices.QueueAppPing(
                        claims.DeviceID,outcome.Attestation.ProbeID,frame.Ciphertext)
                    if queueErr!=nil{return}
                    var result string
                    select{
                    case result=<-queued:
                        if result==""{return}
                    case <-time.After(23*time.Second):return
                    case <-incoming:return
                    case <-r.Context().Done():return
                    }
                    sealed,_:=json.Marshal(map[string]any{
                        "type":"e2ee_app_ping_result",
                        "ciphertext":result,"remote_control_available":false,
                    })
                    outCtx,release:=context.WithTimeout(r.Context(),5*time.Second)
                    writeErr=conn.Write(outCtx,websocket.MessageText,sealed)
                    release()
                    if writeErr!=nil{return}
                }
                _=conn.Close(websocket.StatusNormalClosure,"bounded app ping finished")
                return
            }
            _=conn.Close(websocket.StatusNormalClosure,"encrypted login probe completed")
            return
		}
		// Echo mode must retain the very first frame, with no pipe switch.
		if envelope.Type!="e2ee_echo" {return}
		incomingEcho=first
	case <-time.After(15*time.Second):return
	case <-r.Context().Done():return
	}
	// Diagnostic stream v2: at most THREE stop-and-wait AES-GCM records.
	// Only a fixed encrypted nonce challenge exists. Arbitrary HTTP, WS,
	// terminal, files, methods, paths and credentials remain blocked.
	for round:=0;round<MaxDiagnosticRounds;round++ {
		var requestEcho browserFrame
		if round==0 {
			requestEcho=incomingEcho
		} else {
		select {
		case requestEcho=<-incoming:
		case <-time.After(15*time.Second):
			return
		case <-r.Context().Done():
			return
		}
		}
		if requestEcho.err!=nil || requestEcho.kind!=websocket.MessageText{return}
		echo,err:=decodeBrowserEncryptedFrame(requestEcho.body)
		if err!=nil || echo.Type!="e2ee_echo" {return}
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
