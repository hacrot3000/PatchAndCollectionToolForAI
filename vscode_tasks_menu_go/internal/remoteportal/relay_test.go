package remoteportal

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "encoding/base64"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remoterelay"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "github.com/coder/websocket"
)
func TestRelayProofAndSocketHandshake(t *testing.T) {
 pub,private,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 verifier,err:=remoterelay.NewVerifier(base64.StdEncoding.EncodeToString(pub),"https://portal.example.com")
 if err!=nil{t.Fatal(err)}
 var handler *remoterelay.Handler
 srv:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  handler.ServeHTTP(w,r)
 }))
 defer srv.Close()
 registry,err:=remoterelay.NewRegistry(srv.URL)
 if err!=nil{t.Fatal(err)}
 handler=&remoterelay.Handler{Tickets:verifier,Devices:registry}
 agent,err:=NewAgent("https://portal.example.com",filepath.Join(t.TempDir(),"private","key.seed"),srv.Client())
 if err!=nil{t.Fatal(err)}
 if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(pub));err!=nil{t.Fatal(err)}
 if err:=agent.ConnectRelay(context.Background(),srv.URL);err==nil {
  t.Fatal("Relay connection without Portal lease accepted")
 }
 claims:=remoterelay.DeviceLease{
  V:1,Iss:"https://portal.example.com",Aud:"taskdeck-relay-device",
  DeviceID:agent.DeviceID(),Iat:time.Now().Unix(),Exp:time.Now().Unix()+70,
 }
 raw,err:=json.Marshal(claims);if err!=nil{t.Fatal(err)}
 body:=base64.RawURLEncoding.EncodeToString(raw)
 sig:=ed25519.Sign(private,[]byte("taskdeck-relay-device-lease-v1."+body))
 agent.leaseMu.Lock()
 agent.deviceLease=body+"."+base64.RawURLEncoding.EncodeToString(sig)
 agent.leaseMu.Unlock()
 challenge:=base64.RawURLEncoding.EncodeToString(make([]byte,32))
 proof,err:=agent.signRelayProof(srv.URL,challenge)
 if err!=nil{t.Fatal(err)}
 signed:=remoterelay.DeviceProof{DeviceID:proof.DeviceID,PublicKey:proof.PublicKey,
 Audience:proof.Audience,Challenge:proof.Challenge,Timestamp:proof.Timestamp,Nonce:proof.Nonce,Signature:proof.Signature}
 if err:=remoterelay.VerifyDeviceProof(signed,srv.URL,time.Now());err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second)
 defer cancel()
 finished:=make(chan error,1)
 go func(){finished<-agent.connectRelay(ctx,srv.URL,35*time.Millisecond)}()
 deadline:=time.After(2*time.Second)
 seen:=false
 for !seen {
  select {case <-deadline:t.Fatal("Relay never accepted authenticated Agent")
  case <-time.After(10*time.Millisecond):
    seen=registry.Online(agent.DeviceID(),time.Now())}
 }
 // A browser with a valid short-lived Portal ticket must reach this exact
 // authenticated Agent and receive only a safe reachability confirmation.
 clientNonce:=strings.Repeat("d",32)
 browserEphemeral,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 ticketBinding,err:=remotetransport.BrowserTicketBinding(clientNonce,browserEphemeral.Public)
 if err!=nil{t.Fatal(err)}
 issued:=time.Now().Unix()
 browserClaims:=remoterelay.Claims{AuthEpoch:1,V:2,BrowserBinding:ticketBinding,Issuer:"https://portal.example.com",
  Audience:"taskdeck-relay",Subject:"22",DeviceID:agent.DeviceID(),Role:"viewer",
  IssuedAt:issued,Expires:issued+30,JTI:strings.Repeat("e",32)}
 browserRaw,err:=json.Marshal(browserClaims);if err!=nil{t.Fatal(err)}
 browserBody:=base64.RawURLEncoding.EncodeToString(browserRaw)
 browserSig:=ed25519.Sign(private,[]byte("taskdeck-relay-ticket-v1."+browserBody))
 browserTicket:=browserBody+"."+base64.RawURLEncoding.EncodeToString(browserSig)
 headers:=http.Header{}
 headers.Set("Origin","https://portal.example.com")
 browser,_,err:=websocket.Dial(ctx,srv.URL+"/v1/browser",&websocket.DialOptions{
  HTTPClient:srv.Client(),HTTPHeader:headers,
 })
 if err!=nil{t.Fatal(err)}
 defer browser.CloseNow()
 browserRequest,err:=json.Marshal(map[string]string{
  "ticket":browserTicket,"client_nonce":clientNonce,
  "browser_key":base64.StdEncoding.EncodeToString(browserEphemeral.Public),
 })
 if err!=nil{t.Fatal(err)}
 if err:=browser.Write(ctx,websocket.MessageText,browserRequest);err!=nil{t.Fatal(err)}
 kind,received,err:=browser.Read(ctx)
 if err!=nil{t.Fatal(err)}
 var result struct{
  TicketValid bool `json:"ticket_valid"`
  AgentReachable bool `json:"agent_reachable"`
  RemoteControlAvailable bool `json:"remote_control_available"`
  DeviceID string `json:"device_id"`
  AgentProof remoterelay.ProbeAttestation `json:"agent_proof"`
 }
 if kind!=websocket.MessageText || json.Unmarshal(received,&result)!=nil ||
  !result.TicketValid || !result.AgentReachable || result.RemoteControlAvailable ||
  result.DeviceID!=agent.DeviceID() || result.AgentProof.ClientNonce!=clientNonce {

  t.Fatalf("browser-to-agent reachability protocol invalid: %s",string(received))
 }
 // Check the Agent's signature separately from the Relay's reachable flag.
 proofPub,err:=base64.StdEncoding.DecodeString(result.AgentProof.PublicKey)
 if err!=nil{t.Fatal(err)}
 proofSig,err:=base64.StdEncoding.DecodeString(result.AgentProof.Signature)
 if err!=nil{t.Fatal(err)}
 wire:=strings.Join([]string{
  remoterelay.BrowserAttestationProtocol,result.DeviceID,
  result.AgentProof.ProbeID,clientNonce,
 },string([]byte{10}))
 if !ed25519.Verify(proofPub,[]byte(wire),proofSig) {
  t.Fatal("Agent identity proof does not verify with browser nonce")
 }
 // The browser, not Relay, verifies and decrypts the Agent's E2EE proof.
 if result.AgentProof.Crypto==nil {t.Fatal("Agent did not return encrypted ready handshake")}
 cryptoHello:=result.AgentProof.Crypto
 identity,err:=base64.StdEncoding.DecodeString(cryptoHello.Identity);if err!=nil{t.Fatal(err)}
 ephemeral,err:=base64.StdEncoding.DecodeString(cryptoHello.Ephemeral);if err!=nil{t.Fatal(err)}
 signature,err:=base64.StdEncoding.DecodeString(cryptoHello.Signature);if err!=nil{t.Fatal(err)}
 encrypted,err:=base64.StdEncoding.DecodeString(cryptoHello.EncryptedReady);if err!=nil{t.Fatal(err)}
 channel,err:=remotetransport.AcceptHello(browserEphemeral,agent.DeviceID(),result.AgentProof.ProbeID,
  clientNonce,remotetransport.Hello{Identity:identity,Ephemeral:ephemeral,Signature:signature})
 if err!=nil{t.Fatal(err)}
 ready,err:=channel.Open(encrypted)
 if err!=nil || string(ready)!="taskdeck-e2ee-ready-v1"{
  t.Fatalf("encrypted Agent handshake invalid: %v %s",err,string(ready))
 }
 // Browser sends three distinct encrypted nonces through the Relay and
 // waits for each Agent response before transmitting the next record.
 var finalEncryptedEcho []byte
 for round:=0;round<3;round++ {
  echoNonce:=strings.Repeat(string(rune('a'+round)),32)
  encryptedEcho,err:=channel.Seal([]byte("taskdeck-e2ee-echo-v1:"+echoNonce))
  if err!=nil{t.Fatal(err)}
  echoRequest,err:=json.Marshal(map[string]string{
   "type":"e2ee_echo","ciphertext":base64.StdEncoding.EncodeToString(encryptedEcho),
  })
  if err!=nil{t.Fatal(err)}
  if err:=browser.Write(ctx,websocket.MessageText,echoRequest);err!=nil{t.Fatal(err)}
  kind,echoReceived,err:=browser.Read(ctx)
  if err!=nil{t.Fatalf("round %d encrypted Relay echo failed: %v",round,err)}
  var echoResponse struct {
   Type string `json:"type"`
   Ciphertext string `json:"ciphertext"`
   RemoteControlAvailable bool `json:"remote_control_available"`
  }
  if kind!=websocket.MessageText || json.Unmarshal(echoReceived,&echoResponse)!=nil ||
   echoResponse.Type!="e2ee_echo_reply" || echoResponse.RemoteControlAvailable {
   t.Fatalf("unexpected Relay encrypted echo envelope: %s",string(echoReceived))
  }
  echoBytes,err:=base64.StdEncoding.DecodeString(echoResponse.Ciphertext)
  if err!=nil{t.Fatal(err)}
  echoPlain,err:=channel.Open(echoBytes)
  if err!=nil || string(echoPlain)!="taskdeck-e2ee-echo-reply-v1:"+echoNonce {
   t.Fatalf("round %d Agent E2EE echo mismatch: %v %s",round,err,string(echoPlain))
  }
  finalEncryptedEcho=echoBytes
 }
 if _,err:=channel.Open(finalEncryptedEcho);err==nil{t.Fatal("encrypted echo replay accepted")}
 if channel.Established(){t.Fatal("replayed ciphertext did not invalidate channel")}
 // A hostile Origin or a query-string ticket must never upgrade.
 hostile:=http.Header{}
 hostile.Set("Origin","https://untrusted.example")
 if ws,reply,err:=websocket.Dial(ctx,srv.URL+"/v1/browser",&websocket.DialOptions{
  HTTPClient:srv.Client(),HTTPHeader:hostile,
 });err==nil{
  ws.CloseNow();t.Fatal("cross-site WebSocket accepted")
 }else if reply==nil || reply.StatusCode!=http.StatusForbidden{
  t.Fatalf("unexpected CSWSH rejection status: %v",err)
 }
 if ws,reply,err:=websocket.Dial(ctx,srv.URL+"/v1/browser?ticket=leak",&websocket.DialOptions{
  HTTPClient:srv.Client(),HTTPHeader:headers,
 });err==nil{
  ws.CloseNow();t.Fatal("ticket in query was accepted")
 }else if reply==nil || reply.StatusCode!=http.StatusForbidden{
  t.Fatalf("query-string token rejection status: %v",err)
 }
 cancel()
 select {case err:=<-finished:if err!=nil{t.Fatal(err)}
 case <-time.After(2*time.Second):t.Fatal("Agent failed to disconnect")}
}

// A PHP-origin Relay is fully attacker-controlled in the compromise model.
// No syntactic ambiguity or extra command may reach Agent business logic.
func TestRelayCommandEnvelopeRejectsCompromisedPHPJSON(t *testing.T) {
 for _,tc:=range []struct{name,wire,allowedType string}{
  {"valid reachability probe","{\"type\":\"reachability_probe\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"client_nonce\":\"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\",\"browser_key\":\"browser-ephemeral-public-key\",\"portal_ticket\":\"portal-ticket\"}","reachability_probe"},
  {"valid login","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\"}","e2ee_login"},
  {"valid echo","{\"type\":\"e2ee_echo\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\"}","e2ee_echo"},
  {"duplicate operation","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\",\"type\":\"reachability_probe\"}",""},
  {"case folded Type alias","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\",\"Type\":\"reachability_probe\"}",""},
  {"case folded Ciphertext alias","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\",\"Ciphertext\":\"injected\"}",""},
  {"escaped duplicate field","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\",\"\\u0074ype\":\"reachability_probe\"}",""},
  {"extra credential field","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\",\"password\":\"stolen\"}",""},
  {"extra application command","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\",\"command\":\"terminal.run\"}",""},
  {"extra bearer","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\",\"bearer\":\"forged\"}",""},
  {"mixed command schemas","{\"type\":\"reachability_probe\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"client_nonce\":\"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\",\"browser_key\":\"browser-ephemeral-public-key\",\"portal_ticket\":\"portal-ticket\",\"ciphertext\":\"injected\"}",""},
  {"missing signed grant","{\"type\":\"reachability_probe\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"client_nonce\":\"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\",\"browser_key\":\"browser-ephemeral-public-key\"}",""},
  {"login missing ciphertext","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"}",""},
  {"null ciphertext","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":null}",""},
  {"object ciphertext","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":{\"data\":\"x\"}}",""},
  {"array ciphertext","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":[\"x\"]}",""},
  {"trailing object","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\"}{\"type\":\"e2ee_echo\"}",""},
  {"trailing array","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\"}[]",""},
  {"raw array","[{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\"}]",""},
  {"unknown type","{\"type\":\"terminal.run\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\"}",""},
  {"duplicate nested object","{\"type\":\"e2ee_login\",\"probe_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"ciphertext\":\"opaque-base64-aead\",\"metadata\":{\"x\":1,\"x\":2}}",""},
 } {
  t.Run(tc.name,func(t *testing.T){
   parsed,err:=decodeRelayCommandFrame([]byte(tc.wire))
   if tc.allowedType=="" {
    if err==nil {t.Fatalf("untrusted PHP command accepted: %#v",parsed)}
    return
   }
   if err!=nil || parsed.Type!=tc.allowedType {
    t.Fatalf("legitimate strict Relay frame rejected: %#v %v",parsed,err)
   }
  })
 }
}

// A hostile PHP Portal can mint 30-second signed tickets but cannot extend
// TaskDeck's independent local login gate. The first browser rendezvous,
// approved-key preflight and subsequent encrypted login must all be polled
// while this short-lived ticket is fresh; a 20s idle heartbeat made this
// mathematically impossible (three Agent polls took 60 seconds).
// The Relay separately admits only one authenticated device frame/2s
// sustained; do not solve this by polling faster than the defense budget.
func TestRelayDefaultHeartbeatPreservesTicketAndDeviceFrameBudget(t *testing.T){
 if defaultRelayHeartbeatInterval<2*time.Second {
  t.Fatalf("Agent heartbeat %s exceeds Relay's sustained frame budget",
   defaultRelayHeartbeatInterval)
 }
 if 3*defaultRelayHeartbeatInterval>=30*time.Second {
  t.Fatalf("Agent heartbeat %s consumes 30s signed ticket before key/password",
   defaultRelayHeartbeatInterval)
 }
 if defaultRelayHeartbeatInterval>3*time.Second {
  t.Fatalf("Agent heartbeat %s leaves too little time for native password entry",
   defaultRelayHeartbeatInterval)
 }
}
