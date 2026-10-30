package remoterelay

import (
 "bytes"
 "encoding/json"
 "errors"
 "io"
 "unicode/utf8"
)

// The Relay is untrusted by the host and the browser is untrusted by the
// Relay. Do not use encoding/json's struct unmarshalling on authorization or
// routing envelopes: it silently accepts duplicate keys, case-folded aliases,
// unknown fields, and trailing data. Keep this parser deliberately narrow.
var errMalformedBrowserFrame=errors.New("invalid bounded browser E2EE envelope")

func exactBrowserStrings(raw []byte, maxBytes int, keys ...string) (map[string]string,error) {
 if len(raw)<2 || len(raw)>maxBytes || !utf8.Valid(raw) {
  return nil,errMalformedBrowserFrame
 }
 expected:=make(map[string]bool,len(keys))
 for _,key:=range keys { expected[key]=true }
 values:=make(map[string]string,len(keys))
 decoder:=json.NewDecoder(bytes.NewReader(raw))
 first,err:=decoder.Token()
 if err!=nil || first!=json.Delim('{') {return nil,errMalformedBrowserFrame}
 for decoder.More(){
  token,err:=decoder.Token()
  name,ok:=token.(string)
  if err!=nil || !ok || !expected[name] {return nil,errMalformedBrowserFrame}
  if _,duplicate:=values[name];duplicate {return nil,errMalformedBrowserFrame}
  var value string
  if decoder.Decode(&value)!=nil || value=="" {return nil,errMalformedBrowserFrame}
  values[name]=value
 }
 last,err:=decoder.Token()
 if err!=nil || last!=json.Delim('}') || len(values)!=len(expected) {
  return nil,errMalformedBrowserFrame
 }
 if _,err=decoder.Token();!errors.Is(err,io.EOF) {return nil,errMalformedBrowserFrame}
 return values,nil
}

// Never consume a one-time ticket unless the whole exact, non-ambiguous
// browser handshake has been checked. Browser key validity is also enforced
// by ConsumeForBrowser and the E2EE transcript verifier.
type browserHandshake struct {
 Ticket string
 ClientNonce string
 BrowserKey string
}
func decodeBrowserHandshake(raw []byte)(browserHandshake,error){
 var empty browserHandshake
 values,err:=exactBrowserStrings(raw,12288,"ticket","client_nonce","browser_key")
 if err!=nil || len(values["ticket"])>4096 ||
  !validProbeNonce(values["client_nonce"]) ||
  len(values["browser_key"])>256 {
  return empty,errMalformedBrowserFrame
 }
 return browserHandshake{
  Ticket:values["ticket"],ClientNonce:values["client_nonce"],
  BrowserKey:values["browser_key"],
 },nil
}

// Only the three existing bounded login/diagnostic wire operations exist.
// Explicitly NO URL, path, HTTP method, PTY, command or session token field
// may be provided. This decoder does not decrypt or inspect E2EE plaintext.
type browserEncryptedFrame struct {
 Type string
 Ciphertext string
}
func decodeBrowserEncryptedFrame(raw []byte)(browserEncryptedFrame,error){
 var empty browserEncryptedFrame
 values,err:=exactBrowserStrings(raw,10000,"type","ciphertext")
 if err!=nil {return empty,errMalformedBrowserFrame}
 typ:=values["type"]
 ciphertext:=values["ciphertext"]
 switch typ {
 case "e2ee_echo","e2ee_client_key_check":
  if len(raw)>2048 || !validEchoCiphertext(ciphertext) {
   return empty,errMalformedBrowserFrame
  }
 case "e2ee_login":
  if !validLoginCiphertext(ciphertext) {
   return empty,errMalformedBrowserFrame
  }
 default:
  return empty,errMalformedBrowserFrame
 }
 return browserEncryptedFrame{Type:typ,Ciphertext:ciphertext},nil
}

 // Relay never sees whether an encrypted preflight returned approved=true:
 // only the owner-pinned Agent can authenticate that verdict. This helper
 // enforces the mandatory SEQUENCE, not the user's authorization.
 // Password-only V1 and echo-after-key-check fail before QueueLogin/QueueEcho.
func allowedBrowserProtectedTransition(hadKeyCheck bool, operation string) bool {
 if hadKeyCheck {return operation=="e2ee_login"}
 return operation=="e2ee_echo"
}

 
// The offline file route is an intentionally NO-PASSWORD diagnostic.
// Even a PHP-forged valid ticket plus an E2EE-approved key-check cannot
// turn it into a TaskDeck credential-check or application endpoint.
func allowedBrowserProtectedTransitionForRoute(hadKeyCheck bool,operation string,allowLogin bool)bool{
 if operation=="e2ee_login" && !allowLogin{return false}
 return allowedBrowserProtectedTransition(hadKeyCheck,operation)
}
