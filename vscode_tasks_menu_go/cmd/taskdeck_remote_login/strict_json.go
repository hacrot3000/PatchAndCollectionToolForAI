package main

import (
 "bytes"
 "encoding/json"
 "errors"
 "io"
)

// decodeRelayObject rejects ambiguous/malleable unauthenticated Relay JSON
// *before* interpreting any claimed Agent identity or login response.
// Unlike json.Unmarshal, it rejects duplicate keys at ANY nesting depth,
// unknown fields, trailing JSON values and excessive nesting. The encrypted
// TaskDeck payload has its own independent strict Go parser.
func decodeRelayObject(raw []byte, dst interface{}) error {
 if len(raw)==0 || len(raw)>12288 {
  return errors.New("Relay JSON exceeds strict message bounds")
 }
 d:=json.NewDecoder(bytes.NewReader(raw))
 count:=0
 if err:=scanRelayJSON(d,0,&count);err!=nil{return err}
 if _,err:=d.Token();!errors.Is(err,io.EOF){
  return errors.New("Relay JSON has trailing content")
 }
 typed:=json.NewDecoder(bytes.NewReader(raw))
 typed.DisallowUnknownFields()
 if err:=typed.Decode(dst);err!=nil{return err}
 if _,err:=typed.Token();!errors.Is(err,io.EOF){
  return errors.New("Relay JSON contains unexpected trailing fields")
 }
 return nil
}

func scanRelayJSON(d *json.Decoder, depth int, count *int) error {
 if depth>6 || *count>=256 {return errors.New("Relay JSON nesting/token budget exceeded")}
 *count++
 token,err:=d.Token()
 if err!=nil{return err}
 delimiter,ok:=token.(json.Delim)
 if !ok{return nil}
 switch delimiter {
 case '{':
  seen:=make(map[string]bool)
  for d.More() {
   *count++
   if *count>256{return errors.New("Relay JSON token budget exceeded")}
   nameToken,err:=d.Token()
   if err!=nil{return err}
   name,ok:=nameToken.(string)
   if !ok || seen[name]{return errors.New("duplicate or invalid Relay JSON key")}
   seen[name]=true
   if err:=scanRelayJSON(d,depth+1,count);err!=nil{return err}
  }
  end,err:=d.Token()
  if err!=nil || end!=json.Delim('}') {return errors.New("malformed Relay JSON object")}
 case '[':
  for d.More() {
   if err:=scanRelayJSON(d,depth+1,count);err!=nil{return err}
  }
  end,err:=d.Token()
  if err!=nil || end!=json.Delim(']') {return errors.New("malformed Relay JSON array")}
 default:
  return errors.New("unexpected Relay JSON delimiter")
 }
 return nil
}
