package remotetransport

import (
 "bytes"
 "testing"
)
func TestEncryptedClientKeyStatusBeforePasswordAndStrictReplayProtection(t *testing.T){
 browser,agent,_:=participants(t)
 frame,err:=ClientKeyCheckFrame(browser,ClientKeyCheck{
  Username:"alice",PublicKey:"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
  Signature:"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="})
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(frame,[]byte("alice")){t.Fatal("Relay saw preflight username")}
 req,err:=OpenClientKeyCheck(agent,frame)
 if err!=nil||req.Username!="alice"{t.Fatalf("Agent could not decrypt key check: %v",err)}
 status,err:=SealClientKeyStatus(agent,false)
 if err!=nil{t.Fatal(err)}
 approved,err:=OpenClientKeyStatus(browser,status)
 if err!=nil||approved{t.Fatalf("unapproved key status was not authenticated: %v",err)}
 if _,err:=OpenClientKeyCheck(agent,frame);err==nil{t.Fatal("replayed client key proof accepted")}
}
func TestClientKeyCheckRejectsMalleableFieldsBeforeLocalAuthorization(t *testing.T){
 for _,plain:=range []string{
  `{"type":"taskdeck-e2ee-client-key-check-v1","username":"alice","public_key":"A","signature":"B","username":"bob"}`,
  `{"Type":"taskdeck-e2ee-client-key-check-v1","username":"alice","public_key":"A","signature":"B"}`,
  `{"type":"taskdeck-e2ee-client-key-check-v1","username":"alice","public_key":"A","signature":"B","admin":true}`,
  `{"type":"terminal.run","username":"alice","public_key":"A","signature":"B"}`,
 }{
  browser,agent,_:=participants(t)
  frame,err:=browser.Seal([]byte(plain));if err!=nil{t.Fatal(err)}
  if _,err:=OpenClientKeyCheck(agent,frame);err==nil{t.Fatalf("untrusted key-check JSON accepted: %s",plain)}
  if agent.Established(){t.Fatal("invalid E2EE key-check did not close channel")}
 }
}
func TestEncryptedV2LoginRequiresCompleteClientFields(t *testing.T){
 browser,agent,_:=participants(t)
 frame,err:=RemoteLoginRequestWithClientKey(browser,"alice","local password",
  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==")
 if err!=nil{t.Fatal(err)}
 request,err:=OpenRemoteLogin(agent,frame)
 if err!=nil||request.ClientPublicKey==""||request.ClientSignature==""{
  t.Fatalf("keyed login failed to retain bound fields: %v",err)
 }
}
