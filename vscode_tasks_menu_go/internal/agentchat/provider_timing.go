package agentchat

import (
 "net/http"
 "net/http/httptrace"
 "crypto/tls"
 "sync"
 "time"
)

// ProviderTiming is safe per-request HTTP phase metadata. Values are elapsed
// milliseconds, not destination addresses or body content. Zero means either
// unobserved or shorter than a millisecond. The fields are useful even after a
// timeout before any response headers are received.
type ProviderTiming struct {
 DNSMS int64
 TCPMS int64
 TLSMS int64
 UploadMS int64
 HeaderWaitMS int64
 TTFBMS int64
}

type providerTiming struct {
 mu sync.Mutex
 started time.Time
 dnsStart, dnsDone time.Time
 tcpStart, tcpDone time.Time
 tlsStart, tlsDone time.Time
 connected, wrote, firstByte time.Time
}

func millisBetween(start, end time.Time) int64 {
 if start.IsZero()||end.IsZero()||end.Before(start){return 0}
 return end.Sub(start).Milliseconds()
}

// withProviderTiming attaches a standard Go httptrace to the existing
// authenticated, DNS-pinned request; it does not alter the HTTP transport,
// connection selection, proxy policy, TLS verification or payload bytes.
func withProviderTiming(req *http.Request)(*http.Request,*providerTiming){
 t:=&providerTiming{started:time.Now()}
 trace:=&httptrace.ClientTrace{
  DNSStart:func(httptrace.DNSStartInfo){t.mu.Lock();t.dnsStart=time.Now();t.mu.Unlock()},
  DNSDone:func(httptrace.DNSDoneInfo){t.mu.Lock();t.dnsDone=time.Now();t.mu.Unlock()},
  ConnectStart:func(_, _ string){t.mu.Lock();t.tcpStart=time.Now();t.mu.Unlock()},
  ConnectDone:func(_, _ string,_ error){t.mu.Lock();t.tcpDone=time.Now();t.mu.Unlock()},
  TLSHandshakeStart:func(){t.mu.Lock();t.tlsStart=time.Now();t.mu.Unlock()},
  TLSHandshakeDone:func(_ tls.ConnectionState,_ error){t.mu.Lock();t.tlsDone=time.Now();t.mu.Unlock()},
  GotConn:func(httptrace.GotConnInfo){t.mu.Lock();t.connected=time.Now();t.mu.Unlock()},
  WroteRequest:func(httptrace.WroteRequestInfo){t.mu.Lock();t.wrote=time.Now();t.mu.Unlock()},
  GotFirstResponseByte:func(){t.mu.Lock();t.firstByte=time.Now();t.mu.Unlock()},
 }
 return req.WithContext(httptrace.WithClientTrace(req.Context(),trace)),t
}

func (t *providerTiming) snapshot() ProviderTiming {
 if t==nil{return ProviderTiming{}}
 t.mu.Lock()
 defer t.mu.Unlock()
 now:=time.Now()
 end:=func(done time.Time)time.Time{if done.IsZero(){return now};return done}
 p:=ProviderTiming{
  DNSMS:millisBetween(t.dnsStart,end(t.dnsDone)),
  TCPMS:millisBetween(t.tcpStart,end(t.tcpDone)),
  TLSMS:millisBetween(t.tlsStart,end(t.tlsDone)),
  UploadMS:millisBetween(t.connected,end(t.wrote)),
  TTFBMS:millisBetween(t.started,t.firstByte),
 }
 if !t.wrote.IsZero(){
  p.HeaderWaitMS=millisBetween(t.wrote,end(t.firstByte))
 }
 return p
}
