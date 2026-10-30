package remoterelay

import (
 "crypto/ed25519"
 "crypto/rand"
 "encoding/base64"
 "encoding/json"
 "strings"
 "strconv"
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

func TestCompromisedPHPSignedLeaseAmbiguityDenied(t *testing.T){
 pub,sk,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 verifier,err:=NewVerifier(base64.StdEncoding.EncodeToString(pub),"https://portal.example.com")
 if err!=nil{t.Fatal(err)}
 now:=time.Now()
 c:=DeviceLease{V:1,Iss:"https://portal.example.com",Aud:"taskdeck-relay-device",
  DeviceID:strings.Repeat("a",64),Iat:now.Unix(),Exp:now.Unix()+70}
 raw,err:=json.Marshal(c);if err!=nil{t.Fatal(err)}
 original:=string(raw)
 bad:=[]string{
  strings.Replace(original,"\"device_id\":\""+c.DeviceID+"\"",
   "\"device_id\":\""+c.DeviceID+"\",\"device_id\":\""+strings.Repeat("b",64)+"\"",1),
  strings.Replace(original,"\"aud\":\"taskdeck-relay-device\"",
   "\"\\u0061ud\":\"taskdeck-relay-device\",\"aud\":\"taskdeck-relay-device\"",1),
  strings.Replace(original,"\"iss\":\"https://portal.example.com\"",
   "\"Iss\":\"https://portal.example.com\"",1),
  strings.Replace(original,"\"v\":1","\"v\":\"1\"",1),
  strings.Replace(original,"\"iat\":"+strconv.FormatInt(now.Unix(),10),"\"iat\":1e9",1),
  strings.TrimSuffix(original,"}")+",\"taskdeck_admin\":true}",
  original+"{}",
 }
 sign:=func(payload string)string{
  body:=base64.RawURLEncoding.EncodeToString([]byte(payload))
  sig:=ed25519.Sign(sk,[]byte(leasePrefix+body))
  return body+"."+base64.RawURLEncoding.EncodeToString(sig)
 }
 for _,payload:=range bad{
  if payload==original{t.Fatal("ineffective signed lease test")}
  if verifier.VerifyDeviceLease(sign(payload),c.DeviceID,now)==nil{
   t.Fatalf("signed ambiguous PHP lease accepted: %s",payload)
  }
 }
 if err:=verifier.VerifyDeviceLease(sign(original),c.DeviceID,now);err!=nil{
  t.Fatalf("canonical lease rejected: %v",err)
 }
}
