package server

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

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/remoteportal"
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
 agent,err:=remoteportal.NewAgent(srv.URL,
  filepath.Join(t.TempDir(),"owner-only","identity.seed"),srv.Client())
 if err!=nil{t.Fatal(err)}
 if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(portalPublic));err!=nil{t.Fatal(err)}
 if err:=agent.ConfigureEncryptedLoginVerifier(taskdeck.VerifyEncryptedRemoteLogin);err!=nil{t.Fatal(err)}
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
 wrongPassword:="compromised-portal-does-not-know-local-password"
 login,err:=remotetransport.RemoteLoginRequestFrame(channel,"alice",wrongPassword)
 if err!=nil{t.Fatal(err)}
 if strings.Contains(base64.StdEncoding.EncodeToString(login),wrongPassword) {
  t.Fatal("TaskDeck password escaped E2EE")
 }
 attempt,err:=json.Marshal(map[string]string{
  "type":"e2ee_login","ciphertext":base64.StdEncoding.EncodeToString(login),
 })
 if err!=nil{t.Fatal(err)}
 if err:=conn.Write(ctx,websocket.MessageText,attempt);err!=nil{t.Fatal(err)}
 kind,answer,err:=conn.Read(ctx)
 if err!=nil {
  // A denial that closes the browser is also fail-closed.
  t.Logf("compromised PHP denied by Agent before result: %v",err)
  return
 }
 if kind!=websocket.MessageText {t.Fatal("binary response from Relay")}
 var outcome struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(answer,&outcome);err!=nil{t.Fatal(err)}
 if outcome.Type!="e2ee_login_result" || outcome.RemoteControlAvailable {
  t.Fatalf("compromised Portal got application access: %s",answer)
 }
 ciphertext,err:=base64.StdEncoding.DecodeString(outcome.Ciphertext)
 if err!=nil{t.Fatal(err)}
 loggedIn,err:=remotetransport.OpenRemoteLoginResult(channel,ciphertext)
 if err!=nil{t.Fatal(err)}
 if loggedIn {t.Fatal("P0: forged Portal admin ticket bypassed TaskDeck LOCAL password")}
 // Even after a fully signed Portal ticket + successful device rendezvous,
 // the Relay destroys the browser session; no TaskDeck APIs are forwarded.
 if _,_,err:=conn.Read(ctx);err==nil {
  t.Fatal("compromised Portal obtained a reusable remote application channel")
 }
}
