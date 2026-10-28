package remoterelay

import (
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "strconv"
 "strings"
 "testing"
 "time"
)
func tokenFor(t *testing.T, sk ed25519.PrivateKey, c Claims) string {
 t.Helper()
 raw,err:=json.Marshal(c);if err!=nil{t.Fatal(err)}
 body:=base64.RawURLEncoding.EncodeToString(raw)
 sig:=ed25519.Sign(sk,[]byte(ticketPrefix+body))
 return body+"."+base64.RawURLEncoding.EncodeToString(sig)
}
func TestSignedTicketAndDuplicateRejection(t *testing.T) {
 pk,sk,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 v,err:=NewVerifier(base64.StdEncoding.EncodeToString(pk),"https://portal.example.com")
 if err!=nil{t.Fatal(err)}
 now:=time.Now()
 c:=Claims{V:1,Issuer:"https://portal.example.com",Audience:"taskdeck-relay",
 Subject:"21",DeviceID:strings.Repeat("a",64),Role:"viewer",
 IssuedAt:now.Unix(),Expires:now.Unix()+30,JTI:strings.Repeat("b",32)}
 tok:=tokenFor(t,sk,c)
 got,err:=v.Consume(tok,now)
 if err!=nil||got.Role!="viewer"{t.Fatalf("valid ticket rejected: %v",err)}
 if _,err:=v.Consume(tok,now);err==nil{t.Fatal("duplicate accepted")}
 if _,err:=v.Consume(tok+"x",now);err==nil{t.Fatal("modified ticket accepted")}
 c.JTI=strings.Repeat("c",32);c.Role="root"
 if _,err:=v.Consume(tokenFor(t,sk,c),now);err==nil{t.Fatal("invalid role accepted")}
 c.Role="viewer";c.Expires=now.Unix()-1
 if _,err:=v.Consume(tokenFor(t,sk,c),now);err==nil{t.Fatal("expired ticket accepted")}
}
func TestRelayDeviceProof(t *testing.T) {
 pk,sk,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 now:=time.Now();digest:=sha256.Sum256(pk)
 p:=DeviceProof{DeviceID:hex.EncodeToString(digest[:]),
 PublicKey:base64.StdEncoding.EncodeToString(pk),
 Audience:"https://relay.example.com",Challenge:base64.RawURLEncoding.EncodeToString(make([]byte,32)),Timestamp:now.Unix(),Nonce:strings.Repeat("a",32)}
 wire:=strings.Join([]string{DeviceChallengeProtocol,"connect",p.DeviceID,p.Audience,strconv.FormatInt(p.Timestamp,10),p.Nonce,p.Challenge},string([]byte{10}))
 p.Signature=base64.StdEncoding.EncodeToString(ed25519.Sign(sk,[]byte(wire)))
 if err:=VerifyDeviceProof(p,p.Audience,now);err!=nil{t.Fatal(err)}
 changed:=p;changed.Challenge=base64.RawURLEncoding.EncodeToString(make([]byte,33))
 if err:=VerifyDeviceProof(changed,p.Audience,now);err==nil{t.Fatal("wrong challenge length accepted")}
 changed=p;changed.Challenge=base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a",32)))
 if err:=VerifyDeviceProof(changed,p.Audience,now);err==nil{t.Fatal("different server challenge accepted same signature")}
 p.DeviceID=strings.Repeat("b",64)
 if err:=VerifyDeviceProof(p,p.Audience,now);err==nil{t.Fatal("incorrect fingerprint accepted")}
}
