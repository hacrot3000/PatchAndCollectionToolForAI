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
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)
func withBrowserBinding(t *testing.T,c Claims,nonce string,public []byte) Claims {
 t.Helper()
 binding,err:=remotetransport.BrowserTicketBinding(nonce,public)
 if err!=nil{t.Fatal(err)}
 c.BrowserBinding=binding
 return c
}

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
 c:=Claims{V:2,BrowserBinding:strings.Repeat("f",64),Issuer:"https://portal.example.com",Audience:"taskdeck-relay",
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

func TestPortalPHPGoBrowserKeyBindingVector(t *testing.T) {
 const publicHex="04"+"6b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c296"+
 "4fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f5"
 public,err:=hex.DecodeString(publicHex)
 if err!=nil{t.Fatal(err)}
 got,err:=remotetransport.BrowserTicketBinding(strings.Repeat("a",32),public)
 if err!=nil{t.Fatal(err)}
 const want="2a85833552e8544f7231ff2f66cccd38559d668b046128fb5dd8022a9b84b31b"
 if got!=want{t.Fatalf("Go Portal-binding vector differs from PHP: %s",got)}
}

func TestPortalV2TicketRejectsECDHSubstitutionAndLegacyV1(t *testing.T) {
 pk,sk,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 verifier,err:=NewVerifier(base64.StdEncoding.EncodeToString(pk),"https://portal.example.com")
 if err!=nil{t.Fatal(err)}
 first,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 other,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 nonce:=strings.Repeat("a",32)
 binding,err:=remotetransport.BrowserTicketBinding(nonce,first.Public)
 if err!=nil{t.Fatal(err)}
 now:=time.Now()
 claims:=Claims{V:2,BrowserBinding:binding,Issuer:"https://portal.example.com",
  Audience:"taskdeck-relay",Subject:"21",DeviceID:strings.Repeat("e",64),
  Role:"viewer",IssuedAt:now.Unix(),Expires:now.Unix()+30,
  JTI:strings.Repeat("d",32)}
 if err:=VerifyBrowserBinding(claims,nonce,base64.StdEncoding.EncodeToString(first.Public));err!=nil {
  t.Fatal(err)
 }
 if err:=VerifyBrowserBinding(claims,nonce,base64.StdEncoding.EncodeToString(other.Public));err==nil {
  t.Fatal("signed Portal ticket allowed substituted ECDH public key")
 }
 if err:=VerifyBrowserBinding(claims,strings.Repeat("b",32),
  base64.StdEncoding.EncodeToString(first.Public));err==nil {
  t.Fatal("signed Portal ticket allowed substituted browser nonce")
 }
 if err:=VerifyBrowserBinding(claims,nonce,
  base64.StdEncoding.EncodeToString(make([]byte,65)));err==nil {
  t.Fatal("invalid P-256 point accepted")
 }
 token:=tokenFor(t,sk,claims)
 verified,err:=verifier.Consume(token,now)
 if err!=nil{t.Fatal(err)}
 if err:=VerifyBrowserBinding(verified,nonce,base64.StdEncoding.EncodeToString(first.Public));err!=nil{
  t.Fatal(err)
 }
 claims.V=1
 claims.JTI=strings.Repeat("c",32)
 if _,err:=verifier.Consume(tokenFor(t,sk,claims),now);err==nil {
  t.Fatal("unbound legacy Portal v1 ticket accepted")
 }
 claims.V=2
 claims.BrowserBinding=""
 claims.JTI=strings.Repeat("b",32)
 if _,err:=verifier.Consume(tokenFor(t,sk,claims),now);err==nil {
  t.Fatal("Portal v2 ticket without browser ECDH commitment accepted")
 }
}
