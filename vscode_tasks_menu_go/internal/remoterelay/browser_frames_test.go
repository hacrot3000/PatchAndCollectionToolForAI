package remoterelay

import (
 "encoding/base64"
 "fmt"
 "strings"
 "testing"
)

var opaqueBrowserTestCipher=base64.StdEncoding.EncodeToString(make([]byte,32))

func TestExactBrowserHandshakeRejectsAmbiguousFrames(t *testing.T) {
 legitimate:=`{"ticket":"valid-ticket","client_nonce":"0123456789abcdef0123456789abcdef","browser_key":"key"}`
 good,err:=decodeBrowserHandshake([]byte(legitimate))
 if err!=nil || good.Ticket!="valid-ticket" {t.Fatalf("valid handshake: %+v %v",good,err)}
 bad:=map[string]string{
  "duplicate ticket":`{"ticket":"first","ticket":"second","client_nonce":"0123456789abcdef0123456789abcdef","browser_key":"key"}`,
  "case alias":`{"Ticket":"another","ticket":"real","client_nonce":"0123456789abcdef0123456789abcdef","browser_key":"key"}`,
  "unknown routing field":`{"ticket":"valid","client_nonce":"0123456789abcdef0123456789abcdef","browser_key":"key","device_id":"target"}`,
  "missing binding":`{"ticket":"valid","client_nonce":"0123456789abcdef0123456789abcdef"}`,
  "nested key":`{"ticket":"valid","client_nonce":"0123456789abcdef0123456789abcdef","browser_key":{"k":"key"}}`,
  "trailing command":legitimate+` {"method":"POST"}`,
  "case swapped nonce":`{"ticket":"valid","client_nonce":"0123456789ABCDEF0123456789ABCDEF","browser_key":"key"}`,
  "empty ticket":`{"ticket":"","client_nonce":"0123456789abcdef0123456789abcdef","browser_key":"key"}`,
  "large ticket":fmt.Sprintf(`{"ticket":"%s","client_nonce":"0123456789abcdef0123456789abcdef","browser_key":"key"}`,strings.Repeat("X",4097)),
 }
 for name,wire:=range bad {
  t.Run(name,func(t *testing.T){
   if got,err:=decodeBrowserHandshake([]byte(wire));err==nil {
    t.Fatalf("accepted ambiguous handshake: %+v",got)
   }
  })
 }
}

func TestExactBrowserProtectedEnvelopeRejectsInjectionAndReplayShapes(t *testing.T){
 for _,kind:=range []string{"e2ee_echo","e2ee_client_key_check","e2ee_login"}{
  valid:=fmt.Sprintf(`{"type":%q,"ciphertext":%q}`,kind,opaqueBrowserTestCipher)
  parsed,err:=decodeBrowserEncryptedFrame([]byte(valid))
  if err!=nil || parsed.Type!=kind || parsed.Ciphertext!=opaqueBrowserTestCipher {
   t.Fatalf("valid %s frame rejected: %+v %v",kind,parsed,err)
  }
 }
 bad:=map[string]string{
  "duplicate type":fmt.Sprintf(`{"type":"e2ee_echo","type":"e2ee_login","ciphertext":%q}`,opaqueBrowserTestCipher),
  "duplicate ciphertext":fmt.Sprintf(`{"type":"e2ee_login","ciphertext":%q,"ciphertext":%q}`,opaqueBrowserTestCipher,opaqueBrowserTestCipher),
  "case-insensitive spoof":fmt.Sprintf(`{"type":"e2ee_echo","Type":"e2ee_login","ciphertext":%q}`,opaqueBrowserTestCipher),
  "command injection":fmt.Sprintf(`{"type":"e2ee_login","ciphertext":%q,"command":"rm -rf /"}`,opaqueBrowserTestCipher),
  "route injection":fmt.Sprintf(`{"type":"e2ee_login","ciphertext":%q,"path":"/api/terminal"}`,opaqueBrowserTestCipher),
  "fake approved":fmt.Sprintf(`{"type":"e2ee_client_key_check","ciphertext":%q,"approved":"true"}`,opaqueBrowserTestCipher),
  "missing ciphertext":`{"type":"e2ee_login"}`,
  "boolean type":fmt.Sprintf(`{"type":true,"ciphertext":%q}`,opaqueBrowserTestCipher),
  "array type":fmt.Sprintf(`{"type":["e2ee_login"],"ciphertext":%q}`,opaqueBrowserTestCipher),
  "invalid b64":`{"type":"e2ee_login","ciphertext":"not;valid;base64"}`,
  "second document":fmt.Sprintf(`{"type":"e2ee_echo","ciphertext":%q}{"type":"e2ee_login"}`,opaqueBrowserTestCipher),
  "unapproved operation":fmt.Sprintf(`{"type":"e2ee_action","ciphertext":%q}`,opaqueBrowserTestCipher),
  "oversize echo":fmt.Sprintf(`{"type":"e2ee_echo","ciphertext":%q}`,base64.StdEncoding.EncodeToString(make([]byte,1600))),
 }
 for name,wire:=range bad {
  t.Run(name,func(t *testing.T){
   if parsed,err:=decodeBrowserEncryptedFrame([]byte(wire));err==nil {
    t.Fatalf("accepted malicious protected frame: %+v",parsed)
   }
  })
 }
}

func TestBrowserProtectedTransitionRequiresHostKeyPreflight(t *testing.T) {
 cases:=[]struct{
  keyCheck bool
  operation string
  allowed bool
 }{
  {false,"e2ee_login",false}, // PHP cannot send legacy password-only
  {false,"e2ee_echo",true},
  {false,"e2ee_client_key_check",false}, // first check handled separately
  {true,"e2ee_login",true}, // Agent still requires encrypted approved=true
  {true,"e2ee_echo",false},
  {true,"e2ee_client_key_check",false},
  {true,"terminal.exec",false},
 }
 for _,tc:=range cases {
  if got:=allowedBrowserProtectedTransition(tc.keyCheck,tc.operation);got!=tc.allowed {
   t.Fatalf("keyCheck=%v operation=%s: got %v want %v",tc.keyCheck,tc.operation,got,tc.allowed)
  }
 }
}

func TestOfflineFileRouteNeverAcceptsTaskDeckPasswordOrApplicationFrame(t *testing.T){
 for _,tc:=range []struct{preflight bool;op string;allow bool}{
  {false,"e2ee_echo",true},
  {true,"e2ee_echo",false},
  {true,"e2ee_client_key_check",false},
  {true,"e2ee_login",false},
  {false,"e2ee_login",false},
  {true,"terminal.exec",false},
  {true,"file.write",false},
 }{
  if got:=allowedBrowserProtectedTransitionForRoute(tc.preflight,tc.op,false);got!=tc.allow{
   t.Errorf("offline preflight=%v op=%s: got %v want %v",tc.preflight,tc.op,got,tc.allow)
  }
 }
 if !allowedBrowserProtectedTransitionForRoute(true,"e2ee_login",true){
  t.Fatal("existing native login-only path was accidentally disabled")
 }
}
