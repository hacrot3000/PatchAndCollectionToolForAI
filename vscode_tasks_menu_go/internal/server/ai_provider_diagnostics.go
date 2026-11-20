package server

// Provider diagnostics intentionally expose only numeric status and an
// allowlisted error code. A provider error body can echo prompts, keys or
// other private data and must never reach the browser or TaskDeck logs.
import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net"
 "net/http"
 "regexp"
 "strconv"
 "strings"
 "time"
)

var aiSafeProviderCode=regexp.MustCompile("^[A-Za-z0-9_.-]{1,64}$")

func aiProviderErrorCode(body []byte)string{
 if len(body)>4096{return ""}
 var result struct{
  Error struct{
   Code json.RawMessage `json:"code"`
   Status string `json:"status"`
   Type string `json:"type"`
  } `json:"error"`
 }
 if json.Unmarshal(body,&result)!=nil{return ""}
 candidate:=result.Error.Status
 if candidate==""{candidate=result.Error.Type}
 if candidate==""&&len(result.Error.Code)>0{
  var code string
  if json.Unmarshal(result.Error.Code,&code)==nil{candidate=code}else{
   var num int
   if json.Unmarshal(result.Error.Code,&num)==nil&&num>=100&&num<10000{candidate=strconv.Itoa(num)}
  }
 }
 if !aiSafeProviderCode.MatchString(candidate){return ""}
 // Accept only known provider error enums. Arbitrary status strings may
 // accidentally encode credentials or private content despite their shape.
 switch strings.ToLower(candidate) {
 case "invalid_argument","not_found","resource_exhausted","permission_denied",
  "unauthenticated","failed_precondition","unavailable","internal",
  "deadline_exceeded","cancelled","aborted","out_of_range",
  "insufficient_quota","rate_limit_exceeded","invalid_request_error",
  "authentication_error","permission_error","not_found_error",
  "overloaded_error","api_error","service_unavailable","model_not_found",
  "invalid_api_key","billing_not_active","quota_exceeded","429","401",
  "403","404","400","500","503":
  return candidate
 }
 return ""
}

func aiProviderStatusSummary(status int)string{
 switch status{
 case http.StatusBadRequest:return "invalid request or model parameters"
 case http.StatusUnauthorized:return "API key rejected by provider"
 case http.StatusForbidden:return "API key lacks provider/model access"
 case http.StatusNotFound:return "model not available for this API or account"
 case http.StatusTooManyRequests:return "provider rate limit or quota exceeded"
 case http.StatusServiceUnavailable:return "provider service unavailable"
 default:
  if status>=500{return "provider server error"}
  return "provider rejected request"
 }
}

func aiSetProviderDiagnosticHeaders(w http.ResponseWriter,status int,code string,elapsed time.Duration){
 if status>0{w.Header().Set("X-TaskDeck-AI-Provider-Status",strconv.Itoa(status))}
 if aiSafeProviderCode.MatchString(code){w.Header().Set("X-TaskDeck-AI-Error-Code",code)}
 ms:=elapsed.Milliseconds()
 if ms<0{ms=0}
 w.Header().Set("X-TaskDeck-AI-Duration-Ms",strconv.FormatInt(ms,10))
 w.Header().Set("Cache-Control","no-store")
}

func aiWriteProviderFailure(w http.ResponseWriter,provider string,status int,code string,elapsed time.Duration){
 aiSetProviderDiagnosticHeaders(w,status,code,elapsed)
 http.Error(w,fmt.Sprintf("AI provider %s HTTP %d: %s",provider,status,aiProviderStatusSummary(status)),http.StatusBadGateway)
}

func aiWriteProviderNetworkFailure(w http.ResponseWriter,provider string,err error,ctx context.Context,elapsed time.Duration){
 timeout:=errors.Is(err,context.DeadlineExceeded)||errors.Is(ctx.Err(),context.DeadlineExceeded)
 var nerr net.Error
 if errors.As(err,&nerr)&&nerr.Timeout(){timeout=true}
 if timeout{
  aiSetProviderDiagnosticHeaders(w,0,"provider_timeout",elapsed)
  http.Error(w,"AI provider "+provider+" timed out; increase request timeout in Settings",http.StatusGatewayTimeout)
  return
 }
 if errors.Is(err,context.Canceled)||errors.Is(ctx.Err(),context.Canceled){
  aiSetProviderDiagnosticHeaders(w,0,"request_canceled",elapsed)
  http.Error(w,"AI request canceled",http.StatusRequestTimeout)
  return
 }
 aiSetProviderDiagnosticHeaders(w,0,"provider_network_error",elapsed)
 http.Error(w,"AI provider "+provider+" network request failed",http.StatusBadGateway)
}

func aiNormalizeQuestion(mode,question string)string{
 question=strings.TrimSpace(question)
 if question!=""{return question}
 switch mode{
 case "suggest":return "Suggest safe next diagnostic steps for the terminal command and output."
 case "plan":return "Propose a read-only investigation plan based on the terminal command and output. Do not execute commands."
 default:return "Explain the last terminal command, its output, and any error. Do not run commands."
 }
}
