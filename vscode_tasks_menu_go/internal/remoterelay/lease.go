package remoterelay

import (
 "bytes"
 "crypto/ed25519"
 "encoding/base64"
 "encoding/json"
 "errors"
 "io"
 "strings"
 "time"
)
const leasePrefix="taskdeck-relay-device-lease-v1."
type DeviceLease struct {
 V int `json:"v"`
 Iss string `json:"iss"`
 Aud string `json:"aud"`
 DeviceID string `json:"device_id"`
 Iat int64 `json:"iat"`
 Exp int64 `json:"exp"`
}
func (v *Verifier) VerifyDeviceLease(token, device string, now time.Time) error {
 if v==nil || len(token)<100 || len(token)>2048 || strings.Count(token,".")!=1 {return errors.New("malformed lease")}
 parts:=strings.SplitN(token,".",2)
 body,err:=base64.RawURLEncoding.Strict().DecodeString(parts[0])
 if err!=nil || len(body)==0 || len(body)>1024 {return errors.New("invalid lease body")}
 sig,err:=base64.RawURLEncoding.Strict().DecodeString(parts[1])
 if err!=nil || len(sig)!=ed25519.SignatureSize ||
 !ed25519.Verify(v.key,[]byte(leasePrefix+parts[0]),sig) {return errors.New("invalid lease signature")}
 var c DeviceLease
 dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
 if err:=dec.Decode(&c);err!=nil{return errors.New("invalid lease claims")}
 var trailing json.RawMessage
 if err:=dec.Decode(&trailing);!errors.Is(err,io.EOF){return errors.New("trailing lease content")}
 sec:=now.Unix()
 if c.V!=1 || c.Iss!=v.issuer || c.Aud!="taskdeck-relay-device" ||
 c.DeviceID!=device || !deviceIDPattern.MatchString(device) ||
 c.Iat>sec+5 || c.Iat<sec-80 || c.Exp<=sec || c.Exp<=c.Iat || c.Exp>c.Iat+70 {
  return errors.New("device lease expired or mismatched")
 }
 return nil
}
