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

// Relay may accept a signed Browser ticket that Agent (correctly) rejects
// due to an independently pinned, different Portal public key. Such a
// Browser failure must NOT tear down the already-authenticated device WSS.
func TestRejectedPortalGrantKeepsLiveAgentConnection(t *testing.T) {
 portalPub,portalPrivate,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 wrongPub,_,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 tickets,err:=remoterelay.NewVerifier(
  base64.StdEncoding.EncodeToString(portalPub),"https://portal.example.com")
 if err!=nil{t.Fatal(err)}
 var relay *remoterelay.Handler
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  relay.ServeHTTP(w,r)
 }))
 defer server.Close()
 registry,err:=remoterelay.NewRegistry(server.URL)
 if err!=nil{t.Fatal(err)}
 relay=&remoterelay.Handler{Tickets:tickets,Devices:registry}
 agent,err:=NewAgent("https://portal.example.com",
  filepath.Join(t.TempDir(),"identity","key.seed"),server.Client())
 if err!=nil{t.Fatal(err)}
 if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(wrongPub));err!=nil{
  t.Fatal(err)
 }
 now:=time.Now().Unix()
 lease:=remoterelay.DeviceLease{V:1,Iss:"https://portal.example.com",
  Aud:"taskdeck-relay-device",DeviceID:agent.DeviceID(),Iat:now,Exp:now+70}
 encoded,err:=json.Marshal(lease)
 if err!=nil{t.Fatal(err)}
 body:=base64.RawURLEncoding.EncodeToString(encoded)
 signature:=ed25519.Sign(portalPrivate,[]byte("taskdeck-relay-device-lease-v1."+body))
 agent.leaseMu.Lock()
 agent.deviceLease=body+"."+base64.RawURLEncoding.EncodeToString(signature)
 agent.leaseMu.Unlock()

 ctx,cancel:=context.WithTimeout(context.Background(),8*time.Second)
 defer cancel()
 finished:=make(chan error,1)
 go func(){finished<-agent.connectRelay(ctx,server.URL,25*time.Millisecond)}()
 readyDeadline:=time.After(2*time.Second)
 for !registry.Online(agent.DeviceID(),time.Now()){
  select{
  case <-readyDeadline:t.Fatal("Agent did not register before rejected Browser grant")
  case <-time.After(10*time.Millisecond):
  }
 }
 headers:=http.Header{}
 headers.Set("Origin","https://portal.example.com")
 for i:=0;i<2;i++ {
  browserKey,err:=remotetransport.NewEphemeral()
  if err!=nil{t.Fatal(err)}
  nonce:=strings.Repeat(string(rune('a'+i)),32)
  binding,err:=remotetransport.BrowserTicketBinding(nonce,browserKey.Public)
  if err!=nil{t.Fatal(err)}
  claims:=remoterelay.Claims{AuthEpoch:1,V:2,Issuer:"https://portal.example.com",
   Audience:"taskdeck-relay",DeviceID:agent.DeviceID(),Role:"viewer",
   Subject:"21",IssuedAt:time.Now().Unix(),Expires:time.Now().Unix()+30,
   JTI:strings.Repeat(string(rune('c'+i)),32),BrowserBinding:binding,
  }
  ticket:=signPortalTicket(t,portalPrivate,claims)
  ws,_,err:=websocket.Dial(ctx,server.URL+"/v1/browser",&websocket.DialOptions{
   HTTPClient:server.Client(),HTTPHeader:headers,
  })
  if err!=nil{t.Fatal(err)}
  offer,_:=json.Marshal(map[string]string{
   "ticket":ticket,"client_nonce":nonce,
   "browser_key":base64.StdEncoding.EncodeToString(browserKey.Public),
  })
  if err=ws.Write(ctx,websocket.MessageText,offer);err!=nil{ws.CloseNow();t.Fatal(err)}
  typ,response,err:=ws.Read(ctx)
  ws.CloseNow()
  if err!=nil || typ!=websocket.MessageText{
   t.Fatalf("Browser %d rejection did not complete promptly: %v",i,err)
  }
  var verdict struct {
   TicketValid bool `json:"ticket_valid"`
   AgentReachable bool `json:"agent_reachable"`
   RemoteControlAvailable bool `json:"remote_control_available"`
  }
  if json.Unmarshal(response,&verdict)!=nil || !verdict.TicketValid ||
   verdict.AgentReachable || verdict.RemoteControlAvailable {
   t.Fatalf("Browser %d incorrectly trusted Agent with wrong pinned Portal key: %s",i,response)
  }
  select {
  case err:=<-finished:
   t.Fatalf("rejected browser %d disconnected authenticated Agent: %v",i,err)
  default:
  }
  // Ensure Browser handler's release has completed before the next grant.
  deadline:=time.After(2*time.Second)
  for {
   release,err:=registry.Acquire(remoterelay.Claims{DeviceID:agent.DeviceID()},time.Now())
   if err==nil {release();break}
   select {
   case <-deadline:t.Fatal("Portal grant rejection left Agent busy")
   case <-time.After(10*time.Millisecond):
   }
  }
 }
 cancel()
 select {
 case err:=<-finished:
  if err!=nil{t.Fatalf("Agent shutdown after denied browser grants: %v",err)}
 case <-time.After(2*time.Second):t.Fatal("Agent failed to shut down")
 }
}
