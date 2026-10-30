package remoterelay

import (
 "encoding/json"
 "errors"
 "strconv"
)

// A compromised PHP signer may issue a correctly signed but ambiguous token.
// Every claim has exactly one canonical spelling and JSON type.
var errAmbiguousSignedClaims=errors.New("ambiguous signed Portal claims")

func exactSignedFields(body []byte,limit int,strings map[string]int,integers ...string)error{
 fields:=make([]string,0,len(strings)+len(integers))
 for key:=range strings{fields=append(fields,key)}
 fields=append(fields,integers...)
 values,err:=relayExactObject(body,limit,fields...)
 if err!=nil||!relayExactKeys(values,fields...){return errAmbiguousSignedClaims}
 for key,size:=range strings{
  if _,err:=relayString(values[key],size);err!=nil{return errAmbiguousSignedClaims}
 }
 for _,key:=range integers{
  if _,err:=strconv.ParseInt(string(values[key]),10,64);err!=nil{return errAmbiguousSignedClaims}
 }
 return nil
}
func decodeSignedTicketClaims(body []byte)(Claims,error){
 var c Claims
 if err:=exactSignedFields(body,2048,map[string]int{
  "browser_binding":64,"iss":512,"aud":64,"sub":64,"device_id":64,
  "role":32,"jti":32,
 },"v","auth_epoch","iat","exp");err!=nil{return Claims{},err}
 if json.Unmarshal(body,&c)!=nil{return Claims{},errAmbiguousSignedClaims}
 return c,nil
}
func decodeSignedDeviceLease(body []byte)(DeviceLease,error){
 var c DeviceLease
 if err:=exactSignedFields(body,1024,map[string]int{
  "iss":512,"aud":64,"device_id":64,
 },"v","iat","exp");err!=nil{return DeviceLease{},err}
 if json.Unmarshal(body,&c)!=nil{return DeviceLease{},errAmbiguousSignedClaims}
 return c,nil
}
