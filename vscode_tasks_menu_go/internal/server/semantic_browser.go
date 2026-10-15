package server

// Semantic Browser is an opt-in proxy to a separate, unprivileged Chromium worker.
// No remote page HTML/JavaScript is sent to TaskDeck's frontend. The worker must
// be sandboxed independently; a Unix socket alone is NOT a web sandbox.
import (
 "bytes"
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "time"
)

const semanticBrowserRequestLimit = 32 << 10
const semanticBrowserResponseLimit = 1 << 20

type semanticBrowserRequest struct {
 Action string `json:"action"`
 SessionID string `json:"session_id,omitempty"`
 URL string `json:"url,omitempty"`
 ElementID string `json:"element_id,omitempty"`
 Value string `json:"value,omitempty"`
}

func semanticBrowserOwner(r *http.Request) string {
 if p,ok := PrincipalFromContext(r.Context());ok {
  return "user:"+string(p.ProjectID)+":"+string(p.UserID)
 }
 return "single-user"
}

func semanticBrowserSocket() string {
 p := strings.TrimSpace(os.Getenv("TASKDECK_BROWSER_WORKER_SOCKET"))
 if !filepath.IsAbs(p) {return ""}
 if strings.TrimSpace(os.Getenv("TASKDECK_BROWSER_WORKER_TOKEN"))=="" {return ""}
 return p
}

func browserRandomID() (string,error) {
 b:=make([]byte,24)
 if _,err:=rand.Read(b);err!=nil{return "",err}
 return hex.EncodeToString(b),nil
}

func (s *Server) semanticBrowserWorker(ctx context.Context, method, endpoint string, payload any) ([]byte,int,error) {
 socket:=semanticBrowserSocket()
 if socket=="" {return nil,http.StatusServiceUnavailable,errors.New("semantic browser worker is not configured")}
 var body io.Reader
 if payload!=nil {
  raw,err:=json.Marshal(payload)
  if err!=nil{return nil,http.StatusBadRequest,err}
  body=bytes.NewReader(raw)
 }
 ctx,cancel:=context.WithTimeout(ctx,40*time.Second)
 defer cancel()
 transport:=&http.Transport{
  Proxy:nil,
  DisableKeepAlives:true,
  DialContext:func(ctx context.Context,network,address string)(net.Conn,error){
   var dialer net.Dialer
   return dialer.DialContext(ctx,"unix",socket)
  },
 }
 defer transport.CloseIdleConnections()
 req,err:=http.NewRequestWithContext(ctx,method,"http://taskdeck-worker"+endpoint,body)
 if err!=nil{return nil,http.StatusBadRequest,err}
 req.Header.Set("X-TaskDeck-Worker-Token",os.Getenv("TASKDECK_BROWSER_WORKER_TOKEN"))
 req.Header.Set("Content-Type","application/json")
 resp,err:=(&http.Client{Transport:transport,Timeout:40*time.Second}).Do(req)
 if err!=nil{return nil,http.StatusBadGateway,fmt.Errorf("semantic browser worker unavailable: %w",err)}
 defer resp.Body.Close()
 raw,err:=io.ReadAll(io.LimitReader(resp.Body,semanticBrowserResponseLimit+1))
 if err!=nil{return nil,http.StatusBadGateway,err}
 if len(raw)>semanticBrowserResponseLimit {return nil,http.StatusBadGateway,errors.New("semantic browser response exceeds limit")}
 if !json.Valid(raw){return nil,http.StatusBadGateway,errors.New("semantic browser worker returned invalid JSON")}
 return raw,resp.StatusCode,nil
}

func (s *Server) semanticBrowserAPI(w http.ResponseWriter,r *http.Request) {
 w.Header().Set("Cache-Control","no-store")
 // Authorization is also enforced by sharedRoutePermissions BEFORE reaching
 // this handler. Never relay the worker secret or browser cookies.
 if r.Method==http.MethodGet {
  writeJSON(w,http.StatusOK,map[string]any{"enabled":semanticBrowserSocket()!="","kind":"semantic"})
  return
 }
 if r.Method!=http.MethodPost && r.Method!=http.MethodDelete {
  w.Header().Set("Allow","GET, POST, DELETE")
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
 }
 if semanticBrowserSocket()=="" {
  http.Error(w,"Browser worker is not configured",http.StatusServiceUnavailable);return
 }
 var input semanticBrowserRequest
 decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,semanticBrowserRequestLimit))
 decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&input);err!=nil {http.Error(w,"invalid browser request",http.StatusBadRequest);return}
 var trailing any
 if decoder.Decode(&trailing)!=io.EOF {http.Error(w,"invalid trailing request data",http.StatusBadRequest);return}
 owner:=semanticBrowserOwner(r)
 if r.Method==http.MethodDelete{input.Action="close"}
 action:=strings.TrimSpace(input.Action)
 if action=="create" {
  if r.Method!=http.MethodPost||input.SessionID!=""||len(input.URL)>4096 {
   http.Error(w,"invalid create request",http.StatusBadRequest);return
  }
  session,err:=browserRandomID()
  if err!=nil {http.Error(w,"browser session allocation failed",http.StatusInternalServerError);return}
  input.SessionID=session
  raw,status,err:=s.semanticBrowserWorker(r.Context(),http.MethodPost,"/sessions",input)
  if err!=nil {http.Error(w,err.Error(),status);return}
  if status<200||status>=300 {http.Error(w,semanticWorkerError(raw),status);return}
  s.semanticBrowserMu.Lock()
  if s.semanticBrowserOwners==nil{s.semanticBrowserOwners=make(map[string]string)}
  s.semanticBrowserOwners[session]=owner
  s.semanticBrowserMu.Unlock()
  semanticWriteJSON(w,raw)
  return
 }
 if len(input.SessionID)!=48 || len(strings.Trim(input.SessionID,"0123456789abcdef"))!=0 {
  http.Error(w,"invalid session ID",http.StatusBadRequest);return
 }
 s.semanticBrowserMu.Lock()
 current,exists:=s.semanticBrowserOwners[input.SessionID]
 s.semanticBrowserMu.Unlock()
 if !exists||current!=owner {http.Error(w,"browser session not found",http.StatusNotFound);return}
 if action=="close"{
  raw,status,err:=s.semanticBrowserWorker(r.Context(),http.MethodDelete,"/sessions/"+input.SessionID,nil)
  if err!=nil {http.Error(w,err.Error(),status);return}
  if status>=200&&status<300 {
   s.semanticBrowserMu.Lock();delete(s.semanticBrowserOwners,input.SessionID);s.semanticBrowserMu.Unlock()
   semanticWriteJSON(w,raw);return
  }
  http.Error(w,semanticWorkerError(raw),status);return
 }
 switch action{
 case "navigate","back","forward","reload","snapshot","click","fill","press","check","select","scroll":
 default:http.Error(w,"unsupported browser action",http.StatusBadRequest);return
 }
 if len(input.ElementID)>100 || len(input.URL)>4096 || len(input.Value)>16384 {
  http.Error(w,"browser action exceeds limits",http.StatusBadRequest);return
 }
 raw,status,err:=s.semanticBrowserWorker(r.Context(),http.MethodPost,"/sessions/"+input.SessionID,input)
 if err!=nil {http.Error(w,err.Error(),status);return}
 if status==http.StatusNotFound {
  s.semanticBrowserMu.Lock();delete(s.semanticBrowserOwners,input.SessionID);s.semanticBrowserMu.Unlock()
 }
 if status<200||status>=300 {http.Error(w,semanticWorkerError(raw),status);return}
 semanticWriteJSON(w,raw)
}

func semanticWorkerError(raw []byte)string {
 var obj struct{ Error string `json:"error"` }
 if json.Unmarshal(raw,&obj)==nil && obj.Error!="" {
  if len(obj.Error)>400{return obj.Error[:400]}
  return obj.Error
 }
 return "Browser worker request failed"
}
func semanticWriteJSON(w http.ResponseWriter,raw []byte){
 w.Header().Set("Content-Type","application/json; charset=utf-8")
 w.WriteHeader(http.StatusOK)
 _,_=w.Write(raw)
}
