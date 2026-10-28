package remoterelay

import (
 "crypto/ed25519"
 "encoding/base64"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestSingleUseOpaqueEncryptedEchoMailbox(t *testing.T) {
 now:=time.Now()
 r,err:=NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 proof,key:=newAgentProofWithKey(t,now)
 if err:=r.Register(proof,now);err!=nil{t.Fatal(err)}
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 release,err:=r.Acquire(Claims{DeviceID:proof.DeviceID},now)
 if err!=nil{t.Fatal(err)}
 defer release()
 pending,err:=r.BeginProbe(proof.DeviceID,strings.Repeat("a",32),
  base64.StdEncoding.EncodeToString(browser.Public))
 if err!=nil{t.Fatal(err)}
 request,ok:=r.TakeProbe(proof.DeviceID,proof.Nonce)
 if !ok{t.Fatal("missing authenticated probe")}
 hello,agentChannel,err:=remotetransport.ClientHello(
  key,proof.DeviceID,request.ProbeID,request.ClientNonce,browser.Public)
 if err!=nil{t.Fatal(err)}
 ready,err:=agentChannel.Seal([]byte("taskdeck-e2ee-ready-v1"))
 if err!=nil{t.Fatal(err)}
 ack:=signedProbeAttestation(key,proof.DeviceID,request)
 ack.Crypto=&ProbeCrypto{
  Identity:base64.StdEncoding.EncodeToString(hello.Identity),
  Ephemeral:base64.StdEncoding.EncodeToString(hello.Ephemeral),
  Signature:base64.StdEncoding.EncodeToString(hello.Signature),
  EncryptedReady:base64.StdEncoding.EncodeToString(ready),
 }
 if !r.HasActiveProbe(proof.DeviceID,proof.Nonce){t.Fatal("probe should be pending")}
 if !r.AckProbe(proof.DeviceID,proof.Nonce,ack){t.Fatal("encrypted proof rejected")}
 if !((<-pending).OK){t.Fatal("proof unsuccessful")}
 if r.HasActiveProbe(proof.DeviceID,proof.Nonce){t.Fatal("proof left pending")}
 if !r.HasActiveEcho(proof.DeviceID,proof.Nonce){t.Fatal("one-shot echo not allocated")}
 if _,err:=r.QueueEcho(proof.DeviceID,ack.ProbeID,"not a valid frame");err==nil{
  t.Fatal("invalid ciphertext envelope allowed")
 }
 clientChannel,err:=remotetransport.AcceptHello(browser,proof.DeviceID,
  request.ProbeID,request.ClientNonce,hello)
 if err!=nil{t.Fatal(err)}
 clear,err:=clientChannel.Open(ready)
 if err!=nil || string(clear)!="taskdeck-e2ee-ready-v1"{t.Fatal("encrypted ready failed")}
 for round:=0;round<MaxDiagnosticRounds;round++ {
  nonce:=strings.Repeat(string(rune('a'+round)),32)
  encrypted,err:=clientChannel.Seal([]byte("taskdeck-e2ee-echo-v1:"+nonce))
  if err!=nil{t.Fatal(err)}
  encoded:=base64.StdEncoding.EncodeToString(encrypted)
  reply,err:=r.QueueEcho(proof.DeviceID,ack.ProbeID,encoded)
  if err!=nil{t.Fatal(err)}
  if _,err:=r.QueueEcho(proof.DeviceID,ack.ProbeID,encoded);err==nil{
   t.Fatal("second request pipelined before first reply")
  }
  if _,_,ok:=r.TakeEcho(proof.DeviceID,"forged-connection");ok{
   t.Fatal("foreign Agent consumed encrypted mailbox")
  }
  id,got,ok:=r.TakeEcho(proof.DeviceID,proof.Nonce)
  if !ok || id!=ack.ProbeID || got!=encoded{t.Fatal("valid Agent failed to receive ciphertext")}
  if _,_,ok:=r.TakeEcho(proof.DeviceID,proof.Nonce);ok{t.Fatal("encrypted mailbox redelivered")}
  answer,err:=agentChannel.Open(encrypted)
  if err!=nil || string(answer)!="taskdeck-e2ee-echo-v1:"+nonce{t.Fatal("decrypt mismatch")}
  encryptedReply,err:=agentChannel.Seal([]byte("taskdeck-e2ee-echo-reply-v1:"+nonce))
  if err!=nil{t.Fatal(err)}
  ciphertext:=base64.StdEncoding.EncodeToString(encryptedReply)
  if r.AckEcho(proof.DeviceID,proof.Nonce,"wrong-id",ciphertext){t.Fatal("wrong probe accepted")}
  if !r.AckEcho(proof.DeviceID,proof.Nonce,ack.ProbeID,ciphertext){t.Fatal("encrypted answer rejected")}
  if returned:=<-reply;returned!=ciphertext{t.Fatal("mailbox returned incorrect ciphertext")}
  if r.AckEcho(proof.DeviceID,proof.Nonce,ack.ProbeID,ciphertext){t.Fatal("duplicate echo response accepted")}
  roundtrip,err:=clientChannel.Open(encryptedReply)
  if err!=nil || string(roundtrip)!="taskdeck-e2ee-echo-reply-v1:"+nonce {
   t.Fatalf("round %d invalid encrypted response: %v",round,err)
  }
 }
 if r.HasActiveEcho(proof.DeviceID,proof.Nonce){t.Fatal("echo leaked after maximum rounds")}
 if _,err:=r.QueueEcho(proof.DeviceID,ack.ProbeID,base64.StdEncoding.EncodeToString(make([]byte,25)));err==nil{
  t.Fatal("echo reopened after maximum rounds")
 }
}

func TestEchoIsCanceledOnBrowserOrAgentDisconnect(t *testing.T){
 now:=time.Now()
 r,err:=NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 p,key:=newAgentProofWithKey(t,now)
 if err:=r.Register(p,now);err!=nil{t.Fatal(err)}
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 release,err:=r.Acquire(Claims{DeviceID:p.DeviceID},now)
 if err!=nil{t.Fatal(err)}
 _,err=r.BeginProbe(p.DeviceID,strings.Repeat("d",32),
  base64.StdEncoding.EncodeToString(browser.Public))
 if err!=nil{t.Fatal(err)}
 request,ok:=r.TakeProbe(p.DeviceID,p.Nonce)
 if !ok{t.Fatal("probe missing")}
 hello,channel,err:=remotetransport.ClientHello(key,p.DeviceID,
  request.ProbeID,request.ClientNonce,browser.Public)
 if err!=nil{t.Fatal(err)}
 ready,err:=channel.Seal([]byte("taskdeck-e2ee-ready-v1"))
 if err!=nil{t.Fatal(err)}
 ack:=signedProbeAttestation(key,p.DeviceID,request)
 ack.Crypto=&ProbeCrypto{
  Identity:base64.StdEncoding.EncodeToString(hello.Identity),
  Ephemeral:base64.StdEncoding.EncodeToString(hello.Ephemeral),
  Signature:base64.StdEncoding.EncodeToString(hello.Signature),
  EncryptedReady:base64.StdEncoding.EncodeToString(ready),
 }
 if !r.AckProbe(p.DeviceID,p.Nonce,ack){t.Fatal("valid proof rejected")}
 ciphertext:=base64.StdEncoding.EncodeToString(make([]byte,25))
 pending,err:=r.QueueEcho(p.DeviceID,request.ProbeID,ciphertext)
 if err!=nil{t.Fatal(err)}
 release()
 if _,ok:=<-pending;ok{t.Fatal("abandoned E2EE echo was not canceled")}
 if r.HasActiveEcho(p.DeviceID,p.Nonce){t.Fatal("stale browser echo remained active")}
 if r.HasActiveProbe(p.DeviceID,p.Nonce){t.Fatal("stale probe remained active")}
}
var _ ed25519.PrivateKey // do not add an unauthenticated Agent shortcut
