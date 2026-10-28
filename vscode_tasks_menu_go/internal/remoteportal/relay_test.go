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
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remoterelay"
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
 agent,err:=NewAgent(srv.URL,filepath.Join(t.TempDir(),"private","key.seed"),srv.Client())
 if err!=nil{t.Fatal(err)}
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
 ctx,cancel:=context.WithTimeout(context.Background(),220*time.Millisecond)
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
 cancel()
 select {case err:=<-finished:if err!=nil{t.Fatal(err)}
 case <-time.After(2*time.Second):t.Fatal("Agent failed to disconnect")}
}
