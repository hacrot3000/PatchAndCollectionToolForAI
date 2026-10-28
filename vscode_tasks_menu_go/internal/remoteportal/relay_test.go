package remoteportal

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "encoding/base64"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remoterelay"
)
func TestRelayProofAndSocketHandshake(t *testing.T) {
 pub,_,err:=ed25519.GenerateKey(rand.Reader)
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
 proof,err:=agent.signRelayProof(srv.URL)
 if err!=nil{t.Fatal(err)}
 signed:=remoterelay.DeviceProof{DeviceID:proof.DeviceID,PublicKey:proof.PublicKey,
 Audience:proof.Audience,Timestamp:proof.Timestamp,Nonce:proof.Nonce,Signature:proof.Signature}
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
