package server

import (
 "context"
 "errors"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
)

func TestAIProviderHTTPErrorPreservesSafeStatusAndCode(t *testing.T){
 body:=[]byte(`{"error":{"code":404,"message":"Authorization: Bearer extremely_private_prompt_token","status":"NOT_FOUND"}}`)
 code:=aiProviderErrorCode(body)
 if code!="NOT_FOUND"{t.Fatalf("Gemini error status=%q",code)}
 w:=httptest.NewRecorder()
 aiWriteProviderFailure(w,"gemini",404,code,1250*time.Millisecond)
 if w.Code!=http.StatusBadGateway{t.Fatalf("HTTP status=%d",w.Code)}
 if w.Header().Get("X-TaskDeck-AI-Provider-Status")!="404"||
  w.Header().Get("X-TaskDeck-AI-Error-Code")!="NOT_FOUND"||
  w.Header().Get("X-TaskDeck-AI-Duration-Ms")!="1250"{t.Fatalf("missing debug headers %v",w.Header())}
 if strings.Contains(w.Body.String(),"extremely_private_prompt_token"){t.Fatal("provider error body leaked")}
 if !strings.Contains(w.Body.String(),"model not available"){t.Fatalf("unhelpful status: %s",w.Body.String())}
}
func TestAIProviderErrorCodeNeverCopiesUntrustedMessages(t *testing.T){
 for _,tc:=range []struct{body string;want string}{
  {`{"error":{"status":"RESOURCE_EXHAUSTED","message":"user token secret"}}`,"RESOURCE_EXHAUSTED"},
  {`{"error":{"type":"insufficient_quota","message":"password=hunter2"}}`,"insufficient_quota"},
  {`{"error":{"code":"rate_limit_exceeded"}}`,"rate_limit_exceeded"},
  {`{"error":{"status":"bad code with secrets"}}`,""},
  {`{"error":{"status":"
Authorization: Bearer token"}}`,""},
  {"not-json",""},
 }{
  if got:=aiProviderErrorCode([]byte(tc.body));got!=tc.want{t.Errorf("code=%q want %q from %.70s",got,tc.want,tc.body)}
 }
}
func TestAIProviderTimeoutAndCancellationDiagnostics(t *testing.T){
 tmo,done:=context.WithDeadline(context.Background(),time.Now().Add(-time.Second))
 defer done()
 w:=httptest.NewRecorder()
 aiWriteProviderNetworkFailure(w,"gemini",context.DeadlineExceeded,tmo,25000*time.Millisecond)
 if w.Code!=http.StatusGatewayTimeout||
  w.Header().Get("X-TaskDeck-AI-Error-Code")!="provider_timeout"||
  w.Header().Get("X-TaskDeck-AI-Duration-Ms")!="25000"{
  t.Fatalf("timeout diagnostics: status=%d headers=%v",w.Code,w.Header())
 }
 c,cancel:=context.WithCancel(context.Background());cancel()
 w=httptest.NewRecorder()
 aiWriteProviderNetworkFailure(w,"gemini",context.Canceled,c,100*time.Millisecond)
 if w.Code!=http.StatusRequestTimeout||w.Header().Get("X-TaskDeck-AI-Error-Code")!="request_canceled"{
  t.Fatalf("cancellation: %d %v",w.Code,w.Header())
 }
 w=httptest.NewRecorder()
 aiWriteProviderNetworkFailure(w,"gemini",errors.New("private hostname api-secret.example"),context.Background(),10*time.Millisecond)
 if w.Code!=http.StatusBadGateway||strings.Contains(w.Body.String(),"api-secret.example"){t.Fatal("network error leaked internal hostname")}
}
func TestAIQuestionDefaultsForBlankQuestion(t *testing.T){
 if got:=aiNormalizeQuestion("explain","  ");!strings.Contains(got,"last terminal command"){t.Fatal(got)}
 if got:=aiNormalizeQuestion("suggest","");!strings.Contains(got,"diagnostic"){t.Fatal(got)}
 if got:=aiNormalizeQuestion("plan","");!strings.Contains(got,"read-only"){t.Fatal(got)}
 if got:=aiNormalizeQuestion("explain","what can you do for me?");got!="what can you do for me?"{t.Fatal(got)}
}
