package main

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

// All native GUI operations are individually protected at the localhost
// transport boundary; a compromised PHP Portal never becomes a TaskDeck
// principal simply by sending a syntactically valid typed action.
func TestNativeGUICompromisedPHPMatrixDeniedBeforeAnyAgentCall(t *testing.T) {
 const attacker="https://compromised-relay.example"
 const handle="11111111111111111111111111111111"
 actions:=[]guiRequest{
  {Operation:"files_list",Path:"."},
  {Operation:"file_view",Path:"notes.txt"},
  {Operation:"file_lines",Path:"notes.txt",Offset:1},
  {Operation:"file_find_lines",Path:"notes.txt",Literal:"secret"},
  {Operation:"file_edit_review",Path:"notes.txt"},
  {Operation:"file_edit_execute",Path:"notes.txt",Confirmation:"EDIT notes.txt 123456789abc",Content:"evil"},
  {Operation:"line_write_review",Path:"notes.txt",Mode:"delete",Offset:1},
  {Operation:"line_write_execute",Path:"notes.txt",Mode:"delete",Offset:1,Confirmation:"DELETE LINE 1 123456789abc"},
  {Operation:"git_status"},
  {Operation:"git_branches"},
  {Operation:"git_log"},
  {Operation:"git_diff",Path:"notes.txt"},
  {Operation:"git_staged",Path:"notes.txt"},
  {Operation:"git_head",Path:"notes.txt"},
  {Operation:"git_review",Path:"notes.txt"},
  {Operation:"git_write_review",Mode:"stage",Path:"notes.txt"},
  {Operation:"git_write_execute",Mode:"stage",Path:"notes.txt",Confirmation:"STAGE notes.txt"},
  {Operation:"tasks"},
  {Operation:"task_review",Mode:"run",TaskID:1},
  {Operation:"task_execute",Mode:"run",TaskID:1,Confirmation:"RUN TASK 1"},
  {Operation:"task_status",Handle:handle},
  {Operation:"task_output",Handle:handle,Offset:0},
  {Operation:"task_cancel",Handle:handle,Confirmation:"CANCEL TASK "+handle},
 }
 for _,tc:=range []struct{name,origin,key string}{
  {"compromised-PHP-origin-with-stolen-local-key",attacker,strings.Repeat("a",64)},
  {"missing-Origin-header-with-stolen-local-key","",strings.Repeat("a",64)},
  {"trusted-origin-without-native-key","http://127.0.0.1:42119",""},
  {"trusted-origin-with-forged-native-key","http://127.0.0.1:42119",strings.Repeat("b",64)},
 }{
  t.Run(tc.name,func(t *testing.T){
   g,agent:=newTestNativeGUI()
   for _,action:=range actions{
    status,response:=guiRequestTest(t,g,action,tc.origin,tc.key)
    if status!=http.StatusForbidden||response.OK||response.Text!=""||
     response.Confirm!=""||response.Line!=""||len(agent.calls)!=0||g.used!=0{
     t.Fatalf("PHP/Relay crossed native GUI auth gate for %q: status=%d response=%+v agent=%v used=%d",
      action.Operation,status,response,agent.calls,g.used)
    }
   }
  })
 }
}

func TestNativeGUIHostAndRequestSmugglingRejectedBeforeE2EE(t *testing.T){
 g,agent:=newTestNativeGUI()
 for _,tc:=range []struct{name,method,host,origin,key,contentType,body string}{
  {"wrong-host",http.MethodPost,"evil.example",g.origin,g.key,"application/json",`{"operation":"git_status"}`},
  {"same-site-subdomain",http.MethodPost,"127.0.0.1.evil.example",g.origin,g.key,"application/json",`{"operation":"git_status"}`},
  {"foreign-origin",http.MethodPost,"127.0.0.1:42119","https://compromised-php.example",g.key,"application/json",`{"operation":"git_status"}`},
  {"absent-local-key",http.MethodPost,"127.0.0.1:42119",g.origin,"","application/json",`{"operation":"git_status"}`},
  {"get-mutation",http.MethodGet,"127.0.0.1:42119",g.origin,g.key,"application/json",`{"operation":"git_status"}`},
  {"form-content-type",http.MethodPost,"127.0.0.1:42119",g.origin,g.key,"application/x-www-form-urlencoded","operation=git_status"},
  {"json-extra-fields",http.MethodPost,"127.0.0.1:42119",g.origin,g.key,"application/json",`{"operation":"git_status","shell":"sh"}`},
  {"double-json",http.MethodPost,"127.0.0.1:42119",g.origin,g.key,"application/json",`{"operation":"git_status"}{"operation":"task_execute"}`},
  {"malformed-json",http.MethodPost,"127.0.0.1:42119",g.origin,g.key,"application/json",`{"operation":`},
  {"oversized-body",http.MethodPost,"127.0.0.1:42119",g.origin,g.key,"application/json",strings.Repeat("x",8193)},
 }{
  t.Run(tc.name,func(t *testing.T){
   req:=httptest.NewRequest(tc.method,g.origin+"/api/action",bytes.NewBufferString(tc.body))
   req.Host=tc.host
   req.Header.Set("Origin",tc.origin)
   req.Header.Set("X-TaskDeck-Local-Key",tc.key)
   req.Header.Set("Content-Type",tc.contentType)
   result:=httptest.NewRecorder()
   g.handler().ServeHTTP(result,req)
   if result.Code!=http.StatusForbidden&&result.Code!=http.StatusBadRequest{
    t.Fatalf("hostile HTTP request accepted: status=%d body=%s",result.Code,result.Body.String())
   }
   // net/http's ServeMux may reject a forged Host itself using a
   // plain-text 400 response before our JSON GUI handler runs.
   // Any denial format is fine; it must never disclose a secret or
   // dispatch to the Agent.
   raw:=result.Body.Bytes()
   var response guiResponse
   if json.Valid(raw){
    if json.Unmarshal(raw,&response)!=nil||
     response.OK||response.Text!=""||response.Confirm!=""||response.Line!=""{
     t.Fatalf("hostile request disclosed an E2EE result: %+v",response)
    }
   }
   if strings.Contains(string(raw),g.key)||
    strings.Contains(string(raw),"HOST-E2EE:")||
    len(agent.calls)!=0||g.used!=0{
    t.Fatalf("hostile HTTP request reached Agent or disclosed sensitive data: %s %v",string(raw),agent.calls)
   }
  })
 }
}

func TestNativeGUIEmbeddedUIRejectsFramingAndRemoteCode(t *testing.T){
 g,agent:=newTestNativeGUI()
 for _,path:=range []string{"/","/native_gui.js","/native_gui.css"}{
  req:=httptest.NewRequest(http.MethodGet,g.origin+path,nil)
  response:=httptest.NewRecorder()
  g.handler().ServeHTTP(response,req)
  if response.Code!=http.StatusOK{
   t.Fatalf("trusted UI asset %s did not load: %d",path,response.Code)
  }
  h:=response.Header()
  policy:=h.Get("Content-Security-Policy")
  if h.Get("X-Frame-Options")!="DENY"||
   h.Get("Cross-Origin-Resource-Policy")!="same-origin"||
   h.Get("X-Content-Type-Options")!="nosniff"||
   !strings.Contains(h.Get("Cache-Control"),"no-store")||
   !strings.Contains(policy,"default-src 'none'")||
   !strings.Contains(policy,"script-src 'self'")||
   !strings.Contains(policy,"connect-src 'self'")||
   !strings.Contains(policy,"frame-ancestors 'none'"){
   t.Fatalf("remote HTML embedding/source expansion could bypass native GUI trust boundary: %q %v",
    path,h)
  }
  if strings.Contains(response.Body.String(),g.key)||
   strings.Contains(response.Body.String(),"https://compromised-php.example")||
   len(agent.calls)!=0||g.used!=0{
   t.Fatalf("native GUI asset leaked session secret or invoked Agent: %q",path)
  }
 }
}
