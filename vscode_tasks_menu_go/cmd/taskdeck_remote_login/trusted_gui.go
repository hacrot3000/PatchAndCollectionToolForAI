package main

import (
 "context"
 "crypto/rand"
 "crypto/subtle"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/url"
 "os/exec"
 "runtime"
 "sync"
 "time"
)

// The GUI is bundled in the independently verified native client, NEVER
// loaded from a potentially compromised PHP Portal. Only fixed encrypted,
// host-authorized read-only application operations are available.
type trustedGUIActions interface {
 Ping(context.Context) error
 GitStatus(context.Context) (string,error)
}

const trustedGUIMaxActions = 4 // Relay's fixed maximum application rounds.
const trustedGUIBodyLimit = 1024

type trustedGUI struct {
 actions trustedGUIActions
 origin,token string
 closeFn func()
 mu sync.Mutex
 remaining int
 active,closed bool
 closeOnce sync.Once
}

func newTrustedGUI(origin,token string,actions trustedGUIActions,closeFn func())(*trustedGUI,error){
 u,err:=url.Parse(origin)
 if err!=nil || u.Scheme!="http" || u.Hostname()!="127.0.0.1" ||
  u.Port()=="" || u.Host!="127.0.0.1:"+u.Port() || u.Path!="" ||
  u.RawQuery!="" || u.Fragment!="" || u.User!=nil ||
  len(token)!=64 || actions==nil || closeFn==nil {
  return nil,errors.New("untrusted local GUI configuration")
 }
 key,err:=hex.DecodeString(token)
 if err!=nil || len(key)!=32 || hex.EncodeToString(key)!=token {
  return nil,errors.New("invalid local GUI capability")
 }
 return &trustedGUI{actions:actions,origin:origin,token:token,closeFn:closeFn,
  remaining:trustedGUIMaxActions},nil
}

func (ui *trustedGUI) stop(){
 ui.mu.Lock()
 ui.closed=true
 ui.mu.Unlock()
 ui.closeOnce.Do(ui.closeFn)
}

func (ui *trustedGUI) ServeHTTP(w http.ResponseWriter,r *http.Request){
 // DNS rebinding defense: localhost binding alone does not guarantee
 // the request came from the trusted local page.
 host:="127.0.0.1:"+ui.origin[len("http://127.0.0.1:"):]
 w.Header().Set("Cache-Control","no-store, max-age=0")
 w.Header().Set("Pragma","no-cache")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("X-Frame-Options","DENY")
 w.Header().Set("Referrer-Policy","no-referrer")
 w.Header().Set("Cross-Origin-Resource-Policy","same-origin")
 w.Header().Set("Cross-Origin-Opener-Policy","same-origin")
 w.Header().Set("Content-Security-Policy",
  "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; "+
  "img-src 'none'; base-uri 'none'; form-action 'none'; object-src 'none'; frame-ancestors 'none'")
 if r.Host!=host || r.URL.Host!="" || r.URL.RawQuery!="" ||
  r.Header.Get("Sec-Fetch-Site")=="cross-site" {
  http.Error(w,"forbidden",http.StatusForbidden);return
 }
 switch r.URL.Path {
 case "/":
  if r.Method!=http.MethodGet{http.Error(w,"method not allowed",405);return}
  w.Header().Set("Content-Type","text/html; charset=utf-8")
  _,_=io.WriteString(w,trustedGUIHTML)
 case "/app.js":
  if r.Method!=http.MethodGet{http.Error(w,"method not allowed",405);return}
  w.Header().Set("Content-Type","text/javascript; charset=utf-8")
  _,_=io.WriteString(w,trustedGUIJavaScript)
 case "/style.css":
  if r.Method!=http.MethodGet{http.Error(w,"method not allowed",405);return}
  w.Header().Set("Content-Type","text/css; charset=utf-8")
  _,_=io.WriteString(w,trustedGUICSS)
 case "/api/ping","/api/git-status","/api/close":
  ui.action(w,r)
 default:
  http.NotFound(w,r)
 }
}

func (ui *trustedGUI) authorizedPost(r *http.Request)bool{
 if r.Method!=http.MethodPost || r.Header.Get("Origin")!=ui.origin ||
  r.Header.Get("Content-Type")!="" {return false}
 raw:=r.Header.Get("X-TaskDeck-Local-Capability")
 want,_:=hex.DecodeString(ui.token)
 got,err:=hex.DecodeString(raw)
 return err==nil && len(got)==32 && subtle.ConstantTimeCompare(want,got)==1
}

func (ui *trustedGUI) action(w http.ResponseWriter,r *http.Request){
 if !ui.authorizedPost(r){http.Error(w,"forbidden",403);return}
 // No remote paths, shell strings, arguments, JSON payloads or Git options.
 data,err:=io.ReadAll(io.LimitReader(r.Body,trustedGUIBodyLimit+1))
 if err!=nil || len(data)!=0 {
  http.Error(w,"unexpected request body",400);return
 }
 w.Header().Set("Content-Type","application/json; charset=utf-8")
 if r.URL.Path=="/api/close" {
  _=json.NewEncoder(w).Encode(map[string]any{"ok":true,"closed":true})
  ui.stop();return
 }
 ui.mu.Lock()
 if ui.closed||ui.remaining==0 {
  ui.mu.Unlock()
  http.Error(w,"session closed or four-action limit reached",410);return
 }
 if ui.active{
  ui.mu.Unlock()
  http.Error(w,"another operation is active",409);return
 }
 ui.active=true
 ui.remaining--
 remaining:=ui.remaining
 ui.mu.Unlock()
 defer func(){ui.mu.Lock();ui.active=false;ui.mu.Unlock()}()
 ctx,cancel:=context.WithTimeout(r.Context(),16*time.Second)
 defer cancel()
 if r.URL.Path=="/api/ping"{
  err=ui.actions.Ping(ctx)
  if err==nil {
   _=json.NewEncoder(w).Encode(map[string]any{
    "ok":true,"message":"TaskDeck Agent đã xác thực qua E2EE.","remaining":remaining,
   })
   return
  }
 }else{
  var status string
  status,err=ui.actions.GitStatus(ctx)
  if err==nil{
   _=json.NewEncoder(w).Encode(map[string]any{
    "ok":true,"status":status,"remaining":remaining,
   })
   return
  }
 }
 // Poisoned or revoked E2EE sessions are never downgraded to local HTTP.
 ui.mu.Lock()
 ui.closed=true
 ui.mu.Unlock()
 http.Error(w,"TaskDeck denied action or E2EE session expired",403)
}

func runTrustedRemoteGUI(ctx context.Context,actions trustedGUIActions,output io.Writer)error{
 return runTrustedRemoteGUIWithOpener(ctx,actions,output,openTrustedGUIBrowser)
}

// The opener is supplied only by this trusted binary. A test-specific
// no-op lets integration tests exercise a REAL loopback TCP socket without
// launching a user's browser or contacting the Portal.
func runTrustedRemoteGUIWithOpener(ctx context.Context,actions trustedGUIActions,
 output io.Writer,opener func(string))error{
 if ctx==nil||ctx.Err()!=nil||actions==nil||output==nil{
  return errors.New("trusted GUI session unavailable")
 }
 listener,err:=net.Listen("tcp4","127.0.0.1:0")
 if err!=nil{return err}
 defer listener.Close()
 var secret [32]byte
 if _,err=rand.Read(secret[:]);err!=nil{return err}
 token:=hex.EncodeToString(secret[:])
 origin:="http://"+listener.Addr().String()
 guiCtx,stop:=context.WithCancel(ctx)
 defer stop()
 ui,err:=newTrustedGUI(origin,token,actions,stop)
 if err!=nil{return err}
 srv:=&http.Server{
  Handler:ui,ReadHeaderTimeout:3*time.Second,ReadTimeout:10*time.Second,
  WriteTimeout:18*time.Second,IdleTimeout:20*time.Second,
 }
 done:=make(chan error,1)
 go func(){done<-srv.Serve(listener)}()
 address:=origin+"/#"+token // URL fragment NEVER reaches HTTP or Relay.
 fmt.Fprintln(output,"TaskDeck GUI tin cậy (chỉ trên máy client):",address)
 fmt.Fprintln(output,"KHÔNG gửi URL này hoặc mật khẩu TaskDeck vào PHP Portal.")
 fmt.Fprintln(output,"GUI giới hạn session.ping, git.status và bốn yêu cầu/phiên.")
 if opener!=nil {opener(address)}
 select{
 case <-guiCtx.Done():
 case err=<-done:
  if err!=nil&&!errors.Is(err,http.ErrServerClosed){return err}
 }
 ui.stop()
 closeCtx,cancel:=context.WithTimeout(context.Background(),2*time.Second)
 defer cancel()
 _=srv.Shutdown(closeCtx)
 return nil
}

func openTrustedGUIBrowser(target string){
 var command *exec.Cmd
 switch runtime.GOOS{
 case "darwin":command=exec.Command("open",target)
 case "linux":command=exec.Command("xdg-open",target)
 default:return
 }
 // Optional convenience only; no shell evaluation or remote download.
 _=command.Start()
 if command.Process!=nil{_=command.Process.Release()}
}
