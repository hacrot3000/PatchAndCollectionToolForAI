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

// An opaque browser Origin is easily spoofable by sandboxed pages controlled
// by PHP. These tests exercise only the opt-in rendezvous gate; successful
// upgrade cannot produce a TaskDeck principal or application command.
func TestCompromisedPHPCannotEnableOfflineFileDiagnosticRoute(t *testing.T){
 pub,_,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 verifier,err:=NewVerifier(base64.StdEncoding.EncodeToString(pub),"https://portal.example.com")
 if err!=nil{t.Fatal(err)}
 registry,err:=NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 h:=&Handler{Tickets:verifier,Devices:registry}
 for _,origin:=range []string{"null","https://portal.example.com","https://evil.example.com",""}{
  req:=httptest.NewRequest(http.MethodGet,"https://relay.example.com/v1/offline",nil)
  if origin!=""{req.Header.Set("Origin",origin)}
  w:=httptest.NewRecorder()
  h.ServeHTTP(w,req)
  if w.Code!=http.StatusForbidden {t.Fatalf("disabled offline origin %q accepted: %d",origin,w.Code)}
 }
 h.AllowOfflineFileClient=true
 for _,origin:=range []string{"https://portal.example.com","https://evil.example.com","",
  "NULL","null ","http://127.0.0.1","file://"}{
  req:=httptest.NewRequest(http.MethodGet,"https://relay.example.com/v1/offline",nil)
  if origin!=""{req.Header.Set("Origin",origin)}
  w:=httptest.NewRecorder()
  h.ServeHTTP(w,req)
  if w.Code!=http.StatusForbidden {t.Fatalf("forged opaque origin %q accepted: %d",origin,w.Code)}
 }
 for _,path:=range []string{"/v1/offline?ticket=stolen","/v1/offline?device=x"}{
  req:=httptest.NewRequest(http.MethodGet,"https://relay.example.com"+path,nil)
  req.Header.Set("Origin","null")
  w:=httptest.NewRecorder()
  h.ServeHTTP(w,req)
  if w.Code!=http.StatusForbidden{t.Fatalf("query accepted on offline route %q: %d",path,w.Code)}
 }
 // Positive, genuine HTTP WebSocket upgrade with Origin:null. This
 // endpoint can only perform a bounded signed Agent diagnostic.
 srv:=httptest.NewTLSServer(h)
 defer srv.Close()
 header:=http.Header{"Origin":[]string{"null"}}
 conn,response,err:=websocket.Dial(context.Background(),"wss"+srv.URL[len("https"):]+"/v1/offline",
  &websocket.DialOptions{HTTPClient:srv.Client(),HTTPHeader:header})
 if err!=nil {
  code:=0
  if response!=nil{code=response.StatusCode}
  t.Fatalf("local Firefox opaque origin did not upgrade: %v, http=%d",err,code)
 }
 _=conn.Close(websocket.StatusNormalClosure,"diagnostic only")
}

// Configured offline login still requires full ticket, device Agent proof,
// host client-key preflight, and local password verification; this HTTP
// admission test only checks the route is opt-in rather than a capability.
func TestOfflineLoginOptInDoesNotOpenUnconfiguredBrowserRoute(t *testing.T){
 pub,_,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 verifier,err:=NewVerifier(base64.StdEncoding.EncodeToString(pub),"https://portal.example.com");if err!=nil{t.Fatal(err)}
 registry,err:=NewRegistry("https://relay.example.com");if err!=nil{t.Fatal(err)}
 h:=&Handler{Tickets:verifier,Devices:registry,AllowOfflineFileClient:false,AllowOfflineLoginProbe:true}
 req:=httptest.NewRequest(http.MethodGet,"https://relay.example.com/v1/offline",nil)
 req.Header.Set("Origin","null")
 w:=httptest.NewRecorder();h.ServeHTTP(w,req)
 if w.Code!=http.StatusForbidden {t.Fatalf("offline entry opened while disabled: %d",w.Code)}
}
