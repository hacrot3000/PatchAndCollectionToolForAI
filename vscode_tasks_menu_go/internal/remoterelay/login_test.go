package remoterelay

import (
 "encoding/base64"
 "strings"
 "testing"
 "time"
)

func TestEncryptedLoginMailboxOneShotAndSessionIsolation(t *testing.T) {
 now:=time.Now()
 r,err:=NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 proof,_:=newAgentProofWithKey(t,now)
 if err:=r.Register(proof,now);err!=nil{t.Fatal(err)}
 release,err:=r.Acquire(Claims{DeviceID:proof.DeviceID},now)
 if err!=nil{t.Fatal(err)}
 // Install a fresh browser-attested session, with no echo frames sent.
 r.mu.Lock()
 state:=r.devices[proof.DeviceID]
 state.echo=&encryptedEcho{probeID:strings.Repeat("c",32),seq:state.sessionSeq,reply:make(chan string,1)}
 r.mu.Unlock()
 cipher:=base64.StdEncoding.EncodeToString(make([]byte,60))
 pending,err:=r.QueueLogin(proof.DeviceID,strings.Repeat("c",32),cipher)
 if err!=nil{t.Fatal(err)}
 if _,err:=r.QueueLogin(proof.DeviceID,strings.Repeat("c",32),cipher);err==nil{
  t.Fatal("second TaskDeck login accepted on one Portal grant")
 }
 if _,err:=r.QueueEcho(proof.DeviceID,strings.Repeat("c",32),cipher);err==nil{
  t.Fatal("diagnostic echo mixed with in-flight TaskDeck login")
 }
 if _,_,ok:=r.TakeLogin(proof.DeviceID,"wrong-agent-connection");ok{
  t.Fatal("foreign Agent consumed encrypted login")
 }
 id,data,ok:=r.TakeLogin(proof.DeviceID,proof.Nonce)
 if !ok || id!=strings.Repeat("c",32) || data!=cipher{
  t.Fatal("Agent could not retrieve opaque login")
 }
 if _,_,ok:=r.TakeLogin(proof.DeviceID,proof.Nonce);ok{
  t.Fatal("same login delivered twice to Agent")
 }
 if r.AckLogin(proof.DeviceID,proof.Nonce,"wrong-probe",cipher){
  t.Fatal("Agent reply with wrong probe was accepted")
 }
 if !r.AckLogin(proof.DeviceID,proof.Nonce,id,cipher){t.Fatal("valid login result not delivered")}
 if returned:=<-pending;returned!=cipher{t.Fatal("Relay changed encrypted reply")}
 if r.IsCurrentLogin(proof.DeviceID,proof.Nonce,id){t.Fatal("login state remained active")}
 if _,err:=r.QueueLogin(proof.DeviceID,id,cipher);err==nil{
  t.Fatal("completed login was reopened")
 }
 release()
 if r.Online(proof.DeviceID,now) == false {t.Fatal("browser cleanup removed authenticated Agent")}
}

func TestEncryptedLoginCancellationClearsPendingCiphertext(t *testing.T) {
 now:=time.Now()
 r,_:=NewRegistry("https://relay.example.com")
 proof,_:=newAgentProofWithKey(t,now)
 if err:=r.Register(proof,now);err!=nil{t.Fatal(err)}
 release,err:=r.Acquire(Claims{DeviceID:proof.DeviceID},now)
 if err!=nil{t.Fatal(err)}
 r.mu.Lock()
 state:=r.devices[proof.DeviceID]
 state.echo=&encryptedEcho{probeID:strings.Repeat("d",32),seq:state.sessionSeq}
 r.mu.Unlock()
 payload:=base64.StdEncoding.EncodeToString(make([]byte,40))
 pending,err:=r.QueueLogin(proof.DeviceID,strings.Repeat("d",32),payload)
 if err!=nil{t.Fatal(err)}
 release()
 if _,valid:=<-pending;valid{t.Fatal("cancelled login remained usable")}
 if _,_,ok:=r.TakeLogin(proof.DeviceID,proof.Nonce);ok{
  t.Fatal("cancelled login delivered after browser disconnect")
 }
 if r.AckLogin(proof.DeviceID,proof.Nonce,strings.Repeat("d",32),payload){
  t.Fatal("Agent completed login after browser cancellation")
 }
}
