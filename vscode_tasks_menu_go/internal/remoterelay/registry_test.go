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
 p,_:=newAgentProofWithKey(t,now)
 return p
}
func newAgentProofWithKey(t *testing.T,now time.Time) (DeviceProof, ed25519.PrivateKey) {
 t.Helper()
 pub,priv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256(pub)
 p:=DeviceProof{DeviceID:hex.EncodeToString(sum[:]),PublicKey:base64.StdEncoding.EncodeToString(pub),
 Audience:"https://relay.example.com",Challenge:base64.RawURLEncoding.EncodeToString(make([]byte,32)),Timestamp:now.Unix(),Nonce:strings.Repeat("a",32)}
 raw:=strings.Join([]string{DeviceChallengeProtocol,"connect",p.DeviceID,p.Audience,
 strconv.FormatInt(p.Timestamp,10),p.Nonce,p.Challenge},string([]byte{10}))
 p.Signature=base64.StdEncoding.EncodeToString(ed25519.Sign(priv,[]byte(raw)))
 return p,priv
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

func TestStaleReleaseCannotClearNewBrowserSession(t *testing.T) {
	now := time.Now()
	reg, err := NewRegistry("https://relay.example.com")
	if err != nil { t.Fatal(err) }
	proof := newAgentProof(t, now)
	if err := reg.Register(proof, now); err != nil { t.Fatal(err) }
	claims := Claims{DeviceID:proof.DeviceID}
	releaseFirst, err := reg.Acquire(claims, now)
	if err != nil { t.Fatal(err) }
	releaseFirst()
	releaseSecond, err := reg.Acquire(claims, now)
	if err != nil { t.Fatal(err) }
	// A second deferred cleanup from a prior handler must not take down the
	// newer browser's exclusive reservation or its pending reachability check.
	releaseFirst()
	pending, err := reg.BeginProbe(proof.DeviceID,strings.Repeat("c",32))
	if err != nil { t.Fatalf("stale release canceled newer browser session: %v",err) }
	if _, err := reg.Acquire(claims, now); err == nil {
		t.Fatal("third session entered while second reservation was live")
	}
	releaseSecond()
	select {
	case ok:=<-pending:
		if ok.OK {t.Fatal("pending probe was incorrectly confirmed")}
	case <-time.After(time.Second):
		t.Fatal("second session cleanup did not cancel its pending probe")
	}
	releaseThird, err := reg.Acquire(claims, now)
	if err != nil {t.Fatalf("session slot leaked: %v",err)}
	releaseThird()
}
