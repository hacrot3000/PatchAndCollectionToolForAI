package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestLargeLineDeleteTypedE2EEAndIsolatedReceipt(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 const path="src/source.txt"
 digest:=strings.Repeat("a",64)
 client,host,_:=participants(t)
 packet,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionProjectFileDeleteLine,
  Path:path,Offset:123,ExpectedSHA256:digest})
 if err!=nil{t.Fatal(err)}
 for _,plain:=range []string{path,digest,"delete_line"}{
  if bytes.Contains(packet,[]byte(plain)){t.Fatal("opaque relay saw delete data")}
 }
 request,err:=OpenRemoteAction(host,packet)
 if err!=nil||request.Operation!=RemoteActionProjectFileDeleteLine||
  request.Path!=path||request.Offset!=123||request.Content!=""||
  request.ExpectedSHA256!=digest{
  t.Fatalf("bad encrypted line delete: %+v %v",request,err)
 }
 changed:=strings.Repeat("b",64)
 receipt,err:=SealRemoteLargeLineDeleteResult(host,id,changed)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteLargeLineDeleteResult(client,receipt,id)
 if err!=nil||got!=changed{t.Fatal("invalid deletion receipt")}
 client,host,_=participants(t)
 receipt,err=SealRemoteLargeLineEditResult(host,id,changed)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteLargeLineDeleteResult(client,receipt,id);err==nil||client.Established(){
  t.Fatal("edit result spoofed delete receipt")
 }
 client,host,_=participants(t)
 receipt,err=SealRemoteLargeLineDeleteResult(host,id,changed)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteLargeLineEditResult(client,receipt,id);err==nil||client.Established(){
  t.Fatal("delete receipt spoofed edit result")
 }
}

func TestLargeLineDeleteRejectsContentAndForeignSchema(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 sha:=strings.Repeat("a",64)
 for _,a:=range []RemoteAction{
  {Path:".git/config",Offset:1,ExpectedSHA256:sha},
  {Path:"../etc/passwd",Offset:1,ExpectedSHA256:sha},
  {Path:"source.txt",Offset:0,ExpectedSHA256:sha},
  {Path:"source.txt",Offset:2<<20+1,ExpectedSHA256:sha},
  {Path:"source.txt",Offset:1,ExpectedSHA256:"bad"},
  {Path:"source.txt",Offset:1,ExpectedSHA256:sha,Content:"not empty"},
  {Path:"source.txt",Offset:1,ExpectedSHA256:sha,TaskID:1},
 }{
  client,_,_:=participants(t)
  a.RequestID=id
  a.Operation=RemoteActionProjectFileDeleteLine
  frame,err:=SealRemoteAction(client,a)
  if err==nil||len(frame)!=0{t.Fatalf("accepted invalid delete: %+v",a)}
 }
 for _,extra:=range []string{
  `,"command":"sh -c id"`,
  `,"task_id":1`,
  `,"path":"source-other.txt"`,
 }{
  client,host,_:=participants(t)
  jsonText:=`{"type":"taskdeck-e2ee-action-v1","request_id":"`+id+
   `","operation":"project.file.delete_line","path":"source.txt","content":"",`+
   `"offset":1,"expected_sha256":"`+sha+`"`+extra+`}`
  frame,err:=client.Seal([]byte(jsonText))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteAction(host,frame);err==nil||host.Established(){
   t.Fatal("forged delete request was accepted")
  }
 }
}
