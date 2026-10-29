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

// Even a stolen Portal signing seed MUST NOT let PHP register an Agent
// under an existing TaskDeck Device ID. The Relay additionally verifies
// the device-owned, challenge-bound Ed25519 key, not the Portal lease alone.
func TestCompromisedPortalSignerCannotImpersonateTaskDeckAgentOverWSS(t *testing.T) {
 portalPublic,stolenPortalSeed,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 v,err:=NewVerifier(base64.StdEncoding.EncodeToString(portalPublic),"https://portal.example.com")
 if err!=nil{t.Fatal(err)}
 registry,err:=NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 srv:=httptest.NewTLSServer(&Handler{Tickets:v,Devices:registry})
 defer srv.Close()
 realPublic,_,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 deviceFingerprint:=sha256.Sum256(realPublic)
 realDeviceID:=hex.EncodeToString(deviceFingerprint[:])
 _,attackerPrivate,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 attackerPublic:=attackerPrivate.Public().(ed25519.PublicKey)
 ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
 defer cancel()
 ws,_,err:=websocket.Dial(ctx,srv.URL+"/v1/device",
  &websocket.DialOptions{HTTPClient:srv.Client()})
 if err!=nil{t.Fatal(err)}
 defer ws.CloseNow()
 _,first,err:=ws.Read(ctx)
 if err!=nil{t.Fatal(err)}
 var challenge struct{
  Challenge string `json:"challenge"`
  Protocol string `json:"protocol"`
 }
 if err:=json.Unmarshal(first,&challenge);err!=nil || challenge.Protocol!=DeviceChallengeProtocol {
  t.Fatalf("invalid Relay challenge: %v",err)
 }
 now:=time.Now()
 forgedLease:=makeLease(t,stolenPortalSeed,DeviceLease{
  V:1,Iss:"https://portal.example.com",Aud:"taskdeck-relay-device",
  DeviceID:realDeviceID,Iat:now.Unix(),Exp:now.Unix()+70,
 })
 forged:=DeviceProof{
  DeviceID:realDeviceID,PublicKey:base64.StdEncoding.EncodeToString(attackerPublic),
  Audience:"https://relay.example.com",Challenge:challenge.Challenge,
  DeviceLease:forgedLease,Timestamp:now.Unix(),
  Nonce:strings.Repeat("d",32),
 }
 message:=strings.Join([]string{
  DeviceChallengeProtocol,"connect",forged.DeviceID,forged.Audience,
  strconv.FormatInt(forged.Timestamp,10),forged.Nonce,forged.Challenge,
 },"
")
 forged.Signature=base64.StdEncoding.EncodeToString(ed25519.Sign(attackerPrivate,[]byte(message)))
 sent,err:=json.Marshal(forged)
 if err!=nil{t.Fatal(err)}
 if err:=ws.Write(ctx,websocket.MessageText,sent);err!=nil{t.Fatal(err)}
 if _,message,err:=ws.Read(ctx);err==nil {
  t.Fatalf("P0: stolen PHP signing key registered fake TaskDeck Agent: %s",message)
 }
 if registry.Online(realDeviceID,time.Now()) {
  t.Fatal("compromised PHP Portal impersonated real TaskDeck signing identity")
 }
}
