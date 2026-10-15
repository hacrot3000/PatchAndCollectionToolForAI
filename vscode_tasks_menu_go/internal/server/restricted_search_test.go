package server

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func protectedSearchCall(s *Server,body string)*httptest.ResponseRecorder{
 w:=httptest.NewRecorder()
 r:=httptest.NewRequest(http.MethodPost,"/api/restricted/search",strings.NewReader(body))
 s.restrictedGate(http.HandlerFunc(s.restrictedSearchAPI)).ServeHTTP(w,r)
 return w
}
func TestRestrictedSearchOnlyChargedWindow(t *testing.T){
 s:=protectedFixture(t)
 response:=protectedSearchCall(s,`{"path":"main.go","start":1,"count":2,"query":"line02"}`)
 if response.Code!=http.StatusOK{t.Fatalf("search failed: %d %s",response.Code,response.Body)}
 var result struct {
  Matches []struct {Line int `json:"line"`;Text string `json:"text"`} `json:"matches"`
  Scanned int `json:"scanned_lines"`
 }
 if err:=json.Unmarshal(response.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if len(result.Matches)!=1||result.Matches[0].Line!=2||result.Matches[0].Text!="line02\n"||result.Scanned!=2{
  t.Fatalf("unexpected bounded matches %+v",result)
 }
 if strings.Contains(response.Body.String(),"line07"){t.Fatal("unscanned source leaked")}
 denied:=protectedSearchCall(s,`{"path":"main.go","start":3,"count":2,"query":"line04"}`)
 if denied.Code!=http.StatusTooManyRequests{t.Fatalf("quota bypass: %d %s",denied.Code,denied.Body)}
 if strings.Contains(denied.Body.String(),"line04"){t.Fatal("quota denial exposed source")}
}
func TestRestrictedSearchNoUnboundedEnumeration(t *testing.T){
 s:=protectedFixture(t)
 for _,request:=range []string{
  `{"path":".git/config","start":1,"count":1,"query":"private"}`,
  `{"path":"../outside","start":1,"count":1,"query":"abc"}`,
  `{"path":"main.go","start":0,"count":1,"query":"line"}`,
  `{"path":"main.go","start":1,"count":999,"query":"line"}`,
  `{"path":"main.go","start":1,"count":1,"query":"x"}`,
  `{"path":"main.go","start":1,"count":1,"query":"line","scope":"project"}`,
 }{
  result:=protectedSearchCall(s,request)
  if result.Code==http.StatusOK{t.Fatalf("unbounded search accepted: %s",request)}
 }
 s.Config.SharedServerEnabled=true
 result:=protectedSearchCall(s,`{"path":"main.go","start":1,"count":1,"query":"line"}`)
 if result.Code!=http.StatusUnauthorized{t.Fatalf("anonymous shared search: %d",result.Code)}
}
