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

func restrictedDiffCall(s *Server,body string)*httptest.ResponseRecorder{
 w:=httptest.NewRecorder()
 request:=httptest.NewRequest(http.MethodPost,"/api/restricted/diff",strings.NewReader(body))
 s.restrictedGate(http.HandlerFunc(s.restrictedDiffAPI)).ServeHTTP(w,request)
 return w
}
func TestRestrictedWindowDiffRecognizesInsertedAndDeletedLines(t *testing.T){
 got:=restrictedWindowDiff(
  []string{"a\n","b\n","c\n"},
  []string{"a\n","new\n","b\n"},
  10,20,
 )
 if len(got)!=4 {t.Fatalf("operation count=%d, want 4: %+v",len(got),got)}
 if got[0].Kind!="equal"||got[0].LeftLine!=10||got[0].RightLine!=20||
  got[1].Kind!="add"||got[1].RightLine!=21||got[1].Text!="new\n"||
  got[2].Kind!="equal"||got[2].LeftLine!=11||got[2].RightLine!=22||
  got[3].Kind!="remove"||got[3].LeftLine!=12||got[3].Text!="c\n"{
  t.Fatalf("incorrect window edit script: %+v",got)
 }
}
func TestRestrictedDiffResponseChargesBothFilesBeforeDisclosure(t *testing.T){
 s:=protectedFixture(t)
 other:=filepath.Join(s.Workspace,"other.go")
 lines:="line01\ninserted\nline02\nline04\nline05\nline06\nline07\nline08\nline09\nline10\n"
 if err:=os.WriteFile(other,[]byte(lines),0600);err!=nil{t.Fatal(err)}
 response:=restrictedDiffCall(s,`{"left":"main.go","right":"other.go","left_start":1,"right_start":1,"count":3}`)
 if response.Code!=http.StatusOK{t.Fatalf("diff returned %d: %s",response.Code,response.Body)}
 var result struct {
  Ops []restrictedDiffOp `json:"ops"`
  WindowOnly bool `json:"window_only"`
 }
 if err:=json.Unmarshal(response.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if !result.WindowOnly||len(result.Ops)!=4||result.Ops[1].Kind!="add"||
  result.Ops[3].Kind!="remove"{t.Fatalf("unexpected restricted diff response: %+v",result)}
 if strings.Contains(response.Body.String(),"line08"){t.Fatal("unrequested source text leaked")}
 // Both files have exhausted the three-line unique-per-file budget.
 denied:=restrictedDiffCall(s,`{"left":"main.go","right":"other.go","left_start":4,"right_start":4,"count":1}`)
 if denied.Code!=http.StatusTooManyRequests{t.Fatalf("diff bypassed cumulative budget: %d",denied.Code)}
 if strings.Contains(denied.Body.String(),"line04"){t.Fatal("budget refusal leaked content")}
}
func TestRestrictedDiffRequiresAllowedPathAndIdentity(t *testing.T){
 s:=protectedFixture(t)
 hidden:=restrictedDiffCall(s,`{"left":".git/config","right":"main.go","left_start":1,"right_start":1,"count":1}`)
 if hidden.Code!=http.StatusForbidden{t.Fatalf("hidden source returned status=%d",hidden.Code)}
 s.Config.SharedServerEnabled=true
 anon:=restrictedDiffCall(s,`{"left":"main.go","right":"main.go","left_start":1,"right_start":1,"count":1}`)
 if anon.Code!=http.StatusUnauthorized{t.Fatalf("anonymous shared diff returned %d",anon.Code)}
}
