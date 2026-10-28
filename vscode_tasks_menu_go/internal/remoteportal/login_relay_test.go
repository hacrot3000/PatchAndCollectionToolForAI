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
 ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
 defer cancel()
 errors:=make(chan error,1)
 go func(){errors<-agent.connectRelay(ctx,srv.URL,35*time.Millisecond)}()
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
  Subject:"21",IssuedAt:time.Now().Unix(),Expires:time.Now().Unix()+30,
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
 frame,err:=remotetransport.RemoteLoginRequestFrame(browserChannel,"test-user","dummy-secret")
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
 cancel()
 select{
 case <-errors:
 case <-time.After(2*time.Second):t.Fatal("authenticated Agent did not stop")
 }
}
