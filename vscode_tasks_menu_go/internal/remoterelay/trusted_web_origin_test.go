package remoterelay

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "encoding/base64"
 "net/http"
 "net/http/httptest"
 "testing"

 "github.com/coder/websocket"
)

func TestTrustedStaticWebClientOriginMustBeIndependent(t *testing.T) {
 portal:="https://php.example.com"
 relay:="https://relay.example.com"
 for _,tc:=range []struct{origin string;allowed bool}{
  {"",false},
  {portal,false},
  {relay,false},
  {"http://ui.example.com",false},
  {"https://ui.example.com/path",false},
  {"https://ui.example.com/",false},
  {"https://ui.example.com?foo=1",false},
  {"https://*.example.com",false},
  {"https://ui.example.com#fragment",false},
  {"https://user@ui.example.com",false},
  {"https://ui.example.com",true},
 }{
  got:=ValidTrustedWebOrigin(tc.origin,portal,relay)
  if got!=tc.allowed{t.Errorf("origin %q: got %v want %v",tc.origin,got,tc.allowed)}
 }
}

func TestCompromisedPHPBrowserCannotUseTrustedWebClientSocket(t *testing.T) {
 pub,_,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 v,err:=NewVerifier(base64.StdEncoding.EncodeToString(pub),"https://php.example.com")
 if err!=nil{t.Fatal(err)}
 reg,err:=NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 for _,tc:=range []struct{trustedOrigin,requestOrigin string}{
  {"","https://ui.example.com"},
  {"https://php.example.com","https://php.example.com"},
  {"https://relay.example.com","https://relay.example.com"},
  {"https://ui.example.com","https://php.example.com"},
  {"https://ui.example.com","https://ui.example.com.evil.invalid"},
  {"https://ui.example.com","https://ui.example.com:443"},
  {"https://ui.example.com","null"},
  {"https://ui.example.com",""},
 }{
  h:=&Handler{Tickets:v,Devices:reg,TrustedWebOrigin:tc.trustedOrigin}
  req:=httptest.NewRequest(http.MethodGet,"https://relay.example.com/v1/trusted-web",nil)
  if tc.requestOrigin!="" {req.Header.Set("Origin",tc.requestOrigin)}
  w:=httptest.NewRecorder()
  h.ServeHTTP(w,req)
  if w.Code!=http.StatusForbidden{
   t.Fatalf("PHP/unconfigured origin accepted (%q,%q): %d",
    tc.trustedOrigin,tc.requestOrigin,w.Code)
  }
 }
 // Positive case: the independently hosted browser's exact origin does
 // upgrade successfully. This route still implements diagnostics ONLY.
 h:=&Handler{Tickets:v,Devices:reg,TrustedWebOrigin:"https://ui.example.com"}
 srv:=httptest.NewTLSServer(h)
 defer srv.Close()
 headers:=http.Header{}
 headers.Set("Origin","https://ui.example.com")
 conn,response,err:=websocket.Dial(context.Background(),
  "wss"+srv.URL[len("https"):]+"/v1/trusted-web",
  &websocket.DialOptions{HTTPClient:srv.Client(),HTTPHeader:headers})
 if err!=nil {
  status:=0
  if response!=nil{status=response.StatusCode}
  t.Fatalf("trusted independent browser origin was denied: %v (HTTP %d)",err,status)
 }
 _=conn.Close(websocket.StatusNormalClosure,"close without sending a ticket")
}
