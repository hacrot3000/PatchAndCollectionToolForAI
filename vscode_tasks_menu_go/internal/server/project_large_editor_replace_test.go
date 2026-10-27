package server

import (
 "bytes"
 "encoding/json"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func largeEditorReplaceRequest(t *testing.T,s *Server,input largeEditorReplaceInput) (int,map[string]any,string) {
 t.Helper()
 data,err:=json.Marshal(input)
 if err!=nil{t.Fatal(err)}
 r:=projectLargeEditorCall(s,http.MethodPost,"/api/project/file-window/replace",data)
 var result map[string]any
 if r.Code==http.StatusOK{
  if err=json.Unmarshal(r.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 }
 return r.Code,result,r.Body.String()
}
func currentLargeEditorReplaceVersion(t *testing.T,s *Server) largeEditorReplaceInput {
 t.Helper()
 r:=projectLargeEditorCall(s,http.MethodGet,"/api/project/file-window?path=big.txt&start=1&count=30",nil)
 if r.Code!=http.StatusOK{t.Fatalf("window read: %d %s",r.Code,r.Body.String())}
 var w largeEditorWindowResponse
 if err:=json.Unmarshal(r.Body.Bytes(),&w);err!=nil{t.Fatal(err)}
 return largeEditorReplaceInput{Path:"big.txt",Size:w.Size,MtimeNS:w.MtimeNS,SHA256:w.SHA256}
}

func TestLargeEditorReplaceAllStreamsLiteralSource(t *testing.T){
 root:=t.TempDir()
 before:=[]byte(strings.Repeat("alpha /&\\ alpha ⚡\r\n",160000))
 file:=filepath.Join(root,"big.txt")
 if err:=os.WriteFile(file,before,0o644);err!=nil{t.Fatal(err)}
 s:=&Server{Workspace:root}
 req:=currentLargeEditorReplaceVersion(t,s)
 req.Find="alpha"
 req.Replacement="B&/\\"
 req.All=true
 req.MatchCase=true
 code,result,message:=largeEditorReplaceRequest(t,s,req)
 if code!=http.StatusOK{t.Fatalf("replace all: %d %s",code,message)}
 if result["replacements"]!=float64(320000){t.Fatalf("wrong replacement count: %#v",result)}
 got,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
 want:=bytes.ReplaceAll(before,[]byte("alpha"),[]byte("B&/\\"))
 if !bytes.Equal(got,want){t.Fatal("stream replacement did not preserve unaffected bytes or literal replacement")}
 if bytes.Contains(got,[]byte("alpha")){t.Fatal("leftover match")}
 // Old SHA-256 must not authorize a second replace after the successful write.
 code,_,_=largeEditorReplaceRequest(t,s,req)
 if code!=http.StatusConflict{t.Fatalf("stale replay accepted: %d",code)}
}

func TestLargeEditorReplaceOneUsesExactLineAndColumn(t *testing.T){
 root:=t.TempDir()
 before:=[]byte(strings.Repeat("needle needle needle\n",110000))
 file:=filepath.Join(root,"big.txt")
 if err:=os.WriteFile(file,before,0o644);err!=nil{t.Fatal(err)}
 s:=&Server{Workspace:root}
 req:=currentLargeEditorReplaceVersion(t,s)
 req.Find="needle";req.Replacement="exact";req.Line=1000;req.Column=7
 code,result,message:=largeEditorReplaceRequest(t,s,req)
 if code!=http.StatusOK{t.Fatalf("single replace: %d %s",code,message)}
 if result["replacements"]!=float64(1){t.Fatalf("wrong single replacement count: %#v",result)}
 got,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
 want:=bytes.Replace(before,[]byte("needle needle needle\n"),[]byte("needle exact needle\n"),1)
 // The one edited line is 1000, not the first.
 want=append([]byte(nil),before...)
 pos:=(req.Line-1)*len("needle needle needle\n")
 copy(want[pos:pos+len("needle needle needle\n")],[]byte("needle exact needle\n"))
 if !bytes.Equal(got,want){t.Fatal("single replace modified wrong match")}
}

func TestLargeEditorReplaceCaseFoldNoMatchAndConflict(t *testing.T){
 root:=t.TempDir();file:=filepath.Join(root,"big.txt")
 original:=[]byte(strings.Repeat("CAFÉ café \u00e4 Ä\n",130000))
 if err:=os.WriteFile(file,original,0o644);err!=nil{t.Fatal(err)}
 s:=&Server{Workspace:root}
 req:=currentLargeEditorReplaceVersion(t,s)
 req.Find="café";req.MatchCase=false;req.All=true;req.Replacement="done"
 code,result,message:=largeEditorReplaceRequest(t,s,req)
 if code!=http.StatusOK{t.Fatalf("unicode fold: %d %s",code,message)}
 if result["replacements"]!=float64(260000){t.Fatalf("unicode fold count: %#v",result)}
 got,_:=os.ReadFile(file)
 if bytes.Contains(got,[]byte("CAFÉ"))||bytes.Contains(got,[]byte("café")){t.Fatal("case-insensitive Unicode replacement missed matches")}
 req=currentLargeEditorReplaceVersion(t,s)
 req.Find="not found";req.All=true;req.Replacement="literal"
 code,result,message=largeEditorReplaceRequest(t,s,req)
 if code!=http.StatusOK||result["replacements"]!=float64(0){t.Fatalf("no-match result: %d %s",code,message)}
 unchanged,_:=os.ReadFile(file)
 if !bytes.Equal(got,unchanged){t.Fatal("no-match modified source")}
 req=currentLargeEditorReplaceVersion(t,s)
 req.Find="done";req.All=true;req.Replacement="again"
 if err:=os.WriteFile(file,append(unchanged,[]byte("external\n")...),0o644);err!=nil{t.Fatal(err)}
 code,_,_=largeEditorReplaceRequest(t,s,req)
 if code!=http.StatusConflict{t.Fatalf("stale version accepted: %d",code)}
}

func TestLargeEditorReplaceRejectsInvalidRequestAndWrongMatch(t *testing.T){
 root:=t.TempDir();file:=filepath.Join(root,"big.txt")
 before:=[]byte(strings.Repeat("needle needle\n",160000))
 if err:=os.WriteFile(file,before,0o644);err!=nil{t.Fatal(err)}
 s:=&Server{Workspace:root}
 good:=currentLargeEditorReplaceVersion(t,s)
 tests:=[]struct{name string;alter func(*largeEditorReplaceInput);want int}{
  {"wrong_column",func(r *largeEditorReplaceInput){r.Find="needle";r.Line=1;r.Column=3},http.StatusConflict},
  {"newline_find",func(r *largeEditorReplaceInput){r.Find="needle\n";r.All=true},http.StatusBadRequest},
  {"null_replacement",func(r *largeEditorReplaceInput){r.Find="needle";r.Replacement="x\x00y";r.All=true},http.StatusBadRequest},
  {"no_coordinates",func(r *largeEditorReplaceInput){r.Find="needle";r.Line=0;r.Column=0},http.StatusBadRequest},
 }
 for _,tc:=range tests{
  t.Run(tc.name,func(t *testing.T){req:=good;tc.alter(&req);code,_,body:=largeEditorReplaceRequest(t,s,req);if code!=tc.want{t.Fatalf("got %d want %d body=%s",code,tc.want,body)}})
 }
 after,_:=os.ReadFile(file)
 if !bytes.Equal(before,after){t.Fatal("invalid requests modified source")}
}
