package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestRemoteFindLinesTypedE2EEAndDistinctResult(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,host,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionProjectFileFindLines,
  Path:"src/large file.go",Content:"needle string",
 })
 if err!=nil{t.Fatal(err)}
 for _,plain:=range []string{"project.file.find_lines","src/large file.go","needle string"}{
  if bytes.Contains(request,[]byte(plain)){t.Fatal("PHP saw confidential search request")}
 }
 got,err:=OpenRemoteAction(host,request)
 if err!=nil||got.Operation!=RemoteActionProjectFileFindLines||
  got.Path!="src/large file.go"||got.Content!="needle string"||
  got.Offset!=0{t.Fatalf("typed search failed: %+v %v",got,err)}
 result:=[]byte("FILE_SHA256 "+strings.Repeat("a",64)+
  "\nTERM_SHA256 "+strings.Repeat("b",64)+
  "\nTOTAL_LINES 5\nMATCHES 1\n-- COMPLETE MATCHING LINES --\n4: needle string\n")
 frame,err:=SealRemoteFindLinesResult(host,id,result)
 if err!=nil{t.Fatal(err)}
 text,err:=OpenRemoteFindLinesResult(client,frame,id)
 if err!=nil||text!=string(result){t.Fatalf("search result rejected: %q %v",text,err)}
 client,host,_=participants(t)
 frame,err=SealRemoteFindLinesResult(host,id,result)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteFileLinesResult(client,frame,id);err==nil||client.Established(){
  t.Fatal("search result impersonated read-only file page")
 }
}

func TestRemoteFindLinesRejectsForeignFieldsAndUnsafeLiterals(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 for _,c:=range []RemoteAction{
  {Operation:RemoteActionProjectFileFindLines,Path:"src/a.go",Content:""},
  {Operation:RemoteActionProjectFileFindLines,Path:"src/a.go",Content:" n"},
  {Operation:RemoteActionProjectFileFindLines,Path:"src/a.go",Content:"x\ny"},
  {Operation:RemoteActionProjectFileFindLines,Path:"src/a.go",Content:"\x1b[31m"},
  {Operation:RemoteActionProjectFileFindLines,Path:"src/a.go",Content:"hello\u202e"},
  {Operation:RemoteActionProjectFileFindLines,Path:"src/a.go",Content:strings.Repeat("x",97)},
  {Operation:RemoteActionProjectFileFindLines,Path:".git/config",Content:"password"},
  {Operation:RemoteActionProjectFileFindLines,Path:"../hidden",Content:"needle"},
  {Operation:RemoteActionProjectFileFindLines,Path:"src/a.go",Content:"needle",Offset:1},
  {Operation:RemoteActionProjectFileFindLines,Path:"src/a.go",Content:"needle",TaskID:1},
  {Operation:RemoteActionProjectFileFindLines,Path:"src/a.go",Content:"needle",ExpectedSHA256:strings.Repeat("b",64)},
 }{
  client,_,_:=participants(t)
  c.RequestID=id
  frame,err:=SealRemoteAction(client,c)
  if err==nil||len(frame)>0{t.Fatalf("unsafe literal or foreign field accepted: %+v",c)}
 }
 client,host,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.find_lines","path":"src/a.go","content":"hello","command":"sh"}`)
 frame,err:=client.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(host,frame);err==nil||host.Established(){
  t.Fatal("extra command bypassed strict E2EE schema")
 }
}
