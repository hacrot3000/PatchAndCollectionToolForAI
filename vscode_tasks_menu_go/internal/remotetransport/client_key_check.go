package remotetransport

import (
 "bytes"
 "encoding/json"
 "errors"
 "io"
)

const ClientKeyCheckProtocol="taskdeck-e2ee-client-key-check-v1"
const ClientKeyStatusProtocol="taskdeck-e2ee-client-key-status-v1"
type ClientKeyCheck struct {
 Username string `json:"username"`
 PublicKey string `json:"public_key"`
 Signature string `json:"signature"`
}
type keyStatusWire struct {
 Type string `json:"type"`
 Approved bool `json:"approved"`
}
type keyCheckWire struct {
 Type string `json:"type"`
 Username string `json:"username"`
 PublicKey string `json:"public_key"`
 Signature string `json:"signature"`
}
var ErrInvalidClientKeyCheck=errors.New("invalid encrypted client key check")
func ClientKeyCheckFrame(ch *Channel,request ClientKeyCheck)([]byte,error){
 if ch==nil||!ch.Established()||request.Username==""||len(request.Username)>128||
  len(request.PublicKey)!=44||len(request.Signature)>128||len(request.Signature)<86 {
  return nil,ErrInvalidClientKeyCheck
 }
 raw,err:=json.Marshal(keyCheckWire{Type:ClientKeyCheckProtocol,Username:request.Username,
  PublicKey:request.PublicKey,Signature:request.Signature})
 if err!=nil{return nil,err}
 return ch.Seal(raw)
}
// decodeKeyObject enforces exact canonical keys without duplicate/case aliases,
// primitive-only values and no trailing JSON. The caller already authenticated
// the E2EE record; no Portal-supplied plaintext may reach this decoder.
func decodeKeyObject(raw []byte,allowed []string,dst interface{})error{
 if len(raw)<2||len(raw)>1024{return ErrInvalidClientKeyCheck}
 allowedMap:=make(map[string]bool,len(allowed))
 for _,name:=range allowed{allowedMap[name]=false}
 d:=json.NewDecoder(bytes.NewReader(raw))
 first,err:=d.Token();if err!=nil||first!=json.Delim('{'){return ErrInvalidClientKeyCheck}
 for d.More(){
  token,err:=d.Token()
  key,ok:=token.(string)
  if err!=nil||!ok{return ErrInvalidClientKeyCheck}
  seen,known:=allowedMap[key];if !known||seen{return ErrInvalidClientKeyCheck}
  allowedMap[key]=true
  var primitive interface{}
  if err:=d.Decode(&primitive);err!=nil{return ErrInvalidClientKeyCheck}
  switch primitive.(type){case string,bool:default:return ErrInvalidClientKeyCheck}
 }
 if token,err:=d.Token();err!=nil||token!=json.Delim('}'){return ErrInvalidClientKeyCheck}
 if _,err:=d.Token();err!=io.EOF{return ErrInvalidClientKeyCheck}
 for _,seen:=range allowedMap{if !seen{return ErrInvalidClientKeyCheck}}
 typed:=json.NewDecoder(bytes.NewReader(raw));typed.DisallowUnknownFields()
 if err:=typed.Decode(dst);err!=nil{return ErrInvalidClientKeyCheck}
 return nil
}
func OpenClientKeyCheck(ch *Channel,frame []byte)(ClientKeyCheck,error){
 var empty ClientKeyCheck
 if ch==nil||!ch.Established(){return empty,ErrInvalidClientKeyCheck}
 clear,err:=ch.Open(frame)
 if err!=nil{return empty,ErrInvalidClientKeyCheck}
 var request keyCheckWire
 if err:=decodeKeyObject(clear,[]string{"type","username","public_key","signature"},&request);
  err!=nil||request.Type!=ClientKeyCheckProtocol||request.Username==""||len(request.Username)>128||
  len(request.PublicKey)!=44||len(request.Signature)<86||len(request.Signature)>128 {
  ch.Close();return empty,ErrInvalidClientKeyCheck
 }
 return ClientKeyCheck{Username:request.Username,PublicKey:request.PublicKey,
  Signature:request.Signature},nil
}
func SealClientKeyStatus(ch *Channel,approved bool)([]byte,error){
 if ch==nil||!ch.Established(){return nil,ErrInvalidClientKeyCheck}
 raw,err:=json.Marshal(keyStatusWire{Type:ClientKeyStatusProtocol,Approved:approved})
 if err!=nil{return nil,err}
 return ch.Seal(raw)
}
func OpenClientKeyStatus(ch *Channel,frame []byte)(bool,error){
 if ch==nil||!ch.Established(){return false,ErrInvalidClientKeyCheck}
 clear,err:=ch.Open(frame)
 if err!=nil{return false,ErrInvalidClientKeyCheck}
 var response keyStatusWire
 if err:=decodeKeyObject(clear,[]string{"type","approved"},&response);
  err!=nil||response.Type!=ClientKeyStatusProtocol{
  ch.Close();return false,ErrInvalidClientKeyCheck
 }
 return response.Approved,nil
}
