package remoteportal

import (
 "bytes"
 "encoding/json"
 "errors"
 "io"
 "unicode/utf8"
)

var errInvalidPortalVerdict=errors.New("untrusted Portal grant verdict rejected")

// Strict response parsing is defense-in-depth: a completely compromised
// Portal can still reply allowed=true, which NEVER replaces native
// client-key possession, TaskDeck local password or project RBAC.
// Reject even a legitimately allowed response if its JSON is ambiguous.
func verifyStrictPortalGrantVerdict(raw []byte)error {
 if len(raw)<2 || len(raw)>256 || !utf8.Valid(raw) {
  return errInvalidPortalVerdict
 }
 d:=json.NewDecoder(bytes.NewReader(raw))
 start,err:=d.Token()
 if err!=nil || start!=json.Delim('{'){return errInvalidPortalVerdict}
 seen:=map[string]bool{}
 for d.More(){
  token,err:=d.Token()
  key,ok:=token.(string)
  if err!=nil || !ok || seen[key] ||
   (key!="ok" && key!="allowed") {
   return errInvalidPortalVerdict
  }
  seen[key]=true
  // Avoid decoding JSON null into a Go bool (which can silently succeed).
  var literal json.RawMessage
  if err:=d.Decode(&literal);err!=nil{return errInvalidPortalVerdict}
  if !bytes.Equal(literal,[]byte("true")) &&
   !bytes.Equal(literal,[]byte("false")) {
   return errInvalidPortalVerdict
  }
  if !bytes.Equal(literal,[]byte("true")) {
   return errors.New("Portal grant rejected or revoked")
  }
 }
 end,err:=d.Token()
 if err!=nil || end!=json.Delim('}') ||
  len(seen)!=2 || !seen["ok"] || !seen["allowed"] {
  return errInvalidPortalVerdict
 }
 if _,err:=d.Token();!errors.Is(err,io.EOF) {
  return errInvalidPortalVerdict
 }
 return nil
}
