package remoterelay

import (
 "encoding/base64"
 "strings"
 "testing"
 "time"
)
func TestClientKeyPreflightAllowsExactlyOneFollowupLogin(t *testing.T){
 now:=time.Now()
 relay,err:=NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 proof,_:=newAgentProofWithKey(t,now)
 if err:=relay.Register(proof,now);err!=nil{t.Fatal(err)}
 release,err:=relay.Acquire(Claims{DeviceID:proof.DeviceID},now)
 if err!=nil{t.Fatal(err)}
 defer release()
 id:=strings.Repeat("d",32)
 relay.mu.Lock()
 relay.devices[proof.DeviceID].echo=&encryptedEcho{
  probeID:id,seq:relay.devices[proof.DeviceID].sessionSeq,
 }
 relay.mu.Unlock()
 cipher:=base64.StdEncoding.EncodeToString(make([]byte,60))
 result,err:=relay.QueueClientKeyCheck(proof.DeviceID,id,cipher)
 if err!=nil{t.Fatal(err)}
 if _,err:=relay.QueueClientKeyCheck(proof.DeviceID,id,cipher);err==nil{
  t.Fatal("second key-check request was accepted")
 }
 gotID,gotCipher,ok:=relay.TakeEcho(proof.DeviceID,proof.Nonce)
 if !ok||gotID!=id||gotCipher!=cipher||!relay.IsPendingClientKeyCheck(proof.DeviceID,proof.Nonce,id){
  t.Fatal("Agent did not receive key-check envelope")
 }
 if !relay.AckEcho(proof.DeviceID,proof.Nonce,id,cipher){t.Fatal("key check reply rejected")}
 if <-result!=cipher{t.Fatal("key-check ciphertext was altered")}
 login,err:=relay.QueueLogin(proof.DeviceID,id,cipher)
 if err!=nil{t.Fatalf("approved key-check round should permit one login: %v",err)}
 if _,err:=relay.QueueLogin(proof.DeviceID,id,cipher);err==nil{t.Fatal("duplicate login accepted")}
 if _,_,ok:=relay.TakeLogin(proof.DeviceID,proof.Nonce);!ok{t.Fatal("Agent could not receive single login")}
 if !relay.AckLogin(proof.DeviceID,proof.Nonce,id,cipher){t.Fatal("Agent login ack rejected")}
 if <-login!=cipher{t.Fatal("login ciphertext mutated")}
 if _,err:=relay.QueueLogin(proof.DeviceID,id,cipher);err==nil{t.Fatal("finished login restarted")}
}
func TestOrdinaryDiagnosticEchoCannotBeUpgradedToLogin(t *testing.T){
 now:=time.Now()
 relay,_:=NewRegistry("https://relay.example.com")
 proof,_:=newAgentProofWithKey(t,now)
 if err:=relay.Register(proof,now);err!=nil{t.Fatal(err)}
 release,err:=relay.Acquire(Claims{DeviceID:proof.DeviceID},now)
 if err!=nil{t.Fatal(err)}
 defer release()
 id:=strings.Repeat("e",32)
 relay.mu.Lock()
 relay.devices[proof.DeviceID].echo=&encryptedEcho{probeID:id,seq:relay.devices[proof.DeviceID].sessionSeq}
 relay.mu.Unlock()
 cipher:=base64.StdEncoding.EncodeToString(make([]byte,60))
 response,err:=relay.QueueEcho(proof.DeviceID,id,cipher)
 if err!=nil{t.Fatal(err)}
 if _,_,ok:=relay.TakeEcho(proof.DeviceID,proof.Nonce);!ok{t.Fatal("echo missing")}
 if !relay.AckEcho(proof.DeviceID,proof.Nonce,id,cipher){t.Fatal("echo ack failed")}
 <-response
 if _,err:=relay.QueueLogin(proof.DeviceID,id,cipher);err==nil{
  t.Fatal("P0: diagnostic echo converted into privileged password login")
 }
}
