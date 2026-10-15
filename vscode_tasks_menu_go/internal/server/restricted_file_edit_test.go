package server

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "sync"
)

func protectedEditCall(s *Server,body string)*httptest.ResponseRecorder{
 w:=httptest.NewRecorder()
 req:=httptest.NewRequest(http.MethodPost,"/api/restricted/file-edit",strings.NewReader(body))
 s.restrictedGate(http.HandlerFunc(s.restrictedFileEditAPI)).ServeHTTP(w,req)
 return w
}
func protectedEditPayload(path string,start int,sha string,lines []string)string{
 body,_:=json.Marshal(map[string]any{
  "path":path,"start_line":start,"expected_sha256":sha,"lines":lines,
 })
 return string(body)
}
func TestProtectedEditOnlyPreviouslyViewedLines(t *testing.T){
 s:=protectedFixture(t)
 initial,err:=os.ReadFile(filepath.Join(s.Workspace,"main.go"));if err!=nil{t.Fatal(err)}
 hash:=sha256.Sum256(initial)
 digest:=hex.EncodeToString(hash[:])
 view:=protectedCall(s,`{"path":"main.go","start":2,"count":2}`)
 if view.Code!=http.StatusOK{t.Fatalf("view: %d %s",view.Code,view.Body)}
 request:=protectedEditPayload("main.go",2,digest,[]string{"new02","new03"})
 edit:=protectedEditCall(s,request)
 if edit.Code!=http.StatusOK{t.Fatalf("save: %d %s",edit.Code,edit.Body)}
 if strings.Contains(edit.Body.String(),"line01")||strings.Contains(edit.Body.String(),"new02"){
  t.Fatalf("save returned source text: %s",edit.Body)
 }
 saved,err:=os.ReadFile(filepath.Join(s.Workspace,"main.go"))
 if err!=nil{t.Fatal(err)}
 lines:=strings.Split(string(saved),"\n")
 if len(lines)<10||lines[0]!="line01"||lines[1]!="new02"||lines[2]!="new03"||lines[3]!="line04"{
  t.Fatalf("outside-view source changed: %q",saved)
 }
 stale:=protectedEditCall(s,request)
 if stale.Code!=http.StatusConflict{t.Fatalf("stale hash accepted: %d %s",stale.Code,stale.Body)}
}
func TestProtectedEditBlocksBlindFileWrites(t *testing.T){
 s:=protectedFixture(t)
 data,err:=os.ReadFile(filepath.Join(s.Workspace,"main.go"));if err!=nil{t.Fatal(err)}
 hash:=sha256.Sum256(data)
 digest:=hex.EncodeToString(hash[:])
 blind:=protectedEditCall(s,protectedEditPayload("main.go",4,digest,[]string{"stolen"}))
 if blind.Code!=http.StatusForbidden{t.Fatalf("blind edit accepted: %d %s",blind.Code,blind.Body)}
 if !strings.Contains(blind.Body.String(),"view this exact source window"){
  t.Fatalf("unexpected blind-edit response: %s",blind.Body)
 }
 unchanged,_:=os.ReadFile(filepath.Join(s.Workspace,"main.go"))
 if string(unchanged)!=string(data){t.Fatal("blind request modified file")}
 // Previously seeing line 1 does not authorize an unrelated region.
 read:=protectedCall(s,`{"path":"main.go","start":1,"count":1}`)
 if read.Code!=http.StatusOK{t.Fatal(read.Body)}
 blind=protectedEditCall(s,protectedEditPayload("main.go",5,digest,[]string{"blind"}))
 if blind.Code!=http.StatusForbidden{t.Fatalf("other source line editable: %d",blind.Code)}
}
func TestProtectedEditRejectsNewlinesAndUnknownJSONFields(t *testing.T){
 s:=protectedFixture(t)
 view:=protectedCall(s,`{"path":"main.go","start":1,"count":1}`)
 if view.Code!=http.StatusOK{t.Fatal(view.Body)}
 data,_:=os.ReadFile(filepath.Join(s.Workspace,"main.go"))
 hash:=sha256.Sum256(data);digest:=hex.EncodeToString(hash[:])
 for _,lines:=range [][]string{
  {"a\nINJECTED"}, {"a\rINJECTED"}, {"a\x00bad"},
 }{
  got:=protectedEditCall(s,protectedEditPayload("main.go",1,digest,lines))
  if got.Code!=http.StatusBadRequest{
   t.Fatalf("invalid line %q was accepted: %d %s",lines,got.Code,got.Body)
  }
 }
 malformed:=protectedEditCall(s,fmt.Sprintf(`{"path":"main.go","start_line":1,"lines":["x"],"expected_sha256":%q,"extra":true}`,digest))
 if malformed.Code!=http.StatusBadRequest{t.Fatalf("unknown edit fields accepted: %d",malformed.Code)}
 hidden:=protectedEditCall(s,protectedEditPayload(".git/config",1,digest,[]string{"x"}))
 if hidden.Code!=http.StatusForbidden{t.Fatalf("editing Git metadata allowed: %d",hidden.Code)}
}
func TestProtectedEditDisabledOnUnrestrictedAndSharedWithoutPrincipal(t *testing.T){
 s:=protectedFixture(t)
 s.Config.Restricted=nil
 rr:=httptest.NewRecorder()
 s.restrictedFileEditAPI(rr,httptest.NewRequest(http.MethodPost,"/api/restricted/file-edit",strings.NewReader("{}")))
 if rr.Code!=http.StatusNotFound{t.Fatalf("unrestricted endpoint enabled: %d",rr.Code)}
 s=protectedFixture(t);s.Config.SharedServerEnabled=true
 data,_:=os.ReadFile(filepath.Join(s.Workspace,"main.go"))
 hash:=sha256.Sum256(data)
 result:=protectedEditCall(s,protectedEditPayload("main.go",1,hex.EncodeToString(hash[:]),[]string{"other"}))
 if result.Code!=http.StatusUnauthorized{t.Fatalf("unauthenticated shared edit accepted: %d",result.Code)}
}


func TestProtectedEditCannotTargetHiddenFileViaVisibleSymlink(t *testing.T){
 s:=protectedFixture(t)
 link:=filepath.Join(s.Workspace,"normal-looking.go")
 if err:=os.Symlink(filepath.Join(".git","config"),link);err!=nil{t.Fatal(err)}
 data,err:=os.ReadFile(filepath.Join(s.Workspace,".git","config"));if err!=nil{t.Fatal(err)}
 hash:=sha256.Sum256(data)
 reply:=protectedEditCall(s,protectedEditPayload("normal-looking.go",1,
  hex.EncodeToString(hash[:]),[]string{"MALICIOUS"}))
 if reply.Code!=http.StatusForbidden {
  t.Fatalf("symlink alias allowed hidden Git config edit: %d %s",reply.Code,reply.Body)
 }
 after,_:=os.ReadFile(filepath.Join(s.Workspace,".git","config"))
 if string(after)!=string(data){t.Fatal("hidden Git config modified")}
}


func TestConcurrentProtectedEditsCannotOverwriteEachOther(t *testing.T){
 s:=protectedFixture(t)
 current,err:=os.ReadFile(filepath.Join(s.Workspace,"main.go"));if err!=nil{t.Fatal(err)}
 expected:=sha256.Sum256(current)
 snapshot:=hex.EncodeToString(expected[:])
 view:=protectedCall(s,`{"path":"main.go","start":2,"count":1}`)
 if view.Code!=http.StatusOK{t.Fatal(view.Body)}
 var wg sync.WaitGroup
 outcomes:=make(chan int,2)
 for _,replacement:=range []string{"update_A","update_B"}{
  wg.Add(1)
  go func(repl string){
   defer wg.Done()
   reply:=protectedEditCall(s,protectedEditPayload("main.go",2,snapshot,[]string{repl}))
   outcomes<-reply.Code
  }(replacement)
 }
 wg.Wait();close(outcomes)
 success,conflict:=0,0
 for code:=range outcomes{
  if code==http.StatusOK{success++
  }else if code==http.StatusConflict{conflict++
  }else{t.Errorf("unexpected concurrent save result: %d",code)}
 }
 if success!=1||conflict!=1{
  t.Fatalf("concurrent source edits were not serialized: successes=%d conflicts=%d",success,conflict)
 }
}


func TestProtectedEditPreservesFileBOMAndCRLF(t *testing.T){
 s:=protectedFixture(t)
 file:=filepath.Join(s.Workspace,"main.go")
 original:=append([]byte{0xEF,0xBB,0xBF},[]byte("first\r\nsecond\r\nthird\r\n")...)
 if err:=os.WriteFile(file,original,0600);err!=nil{t.Fatal(err)}
 // The default max-reveal percentage permits a single line in this file.
 read:=protectedCall(s,`{"path":"main.go","start":1,"count":1}`)
 if read.Code!=http.StatusOK{t.Fatalf("view failed: %d %s",read.Code,read.Body)}
 hash:=sha256.Sum256(original)
 edit:=protectedEditCall(s,protectedEditPayload("main.go",1,
  hex.EncodeToString(hash[:]),[]string{"changed"}))
 if edit.Code!=http.StatusOK{t.Fatalf("save failed: %d %s",edit.Code,edit.Body)}
 saved,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
 expected:=append([]byte{0xEF,0xBB,0xBF},[]byte("changed
second
third
")...)
 if string(saved)!=string(expected){t.Fatalf("BOM or CRLF lost: %v",saved)}
 if strings.Contains(edit.Body.String(),"second"){t.Fatal("neighbor source leaked in save response")}
}
