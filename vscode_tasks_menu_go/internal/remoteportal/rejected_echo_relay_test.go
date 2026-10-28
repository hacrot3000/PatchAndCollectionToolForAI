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

func TestMalformedEncryptedEchoDoesNotDisconnectLiveAgent(t *testing.T) {
 portalPub,portalPrivate,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 verifier,err:=remoterelay.NewVerifier(base64.StdEncoding.EncodeToString(portalPub),
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
  filepath.Join(t.TempDir(),"agent","identity.seed"),srv.Client())
 if err!=nil{t.Fatal(err)}
 if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(portalPub));err!=nil{
  t.Fatal(err)
 }
 now:=time.Now().Unix()
 lease:=remoterelay.DeviceLease{V:1,Iss:"https://portal.example.com",
  Aud:"taskdeck-relay-device",DeviceID:agent.DeviceID(),Iat:now,Exp:now+70}
 leaseRaw,_:=json.Marshal(lease)
 payload:=base64.RawURLEncoding.EncodeToString(leaseRaw)
 sig:=ed25519.Sign(portalPrivate,[]byte("taskdeck-relay-device-lease-v1."+payload))
 agent.leaseMu.Lock()
 agent.deviceLease=payload+"."+base64.RawURLEncoding.EncodeToString(sig)
 agent.leaseMu.Unlock()
 ctx,cancel:=context.WithTimeout(context.Background(),12*time.Second)
 defer cancel()
 finished:=make(chan error,1)
 go func(){finished<-agent.connectRelay(ctx,srv.URL,35*time.Millisecond)}()
 startup:=time.After(2*time.Second)
 for !registry.Online(agent.DeviceID(),time.Now()) {
  select {
  case <-startup:t.Fatal("Agent never authenticated")
  case <-time.After(10*time.Millisecond):
  }
 }
 header:=http.Header{}
 header.Set("Origin","https://portal.example.com")
 for i:=0;i<2;i++ {
  // Wait for Browser reservation cleanup rather than sleeping.
  deadline:=time.After(2*time.Second)
  for {
   release,e:=registry.Acquire(remoterelay.Claims{DeviceID:agent.DeviceID()},time.Now())
   if e==nil {release();break}
   select {
   case <-deadline:t.Fatal("bad echo left device busy")
   case <-time.After(10*time.Millisecond):
   }
  }
  key,e:=remotetransport.NewEphemeral()
  if e!=nil{t.Fatal(e)}
  nonce:=strings.Repeat(string(rune('a'+i)),32)
  binding,e:=remotetransport.BrowserTicketBinding(nonce,key.Public)
  if e!=nil{t.Fatal(e)}
  issued:=time.Now().Unix()
  ticket:=signPortalTicket(t,portalPrivate,remoterelay.Claims{
   V:2,Issuer:"https://portal.example.com",Audience:"taskdeck-relay",
   DeviceID:agent.DeviceID(),Role:"viewer",Subject:"21",
   IssuedAt:issued,Expires:issued+30,
   JTI:strings.Repeat(string(rune('d'+i)),32),BrowserBinding:binding,
  })
  conn,_,e:=websocket.Dial(ctx,srv.URL+"/v1/browser",&websocket.DialOptions{
   HTTPClient:srv.Client(),HTTPHeader:header,
  })
  if e!=nil{t.Fatal(e)}
  start,_:=json.Marshal(map[string]string{
   "ticket":ticket,"client_nonce":nonce,
   "browser_key":base64.StdEncoding.EncodeToString(key.Public),
  })
  if e=conn.Write(ctx,websocket.MessageText,start);e!=nil{conn.CloseNow();t.Fatal(e)}
  _,body,e:=conn.Read(ctx)
  if e!=nil{conn.CloseNow();t.Fatalf("Browser proof failed: %v",e)}
  var proof struct{
   AgentReachable bool `json:"agent_reachable"`
   AgentProof remoterelay.ProbeAttestation `json:"agent_proof"`
  }
  if json.Unmarshal(body,&proof)!=nil || !proof.AgentReachable ||
   proof.AgentProof.Crypto==nil {
   conn.CloseNow()
   t.Fatalf("Agent probe failed after previous malformed echo: %s",body)
  }
  crypto:=proof.AgentProof.Crypto
  identity,_:=base64.StdEncoding.DecodeString(crypto.Identity)
  ephemeral,_:=base64.StdEncoding.DecodeString(crypto.Ephemeral)
  signature,_:=base64.StdEncoding.DecodeString(crypto.Signature)
  readyFrame,_:=base64.StdEncoding.DecodeString(crypto.EncryptedReady)
  channel,e:=remotetransport.AcceptHello(key,agent.DeviceID(),proof.AgentProof.ProbeID,nonce,
   remotetransport.Hello{Identity:identity,Ephemeral:ephemeral,Signature:signature})
  if e!=nil{conn.CloseNow();t.Fatal(e)}
  if ready,e:=channel.Open(readyFrame);e!=nil ||
   string(ready)!="taskdeck-e2ee-ready-v1" {
   conn.CloseNow();t.Fatalf("E2EE ready invalid: %v",e)
  }
  var challenge string
  if i==0 {challenge="terminal.run"}
  if i==1 {challenge="taskdeck-e2ee-echo-v1:"+strings.Repeat("f",32)}
  encrypted,e:=channel.Seal([]byte(challenge))
  if e!=nil{conn.CloseNow();t.Fatal(e)}
  wire,_:=json.Marshal(map[string]string{
   "type":"e2ee_echo","ciphertext":base64.StdEncoding.EncodeToString(encrypted),
  })
  if e=conn.Write(ctx,websocket.MessageText,wire);e!=nil{conn.CloseNow();t.Fatal(e)}
  _,response,e:=conn.Read(ctx)
  if i==0 {
   if e==nil{conn.CloseNow();t.Fatalf("malformed echo received an application response: %s",response)}
  } else {
   if e!=nil{conn.CloseNow();t.Fatalf("Agent became unavailable after malformed echo: %v",e)}
   var msg struct {
    Type string `json:"type"`
    Ciphertext string `json:"ciphertext"`
    RemoteControlAvailable bool `json:"remote_control_available"`
   }
   if json.Unmarshal(response,&msg)!=nil || msg.Type!="e2ee_echo_reply" ||
    msg.Ciphertext=="" || msg.RemoteControlAvailable {
    conn.CloseNow();t.Fatalf("invalid follow-up echo response: %s",response)
   }
   ciphertext,_:=base64.StdEncoding.DecodeString(msg.Ciphertext)
   clear,e:=channel.Open(ciphertext)
   if e!=nil || string(clear)!="taskdeck-e2ee-echo-reply-v1:"+strings.Repeat("f",32) {
    conn.CloseNow();t.Fatalf("follow-up E2EE echo authentication failed: %v",e)
   }
  }
  conn.CloseNow()
  select {
  case err:=<-finished:t.Fatalf("Browser echo %d disconnected Agent: %v",i,err)
  default:
  }
 }
 cancel()
 select {
 case err:=<-finished:
  if err!=nil{t.Fatalf("Agent shutdown failed: %v",err)}
 case <-time.After(2*time.Second):t.Fatal("Agent connection was not cancelled")
 }
}
