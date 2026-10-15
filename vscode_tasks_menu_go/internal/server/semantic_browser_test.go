package server

import (
 "context"
 "encoding/json"
 "io"
 "net"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
 "fmt"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

func browserHTTP(t *testing.T,s *Server,method,body string,principal *identity.Principal)*httptest.ResponseRecorder{
 t.Helper()
 req:=httptest.NewRequest(method,"/api/semantic-browser",strings.NewReader(body))
 if principal!=nil{req=req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},*principal))}
 rec:=httptest.NewRecorder()
 s.semanticBrowserAPI(rec,req)
 return rec
}

func TestSemanticBrowserDisabledByDefault(t *testing.T){
 t.Setenv("TASKDECK_BROWSER_WORKER_SOCKET","")
 t.Setenv("TASKDECK_BROWSER_WORKER_TOKEN","")
 s:=&Server{}
 status:=browserHTTP(t,s,"GET","",nil)
 if status.Code!=http.StatusOK||!strings.Contains(status.Body.String(),`"enabled":false`){t.Fatalf("disabled status: %d %s",status.Code,status.Body.String())}
 response:=browserHTTP(t,s,"POST",`{"action":"create","url":"https://example.com"}`,nil)
 if response.Code!=http.StatusServiceUnavailable{t.Fatalf("disabled worker must refuse create: %d",response.Code)}
}

func TestSemanticBrowserPermissionMapping(t *testing.T){
 r:=httptest.NewRequest("POST","/api/semantic-browser",nil)
 permissions:=sharedRoutePermissions(r)
 if len(permissions)!=1||permissions[0]!=identity.PermissionBrowserUse{t.Fatalf("incorrect permissions: %v",permissions)}
 if !identity.KnownPermission(identity.PermissionBrowserUse){t.Fatal("browser permission is not registered")}
}

func TestSemanticBrowserSessionOwnerAndWorkerProxy(t *testing.T){
 sock:=filepath.Join(t.TempDir(),"worker.sock")
 listener,err:=net.Listen("unix",sock)
 if err!=nil{t.Fatal(err)}
 defer listener.Close()
 var forwardedRevision int
 var workerToken string
 worker:=&http.Server{Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  workerToken=r.Header.Get("X-TaskDeck-Worker-Token")
  w.Header().Set("Content-Type","application/json")
  if r.Method=="DELETE"{_,_=io.WriteString(w,`{"ok":true}`);return}
  var input semanticBrowserRequest
  if err:=json.NewDecoder(r.Body).Decode(&input);err!=nil{http.Error(w,"invalid worker input",400);return}
  if input.Action=="click"{forwardedRevision=input.Revision}
  _=json.NewEncoder(w).Encode(map[string]any{"session_id":input.SessionID,"revision":3,"url":"https://example.com","elements":[]any{}})
 })}
 go func(){_ =worker.Serve(listener)}()
 defer worker.Close()
 t.Setenv("TASKDECK_BROWSER_WORKER_SOCKET",sock)
 t.Setenv("TASKDECK_BROWSER_WORKER_TOKEN",strings.Repeat("a",32))
 alice:=&identity.Principal{UserID:"alice",ProjectID:"project-1"}
 bob:=&identity.Principal{UserID:"bob",ProjectID:"project-1"}
 s:=&Server{}
 created:=browserHTTP(t,s,"POST",`{"action":"create","url":"https://example.com"}`,alice)
 if created.Code!=200{t.Fatalf("create failed: %d %s",created.Code,created.Body.String())}
 var result semanticBrowserRequest
 if err:=json.Unmarshal(created.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if len(result.SessionID)!=48{t.Fatalf("unexpected session ID %q",result.SessionID)}
 action:=`{"session_id":"`+result.SessionID+`","action":"click","element_id":"td-sem-1","revision":2}`
 denied:=browserHTTP(t,s,"POST",action,bob)
 if denied.Code!=http.StatusNotFound{t.Fatalf("cross-user session must be hidden: %d",denied.Code)}
 allowed:=browserHTTP(t,s,"POST",action,alice)
 if allowed.Code!=200||forwardedRevision!=2{t.Fatalf("worker action failed: %d revision %d",allowed.Code,forwardedRevision)}
 if workerToken!=strings.Repeat("a",32){t.Fatal("worker secret was not sent over Unix transport")}
 closed:=browserHTTP(t,s,"DELETE",`{"session_id":"`+result.SessionID+`"}`,alice)
 if closed.Code!=200{t.Fatalf("close failed: %d",closed.Code)}
 gone:=browserHTTP(t,s,"POST",action,alice)
 if gone.Code!=http.StatusNotFound{t.Fatalf("closed session still accessible: %d",gone.Code)}
}

func TestSemanticBrowserRejectsUntrustedSessionIDs(t *testing.T){
 t.Setenv("TASKDECK_BROWSER_WORKER_SOCKET",filepath.Join(os.TempDir(),"missing-taskdeck-worker.sock"))
 t.Setenv("TASKDECK_BROWSER_WORKER_TOKEN",strings.Repeat("b",32))
 s:=&Server{}
 bad:=browserHTTP(t,s,"POST",`{"action":"click","session_id":"../../api/git","element_id":"whatever"}`,nil)
 if bad.Code!=http.StatusBadRequest{t.Fatalf("must reject forged path: %d",bad.Code)}
}

func TestSemanticBrowserLimitsOwnedSessionsBeforeWorkerCall(t *testing.T){
 t.Setenv("TASKDECK_BROWSER_WORKER_SOCKET",filepath.Join(os.TempDir(),"missing-browser-worker.sock"))
 t.Setenv("TASKDECK_BROWSER_WORKER_TOKEN",strings.Repeat("c",32))
 s:=&Server{semanticBrowserOwners:make(map[string]semanticBrowserOwnership)}
 for i:=0;i<4;i++{
  s.semanticBrowserOwners[fmt.Sprintf("%048x",i)]=semanticBrowserOwnership{owner:"single-user",seen:time.Now()}
 }
 response:=browserHTTP(t,s,"POST",`{"action":"create","url":"https://example.com"}`,nil)
 if response.Code!=http.StatusTooManyRequests{t.Fatalf("tab cap not enforced: %d",response.Code)}
 // Expired ownership records must not block a fresh create (worker may still
 // be unavailable; a 502 is expected instead of a quota 429).
 for id:=range s.semanticBrowserOwners {s.semanticBrowserOwners[id]=semanticBrowserOwnership{owner:"single-user",seen:time.Now().Add(-20*time.Minute)}}
 again:=browserHTTP(t,s,"POST",`{"action":"create","url":"https://example.com"}`,nil)
 if again.Code==http.StatusTooManyRequests{t.Fatal("expired owners were not pruned")}
}

func TestSemanticBrowserAtomicLoginBatchIsForwardedWithoutSecretLogging(t *testing.T){
 socket:=filepath.Join(t.TempDir(),"forms.sock")
 listener,err:=net.Listen("unix",socket)
 if err!=nil{t.Fatal(err)}
 defer listener.Close()
 var forwarded semanticBrowserRequest
 server:=&http.Server{Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if err:=json.NewDecoder(r.Body).Decode(&forwarded);err!=nil{http.Error(w,"invalid",400);return}
  w.Header().Set("Content-Type","application/json")
  _,_=io.WriteString(w,`{"session_id":"`+forwarded.SessionID+`","revision":2,"elements":[]}`)
 })}
 go func(){_=server.Serve(listener)}()
 defer server.Close()
 t.Setenv("TASKDECK_BROWSER_WORKER_SOCKET",socket)
 t.Setenv("TASKDECK_BROWSER_WORKER_TOKEN",strings.Repeat("z",32))
 session:=strings.Repeat("f",48)
 s:=&Server{semanticBrowserOwners:map[string]semanticBrowserOwnership{session:{owner:"single-user",seen:time.Now()}}}
 req:=`{"action":"interact","session_id":"`+session+`","revision":2,"frame_index":1,"target_action":"click","element_id":"td-sem-4","field_updates":[{"kind":"fill","element_id":"td-sem-1","frame_index":1,"value":"alice"},{"kind":"fill","element_id":"td-sem-2","frame_index":1,"value":"example-password"}]}`
 response:=browserHTTP(t,s,"POST",req,nil)
 if response.Code!=http.StatusOK{t.Fatalf("batch failed %d: %s",response.Code,response.Body.String())}
 if forwarded.Action!="interact"||forwarded.TargetAction!="click"||forwarded.FrameIndex!=1||forwarded.Revision!=2||len(forwarded.FieldUpdates)!=2{t.Fatalf("malformed batch: %+v",forwarded)}
 if forwarded.FieldUpdates[1].Value!="example-password"{t.Fatal("password was lost before the worker could fill the real website input")}
 if strings.Contains(response.Body.String(),"example-password"){t.Fatal("sensitive field value leaked in frontend response")}
}
