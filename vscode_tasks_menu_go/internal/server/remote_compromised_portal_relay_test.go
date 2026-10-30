package server

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

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/remoteportal"
 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
 "bletonfc/vscode_tasks_menu/internal/remoterelay"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "github.com/coder/websocket"
)

// A real WSS/Agent/TaskDeck-local identity integration under the strongest
// PHP compromise: the attacker knows Portal's Ed25519 signing SEED, can mint
// arbitrary admin tickets and Agent leases, and makes all HTTPS grant-checks
// return "allowed":true. No mocked local password callback is permitted.
// This intentionally does not grant HTTP/file/terminal/command dispatch.
func TestRemoteCompromisedPHPPortalWithSigningKeyCannotAuthenticateOverRealRelay(t *testing.T) {
 taskdeck:=sharedLoginTestServer(t)
 taskdeck.Config.RemoteAccessEnabled=true
 taskdeck.Config.Protocol=config.ProtocolHTTPS

 portalPublic,compromisedPortalSeed,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil {t.Fatal(err)}
 var srv *httptest.Server
 var relay *remoterelay.Handler
 portal := http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
  switch r.URL.Path {
  case "/api/agent/heartbeat":
   // Attacker can issue a legitimate-looking signed device lease.
   var req struct{DeviceID string `json:"device_id"`}
   if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil {
    http.Error(w,"bad heartbeat",400);return
   }
   now:=time.Now().Unix()
   lease:=remoterelay.DeviceLease{
    V:1,Iss:srv.URL,Aud:"taskdeck-relay-device",
    DeviceID:req.DeviceID,Iat:now,Exp:now+70,
   }
   body,err:=json.Marshal(lease)
   if err!=nil{t.Error(err);return}
   encoded:=base64.RawURLEncoding.EncodeToString(body)
   signature:=ed25519.Sign(compromisedPortalSeed,
    []byte("taskdeck-relay-device-lease-v1."+encoded))
   token:=encoded+"."+base64.RawURLEncoding.EncodeToString(signature)
   _=json.NewEncoder(w).Encode(map[string]interface{}{
    "ok":true,"device_lease":token,
   })
  case "/api/agent/grant-check":
   // Entire compromise: ignores pairing, user, ticket and MFA and
   // always returns permission. This must NOT authenticate TaskDeck.
   _=json.NewEncoder(w).Encode(map[string]bool{"ok":true,"allowed":true})
  default:
   relay.ServeHTTP(w,r)
  }
 })
 srv=httptest.NewTLSServer(portal)
 defer srv.Close()
 registry,err:=remoterelay.NewRegistry(srv.URL)
 if err!=nil{t.Fatal(err)}
 verifier,err:=remoterelay.NewVerifier(base64.StdEncoding.EncodeToString(portalPublic),srv.URL)
 if err!=nil{t.Fatal(err)}
 relay=&remoterelay.Handler{Tickets:verifier,Devices:registry}
 // Use the real workspace-bound identity path and host allowlist, not a
 // temporary unrelated Agent registry that would trivially reject EVERY key.
 // This ensures a copied but locally approved public key is tested.
 keyPath,err:=remoteportal.DefaultIdentityPath(taskdeck.Workspace)
 if err!=nil{t.Fatal(err)}
 agent,err:=remoteportal.NewAgent(srv.URL,keyPath,srv.Client())
 if err!=nil{t.Fatal(err)}
 if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(portalPublic));err!=nil{t.Fatal(err)}
 if err:=agent.ConfigureEncryptedLoginVerifier(taskdeck.VerifyEncryptedRemoteLogin);err!=nil{t.Fatal(err)}
 if err:=agent.ConfigureClientKeyLocalUserLookup(taskdeck.LookupRemoteClientLocalUserID);err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),100*time.Second)
 defer cancel()
 if err:=agent.Heartbeat(ctx);err!=nil{t.Fatalf("compromised Portal lease rejected: %v",err)}
 ended:=make(chan error,1)
 go func(){ended<-agent.ConnectRelay(ctx,srv.URL)}()
 deadline:=time.After(3*time.Second)
 for !registry.Online(agent.DeviceID(),time.Now()) {
  select {
  case <-deadline:t.Fatal("authentic Agent failed to register with compromised-Portal lease")
  case err:=<-ended:t.Fatalf("Agent disconnected: %v",err)
  case <-time.After(10*time.Millisecond):
  }
 }
 // The attacker selects arbitrary admin access, session JTI, browser key and
 // valid Portal signature. The real TaskDeck username exists, password wrong.
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 nonce:=strings.Repeat("c",32)
 binding,err:=remotetransport.BrowserTicketBinding(nonce,browser.Public)
 if err!=nil{t.Fatal(err)}
 now:=time.Now().Unix()
 claims:=remoterelay.Claims{
  V:2,Issuer:srv.URL,Audience:"taskdeck-relay",Subject:"1",
  DeviceID:agent.DeviceID(),AuthEpoch:1,Role:"admin",
  IssuedAt:now,Expires:now+30,JTI:strings.Repeat("e",32),
  BrowserBinding:binding,
 }
 payload,err:=json.Marshal(claims)
 if err!=nil{t.Fatal(err)}
 encoded:=base64.RawURLEncoding.EncodeToString(payload)
 signature:=ed25519.Sign(compromisedPortalSeed,
  []byte("taskdeck-relay-ticket-v1."+encoded))
 ticket:=encoded+"."+base64.RawURLEncoding.EncodeToString(signature)
 headers:=http.Header{}
 headers.Set("Origin",srv.URL)
 conn,_,err:=websocket.Dial(ctx,srv.URL+"/v1/browser",
  &websocket.DialOptions{HTTPClient:srv.Client(),HTTPHeader:headers})
 if err!=nil{t.Fatal(err)}
 defer conn.CloseNow()
 first,err:=json.Marshal(map[string]string{
  "ticket":ticket,"client_nonce":nonce,
  "browser_key":base64.StdEncoding.EncodeToString(browser.Public),
 })
 if err!=nil{t.Fatal(err)}
 if err:=conn.Write(ctx,websocket.MessageText,first);err!=nil{t.Fatal(err)}
 kind,initial,err:=conn.Read(ctx)
 if err!=nil || kind!=websocket.MessageText {t.Fatalf("real Relay rendezvous failed: %v",err)}
 var proof struct{
  TicketValid bool `json:"ticket_valid"`
  AgentReachable bool `json:"agent_reachable"`
  RemoteControlAvailable bool `json:"remote_control_available"`
  DeviceID string `json:"device_id"`
  AgentProof remoterelay.ProbeAttestation `json:"agent_proof"`
 }
 if err:=json.Unmarshal(initial,&proof);err!=nil{t.Fatal(err)}
 if !proof.TicketValid || !proof.AgentReachable || proof.RemoteControlAvailable ||
  proof.DeviceID!=agent.DeviceID() || proof.AgentProof.Crypto==nil {
  t.Fatalf("bad signed Agent attestation: %s",initial)
 }
 c:=proof.AgentProof.Crypto
 identity,err:=base64.StdEncoding.DecodeString(c.Identity)
 if err!=nil{t.Fatal(err)}
 ephemeral,err:=base64.StdEncoding.DecodeString(c.Ephemeral)
 if err!=nil{t.Fatal(err)}
 sig,err:=base64.StdEncoding.DecodeString(c.Signature)
 if err!=nil{t.Fatal(err)}
 ready,err:=base64.StdEncoding.DecodeString(c.EncryptedReady)
 if err!=nil{t.Fatal(err)}
 channel,err:=remotetransport.AcceptHello(browser,agent.DeviceID(),
  proof.AgentProof.ProbeID,nonce,
  remotetransport.Hello{Identity:identity,Ephemeral:ephemeral,Signature:sig})
 if err!=nil{t.Fatal(err)}
 defer channel.Close()
 clear,err:=channel.Open(ready)
 if err!=nil || string(clear)!="taskdeck-e2ee-ready-v1" {
  t.Fatalf("authenticated Agent E2EE ready failed: %v",err)
 }
 // The compromised Portal can steal the exact PUBLIC software key
 // already approved through the host-only TaskDeck CLI, and may also know
 // the user's real password. It cannot copy the private Ed25519 client key.
 ownerPublic,_,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 approvedKey:=base64.StdEncoding.EncodeToString(ownerPublic)
 hostRegistry,err:=remoteclientkey.DefaultPath(taskdeck.Workspace)
 if err!=nil{t.Fatal(err)}
 alice,err:=taskdeck.Identity.UserByUsername(ctx,"alice")
 if err!=nil{t.Fatal(err)}
 if _,err:=remoteclientkey.AddForUser(hostRegistry,alice.Username,
  string(alice.ID),approvedKey,"trusted client owned by real user");err!=nil{
  t.Fatal(err)
 }
 _,attackerKey,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 // This is an intentionally invalid signature for the approved key.
 forgedClientSignature:=base64.StdEncoding.EncodeToString(ed25519.Sign(
  attackerKey,remoteclientkey.ProofMessage("alice",channel.RemoteClientProofBinding())))
 preflight,err:=remotetransport.ClientKeyCheckFrame(channel,remotetransport.ClientKeyCheck{
  Username:"alice",PublicKey:approvedKey,Signature:forgedClientSignature,
 })
 if err!=nil{t.Fatal(err)}
 keyRequest,err:=json.Marshal(map[string]string{
  "type":"e2ee_client_key_check",
  "ciphertext":base64.StdEncoding.EncodeToString(preflight),
 })
 if err!=nil{t.Fatal(err)}
 if err:=conn.Write(ctx,websocket.MessageText,keyRequest);err!=nil{t.Fatal(err)}
 kind,keyAnswer,err:=conn.Read(ctx)
 if err!=nil || kind!=websocket.MessageText{
  t.Fatalf("key preflight did not produce encrypted denial: %v",err)
 }
 var keyOutcome struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(keyAnswer,&keyOutcome);err!=nil{t.Fatal(err)}
 if keyOutcome.Type!="e2ee_client_key_check_result" ||
  keyOutcome.RemoteControlAvailable {t.Fatalf("untrusted Portal exposed authority: %s",keyAnswer)}
 encryptedKeyStatus,err:=base64.StdEncoding.Strict().DecodeString(keyOutcome.Ciphertext)
 if err!=nil{t.Fatal(err)}
 approved,err:=remotetransport.OpenClientKeyStatus(channel,encryptedKeyStatus)
 if err!=nil || approved{
  t.Fatalf("P0: PHP copied an approved PUBLIC key and passed private-key gate: %v %v",approved,err)
 }
 // A malicious modified PHP can still try to pipe a password-based login
 // despite denied host preflight. The Agent must refuse it on this session.
 const knownRealPassword="private-admin-password"
 login,err:=remotetransport.RemoteLoginRequestWithClientKey(channel,
  "alice",knownRealPassword,approvedKey,forgedClientSignature)
 if err!=nil{t.Fatal(err)}
 attempt,err:=json.Marshal(map[string]string{
  "type":"e2ee_login","ciphertext":base64.StdEncoding.EncodeToString(login),
 })
 if err!=nil{t.Fatal(err)}
 if err:=conn.Write(ctx,websocket.MessageText,attempt);err!=nil{t.Fatal(err)}
 kind,answer,err:=conn.Read(ctx)
 if err==nil {
  if kind!=websocket.MessageText{t.Fatal("unexpected binary app response")}
  var outcome struct{
   Type string `json:"type"`
   Ciphertext string `json:"ciphertext"`
   RemoteControlAvailable bool `json:"remote_control_available"`
  }
  if err:=json.Unmarshal(answer,&outcome);err!=nil{t.Fatal(err)}
  if outcome.RemoteControlAvailable {t.Fatal("P0: hacked PHP unlocked Remote Control")}
  if outcome.Type=="e2ee_login_result" {
   sealed,err:=base64.StdEncoding.Strict().DecodeString(outcome.Ciphertext)
   if err!=nil{t.Fatal(err)}
   loggedIn,err:=remotetransport.OpenRemoteLoginResult(channel,sealed)
   if err==nil && loggedIn {
    t.Fatal("P0: stolen Portal signer+real password+copied public key bypassed native key")
   }
  } else if outcome.Type!="e2ee_login_rejected" {
   t.Fatalf("compromised Portal obtained an application response: %s",answer)
  }
 }
 if _,_,err:=conn.Read(ctx);err==nil{
  t.Fatal("compromised Portal retained reusable app session after denial")
 }
}


// The PHP Portal may possess its signing seed and may fabricate a syntactically
// valid admin ticket, bearer header, and same-looking cookie. None of those
// are a TaskDeck local browser session. Exercise the *real* HTTP middleware
// rather than relying only on the unmounted E2EE authentication callback.
func TestCompromisedPHPAdminTicketNeverAuthenticatesTaskDeckHTTP(t *testing.T) {
 s:=sharedLoginTestServer(t)
 s.Config.RemoteAccessEnabled=true
 s.Config.Protocol=config.ProtocolHTTPS
 handler:=s.Handler()
 _,portalSigner,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 fakeClaims:=map[string]interface{}{
  "v":2,"role":"admin","device_id":strings.Repeat("a",64),
  "sub":"1","jti":strings.Repeat("b",32),
 }
 raw,err:=json.Marshal(fakeClaims)
 if err!=nil{t.Fatal(err)}
 encoded:=base64.RawURLEncoding.EncodeToString(raw)
 forgedTicket:=encoded+"."+base64.RawURLEncoding.EncodeToString(
  ed25519.Sign(portalSigner,[]byte("taskdeck-relay-ticket-v1."+encoded)))
 for _,tc:=range []struct{method,path string}{
  {http.MethodGet,"/api/config/remote-access"},
  {http.MethodPost,"/api/config/remote-access"},
  {http.MethodGet,"/api/tasks"},
  {http.MethodGet,"/api/sessions"},
  {http.MethodGet,"/api/auth/me"},
  {http.MethodPost,"/api/auth/password"},
  {http.MethodGet,"/api/project/file"},
 } {
  t.Run(tc.method+" "+tc.path,func(t *testing.T){
   request:=httptest.NewRequest(tc.method,"https://taskdeck.example.com"+tc.path,nil)
   request.RemoteAddr="198.51.100.25:43434"
   request.Header.Set("Origin","https://taskdeck.example.com")
   request.Header.Set("Sec-Fetch-Site","same-origin")
   request.Header.Set("Content-Type","application/json")
   request.Header.Set("Authorization","Bearer "+forgedTicket)
   request.Header.Set("X-Portal-Role","admin")
   request.Header.Set("X-TaskDeck-Remote-Ticket",forgedTicket)
   request.AddCookie(&http.Cookie{Name:s.sharedCookieName(),Value:forgedTicket})
   recorder:=httptest.NewRecorder()
   handler.ServeHTTP(recorder,request)
   if recorder.Code!=http.StatusUnauthorized && recorder.Code!=http.StatusForbidden {
    t.Fatalf("P0: compromised PHP ticket opened TaskDeck HTTP route: %d body=%q",
     recorder.Code,recorder.Body.String())
   }
   for _,cookie:=range recorder.Result().Cookies() {
    if cookie.Name==s.sharedCookieName() && cookie.MaxAge>=0 {
     t.Fatal("PHP forged ticket caused TaskDeck to issue a browser session")
    }
   }
  })
 }
}
