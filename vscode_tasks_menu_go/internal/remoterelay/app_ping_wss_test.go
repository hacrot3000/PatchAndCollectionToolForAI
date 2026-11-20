package remoterelay

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "net/http/httptest"
 "strconv"
 "strings"
 "testing"
 "time"

 "github.com/coder/websocket"
)

// This is the REAL TLS/WSS Agent-side relay, not an in-memory method test.
// It intentionally does not assert a TaskDeck application login exists yet:
// there is still no Browser→Agent app-login/authorization handler exposed.
type appWSSAgent struct {
 socket *websocket.Conn
 ctx context.Context
 registry *Registry
 deviceID,deviceNonce,lease,probeID string
 public ed25519.PublicKey
 private ed25519.PrivateKey
 portalPrivate ed25519.PrivateKey
 release func()
}
func openAppWSSAgent(t *testing.T) *appWSSAgent {
 t.Helper()
 portalPublic,portalPrivate,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 verify,err:=NewVerifier(base64.StdEncoding.EncodeToString(portalPublic),
  "https://portal.example.com")
 if err!=nil{t.Fatal(err)}
 registry,err:=NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 host:=httptest.NewTLSServer(&Handler{Tickets:verify,Devices:registry})
 t.Cleanup(host.Close)
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second)
 t.Cleanup(cancel)
 socket,_,err:=websocket.Dial(ctx,host.URL+"/v1/device",
  &websocket.DialOptions{HTTPClient:host.Client()})
 if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){socket.CloseNow()})
 kind,challengeRaw,err:=socket.Read(ctx)
 var challenge struct{
  Protocol string `json:"protocol"`
  Challenge string `json:"challenge"`
 }
 if err!=nil || kind!=websocket.MessageText ||
  json.Unmarshal(challengeRaw,&challenge)!=nil ||
  challenge.Protocol!=DeviceChallengeProtocol{
  t.Fatalf("invalid WSS challenge: %q %v",challengeRaw,err)
 }
 public,private,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256(public)
 id:=hex.EncodeToString(sum[:])
 now:=time.Now()
 lease:=makeLease(t,portalPrivate,DeviceLease{
  V:1,Iss:"https://portal.example.com",Aud:"taskdeck-relay-device",
  DeviceID:id,Iat:now.Unix(),Exp:now.Unix()+70,
 })
 proof:=DeviceProof{
  DeviceID:id,PublicKey:base64.StdEncoding.EncodeToString(public),
  Audience:"https://relay.example.com",Challenge:challenge.Challenge,
  DeviceLease:lease,Timestamp:now.Unix(),Nonce:strings.Repeat("a",32),
 }
 proofWire:=strings.Join([]string{DeviceChallengeProtocol,"connect",id,proof.Audience,
  strconv.FormatInt(proof.Timestamp,10),proof.Nonce,proof.Challenge},"\n")
 proof.Signature=base64.StdEncoding.EncodeToString(ed25519.Sign(private,[]byte(proofWire)))
 packed,_:=json.Marshal(proof)
 if err:=socket.Write(ctx,websocket.MessageText,packed);err!=nil{t.Fatal(err)}
 kind,result,err:=socket.Read(ctx)
 if err!=nil || kind!=websocket.MessageText || string(result)!=`{"registered":true}`{
  t.Fatalf("Agent WSS registration failed: %q %v",result,err)
 }
 release,err:=registry.Acquire(Claims{DeviceID:id},now)
 if err!=nil{t.Fatal(err)}
 t.Cleanup(release)
 const probeID="cccccccccccccccccccccccccccccccc"
 registry.mu.Lock()
 registry.devices[id].lastLoginProbeID=probeID
 registry.mu.Unlock()
 return &appWSSAgent{socket:socket,ctx:ctx,registry:registry,
  deviceID:id,deviceNonce:proof.Nonce,lease:lease,probeID:probeID,
  public:public,private:private,portalPrivate:portalPrivate,release:release}
}
func (a *appWSSAgent) send(t *testing.T,fields map[string]string) ([]byte,error){
 t.Helper()
 fields["device_lease"]=a.lease
 payload,err:=json.Marshal(fields)
 if err!=nil{t.Fatal(err)}
 if err:=a.socket.Write(a.ctx,websocket.MessageText,payload);err!=nil{return nil,err}
 typ,reply,err:=a.socket.Read(a.ctx)
 if err!=nil{return nil,err}
 if typ!=websocket.MessageText{t.Fatal("Agent reply was not JSON text")}
 return reply,nil
}

func TestOpaqueAppPingTraversesActualAgentWSSWithoutPlaintext(t *testing.T){
 a:=openAppWSSAgent(t)
 signed:=base64.StdEncoding.EncodeToString(ed25519.Sign(a.private,
  appReadyWire(a.deviceID,a.deviceNonce,a.probeID)))
 reply,err:=a.send(t,map[string]string{
  "type":"app_ready","probe_id":a.probeID,
  "public_key":base64.StdEncoding.EncodeToString(a.public),
  "signature":signed,
 })
 if err!=nil || string(reply)!="pong"{t.Fatalf("signed Agent readiness rejected: %s %v",reply,err)}
 // The mailbox transports an opaque ciphertext-shaped record; it does not
 // parse or execute a path, command, HTTP request or TaskDeck principal.
 opaque:=base64.StdEncoding.EncodeToString(make([]byte,90))
 relayID,waiting,err:=a.registry.QueueAppPing(a.deviceID,a.probeID,opaque)
 if err!=nil{t.Fatal(err)}
 reply,err=a.send(t,map[string]string{"type":"ping"})
 if err!=nil{t.Fatal(err)}
 var got map[string]string
 if err:=json.Unmarshal(reply,&got);err!=nil{t.Fatal(err)}
 if len(got)!=4 || got["type"]!="app_ping" || got["probe_id"]!=a.probeID ||
  got["relay_id"]!=relayID || got["ciphertext"]!=opaque{
  t.Fatalf("Relay changed opaque app WSS request: %s",reply)
 }
 for _,secretField:=range []string{"password","command","path","principal","token","permission"}{
  if _,found:=got[secretField];found{t.Fatal("Relay injected TaskDeck application data")}
 }
 returned,err:=a.send(t,map[string]string{
  "type":"app_ping_ack","probe_id":a.probeID,
  "relay_id":relayID,"ciphertext":opaque,
 })
 if err!=nil || string(returned)!="pong"{t.Fatalf("Agent ACK not bound: %s %v",returned,err)}
 select{
 case response,ok:=<-waiting:
  if !ok || response!=opaque{t.Fatal("opaque response changed in Relay")}
 case <-time.After(2*time.Second):t.Fatal("WSS app response deadlocked")
 }
 if a.registry.AckAppPing(a.deviceID,a.deviceNonce,a.probeID,relayID,opaque){
  t.Fatal("replayed Agent app ACK was accepted")
 }
}

func TestCompromisedPHPSignerCannotOpenAgentWSSApplicationMailbox(t *testing.T){
 a:=openAppWSSAgent(t)
 // Attacker knows the ENTIRE PHP Ed25519 signer and its device lease;
 // it does not know the host Agent Ed25519 private key.
 forged:=base64.StdEncoding.EncodeToString(ed25519.Sign(a.portalPrivate,
  appReadyWire(a.deviceID,a.deviceNonce,a.probeID)))
 if _,err:=a.send(t,map[string]string{
  "type":"app_ready","probe_id":a.probeID,
  "public_key":base64.StdEncoding.EncodeToString(a.public),
  "signature":forged,
 });err==nil{
  t.Fatal("PHP signature unexpectedly authenticated host Agent app-ready")
 }
 if _,_,err:=a.registry.QueueAppPing(a.deviceID,a.probeID,
  base64.StdEncoding.EncodeToString(make([]byte,90)));err==nil{
  t.Fatal("compromised PHP opened an app mailbox without Agent private key")
 }
}
