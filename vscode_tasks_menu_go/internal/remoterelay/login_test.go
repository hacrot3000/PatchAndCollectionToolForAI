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

func TestRejectedLoginLeavesAgentRegisteredAndCannotCancelNewBrowser(t *testing.T) {
 now:=time.Now()
 r,err:=NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 proof,_:=newAgentProofWithKey(t,now)
 if err:=r.Register(proof,now);err!=nil{t.Fatal(err)}
 oldRelease,err:=r.Acquire(Claims{DeviceID:proof.DeviceID},now)
 if err!=nil{t.Fatal(err)}
 const oldID="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
 r.mu.Lock()
 r.devices[proof.DeviceID].echo=&encryptedEcho{
  probeID:oldID,seq:r.devices[proof.DeviceID].sessionSeq,
 }
 r.mu.Unlock()
 frame:=base64.StdEncoding.EncodeToString(make([]byte,80))
 oldReply,err:=r.QueueLogin(proof.DeviceID,oldID,frame)
 if err!=nil{t.Fatal(err)}
 if _,_,ok:=r.TakeLogin(proof.DeviceID,proof.Nonce);!ok{t.Fatal("Agent did not receive login")}
 if r.RejectLogin(proof.DeviceID,proof.Nonce,"wrong-probe") {
  t.Fatal("invalid Agent rejection consumed active browser")
 }
 if !r.RejectLogin(proof.DeviceID,proof.Nonce,oldID) {
  t.Fatal("valid Agent rejection was not delivered")
 }
 if _,ok:=<-oldReply;ok{t.Fatal("rejected mailbox returned ciphertext")}
 if !r.Online(proof.DeviceID,time.Now()){t.Fatal("rejection unregistered authenticated Agent")}
 oldRelease()

 // The same authenticated Agent can take a new browser reservation.
 newRelease,err:=r.Acquire(Claims{DeviceID:proof.DeviceID},now)
 if err!=nil{t.Fatalf("rejected login left Agent busy: %v",err)}
 defer newRelease()
 const newID="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
 r.mu.Lock()
 r.devices[proof.DeviceID].echo=&encryptedEcho{
  probeID:newID,seq:r.devices[proof.DeviceID].sessionSeq,
 }
 r.mu.Unlock()
 newReply,err:=r.QueueLogin(proof.DeviceID,newID,frame)
 if err!=nil{t.Fatal(err)}
 if _,_,ok:=r.TakeLogin(proof.DeviceID,proof.Nonce);!ok{t.Fatal("new login not delivered")}
 if r.RejectLogin(proof.DeviceID,proof.Nonce,oldID) {
  t.Fatal("late old rejection terminated a new browser session")
 }
 if !r.IsCurrentLogin(proof.DeviceID,proof.Nonce,newID) {
  t.Fatal("late rejection corrupted new login mailbox")
 }
 if !r.AckLogin(proof.DeviceID,proof.Nonce,newID,frame){t.Fatal("new response failed")}
 if got:=<-newReply;got!=frame{t.Fatal("new browser received unexpected ciphertext")}
}
