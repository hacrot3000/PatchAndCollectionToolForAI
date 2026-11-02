package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestLargeLineInsertStrictTypedE2EEAndReceiptIsolation(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 sha:=strings.Repeat("a",64)
 c,h,_:=participants(t)
 req,err:=SealRemoteAction(c,RemoteAction{
  RequestID:id,Operation:RemoteActionProjectFileInsertLine,
  Path:"source.txt",Offset:300,Content:"new UTF-8 ✓",ExpectedSHA256:sha})
 if err!=nil{t.Fatal(err)}
 for _,literal:=range []string{"source.txt","new UTF-8",sha}{
  if bytes.Contains(req,[]byte(literal)){t.Fatal("opaque PHP relay leaked insert request")}
 }
 act,err:=OpenRemoteAction(h,req)
 if err!=nil||act.Operation!=RemoteActionProjectFileInsertLine||
  act.Offset!=300||act.Path!="source.txt"||
  act.Content!="new UTF-8 ✓"||act.ExpectedSHA256!=sha{
  t.Fatalf("decoded wrong type or content: %+v %v",act,err)
 }
 newSHA:=strings.Repeat("b",64)
 reply,err:=SealRemoteLargeLineInsertResult(h,id,newSHA)
 if err!=nil{t.Fatal(err)}
 digest,err:=OpenRemoteLargeLineInsertResult(c,reply,id)
 if err!=nil||digest!=newSHA{t.Fatal("insert SHA response not verified")}
 for _,other:=range []string{"delete","edit"}{
  c,h,_=participants(t)
  if other=="delete"{reply,err=SealRemoteLargeLineDeleteResult(h,id,newSHA)
  }else{reply,err=SealRemoteLargeLineEditResult(h,id,newSHA)}
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteLargeLineInsertResult(c,reply,id);err==nil||c.Established(){
   t.Fatal("foreign write receipt spoofed insert")
  }
 }
}

func TestLargeLineInsertRejectsUnsafeFieldsAndForeignKeys(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 sha:=strings.Repeat("a",64)
 for _,a:=range []RemoteAction{
  {Path:"../etc/passwd",Offset:1,ExpectedSHA256:sha,Content:"new"},
  {Path:".git/config",Offset:1,ExpectedSHA256:sha,Content:"new"},
  {Path:"source.txt",Offset:0,ExpectedSHA256:sha,Content:"new"},
  {Path:"source.txt",Offset:2<<20+1,ExpectedSHA256:sha,Content:"new"},
  {Path:"source.txt",Offset:1,ExpectedSHA256:sha,Content:"x\ny"},
  {Path:"source.txt",Offset:1,ExpectedSHA256:sha,Content:"\x1b[31m"},
  {Path:"source.txt",Offset:1,ExpectedSHA256:sha,Content:strings.Repeat("x",513)},
  {Path:"source.txt",Offset:1,ExpectedSHA256:"wrong",Content:"new"},
  {Path:"source.txt",Offset:1,ExpectedSHA256:sha,Content:"new",TaskID:1},
 }{
  c,_,_:=participants(t)
  a.Operation=RemoteActionProjectFileInsertLine
  a.RequestID=id
  if packet,err:=SealRemoteAction(c,a);err==nil||len(packet)>0{
   t.Fatalf("client accepted unsafe insert: %+v",a)
  }
 }
 for _,extra:=range []string{
  `,"command":"rm -rf /"`, `,"task_id":1`, `,"path":"override"`,
 }{
  c,h,_:=participants(t)
  raw:=`{"type":"taskdeck-e2ee-action-v1","request_id":"`+id+
   `","operation":"project.file.insert_line","path":"source.txt","content":"hello",`+
   `"offset":1,"expected_sha256":"`+sha+`"`+extra+`}`
  packet,err:=c.Seal([]byte(raw))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteAction(h,packet);err==nil||h.Established(){
   t.Fatal("forged insert with foreign key was not rejected")
  }
 }
}
