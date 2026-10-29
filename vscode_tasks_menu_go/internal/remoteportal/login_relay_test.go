package remoteportal

import (
 "bytes"
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
 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "github.com/coder/websocket"
)

// Uses real TLS/WebSockets and the same Agent login-only callback shape that
// main.go wires to the actual SQLite TaskDeck verifier. No TaskDeck API exists.
func TestEncryptedTaskDeckLoginThroughRealRelay(t *testing.T) {
 pub,private,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 verifier,err:=remoterelay.NewVerifier(base64.StdEncoding.EncodeToString(pub),
  "https://portal.example.com")
 if err!=nil{t.Fatal(err)}
 var handler *remoterelay.Handler
 srv:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  handler.ServeHTTP(w,r)
 }))
 defer srv.Close()
 registry,err:=remoterelay.NewRegistry(srv.URL)
 if err!=nil{t.Fatal(err)}
 handler=&remoterelay.Handler{Tickets:verifier,Devices:registry}
 agent,err:=NewAgent("https://portal.example.com",
  filepath.Join(t.TempDir(),"private","identity.seed"),srv.Client())
 if err!=nil{t.Fatal(err)}
 if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(pub));err!=nil{t.Fatal(err)}
 // The integration here exercises the Relay and E2EE login. Live Portal
 // authorization is separately exercised against a TLS Portal test host.
 agent.liveGrantCheck=func(_ context.Context,_ *AgentGrant)error{return nil}
 if err:=agent.ConfigureEncryptedLoginVerifier(func(ctx context.Context,
  channel *remotetransport.Channel,grant *AgentGrant,frame []byte)([]byte,error){
  credentials,err:=remotetransport.OpenRemoteLogin(channel,frame)
  if err!=nil{return nil,err}
  _,err=grant.TakeForLogin(channel,time.Now())
  if err!=nil{return nil,err}
  authenticated:=credentials.Username=="test-user" && credentials.Password=="dummy-secret"
  credentials.Password=""
  return remotetransport.SealRemoteLoginResult(channel,authenticated)
 });err!=nil{t.Fatal(err)}
 now:=time.Now().Unix()
 lease:=remoterelay.DeviceLease{V:1,Iss:"https://portal.example.com",
  Aud:"taskdeck-relay-device",DeviceID:agent.DeviceID(),
  Iat:now,Exp:now+70}
 raw,err:=json.Marshal(lease)
 if err!=nil{t.Fatal(err)}
 body:=base64.RawURLEncoding.EncodeToString(raw)
 sig:=ed25519.Sign(private,[]byte("taskdeck-relay-device-lease-v1."+body))
 agent.leaseMu.Lock()
 agent.deviceLease=body+"."+base64.RawURLEncoding.EncodeToString(sig)
 agent.leaseMu.Unlock()
 // Multiple fresh signed tickets now exercise the real Relay's per-socket
 // message budget. Give the legitimate Agent time to pace heartbeats
 // rather than simulating a 35ms authenticated frame flood.
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second)
 defer cancel()
 errors:=make(chan error,1)
 go func(){errors<-agent.connectRelay(ctx,srv.URL,2*time.Second)}()
 onlineDeadline:=time.After(2*time.Second)
 for !registry.Online(agent.DeviceID(),time.Now()){
  select{case <-onlineDeadline:t.Fatal("authenticated Agent never connected")
   case <-time.After(10*time.Millisecond):}
 }
 browserKey,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 nonce:=strings.Repeat("c",32)
 binding,err:=remotetransport.BrowserTicketBinding(nonce,browserKey.Public)
 if err!=nil{t.Fatal(err)}
 claims:=remoterelay.Claims{V:2,Issuer:"https://portal.example.com",
  Audience:"taskdeck-relay",DeviceID:agent.DeviceID(),Role:"viewer",
  Subject:"21",AuthEpoch:1,IssuedAt:time.Now().Unix(),Expires:time.Now().Unix()+30,
  JTI:strings.Repeat("a",32),BrowserBinding:binding}
 ticket:=signPortalTicket(t,private,claims)
 headers:=http.Header{}
 headers.Set("Origin","https://portal.example.com")
 browser,_,err:=websocket.Dial(ctx,srv.URL+"/v1/browser",&websocket.DialOptions{
  HTTPClient:srv.Client(),HTTPHeader:headers,
 })
 if err!=nil{t.Fatal(err)}
 defer browser.CloseNow()
 first,_:=json.Marshal(map[string]string{
  "ticket":ticket,"client_nonce":nonce,
  "browser_key":base64.StdEncoding.EncodeToString(browserKey.Public),
 })
 if err:=browser.Write(ctx,websocket.MessageText,first);err!=nil{t.Fatal(err)}
 typ,payload,err:=browser.Read(ctx)
 if err!=nil || typ!=websocket.MessageText{t.Fatalf("Relay E2EE proof: %v",err)}
 var initial struct{
  TicketValid bool `json:"ticket_valid"`
  AgentReachable bool `json:"agent_reachable"`
  RemoteControlAvailable bool `json:"remote_control_available"`
  AgentProof remoterelay.ProbeAttestation `json:"agent_proof"`
 }
 if json.Unmarshal(payload,&initial)!=nil || !initial.TicketValid ||
  !initial.AgentReachable || initial.RemoteControlAvailable || initial.AgentProof.Crypto==nil{
  t.Fatalf("missing Agent attestation: %s",payload)
 }
 hello:=initial.AgentProof.Crypto
 identity,_:=base64.StdEncoding.DecodeString(hello.Identity)
 ephemeral,_:=base64.StdEncoding.DecodeString(hello.Ephemeral)
 signature,_:=base64.StdEncoding.DecodeString(hello.Signature)
 readyFrame,_:=base64.StdEncoding.DecodeString(hello.EncryptedReady)
 browserChannel,err:=remotetransport.AcceptHello(browserKey,
  agent.DeviceID(),initial.AgentProof.ProbeID,nonce,
  remotetransport.Hello{Identity:identity,Ephemeral:ephemeral,Signature:signature})
 if err!=nil{t.Fatal(err)}
 ready,err:=browserChannel.Open(readyFrame)
 if err!=nil || string(ready)!="taskdeck-e2ee-ready-v1"{t.Fatal("Agent E2EE ready invalid")}
 // The client must prove its local Ed25519 key BEFORE the password prompt.
 // This is real WSS: Relay forwards ciphertext but never learns the status.
 clientPublic,clientPrivate,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 clientEncoded:=base64.StdEncoding.EncodeToString(clientPublic)
 if _,err:=remoteclientkey.Add(agent.clientKeyRegistryPath,"test-user",clientEncoded,"integration-client");err!=nil{t.Fatal(err)}
 clientSignature:=base64.StdEncoding.EncodeToString(ed25519.Sign(clientPrivate,
  remoteclientkey.ProofMessage("test-user",browserChannel.RemoteClientProofBinding())))
 keyFrame,err:=remotetransport.ClientKeyCheckFrame(browserChannel,remotetransport.ClientKeyCheck{
  Username:"test-user",PublicKey:clientEncoded,Signature:clientSignature,
 })
 if err!=nil{t.Fatal(err)}
 check,_:=json.Marshal(map[string]string{
  "type":"e2ee_client_key_check","ciphertext":base64.StdEncoding.EncodeToString(keyFrame),
 })
 if err:=browser.Write(ctx,websocket.MessageText,check);err!=nil{t.Fatal(err)}
 typ,checkResponse,err:=browser.Read(ctx)
 if err!=nil||typ!=websocket.MessageText{t.Fatalf("no E2EE key approval status: %v",err)}
 var resultCheck struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if json.Unmarshal(checkResponse,&resultCheck)!=nil||resultCheck.Type!="e2ee_client_key_check_result"||
  resultCheck.RemoteControlAvailable{t.Fatalf("key-check wrapper invalid: %s",checkResponse)}
 keyStatusFrame,err:=base64.StdEncoding.DecodeString(resultCheck.Ciphertext)
 if err!=nil{t.Fatal(err)}
 approved,err:=remotetransport.OpenClientKeyStatus(browserChannel,keyStatusFrame)
 if err!=nil||!approved{t.Fatalf("valid host-CLI-approved key was rejected: %v",err)}
 frame,err:=remotetransport.RemoteLoginRequestWithClientKey(browserChannel,
  "test-user","dummy-secret",clientEncoded,clientSignature)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(frame,[]byte("dummy-secret")){t.Fatal("Relay could read TaskDeck password")}
 login,_:=json.Marshal(map[string]string{
  "type":"e2ee_login","ciphertext":base64.StdEncoding.EncodeToString(frame),
 })
 if err:=browser.Write(ctx,websocket.MessageText,login);err!=nil{t.Fatal(err)}
 typ,reply,err:=browser.Read(ctx)
 if err!=nil || typ!=websocket.MessageText{t.Fatalf("Relay login result: %v",err)}
 var result struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if json.Unmarshal(reply,&result)!=nil || result.Type!="e2ee_login_result" ||
  result.RemoteControlAvailable || result.Ciphertext==""{
  t.Fatalf("invalid Relay encrypted login response: %s",reply)
 }
 sealed,err:=base64.StdEncoding.DecodeString(result.Ciphertext)
 if err!=nil{t.Fatal(err)}
 authenticated,err:=remotetransport.OpenRemoteLoginResult(browserChannel,sealed)
 if err!=nil || !authenticated {t.Fatalf("E2EE authenticated boolean failed: %v",err)}
 // There is no transfer of browser cookie, TaskDeck principal or login
 // session and the connection must terminate after a single attempt.
 if bytes.Contains(reply,[]byte("dummy-secret")) || bytes.Contains(reply,[]byte("test-user")){
  t.Fatal("Relay response leaked TaskDeck credentials")
 }
 _,_,err=browser.Read(ctx)
 if err==nil{t.Fatal("Relay allowed an application response after one-shot login")}
 // A rejected login MUST close only the Browser reservation and leave
 // Agent's authenticated connection available for another fresh ticket.
 // This is checked across real TLS/WSS, not just the Registry unit tests.
 attempt:=func(nonce,jti string,malformed,expectDenied bool) bool {
  t.Logf("follow-up login attempt nonce=%s malformed=%v denied=%v",nonce,malformed,expectDenied)
  // Wait for the previous Browser handler's release closure, without
  // relying on a fixed sleep or claiming cached presence is liveness.
  deadline:=time.After(2*time.Second)
  for {
   release,e:=registry.Acquire(remoterelay.Claims{DeviceID:agent.DeviceID()},time.Now())
   if e==nil {release();break}
   select {
   case <-deadline:t.Fatal("Agent did not become available for next login")
   case <-time.After(10*time.Millisecond):
   }
  }
  ephemeralBrowser,e:=remotetransport.NewEphemeral()
  if e!=nil{t.Fatal(e)}
  binding,e:=remotetransport.BrowserTicketBinding(nonce,ephemeralBrowser.Public)
  if e!=nil{t.Fatal(e)}
  issued:=time.Now().Unix()
  newClaims:=remoterelay.Claims{
   V:2,Issuer:"https://portal.example.com",Audience:"taskdeck-relay",
   DeviceID:agent.DeviceID(),Role:"viewer",Subject:"21",AuthEpoch:1,
   IssuedAt:issued,Expires:issued+30,JTI:jti,BrowserBinding:binding,
  }
  newTicket:=signPortalTicket(t,private,newClaims)
  ws,_,e:=websocket.Dial(ctx,srv.URL+"/v1/browser",&websocket.DialOptions{
   HTTPClient:srv.Client(),HTTPHeader:headers,
  })
  if e!=nil{t.Fatal(e)}
  defer ws.CloseNow()
  offer,_:=json.Marshal(map[string]string{
   "ticket":newTicket,"client_nonce":nonce,
   "browser_key":base64.StdEncoding.EncodeToString(ephemeralBrowser.Public),
  })
  if e:=ws.Write(ctx,websocket.MessageText,offer);e!=nil{t.Fatal(e)}
  typ,payload,e:=ws.Read(ctx)
  if e!=nil || typ!=websocket.MessageText{t.Fatalf("second Agent handshake: %v",e)}
  var signed struct{
   AgentReachable bool `json:"agent_reachable"`
   RemoteControlAvailable bool `json:"remote_control_available"`
   AgentProof remoterelay.ProbeAttestation `json:"agent_proof"`
  }
  if json.Unmarshal(payload,&signed)!=nil || signed.RemoteControlAvailable {
   t.Fatalf("Relay returned invalid or privilege-escalated Agent proof: %s",payload)
  }
  if !signed.AgentReachable || signed.AgentProof.Crypto==nil {
   // Agent rate limiting can deny the final sixth ticket before the
   // password attempt. Such a denial is still fail-closed: there is no
   // E2EE channel, authenticated TaskDeck principal or application data.
   if expectDenied {
    t.Logf("device-wide login budget refused final probe before password: %s",payload)
    return false
   }
   select {case agentErr:=<-errors:t.Logf("Agent transport exited: %v",agentErr);default:}
   t.Fatalf("Agent unavailable for non-throttled browser login: %s",payload)
  }
  hs:=signed.AgentProof.Crypto
  id,_:=base64.StdEncoding.DecodeString(hs.Identity)
  epk,_:=base64.StdEncoding.DecodeString(hs.Ephemeral)
  sig,_:=base64.StdEncoding.DecodeString(hs.Signature)
  channel,e:=remotetransport.AcceptHello(ephemeralBrowser,agent.DeviceID(),
   signed.AgentProof.ProbeID,nonce,
   remotetransport.Hello{Identity:id,Ephemeral:epk,Signature:sig})
  if e!=nil{t.Fatal(e)}
  ready,_:=base64.StdEncoding.DecodeString(hs.EncryptedReady)
  if clear,e:=channel.Open(ready);e!=nil || string(clear)!="taskdeck-e2ee-ready-v1"{
   t.Fatalf("second Agent ready proof invalid: %v",e)
  }
  var payloadFrame []byte
  if malformed {
   payloadFrame,e=channel.Seal([]byte(`{"type":"terminal.run","username":"test-user","password":"dummy-secret"}`))
  } else {
   payloadFrame,e=remotetransport.RemoteLoginRequestFrame(channel,"test-user","dummy-secret")
  }
  if e!=nil{t.Fatal(e)}
  wire,_:=json.Marshal(map[string]string{
   "type":"e2ee_login","ciphertext":base64.StdEncoding.EncodeToString(payloadFrame),
  })
  if e:=ws.Write(ctx,websocket.MessageText,wire);e!=nil{t.Fatal(e)}
  typ,response,e:=ws.Read(ctx)
  if malformed || expectDenied {
   if e==nil {t.Fatalf("rejected login returned unexpected application message: %s",response)}
   return false
  }
  if e!=nil || typ!=websocket.MessageText {t.Fatalf("Agent disconnected after earlier rejected login: %v",e)}
  var sealedResponse struct {
   Type string `json:"type"`
   Ciphertext string `json:"ciphertext"`
   RemoteControlAvailable bool `json:"remote_control_available"`
  }
  if json.Unmarshal(response,&sealedResponse)!=nil || sealedResponse.Type!="e2ee_login_result" ||
    sealedResponse.RemoteControlAvailable {t.Fatalf("unexpected login result: %s",response)}
  sealed,_:=base64.StdEncoding.DecodeString(sealedResponse.Ciphertext)
  passed,e:=remotetransport.OpenRemoteLoginResult(channel,sealed)
  if e!=nil {t.Fatal(e)}
  return passed
 }
 if attempt(strings.Repeat("d",32),strings.Repeat("b",32),true,false) {
  t.Fatal("malformed encrypted TaskDeck command was accepted")
 }
 if !attempt(strings.Repeat("e",32),strings.Repeat("c",32),false,false) {
  t.Fatal("valid browser could not authenticate after a malformed login")
 }
 // The initial success + malformed attempt + recovered success = 3
 // Agent-wide password-check calls. Two more signed tickets reach the
 // fixed burst cap; a sixth fresh ticket MUST be rejected even though
 // it has another Browser nonce, ECDH key and legitimate Portal signature.
 if !attempt(strings.Repeat("f",32),strings.Repeat("d",32),false,false) {
  t.Fatal("fourth permitted Agent login probe failed")
 }
 if !attempt(strings.Repeat("0",32),strings.Repeat("e",32),false,false) {
  t.Fatal("fifth permitted Agent login probe failed")
 }
 if attempt(strings.Repeat("1",32),strings.Repeat("f",32),false,true) {
  t.Fatal("sixth Portal ticket bypassed the Agent-wide scrypt quota")
 }
 cancel()
 select{
 case <-errors:
 case <-time.After(2*time.Second):t.Fatal("authenticated Agent did not stop")
 }
}
