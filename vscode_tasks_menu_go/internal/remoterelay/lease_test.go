package remoterelay

import (
 "crypto/ed25519"
 "crypto/rand"
 "encoding/base64"
 "encoding/json"
 "strings"
 "testing"
 "time"
)
func makeLease(t *testing.T, private ed25519.PrivateKey, c DeviceLease) string {
 t.Helper()
 raw,err:=json.Marshal(c);if err!=nil{t.Fatal(err)}
 body:=base64.RawURLEncoding.EncodeToString(raw)
 sig:=ed25519.Sign(private,[]byte(leasePrefix+body))
 return body+"."+base64.RawURLEncoding.EncodeToString(sig)
}
func TestSignedDeviceLeaseIsDeviceBoundAndExpires(t *testing.T) {
 pk,sk,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 v,err:=NewVerifier(base64.StdEncoding.EncodeToString(pk),"https://portal.example.com")
 if err!=nil{t.Fatal(err)}
 now:=time.Now()
 device:=strings.Repeat("a",64)
 c:=DeviceLease{V:1,Iss:"https://portal.example.com",Aud:"taskdeck-relay-device",
 DeviceID:device,Iat:now.Unix(),Exp:now.Unix()+70}
 token:=makeLease(t,sk,c)
 if err:=v.VerifyDeviceLease(token,device,now);err!=nil{t.Fatal(err)}
 if err:=v.VerifyDeviceLease(token,device,now.Add(71*time.Second));err==nil{t.Fatal("expired lease")}
 if err:=v.VerifyDeviceLease(token,strings.Repeat("b",64),now);err==nil{t.Fatal("wrong device")}
 if err:=v.VerifyDeviceLease(token+"x",device,now);err==nil{t.Fatal("modified signature")}
 c.Exp=now.Unix()+3600
 if err:=v.VerifyDeviceLease(makeLease(t,sk,c),device,now);err==nil{t.Fatal("excessive duration")}
}
