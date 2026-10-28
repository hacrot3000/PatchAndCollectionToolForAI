package remoterelay

import (
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "strconv"
 "strings"
 "testing"
 "time"
)
func newAgentProof(t *testing.T,now time.Time) DeviceProof {
 t.Helper()
 pub,priv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256(pub)
 p:=DeviceProof{DeviceID:hex.EncodeToString(sum[:]),PublicKey:base64.StdEncoding.EncodeToString(pub),
 Audience:"https://relay.example.com",Challenge:base64.RawURLEncoding.EncodeToString(make([]byte,32)),Timestamp:now.Unix(),Nonce:strings.Repeat("a",32)}
 raw:=strings.Join([]string{DeviceChallengeProtocol,"connect",p.DeviceID,p.Audience,
 strconv.FormatInt(p.Timestamp,10),p.Nonce,p.Challenge},string([]byte{10}))
 p.Signature=base64.StdEncoding.EncodeToString(ed25519.Sign(priv,[]byte(raw)))
 return p
}
func TestRegistryRequiresProofAndIsolatesSessions(t *testing.T) {
 now:=time.Now()
 r,err:=NewRegistry("https://relay.example.com");if err!=nil{t.Fatal(err)}
 p:=newAgentProof(t,now)
 if err:=r.Register(p,now);err!=nil{t.Fatal(err)}
 if !r.Online(p.DeviceID,now){t.Fatal("registered device unavailable")}
 if err:=r.Register(p,now);err==nil{t.Fatal("duplicate device accepted")}
 c:=Claims{DeviceID:p.DeviceID}
 release,err:=r.Acquire(c,now);if err!=nil{t.Fatal(err)}
 if _,err:=r.Acquire(c,now);err==nil{t.Fatal("busy device accepted concurrent session")}
 if _,err:=r.Acquire(Claims{DeviceID:strings.Repeat("b",64)},now);err==nil{t.Fatal("wrong device accepted")}
 release()
 release2,err:=r.Acquire(c,now);if err!=nil{t.Fatal(err)}
 r.Unregister(p.DeviceID,"different")
 if !r.Online(p.DeviceID,now){t.Fatal("stale disconnect removed device")}
 release2()
 r.Unregister(p.DeviceID,p.Nonce)
 if r.Online(p.DeviceID,now){t.Fatal("device remains registered after disconnect")}
 if err:=r.Register(p,now);err==nil{t.Fatal("replayed proof accepted")}
}
