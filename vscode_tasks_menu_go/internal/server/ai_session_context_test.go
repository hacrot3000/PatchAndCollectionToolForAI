package server

import (
 "encoding/json"
 "net/http"
 "strings"
 "testing"
)

func TestAISessionContextIsServerVerifiedAndReadOnly(t *testing.T){
 s:=prepareAITargetTest(t)
 ok:=requestAIWithUser(t,s.aiSessionContextAPI,http.MethodGet,
  "/api/ai/session-context?session_id=session-A",nil,"alice")
 if ok.Code!=http.StatusOK{t.Fatalf("GET failed: %d %s",ok.Code,ok.Body.String())}
 var payload map[string]any
 if err:=json.Unmarshal(ok.Body.Bytes(),&payload);err!=nil{t.Fatal(err)}
 if payload["host"]!="production.example"||payload["launch_cwd"]!="/srv/app"||
  payload["remote"]!=true||payload["verified"]!=true||payload["execution_enabled"]!=false{
  t.Fatalf("unverified context: %#v",payload)
 }
 for _,forbidden:=range []string{"command_preview","output","environment","api_key"}{
  if strings.Contains(ok.Body.String(),forbidden){t.Fatalf("excessive session metadata returned: %s",forbidden)}
 }
}
func TestAISessionContextDeniesForeignMissingOrInactiveSession(t *testing.T){
 s:=prepareAITargetTest(t)
 for _,case_:=range []struct{id,user string;want int}{
  {"session-A","bob",http.StatusForbidden},
  {"missing","alice",http.StatusNotFound},
  {"","alice",http.StatusBadRequest},
 }{
  resp:=requestAIWithUser(t,s.aiSessionContextAPI,http.MethodGet,
   "/api/ai/session-context?session_id="+case_.id,nil,case_.user)
  if resp.Code!=case_.want{t.Errorf("%q/%q expected %d got %d",case_.id,case_.user,case_.want,resp.Code)}
 }
 mock:=s.Sessions.(*aiTestSessions)
 stopped:=mock.metadata["session-A"];stopped.Status="stopped";mock.metadata["session-A"]=stopped
 resp:=requestAIWithUser(t,s.aiSessionContextAPI,http.MethodGet,
  "/api/ai/session-context?session_id=session-A",nil,"alice")
 if resp.Code!=http.StatusConflict{t.Fatalf("stopped session accepted: %d",resp.Code)}
}
