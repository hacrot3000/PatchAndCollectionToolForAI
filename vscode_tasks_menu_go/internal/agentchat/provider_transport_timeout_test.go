package agentchat

import (
 "context"
 "crypto/tls"
 "encoding/json"
 "errors"
 "net"
 "net/http"
 "net/http/httptrace"
 "strings"
 "testing"
 "time"
)

func TestProviderTransportTimeoutConfiguration(t *testing.T){
 if ProviderAttemptTimeout!=600*time.Second{t.Fatalf("AI provider timeout=%s, want 600s",ProviderAttemptTimeout)}
 client:=newProviderClient(ProviderConfig{})
 if client.Timeout!=ProviderAttemptTimeout{t.Fatalf("HTTP timeout=%s",client.Timeout)}
 transport,ok:=client.Transport.(*http.Transport)
 if !ok{t.Fatal("missing hardened HTTP transport")}
 if transport.ResponseHeaderTimeout!=ProviderAttemptTimeout{
  t.Fatalf("header timeout=%s",transport.ResponseHeaderTimeout)
 }
 if transport.Proxy!=nil{t.Fatal("unverified proxy routing bypasses DNS pinning")}
 if transport.TLSHandshakeTimeout<=0{t.Fatal("TLS handshake must remain bounded")}
 if transport.DialContext==nil{t.Fatal("public-IP validation dialer missing")}
}

func TestProviderClassifiesTimeoutByHTTPPhase(t *testing.T){
 cases:=[]struct{name string;err error;code string;retryable bool}{
  {"response headers",errors.New("net/http: timeout awaiting response headers"),"RESPONSE_HEADER_TIMEOUT",true},
  {"overall timeout",errors.New("Post https://example.invalid: context deadline exceeded (Client.Timeout exceeded while awaiting headers)"),"REQUEST_TIMEOUT",true},
  {"tls timeout",errors.New("net/http: TLS handshake timeout"),"TLS_TIMEOUT",true},
  {"tcp timeout",&net.OpError{Op:"dial",Net:"tcp",Err:context.DeadlineExceeded},"DIAL_TIMEOUT",true},
  {"dns timeout",&net.DNSError{Err:"i/o timeout",IsTimeout:true},"DNS_LOOKUP_TIMEOUT",true},
  {"dns failed",errors.New("provider DNS lookup failed"),"DNS_LOOKUP_FAILED",false},
  {"blocked IP",errors.New("private or reserved provider IP refused"),"PRIVATE_IP_BLOCKED",false},
  {"certificate",errors.New("x509 certificate unknown authority"),"TLS_HANDSHAKE_FAILED",false},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   code,_,ambiguous:=classifyProviderTransportError(context.Background(),tc.err)
   if code!=tc.code{t.Fatalf("got %s, want %s",code,tc.code)}
   if retryableProviderTransportCode(code)!=tc.retryable{t.Fatalf("wrong retry classification for %s",code)}
   if tc.retryable&&!ambiguous{t.Fatal("failed HTTP phase must retain provider-egress ambiguity")}
  })
 }
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 code,_,_:=classifyProviderTransportError(ctx,errors.New("timeout awaiting response headers"))
 if code!="REQUEST_CANCELLED"{t.Fatalf("Stop must have precedence: %s",code)}
}

func TestProviderTimingCapturesHeaderWaitWithoutSensitiveData(t *testing.T){
 req,err:=http.NewRequest(http.MethodPost,"https://example.invalid/v1/responses",strings.NewReader("{}"))
 if err!=nil{t.Fatal(err)}
 traced,timing:=withProviderTiming(req)
 callbacks:=httptrace.ContextClientTrace(traced.Context())
 if callbacks==nil{t.Fatal("missing HTTP phase trace")}
 callbacks.DNSStart(httptrace.DNSStartInfo{Host:"example.invalid"})
 time.Sleep(2*time.Millisecond)
 callbacks.DNSDone(httptrace.DNSDoneInfo{})
 callbacks.ConnectStart("tcp","example.invalid:443")
 time.Sleep(2*time.Millisecond)
 callbacks.ConnectDone("tcp","example.invalid:443",nil)
 callbacks.TLSHandshakeStart()
 time.Sleep(2*time.Millisecond)
 callbacks.TLSHandshakeDone(tls.ConnectionState{},nil)
 callbacks.GotConn(httptrace.GotConnInfo{})
 time.Sleep(2*time.Millisecond)
 callbacks.WroteRequest(httptrace.WroteRequestInfo{})
 time.Sleep(2*time.Millisecond)
 before:=timing.snapshot()
 if before.DNSMS<1||before.TCPMS<1||before.TLSMS<1||before.UploadMS<1||
     before.HeaderWaitMS<1||before.TTFBMS!=0{
  t.Fatalf("incomplete HTTP phase breakdown before response: %+v",before)
 }
 callbacks.GotFirstResponseByte()
 after:=timing.snapshot()
 if after.TTFBMS<1||after.HeaderWaitMS<before.HeaderWaitMS{
  t.Fatalf("missing TTFB timing: %+v",after)
 }
}

func TestResponsesStreamingActuallyRequestsSSE(t *testing.T){
 canonical:=[]byte(`{"model":"m","input":[{"role":"user","content":"hello"}],"store":false}`)
 outgoing,err:=responsesStreamBody(canonical)
 if err!=nil{t.Fatal(err)}
 var raw map[string]any
 if err=json.Unmarshal(outgoing,&raw);err!=nil{t.Fatal(err)}
 if raw["stream"]!=true||raw["store"]!=false{
  t.Fatalf("SSE request omitted JSON stream:true or changed store semantics: %s",outgoing)
 }
 if strings.Contains(string(canonical),`"stream"`){t.Fatal("mutated source request")}
 if _,err=responsesStreamBody([]byte(`{"model":`));err==nil{t.Fatal("malformed request accepted")}
}
