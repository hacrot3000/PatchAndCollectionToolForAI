package server

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func protectedCompareCall(s *Server,body string)*httptest.ResponseRecorder{
 w:=httptest.NewRecorder()
 r:=httptest.NewRequest(http.MethodPost,"/api/restricted/compare",strings.NewReader(body))
 s.restrictedGate(http.HandlerFunc(s.restrictedCompareAPI)).ServeHTTP(w,r)
 return w
}
func TestRestrictedCompareChargesBothWindows(t *testing.T){
 s:=protectedFixture(t)
 other:=filepath.Join(s.Workspace,"other.go")
 if err:=os.WriteFile(other,[]byte("line01\nchanged\nthird\nfourth\n"),0600);err!=nil{t.Fatal(err)}
 body:=`{"left":"main.go","right":"other.go","left_start":1,"right_start":1,"count":2}`
 got:=protectedCompareCall(s,body)
 if got.Code!=http.StatusOK{t.Fatalf("compare: %d %s",got.Code,got.Body)}
 var result struct {Rows []struct{Left string `json:"left"`;Right string `json:"right"`;Equal bool `json:"equal"`} `json:"rows"`}
 if err:=json.Unmarshal(got.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if len(result.Rows)!=2||!result.Rows[0].Equal||result.Rows[1].Equal{t.Fatalf("unexpected bounded result %+v",result.Rows)}
 // Only one new line remains in the per-file quota for each file.
 exhausted:=protectedCompareCall(s,`{"left":"main.go","right":"other.go","left_start":3,"right_start":3,"count":2}`)
 if exhausted.Code!=http.StatusTooManyRequests{t.Fatalf("quota bypass %d %s",exhausted.Code,exhausted.Body)}
 if strings.Contains(exhausted.Body.String(),"line04")||strings.Contains(exhausted.Body.String(),"fourth"){t.Fatal("quota refusal exposed source")}
}
func TestRestrictedCompareRejectsUnsafeAndLegacyPaths(t *testing.T){
 s:=protectedFixture(t)
 for _,body:=range []string{
  `{"left":".git/config","right":"main.go","left_start":1,"right_start":1,"count":1}`,
  `{"left":"main.go","right":"../secret","left_start":1,"right_start":1,"count":1}`,
  `{"left":"main.go","right":"main.go","left_start":0,"right_start":1,"count":1}`,
  `{"left":"main.go","right":"main.go","left_start":1,"right_start":1,"count":999}`,
  `{"left":"main.go","right":"main.go","left_start":1,"right_start":1,"count":1,"extra":1}`,
 } {
  got:=protectedCompareCall(s,body)
  if got.Code==http.StatusOK{t.Fatalf("unsafe compare accepted %s",body)}
 }
 s.Config.SharedServerEnabled=true
 got:=protectedCompareCall(s,`{"left":"main.go","right":"main.go","left_start":1,"right_start":1,"count":1}`)
 if got.Code!=http.StatusUnauthorized{t.Fatalf("anonymous shared compare: %d",got.Code)}
}

func TestRestrictedCompareRejectsHugeStart(t *testing.T){
 s:=protectedFixture(t)
 got:=protectedCompareCall(s,`{"left":"main.go","right":"main.go","left_start":2147483647,"right_start":1,"count":3}`)
 if got.Code==http.StatusOK{t.Fatal("out-of-range compare was accepted")}
}
